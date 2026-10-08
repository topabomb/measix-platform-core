package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestWorkspaceDiscoveryEligibleHTTPIsFilteredAndReadOnly(t *testing.T) {
	_, identity, _, ctx, actor := setupFullHandler(t)
	service := platformid.New(platformid.WorkspaceService)
	now := time.Now().UTC()
	identity.Client.WorkspaceService.Create().SetID(service).SetName("Discovery").SetConfigRevision(1).SetActiveConfigRevision(1).SetEnabled(true).SetState("ACTIVE").SetMcpServerID(platformid.New(platformid.MCP)).SetRuntimeRouteID(platformid.New(platformid.Route)).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	identity.Client.WorkspaceServiceConfig.Create().SetWorkspaceServiceID(service).SetRevision(1).SetConfigJSON([]byte(`{}`)).SetCreatedByUserID(actor).SetCreatedAt(now).SaveX(ctx)
	for _, id := range []string{actor, platformid.New(platformid.User)} {
		identity.Client.AgentSpace.Create().SetID(id).SetWorkspaceServiceID(service).SetRemoteUsername(id).SetBindingRevision(1).SetIntent("CONNECTED").SetState("CONNECTED").SetMcpSecretID("synthetic-binding").SetMcpSecretVersion(1).SetDavConfirmed(false).SetRemoteActive(true).SetStopPending(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	}
	svc := capability.NewService(identity.Client)
	before, _ := svc.GetDraft(ctx)
	h := httpapi.NewFull(httpapi.Services{Identity: identity, Workspace: workspace.NewService(identity.Client, nil)})
	cookie, _ := loginAdmin(t, h)
	path := "/api/admin/v1/remote-workspace/services/" + service + "/workspaces"
	for _, tc := range []struct {
		query         string
		status, count int
	}{{"", 200, 2}, {"?discoveryEligible=false", 200, 2}, {"?discoveryEligible=true", 200, 1}, {"?discoveryEligible=true&search=Admin", 200, 1}, {"?discoveryEligible=invalid", 400, 0}} {
		response := doJSON(t, h, "GET", path+tc.query, map[string]string{"Cookie": cookie}, nil)
		if response.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.query, response.Code, response.Body)
		}
		if response.Code == 200 {
			var page adminapi.WorkspaceList
			decodeJSON(t, response, &page)
			if len(page.Items) != tc.count {
				t.Fatalf("%s: %+v", tc.query, page)
			}
		}
	}
	if response := doJSON(t, h, "GET", path+"?discoveryEligible=true", nil, nil); response.Code != 401 {
		t.Fatal("discovery identities exposed without admin session")
	}
	after, _ := svc.GetDraft(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("reading discovery candidates mutated draft")
	}
}

func TestMcpDiscoveryHTTPVersionDiagnosticsPreserveCatalog(t *testing.T) {
	_, identity, _, ctx, actor := setupFullHandler(t)
	version := "2025-11-25"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&msg)
		if msg.Method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		result := map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}}
		if msg.Method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": "read", "inputSchema": map[string]any{"type": "object"}}}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}))
	defer server.Close()
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	ups := upstream.NewService(identity.Client, box)
	up, err := ups.CreateUpstream(ctx, actor, adminapi.UpstreamConfig{Name: "MCP", BaseUrl: server.URL, Auth: adminapi.UpstreamAuth{Type: adminapi.UpstreamAuthTypeNONE}, TransportCapabilities: []adminapi.UpstreamConfigTransportCapabilities{adminapi.UpstreamConfigTransportCapabilitiesHTTPSTREAMINGSSE}, CorrelationMode: adminapi.UpstreamConfigCorrelationModeHEADERECHO, UsageCapabilityLevel: adminapi.LEVEL0, TimeoutDefaults: adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	identity.Client.Upstream.UpdateOneID(up.UpstreamID).SetActiveConfigRevision(1).SetStatus("ACTIVE").SaveX(ctx)
	svc := capability.NewService(identity.Client)
	svc.Secrets = ups
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := platformid.New(platformid.MCP)
	empty := []adminapi.McpToolGrant{}
	draft.Content.Mcp = []adminapi.McpDefinition{{McpServerId: id, DisplayName: "MCP", ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "NONE", RuntimePath: "/mcp", Enabled: true, ToolAccessMode: new(adminapi.McpDefinitionToolAccessMode("ALL")), AllowedTools: &empty}}
	draft.Content.Bindings = []adminapi.RuntimeBindingDefinition{{ResourceId: id, RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: up.UpstreamID, AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE"}}
	draft, err = svc.PutDraft(ctx, actor, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewFull(httpapi.Services{Identity: identity, Capability: svc})
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	path := "/api/admin/v1/draft/mcp/" + id + ":discover"
	response := doJSON(t, h, http.MethodPost, path, headers, adminapi.DiscoverMcpToolsRequest{ExpectedDraftRevision: draft.DraftRevision})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	draft, _ = svc.GetDraft(ctx)
	before, _ := json.Marshal(draft.Content)
	for _, returned := range []string{"2024-11-05", "2026-01-01", "private-url secret-token"} {
		version = returned
		response = doJSON(t, h, http.MethodPost, path, headers, adminapi.DiscoverMcpToolsRequest{ExpectedDraftRevision: draft.DraftRevision})
		var problem adminapi.Problem
		decodeJSON(t, response, &problem)
		if response.Code != 502 || response.Header().Get("Content-Type") != "application/problem+json" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("invalid problem response: %v", response)
		}
		if returned == "private-url secret-token" {
			if problem.Code != "mcp_discovery_protocol" || problem.ReceivedMcpProtocolVersion != nil || strings.Contains(response.Body.String(), "secret-token") {
				t.Fatal("unsafe version diagnostic")
			}
		} else if problem.Code != "mcp_discovery_version" || problem.ReceivedMcpProtocolVersion == nil || *problem.ReceivedMcpProtocolVersion != returned || problem.SupportedMcpProtocolVersions == nil || !reflect.DeepEqual(*problem.SupportedMcpProtocolVersions, capability.SupportedMcpProtocolVersions()) {
			t.Fatalf("missing version diagnostics: %+v", problem)
		}
		current, _ := svc.GetDraft(ctx)
		after, _ := json.Marshal(current.Content)
		if current.DraftRevision != draft.DraftRevision || !bytes.Equal(before, after) {
			t.Fatal("failed discovery mutated draft or catalog")
		}
	}
}

func TestMcpDiscoveryHTTPRequiresAdminCSRFAndCurrentDraft(t *testing.T) {
	_, id, _, ctx, _ := setupFullHandler(t)
	svc := capability.NewService(id.Client)
	h := httpapi.NewFull(httpapi.Services{Identity: id, Capability: svc})
	cookie, csrf := loginAdmin(t, h)
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/v1/draft/mcp/" + platformid.New(platformid.MCP) + ":discover"
	for _, tc := range []struct {
		name    string
		headers map[string]string
		body    any
		status  int
		code    string
	}{
		{"unauthenticated", map[string]string{"X-CSRF-Token": "synthetic-csrf"}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 401, ""},
		{"missing-csrf", map[string]string{"Cookie": cookie}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 400, ""},
		{"invalid-csrf", map[string]string{"Cookie": cookie, "X-CSRF-Token": "synthetic-csrf"}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 401, "unauthenticated"},
		{"missing-revision", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{}, 400, "invalid_request"},
		{"unknown-field", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expectedDraftRevision": draft.DraftRevision, "endpoint": "http://private"}, 400, "invalid_request"},
		{"stale-revision", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expectedDraftRevision": draft.DraftRevision + 1}, 409, "stale_draft_revision"},
		{"unavailable-source", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 422, "mcp_source_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := doJSON(t, h, http.MethodPost, path, tc.headers, tc.body)
			if response.Code != tc.status {
				t.Fatalf("expected %d got %d: %s", tc.status, response.Code, response.Body.String())
			}
			if tc.code != "" {
				var problem struct {
					Code string `json:"code"`
				}
				decodeJSON(t, response, &problem)
				if problem.Code != tc.code {
					t.Fatalf("wrong safe problem %s", problem.Code)
				}
			}
			current, err := svc.GetDraft(ctx)
			if err != nil || current.DraftRevision != draft.DraftRevision {
				t.Fatal("rejected discovery changed draft")
			}
		})
	}
}
