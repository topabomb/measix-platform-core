package capability_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestStarterV5BackgroundHasOnlyIdentityAndContent(t *testing.T) {
	valid := []byte(`{"starters":[{"openingSnapshot":{"format":1,"systemPrompt":"","initialContexts":[{"id":"background","content":"literal {{text}}"}]}}]}`)
	if err := capability.ValidateDraftOpeningJSON(valid); err != nil {
		t.Fatalf("title-free background rejected: %v", err)
	}
	withTitle := strings.Replace(string(valid), `"content":`, `"title":"obsolete","content":`, 1)
	if err := capability.ValidateDraftOpeningJSON([]byte(withTitle)); err == nil {
		t.Fatal("unpublished obsolete background title accepted")
	}
}

func TestStarterV5CompilerRequiresAuthoredOpening(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, err := svc.GetDraft(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := capability.SnapshotInput{DeploymentID: boot.DeploymentID, ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: draft.Content, PublishedAt: now}
	snapshot, _, err := svc.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != 5 {
		t.Fatalf("new compilation schema=%d, want 5", snapshot.SchemaVersion)
	}
	input.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: platformid.New(platformid.Assistant), Title: "Entry", Prompt: "Start", Enabled: false}}
	if _, _, err = svc.CompileSnapshot(input); err == nil {
		t.Fatal("missing opening compiled, including disabled starter")
	}
}

func TestStarterLegacyDraftReadDoesNotInventOpening(t *testing.T) {
	st, boot, _ := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	ctx := context.Background()
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	draft.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: platformid.New(platformid.Assistant), Title: "Entry", Prompt: "Start", Enabled: false}}
	saved, err := svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.Client.ManagedDraft.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	read, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(read.Content)
	var content map[string]any
	json.Unmarshal(raw, &content)
	starter := content["starters"].([]any)[0].(map[string]any)
	if _, present := starter["openingSnapshot"]; present {
		t.Fatal("legacy draft acquired implicit opening")
	}
	after, _ := st.Client.ManagedDraft.Query().Only(ctx)
	if string(before.ContentJSON) != string(after.ContentJSON) || before.DraftRevision != after.DraftRevision || !before.UpdatedAt.Equal(after.UpdatedAt) {
		t.Fatal("read mutated durable draft")
	}
	result, err := svc.ValidateDraft(ctx, saved.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range result.Errors {
		if issue.Code == "missing_starter_opening" && issue.Path == "starters[0].openingSnapshot" {
			return
		}
	}
	t.Fatalf("missing precise opening validation: %+v", result.Errors)
}

func testOpening() *adminapi.StarterOpeningSnapshot {
	return &adminapi.StarterOpeningSnapshot{Format: 1, SystemPrompt: "Fixed system", InitialContexts: []adminapi.StarterInitialContext{}}
}

func TestStarterDraftSystemInheritanceIsCompiledWithoutMutatingDraft(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, _ := svc.GetDraft(context.Background())
	assistantID := platformid.New(platformid.Assistant)
	for _, tc := range []struct{ name, authored, assistant, want string }{
		{"empty inherits", "", "  Assistant {{literal}}\n", "  Assistant {{literal}}\n"},
		{"whitespace inherits", " \t\n\u3000", "Assistant", "Assistant"},
		{"override preserved", "  Override {{literal}}\n", "Assistant", "  Override {{literal}}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opening := testOpening()
			opening.SystemPrompt = tc.authored
			content := draft.Content
			content.Assistants = []adminapi.ManagedAssistantDefinition{{McpBindings: emptyMcpBindings(), AssistantDefinitionId: assistantID, SystemPrompt: tc.assistant}}
			content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: assistantID, OpeningSnapshot: opening}}
			snapshot, hash, err := svc.CompileSnapshot(capability.SnapshotInput{DeploymentID: boot.DeploymentID, ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: content, PublishedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			if got := snapshot.Starters[0].OpeningSnapshot.SystemPrompt; got != tc.want {
				t.Fatalf("compiled system=%q want=%q", got, tc.want)
			}
			if opening.SystemPrompt != tc.authored {
				t.Fatal("compiler mutated draft inheritance marker")
			}
			if got, err := capability.HashSnapshot(snapshot); err != nil || got != hash {
				t.Fatalf("hash=%s err=%v", got, err)
			}
		})
	}
}

func TestStarterInheritanceCannotBypassAssistantSystemValidation(t *testing.T) {
	for _, system := range []string{"", " \t\n"} {
		t.Run(system, func(t *testing.T) {
			st, boot, now := bootstrapI2(t)
			ctx := context.Background()
			box, err := newSecretBox()
			if err != nil {
				t.Fatal(err)
			}
			ups := newUpstreamService(t, st, box, now)
			secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "token")
			if err != nil {
				t.Fatal(err)
			}
			up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
			if err != nil {
				t.Fatal(err)
			}
			svc := capability.NewService(st.Client)
			draft, err := svc.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			assistantID := platformid.New(platformid.Assistant)
			draft.Content = validDraft(up.UpstreamID)
			draft.Content.Assistants = []adminapi.ManagedAssistantDefinition{{McpBindings: emptyMcpBindings(), AssistantDefinitionId: assistantID, DisplayName: "Assistant", ModelId: draft.Content.Models[0].ModelId, SystemPrompt: system, MemorySeed: []string{}, Enabled: true}}
			opening := testOpening()
			opening.SystemPrompt = ""
			draft.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: assistantID, Title: "Entry", Prompt: "Start", OpeningSnapshot: opening}}
			saved, err := svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
			if err != nil {
				t.Fatal(err)
			}
			validation, err := svc.ValidateDraft(ctx, saved.DraftRevision)
			if err != nil {
				t.Fatal(err)
			}
			if len(validation.Errors) != 1 || validation.Errors[0].Code != "missing_system_prompt" || validation.Errors[0].Path != "assistants[0].systemPrompt" {
				t.Fatalf("expected only assistant System issue: %+v", validation.Errors)
			}
			if _, err := svc.StageRelease(ctx, boot.AdminUserID, saved.DraftRevision); !errors.Is(err, capability.ErrInvalidDraft) {
				t.Fatalf("expected invalid draft, got %v", err)
			}
		})
	}
}

func TestPublishedContentRejectsMissingOrMismatchedOpeningAuthority(t *testing.T) {
	content := adminapi.ManagedDraftContent{Starters: []adminapi.AssistantStarterDefinition{{StarterId: "str_original", AssistantDefinitionId: "asd_original", OpeningSnapshot: testOpening()}}}
	const valid = `{"schemaVersion":5,"starters":[{"starterId":"str_original","assistantDefinitionId":"asd_original","openingSnapshot":{"format":1,"systemPrompt":"","initialContexts":[]}}]}`
	for name, raw := range map[string]string{
		"missing version":      strings.Replace(valid, `"schemaVersion":5,`, "", 1),
		"unknown version":      strings.Replace(valid, `"schemaVersion":5`, `"schemaVersion":99`, 1),
		"missing starters":     `{"schemaVersion":5}`,
		"null starters":        `{"schemaVersion":5,"starters":null}`,
		"missing starter":      `{"schemaVersion":5,"starters":[]}`,
		"mismatched starter":   strings.Replace(valid, "str_original", "str_other", 1),
		"mismatched assistant": strings.Replace(valid, "asd_original", "asd_other", 1),
		"missing opening":      `{"schemaVersion":5,"starters":[{"starterId":"str_original","assistantDefinitionId":"asd_original"}]}`,
		"null opening":         `{"schemaVersion":5,"starters":[{"starterId":"str_original","assistantDefinitionId":"asd_original","openingSnapshot":null}]}`,
		"missing System":       strings.Replace(valid, `"systemPrompt":"",`, "", 1),
		"null System":          strings.Replace(valid, `"systemPrompt":""`, `"systemPrompt":null`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := capability.PublishedContent(content, []byte(raw)); err == nil {
				t.Fatal("invalid published authority accepted")
			}
		})
	}
	resolved, version, err := capability.PublishedContent(content, []byte(valid))
	if err != nil || version != 5 || resolved.Starters[0].OpeningSnapshot.SystemPrompt != "" {
		t.Fatalf("published empty literal lost: %+v %d %v", resolved, version, err)
	}
	if content.Starters[0].OpeningSnapshot.SystemPrompt != "Fixed system" {
		t.Fatal("source mutated")
	}
	for name, raw := range map[string]string{
		"opening extension":              strings.Replace(valid, `"format":1`, `"format":1,"futureDisplayHint":true`, 1),
		"v4 ignores later opening field": strings.Replace(valid, `"schemaVersion":5`, `"schemaVersion":4`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			resolved, version, err := capability.PublishedContent(content, []byte(raw))
			if err != nil {
				t.Fatal("published response extension rejected", err)
			}
			if version == 4 && resolved.Starters[0].OpeningSnapshot != nil {
				t.Fatal("v4 interpreted a later-version extension")
			}
			if version == 5 && resolved.Starters[0].OpeningSnapshot.SystemPrompt != "" {
				t.Fatal("literal System changed")
			}
		})
	}
}

func TestStarterOpeningCanonicalOrderAndLiteralText(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, _ := svc.GetDraft(context.Background())
	opening := testOpening()
	opening.SystemPrompt = "  中文 {{user}}\n```system```  "
	opening.InitialContexts = []adminapi.StarterInitialContext{{Id: "z", Content: "{{ untouched }}\n````"}, {Id: "a", Content: ""}}
	draft.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: platformid.New(platformid.Assistant), Title: "Entry", Prompt: "Start", OpeningSnapshot: opening}}
	input := capability.SnapshotInput{DeploymentID: boot.DeploymentID, ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: draft.Content, PublishedAt: now}
	snapshot, hash, err := svc.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	actual := snapshot.Starters[0].OpeningSnapshot
	if actual.SystemPrompt != opening.SystemPrompt || actual.InitialContexts[0].Id != "z" || actual.InitialContexts[0].Content != opening.InitialContexts[0].Content {
		t.Fatalf("authored content changed: %+v", actual)
	}
	if got, err := capability.HashSnapshot(snapshot); err != nil || got != hash {
		t.Fatalf("hash=%s err=%v", got, err)
	}
	opening.InitialContexts[0], opening.InitialContexts[1] = opening.InitialContexts[1], opening.InitialContexts[0]
	_, reordered, err := svc.CompileSnapshot(input)
	if err != nil || reordered == hash {
		t.Fatal("ordered blocks must change hash")
	}
	input.SchemaVersion = 4
	if _, _, err := svc.CompileSnapshot(input); err == nil {
		t.Fatal("v4 silently dropped authored opening")
	}
}

func TestStarterSnapshotSizeBoundary(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, _ := svc.GetDraft(context.Background())
	opening := testOpening()
	draft.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: platformid.New(platformid.Assistant), OpeningSnapshot: opening}}
	input := capability.SnapshotInput{DeploymentID: boot.DeploymentID, ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: draft.Content, PublishedAt: now}
	baseline, _, err := svc.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(baseline)
	// Replace the previously nonempty text: the encoded length is linear for ASCII.
	opening.SystemPrompt = strings.Repeat("x", capability.MaxSnapshotBytes-len(raw)+len(baseline.Starters[0].OpeningSnapshot.SystemPrompt))
	exact, _, err := svc.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(exact)
	if len(encoded) != capability.MaxSnapshotBytes {
		t.Fatalf("boundary=%d", len(encoded))
	}
	opening.SystemPrompt += "x"
	if _, _, err := svc.CompileSnapshot(input); !errors.Is(err, capability.ErrSnapshotTooLarge) {
		t.Fatalf("overflow=%v", err)
	}
}

func TestOpeningRawContractRejectsNullUnknownAndCorruption(t *testing.T) {
	valid := `{"format":1,"systemPrompt":"","initialContexts":[{"id":"x","content":""}]}`
	cases := []string{
		`null`, `{}`, `{"format":null,"systemPrompt":"","initialContexts":[]}`,
		`{"format":2,"systemPrompt":"","initialContexts":[]}`, `{"format":1,"systemPrompt":null,"initialContexts":[]}`,
		`{"format":1,"systemPrompt":"","initialContexts":null}`, `{"format":1,"systemPrompt":"","initialContexts":[],"extra":true}`,
		`{"format":1,"systemPrompt":"","initialContexts":[null]}`,
		strings.Replace(valid, `"content":""`, `"content":null`, 1),
		strings.Replace(valid, `"id":"x"`, `"id":"  "`, 1),

		`{"format":1,"systemPrompt":"","initialContexts":[{"id":"same","content":""},{"id":"same","content":""}]}`,
	}
	for _, opening := range cases {
		t.Run(opening, func(t *testing.T) {
			raw := []byte(`{"starters":[{"openingSnapshot":` + opening + `}]}`)
			if err := capability.ValidateDraftOpeningJSON(raw); err == nil {
				t.Fatal("invalid opening accepted")
			}
		})
	}
	for _, opening := range []string{valid, `{"format":1,"systemPrompt":"","initialContexts":[]}`} {
		if err := capability.ValidateDraftOpeningJSON([]byte(`{"starters":[{"openingSnapshot":` + opening + `}]}`)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDraftBackgroundWithoutTitleRoundTrips(t *testing.T) {
	st, boot, _ := bootstrapI2(t)
	ctx := context.Background()
	svc := capability.NewService(st.Client)
	draft, _ := svc.GetDraft(ctx)
	opening := testOpening()
	opening.InitialContexts = []adminapi.StarterInitialContext{{Id: "draft-note", Content: "  {{literal}}\nBody  "}}
	draft.Content.Starters = []adminapi.AssistantStarterDefinition{{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: platformid.New(platformid.Assistant), Title: "Entry", Prompt: "Start", OpeningSnapshot: opening}}
	if _, err := svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.GetDraft(ctx)
	if err != nil || loaded.Content.Starters[0].OpeningSnapshot.InitialContexts[0] != opening.InitialContexts[0] {
		t.Fatalf("background changed: %v", err)
	}
}
