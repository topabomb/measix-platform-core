package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func emptyMcpBindings() *[]adminapi.AssistantMcpBinding { return &[]adminapi.AssistantMcpBinding{} }

func TestDirectMcpDiscoveryAnswersServerRequestsDuringSSE(t *testing.T) {
	for _, method := range []string{"ping", "sampling/createMessage"} {
		t.Run(method, func(t *testing.T) {
			replies := make(chan map[string]json.RawMessage, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var msg map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
					t.Error(err)
					return
				}
				if len(msg["method"]) == 0 {
					replies <- msg
					w.WriteHeader(http.StatusAccepted)
					return
				}
				var requested string
				json.Unmarshal(msg["method"], &requested)
				switch requested {
				case "initialize":
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{}}}}`, msg["id"])
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
				case "tools/list":
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":\"server-request\",\"method\":%q}\n\n", method)
					w.(http.Flusher).Flush()
					select {
					case reply := <-replies:
						if string(reply["id"]) != `"server-request"` || string(reply["jsonrpc"]) != `"2.0"` {
							t.Errorf("incorrect response identity: %s", reply)
						}
						if method == "ping" {
							if string(reply["result"]) != `{}` || len(reply["error"]) != 0 {
								t.Errorf("ping was not answered: %s", reply)
							}
						} else {
							var problem struct{ Code int }
							if json.Unmarshal(reply["error"], &problem) != nil || problem.Code != -32601 || len(reply["result"]) != 0 {
								t.Errorf("undeclared capability was not rejected: %s", reply)
							}
						}
					case <-time.After(time.Second):
						t.Error("server request was not answered")
					}
					fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"tools\":[]}}\n\n", msg["id"])
				default:
					t.Errorf("unexpected discovery action: %s", requested)
				}
			}))
			defer server.Close()
			tools, err := capability.DiscoverMcpCatalog(context.Background(), server.URL, http.Header{})
			if err != nil || tools == nil || len(tools) != 0 {
				t.Fatalf("complete catalog rejected after server request: %v %v", tools, err)
			}
		})
	}
}

func TestDirectMcpDiscoveryRejectsIncompleteAndUnboundedCatalogs(t *testing.T) {
	for _, kind := range []string{"duplicate", "cursor_loop", "missing_tools", "invalid_schema", "wrong_id", "oversize", "no_capability"} {
		t.Run(kind, func(t *testing.T) {
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
				w.Header().Set("Content-Type", "application/json")
				result := map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}}
				if kind == "no_capability" {
					result["capabilities"] = map[string]any{}
				}
				if msg.Method == "tools/list" {
					def := map[string]any{"name": "read", "inputSchema": map[string]any{"type": "object"}}
					result = map[string]any{"tools": []any{def}}
					switch kind {
					case "duplicate":
						result["tools"] = []any{def, def}
					case "cursor_loop":
						result["nextCursor"] = "again"
					case "missing_tools":
						delete(result, "tools")
					case "invalid_schema":
						def["inputSchema"] = nil
					case "wrong_id":
						msg.ID++
					case "oversize":
						def["description"] = strings.Repeat("x", capability.MaxSnapshotBytes)
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
			}))
			defer server.Close()
			tools, err := capability.DiscoverMcpCatalog(context.Background(), server.URL, http.Header{})
			if err == nil || tools != nil {
				t.Fatalf("invalid catalog returned %v %v", tools, err)
			}
		})
	}
}

func TestDirectMcpDraftDiscoveryApprovalDriftAndConcurrency(t *testing.T) {
	ctx := context.Background()
	store, boot, _ := bootstrapI2(t)
	mode := "initial"
	var duringDiscovery func()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "failure" {
			w.WriteHeader(503)
			fmt.Fprint(w, "private-service-url secret-token")
			return
		}
		var msg struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&msg)
		w.Header().Set("Content-Type", "application/json")
		if msg.Method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		var result any = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}}
		if msg.Method == "tools/list" {
			if duringDiscovery != nil {
				hook := duringDiscovery
				duringDiscovery = nil
				hook()
			}
			description := "read records"
			if mode == "drift" {
				description = "changed read"
			}
			tools := []any{map[string]any{"name": "read", "description": description, "inputSchema": map[string]any{"type": "object"}}}
			if mode == "drift" {
				tools = append(tools, map[string]any{"name": "new_tool", "inputSchema": map[string]any{"type": "object"}})
			}
			if mode == "deleted" {
				tools = []any{}
			}
			result = map[string]any{"tools": tools}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}))
	defer server.Close()
	box, _ := security.NewSecretBox(bytes.Repeat([]byte{0x33}, 32), 1)
	ups := upstream.NewService(store.Client, box)
	secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "test", "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	config := testUpstreamConfig(secret.SecretID, 1)
	config.BaseUrl = server.URL
	up, err := ups.CreateUpstream(ctx, boot.AdminUserID, config)
	if err != nil {
		t.Fatal(err)
	}
	store.Client.Upstream.UpdateOneID(up.UpstreamID).SetActiveConfigRevision(1).SetStatus("ACTIVE").SaveX(ctx)
	svc := capability.NewService(store.Client)
	svc.Secrets = ups
	draft, _ := svc.GetDraft(ctx)
	content := validDraft(up.UpstreamID)
	id := platformid.New(platformid.MCP)
	empty := []adminapi.McpToolGrant{}
	content.Mcp = []adminapi.McpDefinition{{McpServerId: id, DisplayName: "Tools", ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "ENTERPRISE_MANAGED", Enabled: true, RuntimePath: "/mcp", ToolAccessMode: new(adminapi.McpDefinitionToolAccessMode("ALL")), AllowedTools: &empty}}
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{RuntimeRouteId: platformid.New(platformid.Route), ResourceId: id, UpstreamId: up.UpstreamID, AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE"})
	draft, err = svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	discover := func() (capability.DraftView, error) {
		return svc.DiscoverMcpTools(ctx, boot.AdminUserID, id, adminapi.DiscoverMcpToolsRequest{ExpectedDraftRevision: draft.DraftRevision})
	}
	draft, err = discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(*draft.Content.Mcp[0].AllowedTools) != 0 {
		t.Fatal("discovery approved tools")
	}
	if draft.Content.Mcp[0].ToolDiscovery == nil {
		t.Fatal("no durable catalog")
	}
	tool := draft.Content.Mcp[0].ToolDiscovery.Tools[0]
	grants := []adminapi.McpToolGrant{{Name: tool.Name, ContractHash: tool.ContractHash, Definition: tool.Definition, ApprovalPolicy: "REQUIRE_CONFIRMATION"}}
	draft.Content.Mcp[0].ToolAccessMode = new(adminapi.McpDefinitionToolAccessMode("ALLOWLIST"))
	draft.Content.Mcp[0].AllowedTools = &grants
	bindings := []adminapi.AssistantMcpBinding{{McpServerId: id, ToolSelection: "ALLOWLIST", ToolNames: []string{"read"}}}
	draft.Content.Assistants = []adminapi.ManagedAssistantDefinition{{AssistantDefinitionId: platformid.New(platformid.Assistant), DisplayName: "A", SystemPrompt: "Read then summarize", ModelId: content.Models[0].ModelId, MemorySeed: []string{}, McpBindings: &bindings, Enabled: true}}
	draft, err = svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := svc.ValidateDraft(ctx, draft.DraftRevision)
	if err != nil || !valid.Valid {
		t.Fatalf("valid approval rejected: %+v %v", valid, err)
	}
	preview, err := svc.PreviewDraft(ctx, draft.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	initialHash := preview.ProjectionHash
	draft, err = discover()
	if err != nil {
		t.Fatal(err)
	}
	preview, err = svc.PreviewDraft(ctx, draft.DraftRevision)
	if err != nil || preview.ProjectionHash != initialHash {
		t.Fatal("refresh changed projection")
	}
	mode = "drift"
	draft, err = discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(*draft.Content.Mcp[0].AllowedTools) != 1 || (*draft.Content.Mcp[0].AllowedTools)[0].ContractHash != tool.ContractHash {
		t.Fatal("drift automatically approved")
	}
	valid, _ = svc.ValidateDraft(ctx, draft.DraftRevision)
	if valid.Valid {
		t.Fatal("changed contract can publish")
	}
	if _, err = svc.PreviewDraft(ctx, draft.DraftRevision); !errors.Is(err, capability.ErrInvalidDraft) {
		t.Fatal("preview bypassed review")
	}
	drifted := draft.Content.Mcp[0].ToolDiscovery.Tools[1]
	if drifted.Name != "read" {
		t.Fatal("fixture ordering")
	}
	grants = []adminapi.McpToolGrant{{Name: drifted.Name, ContractHash: drifted.ContractHash, Definition: drifted.Definition, ApprovalPolicy: "REQUIRE_CONFIRMATION"}}
	draft.Content.Mcp[0].ToolAccessMode = new(adminapi.McpDefinitionToolAccessMode("ALLOWLIST"))
	draft.Content.Mcp[0].AllowedTools = &grants
	draft, err = svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	mode = "failure"
	before := draft.DraftRevision
	if _, err = discover(); !errors.Is(err, capability.ErrMcpDiscoveryUnavailable) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe failure %v", err)
	}
	current, _ := svc.GetDraft(ctx)
	if current.DraftRevision != before || len(current.Content.Mcp[0].ToolDiscovery.Tools) != 2 {
		t.Fatal("failure erased catalog")
	}
	mode = "initial"
	duringDiscovery = func() {
		latest, _ := svc.GetDraft(ctx)
		latest.Content.Mcp[0].DisplayName = "changed concurrently"
		if _, e := svc.PutDraft(ctx, boot.AdminUserID, latest.DraftRevision, latest.Content); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = discover(); !errors.Is(err, capability.ErrRevisionConflict) {
		t.Fatalf("late result accepted: %v", err)
	}
	current, _ = svc.GetDraft(ctx)
	current.Content.Mcp[0].ToolDiscovery.Tools[0].Definition["description"] = "forged candidate"
	if _, err = svc.PutDraft(ctx, boot.AdminUserID, current.DraftRevision, current.Content); !errors.Is(err, capability.ErrMcpToolEvidence) {
		t.Fatalf("forgery accepted %v", err)
	}
}

func TestDirectMcpDiscoveryPagedSSESessionAndFullContract(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing private credential")
		}
		if r.Method == "DELETE" {
			calls = append(calls, "DELETE")
			w.WriteHeader(204)
			return
		}
		var msg struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&msg)
		calls = append(calls, msg.Method)
		if msg.Method != "initialize" && (r.Header.Get("Mcp-Session-Id") != "s1" || r.Header.Get("MCP-Protocol-Version") != "2025-06-18") {
			t.Error("negotiated headers missing")
		}
		if msg.Method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch msg.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "s1")
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
		case "tools/list":
			name := "read"
			if msg.Params["cursor"] == "next" {
				name = "write"
				w.Header().Set("Content-Type", "text/event-stream")
			}
			result = map[string]any{"tools": []any{map[string]any{"name": name, "description": "Full contract", "inputSchema": map[string]any{"type": "object"}, "_meta": map[string]any{"a": 1}}}}
			if name == "read" {
				result.(map[string]any)["nextCursor"] = "next"
			}
		default:
			t.Errorf("unexpected method %s", msg.Method)
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
		if w.Header().Get("Content-Type") == "text/event-stream" {
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", body)
			w.(http.Flusher).Flush()
		} else {
			w.Write(body)
		}
	}))
	defer server.Close()
	tools, err := capability.DiscoverMcpCatalog(context.Background(), server.URL, http.Header{"Authorization": []string{"Bearer secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || tools[0].Name != "read" || tools[1].Name != "write" {
		t.Fatalf("incomplete catalog: %+v", tools)
	}
	if strings.Join(calls, ",") != "initialize,notifications/initialized,tools/list,tools/list,DELETE" {
		t.Fatalf("unexpected calls %v", calls)
	}
	hash, err := capability.McpToolContractHash(tools[0].Definition)
	if err != nil || hash != tools[0].ContractHash {
		t.Fatalf("hash %s %v", hash, err)
	}
	tools[0].Definition["description"] = "changed"
	changed, _ := capability.McpToolContractHash(tools[0].Definition)
	if hash == changed {
		t.Fatal("description drift ignored")
	}
}
