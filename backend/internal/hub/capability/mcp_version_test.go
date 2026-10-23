package capability_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"measix/platform/internal/hub/capability"
)

func TestDiscoveryNegotiatedVersionsAndSafeRejection(t *testing.T) {
	for _, version := range []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05", "2026-01-01", "private-url secret-token"} {
		for _, sse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sse=%t", version, sse), func(t *testing.T) {
				modern := strings.HasPrefix(version, "2025-")
				listed, closed := false, false
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						closed = true
						if modern && r.Header.Get("MCP-Protocol-Version") != version {
							t.Error("cleanup lost negotiated version")
						}
						w.WriteHeader(204)
						return
					}
					var msg struct {
						ID     int            `json:"id"`
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
					}
					json.NewDecoder(r.Body).Decode(&msg)
					var result any
					switch msg.Method {
					case "initialize":
						if msg.Params["protocolVersion"] != "2025-11-25" {
							t.Error("unexpected requested version")
						}
						w.Header().Set("Mcp-Session-Id", "test-session")
						result = map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}}
					case "notifications/initialized", "tools/list":
						if !modern {
							t.Error("unsupported version proceeded to discovery")
						}
						if r.Header.Get("MCP-Protocol-Version") != version || r.Header.Get("Mcp-Session-Id") != "test-session" {
							t.Error("lost negotiated session headers")
						}
						if msg.Method == "notifications/initialized" {
							w.WriteHeader(202)
							return
						}
						listed = true
						result = map[string]any{"tools": []any{map[string]any{"name": "read", "inputSchema": map[string]any{"type": "object"}}}}
					default:
						t.Errorf("unexpected action %q", msg.Method)
					}
					raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
					if sse {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprintf(w, "data: %s\n\n", raw)
					} else {
						w.Header().Set("Content-Type", "application/json")
						w.Write(raw)
					}
				}))
				defer server.Close()
				tools, err := capability.DiscoverMcpCatalog(context.Background(), server.URL, http.Header{})
				if modern {
					if err != nil || len(tools) != 1 || !listed {
						t.Fatalf("modern discovery failed: %v %v", tools, err)
					}
				} else if version == "private-url secret-token" {
					if !errors.Is(err, capability.ErrMcpDiscoveryProtocol) || strings.Contains(err.Error(), "secret-token") {
						t.Fatalf("unsafe malformed version error: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "unsupported protocol version") || !strings.Contains(err.Error(), version) {
					t.Fatalf("version mismatch was not distinguished: %v", err)
				}
				if !closed {
					t.Fatal("discovery session was not closed")
				}
			})
		}
	}
}
