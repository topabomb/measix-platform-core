package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestEnabledMcpWithoutMandatoryBindingsSurvivesSavePreviewAndRelease(t *testing.T) {
	ctx := context.Background()
	store, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x31}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	ups := upstream.NewService(store.Client, box)
	secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "provider", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	svc := capability.NewService(store.Client)
	svc.Now = func() time.Time { return now }
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	id := platformid.New(platformid.MCP)
	mode := adminapi.McpDefinitionToolAccessMode("ALL")
	grants := []adminapi.McpToolGrant{}
	content.Mcp = []adminapi.McpDefinition{{McpServerId: id, DisplayName: "Optional enterprise service", ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "ENTERPRISE_MANAGED", RuntimePath: "/mcp", Enabled: true, ToolAccessMode: &mode, AllowedTools: &grants}}
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{RuntimeRouteId: platformid.New(platformid.Route), ResourceId: id, UpstreamId: up.UpstreamID, AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE"})
	content.Assistants = []adminapi.ManagedAssistantDefinition{{AssistantDefinitionId: platformid.New(platformid.Assistant), DisplayName: "No mandatory MCP", SystemPrompt: "Help", ModelId: content.Models[0].ModelId, MemorySeed: []string{}, McpBindings: emptyMcpBindings(), Enabled: true}}
	content.Policy.AllowLocalMcp = false
	content.Policy.AllowLocalAssistants = false
	saved, err := svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := svc.ValidateDraft(ctx, saved.DraftRevision)
	if err != nil || !valid.Valid {
		t.Fatalf("unbound enabled enterprise service rejected: %+v %v", valid, err)
	}
	if _, err := svc.PreviewDraft(ctx, saved.DraftRevision); err != nil {
		t.Fatal(err)
	}
	release, err := svc.StageRelease(ctx, boot.AdminUserID, saved.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Client.ManagedRelease.GetX(ctx, release.ReleaseID)
	var snapshot capability.Snapshot
	if err := json.Unmarshal(before.SnapshotJSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != 5 || len(snapshot.Mcp) != 1 || !snapshot.Mcp[0].Enabled || snapshot.Mcp[0].McpServerId != id || snapshot.Assistants[0].McpBindings == nil || len(snapshot.Assistants[0].McpBindings) != 0 || snapshot.Policy.AllowLocalMcp {
		t.Fatalf("optional enterprise service was dropped or made mandatory: %+v", snapshot)
	}
	bindings := []adminapi.AssistantMcpBinding{{McpServerId: id, ToolSelection: "ALLOWLIST", ToolNames: []string{"read"}}}
	saved.Content.Assistants[0].McpBindings = &bindings
	saved, err = svc.PutDraft(ctx, boot.AdminUserID, saved.DraftRevision, saved.Content)
	if err != nil {
		t.Fatal(err)
	}
	saved.Content.Assistants[0].McpBindings = emptyMcpBindings()
	saved, err = svc.PutDraft(ctx, boot.AdminUserID, saved.DraftRevision, saved.Content)
	if err != nil || !saved.Content.Mcp[0].Enabled {
		t.Fatalf("removing the requirement disabled the server: %v", err)
	}
	after := store.Client.ManagedRelease.GetX(ctx, release.ReleaseID)
	if !bytes.Equal(before.SnapshotJSON, after.SnapshotJSON) || !bytes.Equal(before.ReleaseContentJSON, after.ReleaseContentJSON) {
		t.Fatal("editing mandatory bindings changed an immutable release")
	}
}
