package capability_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestCorruptReleaseCannotBecomeEmptySuccessfulDiff(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	id := platformid.New(platformid.Release)
	_, err := st.Client.ManagedRelease.Create().SetID(id).SetManagedGeneration(1).SetStatus("ACTIVE").
		SetReleaseContentJSON([]byte("{bad")).SetSnapshotJSON([]byte("{}")).
		SetSnapshotHash("sha256:test").SetSourceDraftRevision(1).SetCreatedByUserID(boot.AdminUserID).SetCreatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = capability.NewService(st.Client).GetRelease(ctx, id); err == nil {
		t.Fatal("corrupt release reported as successful empty diff")
	}
}

func TestPreviewDiffUsesLatestImmutableRelease(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	baselineJSON, _ := json.Marshal(draft.Content)
	_, err = st.Client.ManagedRelease.Create().
		SetID(platformid.New(platformid.Release)).SetManagedGeneration(7).SetStatus("ACTIVE").
		SetReleaseContentJSON(baselineJSON).SetSnapshotJSON([]byte("{}")).SetSnapshotHash("sha256:baseline").
		SetSourceDraftRevision(int64(draft.DraftRevision)).SetCreatedByUserID(boot.AdminUserID).SetCreatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := draft.Content
	content.Providers = append(content.Providers, adminapi.ProviderDefinition{
		ProviderId: platformid.New(platformid.Provider), DisplayName: "New provider",
		ClientProtocol: adminapi.OPENAICHATCOMPLETIONS, Enabled: true,
	})
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := cap.PreviewDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PublishedGeneration == nil || *preview.PublishedGeneration != 7 {
		t.Fatalf("published generation=%v want 7", preview.PublishedGeneration)
	}
	if preview.DiffSummary.Added != 1 || preview.DiffSummary.Changed != 0 || preview.DiffSummary.Removed != 0 {
		t.Fatalf("authoritative preview diff=%+v", preview.DiffSummary)
	}
}
