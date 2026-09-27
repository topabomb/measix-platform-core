package runtimecontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"testing"

	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

// ERX-C0-002: the production activation path persists the compiler's schema.
func TestPublishAndRepublishPersistSnapshotVersion(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()
	first := publishAndFinalize(t, svc, adminID, draftRevision)
	second, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), first.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"publish": first.ReleaseID, "republish": second.ReleaseID} {
		t.Run(name, func(t *testing.T) {
			row, err := st.Client.ManagedRelease.Get(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot clientapi.ManagedSnapshot
			if err := json.Unmarshal(row.SnapshotJSON, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.SchemaVersion != 5 {
				t.Fatalf("unexpected persisted snapshot version %d", snapshot.SchemaVersion)
			}
		})
	}
}

func TestHistoricalRepublishPreservesSchemaAndSourceBytes(t *testing.T) {
	for _, tc := range []struct {
		version int
		system  string
	}{{4, ""}, {5, "Independent opening"}, {5, ""}, {5, " \t\n"}} {
		version := tc.version
		t.Run(fmt.Sprintf("%d-%q", version, tc.system), func(t *testing.T) {
			ctx := context.Background()
			st, svc, _, relayServer, _, adminID, _, revision := newRuntimeControlEnv(t)
			defer relayServer.Close()
			first := publishAndFinalize(t, svc, adminID, revision)
			source, err := st.Client.ManagedRelease.Get(ctx, first.ReleaseID)
			if err != nil {
				t.Fatal(err)
			}
			var content adminapi.ManagedDraftContent
			if err := json.Unmarshal(source.ReleaseContentJSON, &content); err != nil {
				t.Fatal(err)
			}
			assistantID := platformid.New(platformid.Assistant)
			content.Assistants = []adminapi.ManagedAssistantDefinition{{AssistantDefinitionId: assistantID, DisplayName: "Original", SystemPrompt: "Original assistant", ModelId: content.Models[0].ModelId, MemorySeed: []string{}, McpServerIds: []adminapi.McpServerId{}, Enabled: true}}
			starter := adminapi.AssistantStarterDefinition{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: assistantID, Title: "Entry", Prompt: "Original prompt", Enabled: true}
			if version == 5 {
				starter.OpeningSnapshot = &adminapi.StarterOpeningSnapshot{Format: 1, SystemPrompt: tc.system, InitialContexts: []adminapi.StarterInitialContext{{Id: "b", Title: "Block", Content: "Literal {{body}}"}}}
			}
			content.Starters = []adminapi.AssistantStarterDefinition{starter}
			var original clientapi.ManagedSnapshot
			json.Unmarshal(source.SnapshotJSON, &original)
			snapshot, hash, err := svc.Capability.CompileSnapshot(capability.SnapshotInput{SchemaVersion: version, DeploymentID: string(original.DeploymentId), ReleaseID: source.ID, ManagedGeneration: int(source.ManagedGeneration), Content: content, PublishedAt: source.CreatedAt, PublishedByUserID: adminID})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.MarshalIndent(snapshot, "", " ")
			raw = append(raw, '\n')
			contentRaw, _ := json.Marshal(content)
			if _, err := st.Client.ManagedRelease.UpdateOneID(source.ID).SetSnapshotJSON(raw).SetSnapshotHash(hash).SetReleaseContentJSON(contentRaw).Save(ctx); err != nil {
				t.Fatal(err)
			}
			if version == 5 && (tc.system == "" || tc.system == " \t\n") {
				draft, _ := svc.Capability.GetDraft(ctx)
				saved, err := svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, content)
				if err != nil {
					t.Fatal(err)
				}
				preview, err := svc.Capability.PreviewDraft(ctx, saved.DraftRevision)
				if err != nil || preview.DiffSummary.Changed != 1 {
					t.Fatalf("historical empty literal to inherited System must change Starter: %+v %v", preview.DiffSummary, err)
				}
			}
			current, _ := svc.Capability.GetDraft(ctx)
			current.Content.Assistants = []adminapi.ManagedAssistantDefinition{{AssistantDefinitionId: assistantID, DisplayName: "Changed", SystemPrompt: "Current draft must not leak", ModelId: content.Models[0].ModelId, MemorySeed: []string{}, McpServerIds: []adminapi.McpServerId{}, Enabled: true}}
			if _, err := svc.Capability.PutDraft(ctx, adminID, current.DraftRevision, current.Content); err != nil {
				t.Fatal(err)
			}
			next, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			old, _ := st.Client.ManagedRelease.Get(ctx, source.ID)
			newRow, _ := st.Client.ManagedRelease.Get(ctx, next.ReleaseID)
			for _, releaseID := range []string{old.ID, newRow.ID} {
				releaseView, err := svc.Capability.GetRelease(ctx, releaseID)
				if err != nil || releaseView.SnapshotSchemaVersion != version {
					t.Fatalf("Admin version projection for historical/republished release: %+v %v", releaseView, err)
				}
			}
			if !bytes.Equal(old.SnapshotJSON, raw) || !bytes.Equal(old.ReleaseContentJSON, contentRaw) || old.SnapshotHash != hash {
				t.Fatal("republish mutated historical facts")
			}
			if newRow.ID == old.ID || newRow.ManagedGeneration <= old.ManagedGeneration || newRow.SnapshotHash == old.SnapshotHash {
				t.Fatal("republish did not create fresh release identity")
			}
			var published clientapi.ManagedSnapshot
			json.Unmarshal(newRow.SnapshotJSON, &published)
			if int(published.SchemaVersion) != version || published.Assistants[0].SystemPrompt != "Original assistant" {
				t.Fatalf("republish changed historical semantics: %+v", published)
			}
			if (published.Starters[0].OpeningSnapshot != nil) != (version == 5) {
				t.Fatal("republish changed opening presence")
			}
			if version == 5 && published.Starters[0].OpeningSnapshot.SystemPrompt != tc.system {
				t.Fatal("republish rederived opening")
			}
			got, err := capability.HashSnapshot(published)
			if err != nil || got != newRow.SnapshotHash {
				t.Fatalf("hash mismatch %v", err)
			}
			view, err := svc.Capability.GetSnapshot(ctx, int(old.ManagedGeneration))
			if err != nil || !bytes.Equal(view.JSON, raw) || view.Hash != hash {
				t.Fatal("old snapshot download changed")
			}
		})
	}
}

func TestInheritedStarterPublicationAndRepeatedRepublishUseFrozenSystem(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	draft, _ := svc.Capability.GetDraft(ctx)
	assistantID := platformid.New(platformid.Assistant)
	draft.Content.Assistants = []adminapi.ManagedAssistantDefinition{{AssistantDefinitionId: assistantID, DisplayName: "Assistant", SystemPrompt: "  Frozen assistant\n", ModelId: draft.Content.Models[0].ModelId, MemorySeed: []string{}, McpServerIds: []adminapi.McpServerId{}, Enabled: true}}
	draft.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: assistantID, Title: "Entry", Prompt: "Start", Enabled: true, OpeningSnapshot: &adminapi.StarterOpeningSnapshot{Format: 1, SystemPrompt: " \t", InitialContexts: []adminapi.StarterInitialContext{}}}}
	saved, err := svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Capability.PreviewDraft(ctx, saved.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Starters[0].OpeningSnapshot.SystemPrompt != "  Frozen assistant\n" {
		t.Fatal("preview did not resolve inheritance")
	}
	staged, err := svc.Capability.StageRelease(ctx, adminID, saved.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	stagedRow, _ := st.Client.ManagedRelease.Get(ctx, staged.ReleaseID)
	var stagedSnapshot clientapi.ManagedSnapshot
	json.Unmarshal(stagedRow.SnapshotJSON, &stagedSnapshot)
	if stagedSnapshot.Starters[0].OpeningSnapshot.SystemPrompt != preview.Starters[0].OpeningSnapshot.SystemPrompt {
		t.Fatal("Stage differs from Preview")
	}
	first := publishAndFinalize(t, svc, adminID, saved.DraftRevision)
	original, _ := st.Client.ManagedRelease.Get(ctx, first.ReleaseID)
	var authored adminapi.ManagedDraftContent
	json.Unmarshal(original.ReleaseContentJSON, &authored)
	if authored.Starters[0].OpeningSnapshot.SystemPrompt != " \t" {
		t.Fatal("release lost authored inheritance marker")
	}
	unchanged, err := svc.Capability.PreviewDraft(ctx, saved.DraftRevision)
	if err != nil || unchanged.DiffSummary.Changed != 0 {
		t.Fatalf("unchanged preview: %+v %v", unchanged.DiffSummary, err)
	}
	draft, _ = svc.Capability.GetDraft(ctx)
	draft.Content.Starters[0].OpeningSnapshot.SystemPrompt = draft.Content.Assistants[0].SystemPrompt
	saved, err = svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := svc.Capability.PreviewDraft(ctx, saved.DraftRevision)
	if err != nil || equivalent.DiffSummary.Changed != 0 {
		t.Fatalf("equivalent override caused false diff: %+v %v", equivalent.DiffSummary, err)
	}
	draft, _ = svc.Capability.GetDraft(ctx)
	draft.Content.Starters[0].OpeningSnapshot.SystemPrompt = ""
	draft.Content.Assistants[0].SystemPrompt = "Changed draft assistant"
	saved, err = svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := svc.Capability.PreviewDraft(ctx, saved.DraftRevision)
	if err != nil || changed.DiffSummary.Changed != 2 {
		t.Fatalf("Assistant and inherited Starter must both change: %+v %v", changed.DiffSummary, err)
	}
	releaseID := first.ReleaseID
	for i := 0; i < 2; i++ {
		next, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), releaseID)
		if err != nil {
			t.Fatal(err)
		}
		row, _ := st.Client.ManagedRelease.Get(ctx, next.ReleaseID)
		var snapshot clientapi.ManagedSnapshot
		json.Unmarshal(row.SnapshotJSON, &snapshot)
		if snapshot.Starters[0].OpeningSnapshot.SystemPrompt != "  Frozen assistant\n" {
			t.Fatal("republish reinterpreted authored inheritance marker")
		}
		if !bytes.Equal(row.ReleaseContentJSON, original.ReleaseContentJSON) {
			t.Fatal("republish modified authored source")
		}
		view, err := svc.Capability.GetRelease(ctx, row.ID)
		if err != nil || view.DiffSummary.Changed != 0 {
			t.Fatalf("republish has false effective diff: %+v %v", view.DiffSummary, err)
		}
		releaseID = row.ID
	}
	after, _ := st.Client.ManagedRelease.Get(ctx, original.ID)
	if !bytes.Equal(after.SnapshotJSON, original.SnapshotJSON) || after.SnapshotHash != original.SnapshotHash {
		t.Fatal("source bytes/hash changed")
	}
}

func TestRepublishRejectsUnknownSourceVersionBeforeWriting(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, revision := newRuntimeControlEnv(t)
	defer relayServer.Close()
	first := publishAndFinalize(t, svc, adminID, revision)
	source, _ := st.Client.ManagedRelease.Get(ctx, first.ReleaseID)
	var value map[string]any
	json.Unmarshal(source.SnapshotJSON, &value)
	value["schemaVersion"] = 99
	raw, _ := json.Marshal(value)
	if _, err := st.Client.ManagedRelease.UpdateOneID(source.ID).SetSnapshotJSON(raw).Save(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ := st.Client.ManagedRelease.Query().Count(ctx)
	if _, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), source.ID); err == nil {
		t.Fatal("unknown version was downgraded")
	}
	after, _ := st.Client.ManagedRelease.Query().Count(ctx)
	if before != after {
		t.Fatal("invalid version wrote a release")
	}
}

func TestRepublishV4WithoutStartersRemainsV4(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, revision := newRuntimeControlEnv(t)
	defer relayServer.Close()
	first := publishAndFinalize(t, svc, adminID, revision)
	source, _ := st.Client.ManagedRelease.Get(ctx, first.ReleaseID)
	var content adminapi.ManagedDraftContent
	json.Unmarshal(source.ReleaseContentJSON, &content)
	if len(content.Starters) != 0 {
		t.Fatal("fixture must have no starters")
	}
	var current clientapi.ManagedSnapshot
	json.Unmarshal(source.SnapshotJSON, &current)
	snapshot, hash, err := svc.Capability.CompileSnapshot(capability.SnapshotInput{SchemaVersion: 4, DeploymentID: string(current.DeploymentId), ReleaseID: source.ID, ManagedGeneration: int(source.ManagedGeneration), Content: content, PublishedAt: source.CreatedAt, PublishedByUserID: adminID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snapshot)
	if _, err := st.Client.ManagedRelease.UpdateOneID(source.ID).SetSnapshotJSON(raw).SetSnapshotHash(hash).Save(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := st.Client.ManagedRelease.Get(ctx, next.ReleaseID)
	json.Unmarshal(result.SnapshotJSON, &current)
	if current.SchemaVersion != 4 || len(current.Starters) != 0 {
		t.Fatal("empty v4 release was upgraded")
	}
}
