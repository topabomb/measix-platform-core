package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestWorkspaceMcpDiscoveryUsesSelectedUserAndRejectsRevocation(t *testing.T) {
	ctx := context.Background()
	store, boot, _ := bootstrapI2(t)
	user := boot.AdminUserID
	var revoke bool
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/u/selected-user/mcp" || r.Header.Get("Authorization") != "Bearer workspace-private-token" {
			t.Errorf("wrong workspace path/credential")
		}
		var msg struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&msg)
		if msg.Method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		result := map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}}
		if msg.Method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": "workspace_read", "inputSchema": map[string]any{"type": "object"}}}}
			if revoke {
				store.Client.AgentSpace.UpdateOneID(user).SetIntent("DISCONNECTED").SetStopPending(true).SaveX(ctx)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}))
	defer server.Close()
	box, _ := security.NewSecretBox(bytes.Repeat([]byte{0x54}, 32), 1)
	secrets := upstream.NewService(store.Client, box)
	secret, err := secrets.CreateSecret(ctx, user, "workspace", "workspace-private-token")
	if err != nil {
		t.Fatal(err)
	}
	id, route, service := platformid.New(platformid.MCP), platformid.New(platformid.Route), platformid.New(platformid.WorkspaceService)
	now := time.Now().UTC()
	store.Client.WorkspaceService.Create().SetID(service).SetName("Workspace").SetConfigRevision(1).SetActiveConfigRevision(1).SetEnabled(true).SetState("ACTIVE").SetMcpServerID(id).SetRuntimeRouteID(route).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	cfg, _ := json.Marshal(map[string]any{"mcpOrigin": server.URL})
	store.Client.WorkspaceServiceConfig.Create().SetWorkspaceServiceID(service).SetRevision(1).SetConfigJSON(cfg).SetCreatedByUserID(user).SetCreatedAt(now).SaveX(ctx)
	store.Client.AgentSpace.Create().SetID(user).SetWorkspaceServiceID(service).SetRemoteUsername("selected-user").SetAgentSpaceID("space-test").SetBindingRevision(1).SetIntent("CONNECTED").SetState("CONNECTED").SetMcpSecretID(secret.SecretID).SetMcpSecretVersion(1).SetDavConfirmed(false).SetRemoteActive(true).SetStopPending(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	svc := capability.NewService(store.Client)
	svc.Secrets = secrets
	draft, _ := svc.GetDraft(ctx)
	empty := []adminapi.McpToolGrant{}
	kind := adminapi.RuntimeBindingDefinitionTargetKind("REMOTE_WORKSPACE")
	draft.Content.Mcp = []adminapi.McpDefinition{{McpServerId: id, DisplayName: "Workspace", ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "ENTERPRISE_MANAGED", RuntimePath: "/mcp", Enabled: true, ToolAccessMode: new(adminapi.McpDefinitionToolAccessMode("ALL")), AllowedTools: &empty}}
	draft.Content.Bindings = []adminapi.RuntimeBindingDefinition{{ResourceId: id, RuntimeRouteId: route, UpstreamId: "", TargetKind: &kind, WorkspaceServiceId: &service, AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE"}}
	draft, err = svc.PutDraft(ctx, user, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	req := adminapi.DiscoverMcpToolsRequest{ExpectedDraftRevision: draft.DraftRevision}
	if _, err = svc.DiscoverMcpTools(ctx, user, id, req); !errors.Is(err, capability.ErrMcpSourceUnavailable) || calls != 0 {
		t.Fatalf("discovery without selected identity: %v", err)
	}
	req.UserId = &user
	discovered, err := svc.DiscoverMcpTools(ctx, user, id, req)
	if err != nil {
		t.Fatal(err)
	}
	if discovered.Content.Mcp[0].ToolDiscovery.Tools[0].Name != "workspace_read" || strings.Contains(discovered.Content.Mcp[0].ToolDiscovery.SourceHash, "token") {
		t.Fatal("wrong workspace catalog")
	}
	revoke = true
	req.ExpectedDraftRevision = discovered.DraftRevision
	if _, err = svc.DiscoverMcpTools(ctx, user, id, req); !errors.Is(err, capability.ErrMcpSourceChanged) {
		t.Fatalf("revoked source accepted: %v", err)
	}
	current, _ := svc.GetDraft(ctx)
	if current.DraftRevision != discovered.DraftRevision {
		t.Fatal("revocation changed draft")
	}
}
