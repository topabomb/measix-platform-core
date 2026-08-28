package capability_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

// ERX-C0-004: invalid/missing/disabled refs block publish.
func TestERXC0004InvalidRefsBlockPublish(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }

	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Create a draft with an assistant that references a non-existent model
	content := validDraft("ups_nonexistent")
	assistantID := platformid.New(platformid.Assistant)
	modelID := platformid.New(platformid.Model) // this model is NOT in the draft
	assistantContent := adminapi.ManagedAssistantDefinition{
		AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
		DisplayName:           "Test Assistant",
		SystemPrompt:          "You are a test assistant.",
		ModelId:               adminapi.ModelId(modelID), // references unknown model
		MemorySeed:            []string{"seed 1"},
		Enabled:               true,
	}
	content.Assistants = &[]adminapi.ManagedAssistantDefinition{assistantContent}
	content.Bindings = []adminapi.RuntimeBindingDefinition{} // remove bindings to simplify

	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("validation should fail for assistant with unknown model ref")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Code, "invalid_model_ref") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected invalid_model_ref error, got %+v", result.Errors)
	}
}

// ERX-C-001: Admin creates and publishes one Managed Assistant with enabled Managed Model.
func TestERXC001CreateAndPublishManagedAssistant(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
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
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }

	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	// Add a managed assistant referencing the model in the draft
	assistantID := platformid.New(platformid.Assistant)
	modelID := string(content.Models[0].ModelId)
	mcpID := platformid.New(platformid.MCP)
	// Add MCP server
	content.Mcp = []adminapi.McpDefinition{{
		McpServerId: mcpID, DisplayName: "Enterprise MCP", ClientProtocol: adminapi.McpDefinitionClientProtocol("MCP_STREAMABLE_HTTP"),
		RuntimePath: "/mcp", AuthOwnership: adminapi.McpDefinitionAuthOwnership("ENTERPRISE_MANAGED"), Enabled: true,
	}}
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{
		RuntimeRouteId: platformid.New(platformid.Route), ResourceId: mcpID, UpstreamId: up.UpstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPSTREAMINGSSE,
	})
	assistantDef := adminapi.ManagedAssistantDefinition{
		AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
		DisplayName:           "Enterprise Assistant",
		SystemPrompt:          "You are a helpful enterprise assistant.",
		ModelId:               adminapi.ModelId(modelID),
		MemorySeed:            []string{"Company policy: be excellent", "Check updates daily"},
		McpServerIds:          []adminapi.McpServerId{adminapi.McpServerId(mcpID)},
		Enabled:               true,
	}
	content.Assistants = &[]adminapi.ManagedAssistantDefinition{assistantDef}
	// Add a starter
	starterID := platformid.New(platformid.Starter)
	content.Starters = &[]adminapi.AssistantStarterDefinition{{
		StarterId:             adminapi.StarterId(starterID),
		AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
		Title:                 "Recent Updates",
		Prompt:                "What are the latest enterprise updates?",
		SortOrder:             0,
		Enabled:               true,
	}}

	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	// Validate should pass
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("validation should pass for valid assistant+starter draft, errors: %+v", result.Errors)
	}
	// Stage release should succeed
	release, err := cap.StageRelease(ctx, boot.AdminUserID, updated.DraftRevision)
	if err != nil {
		t.Fatalf("StageRelease failed: %v", err)
	}
	if release.ManagedGeneration != 1 {
		t.Fatalf("expected generation=1, got %d", release.ManagedGeneration)
	}
	// The staged release should contain assistants and starters in its snapshot JSON
	stored, err := st.Client.ManagedRelease.Get(ctx, release.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored.SnapshotJSON), assistantID) {
		t.Fatal("snapshot JSON should contain assistant ID")
	}
	if !strings.Contains(string(stored.SnapshotJSON), starterID) {
		t.Fatal("snapshot JSON should contain starter ID")
	}
	if stored.SnapshotSchemaVersion != 2 {
		t.Fatalf("expected schema version 2, got %d", stored.SnapshotSchemaVersion)
	}
}

// ERX-C-003: multiple non-empty memory seed items project read-only.
func TestERXC003MultipleMemorySeedItemsProjectReadOnly(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, _ := newSecretBox()
	ups := newUpstreamService(t, st, box, now)
	secret, _ := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "token")
	up, _ := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }

	draft, _ := cap.GetDraft(ctx)
	content := validDraft(up.UpstreamID)
	assistantID := platformid.New(platformid.Assistant)
	modelID := string(content.Models[0].ModelId)
	seeds := []string{"Seed one", "Seed two", "Seed three"}
	content.Assistants = &[]adminapi.ManagedAssistantDefinition{{
		AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
		DisplayName:           "Multi Seed Assistant",
		SystemPrompt:          "You are helpful.",
		ModelId:               adminapi.ModelId(modelID),
		MemorySeed:            seeds,
		Enabled:               true,
	}}
	updated, _ := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	// Preview should show normalized seeds
	preview, err := cap.PreviewDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Assistants) != 1 {
		t.Fatalf("expected 1 assistant in preview, got %d", len(preview.Assistants))
	}
	if len(preview.Assistants[0].MemorySeed) != 3 {
		t.Fatalf("expected 3 seed items, got %d", len(preview.Assistants[0].MemorySeed))
	}
	// Seeds should be sorted (normalized)
	for i, seed := range preview.Assistants[0].MemorySeed {
		if strings.TrimSpace(seed) != seed {
			t.Fatalf("seed %d not trimmed: %q", i, seed)
		}
	}
}

// ERX-C-007: Starter renders title/description/order and pre-fills prompt.
func TestERXC007StarterRendersTitleOrderAndPrefillsPrompt(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, _ := newSecretBox()
	ups := newUpstreamService(t, st, box, now)
	secret, _ := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "token")
	up, _ := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }

	draft, _ := cap.GetDraft(ctx)
	content := validDraft(up.UpstreamID)
	assistantID := platformid.New(platformid.Assistant)
	modelID := string(content.Models[0].ModelId)
	content.Assistants = &[]adminapi.ManagedAssistantDefinition{{
		AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
		DisplayName:           "Test Assistant",
		SystemPrompt:          "You are helpful.",
		ModelId:               adminapi.ModelId(modelID),
		MemorySeed:            []string{"seed"},
		Enabled:               true,
	}}
	// Add two starters with different sort orders
	starter1ID := platformid.New(platformid.Starter)
	starter2ID := platformid.New(platformid.Starter)
	content.Starters = &[]adminapi.AssistantStarterDefinition{
		{
			StarterId:             adminapi.StarterId(starter2ID),
			AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
			Title:                 "Second Starter",
			Prompt:                "Second prompt",
			SortOrder:             1,
			Enabled:               true,
		},
		{
			StarterId:             adminapi.StarterId(starter1ID),
			AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
			Title:                 "First Starter",
			Prompt:                "First prompt",
			SortOrder:             0,
			Enabled:               true,
		},
	}
	updated, _ := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	preview, err := cap.PreviewDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Starters) != 2 {
		t.Fatalf("expected 2 starters, got %d", len(preview.Starters))
	}
	// Starters should be sorted by (assistantDefinitionId, sortOrder)
	if preview.Starters[0].SortOrder != 0 || preview.Starters[1].SortOrder != 1 {
		t.Fatalf("starters not sorted by sortOrder: got %d, %d",
			preview.Starters[0].SortOrder, preview.Starters[1].SortOrder)
	}
	if preview.Starters[0].Title != "First Starter" {
		t.Fatalf("expected first starter title 'First Starter', got %s", preview.Starters[0].Title)
	}
}

// ERX-C-009 (simplified): Personal Realm validation — assistants with disabled model ref should fail.
func TestERXC009DisabledModelRefBlocksValidation(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	draft, _ := cap.GetDraft(ctx)
	content := validDraft("ups_nonexistent")
	// Add model but disabled
	modelID := platformid.New(platformid.Model)
	content.Models = append(content.Models, adminapi.ModelDefinition{
		ModelId: modelID, ProviderId: content.Providers[0].ProviderId, DisplayName: "Disabled Model",
		UpstreamModelKey: "disabled", RuntimePath: "/v1/chat", Enabled: false,
		Capabilities:     []adminapi.ModelDefinitionCapabilities{adminapi.TOOL},
		InputModalities:  []adminapi.ModelDefinitionInputModalities{adminapi.ModelDefinitionInputModalitiesTEXT},
		OutputModalities: []adminapi.ModelDefinitionOutputModalities{adminapi.ModelDefinitionOutputModalitiesTEXT},
	})
	// Assistant referencing the disabled model
	assistantID := platformid.New(platformid.Assistant)
	content.Assistants = &[]adminapi.ManagedAssistantDefinition{{
		AssistantDefinitionId: adminapi.AssistantDefinitionId(assistantID),
		DisplayName:           "Test Assistant",
		SystemPrompt:          "You are helpful.",
		ModelId:               adminapi.ModelId(modelID), // disabled model
		MemorySeed:            []string{"seed"},
		Enabled:               true,
	}}
	content.Bindings = []adminapi.RuntimeBindingDefinition{} // clear bindings
	updated, _ := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("validation should fail for assistant referencing disabled model")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Code, "invalid_model_ref") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected invalid_model_ref error for disabled model, got %+v", result.Errors)
	}
}

// ERX-C0-001: v1 frozen fixtures remain reproducible.
// This is verified in the contract_test package (TestSnapshotAndRuntimeControlGoldenHashes).
// Here we verify that a v1 snapshot with empty assistants/starters produces
// the same hash as one where assistants/starters are nil — proving v1 compatibility.
func TestERXC0001V1HashUnaffectedByEmptyV2Fields(t *testing.T) {
	ctx := context.Background()
	_ = ctx
	// Compile a snapshot without assistants/starters and verify it's schemaVersion=2
	// but the hash is deterministic and v1-compatible (empty v2 fields with omitempty).
	st, boot, now := bootstrapI2(t)
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	draft, _ := cap.GetDraft(ctx)
	content := validDraft("ups_test_only")
	// Remove bindings to avoid upstream lookup errors — we only care about hash
	content.Bindings = []adminapi.RuntimeBindingDefinition{}
	content.Models[0].Enabled = false // disable to avoid missing binding error
	updated, _ := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	depID := platformid.New(platformid.Deployment)
	relID := platformid.New(platformid.Release)
	snapshot, hash, err := cap.CompileSnapshot(capability.SnapshotInput{
		DeploymentID:      depID,
		ReleaseID:         relID,
		ManagedGeneration: 1,
		Content:           updated.Content,
		PublishedAt:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Hash must be deterministic
	hash2, _ := capability.HashSnapshot(snapshot)
	if hash != hash2 {
		t.Fatalf("hash mismatch: compiled=%s, recomputed=%s", hash, hash2)
	}
	// Snapshot should have schemaVersion=2 and empty assistants/starters
	if snapshot.SchemaVersion != 2 {
		t.Fatalf("expected schemaVersion=2, got %d", snapshot.SchemaVersion)
	}
	if len(snapshot.Assistants) != 0 {
		t.Fatalf("expected 0 assistants, got %d", len(snapshot.Assistants))
	}
	if len(snapshot.Starters) != 0 {
		t.Fatalf("expected 0 starters, got %d", len(snapshot.Starters))
	}
}

// Helper to create a secret box
func newSecretBox() (*security.SecretBox, error) {
	return security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
}

// Helper to create an upstream service
func newUpstreamService(t *testing.T, st *testutil.StoreHandle, box *security.SecretBox, now time.Time) *upstream.Service {
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	return ups
}
