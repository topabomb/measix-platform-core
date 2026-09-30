package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// Uses the MCP 2025-06-18 Streamable HTTP contract, including optional stateful
// sessions. This is Relay transport evidence, not a live Firecrawl qualification.
func TestMCPStreamableHTTPSessionThroughRelay(t *testing.T) {
	const session = "test-session-41"
	const version = "2025-06-18"
	initialized, closed := false, false
	var sessionMu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionMu.Lock()
		defer sessionMu.Unlock()
		if r.URL.Path != "/v2/mcp" || r.Header.Get("Authorization") != "Bearer synthetic-mcp-key" {
			http.Error(w, "wrong endpoint or credential", http.StatusUnauthorized)
			return
		}
		var message struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if r.Method == http.MethodPost {
			if !strings.Contains(r.Header.Get("Accept"), "application/json") || !strings.Contains(r.Header.Get("Accept"), "text/event-stream") || json.NewDecoder(r.Body).Decode(&message) != nil || message.JSONRPC != "2.0" {
				http.Error(w, "invalid MCP request", http.StatusBadRequest)
				return
			}
		}
		if message.Method == "initialize" {
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(message.Params, &params) != nil || params.ProtocolVersion != version || r.Header.Get("Mcp-Session-Id") != "" {
				http.Error(w, "invalid initialization", http.StatusBadRequest)
				return
			}
			w.Header().Set("Mcp-Session-Id", session)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"strict-mcp","version":"test"}}}`)
			return
		}
		if r.Header.Get("Mcp-Session-Id") != session || r.Header.Get("MCP-Protocol-Version") != version {
			http.Error(w, "missing session or protocol header", http.StatusBadRequest)
			return
		}
		if closed {
			http.Error(w, "session expired", http.StatusNotFound)
			return
		}
		switch {
		case message.Method == "notifications/initialized":
			initialized = true
			w.WriteHeader(http.StatusAccepted)
		case !initialized:
			http.Error(w, "not initialized", http.StatusBadRequest)
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case r.Method == http.MethodDelete:
			closed = true
			w.WriteHeader(http.StatusNoContent)
		case message.Method == "tools/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"fixture_scrape","inputSchema":{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}}]}}`)
		case message.Method == "tools/call":
			if string(message.Params) != `{"name":"fixture_scrape","arguments":{"url":"https://example.com"}}` {
				http.Error(w, "changed tool arguments", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"fixture result\"}],\"isError\":false}}\n\n")
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer upstream.Close()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	state := minimalControlState(t, 1, 1)
	resourceID, routeID, upstreamID := platformid.New(platformid.MCP), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/v2/mcp"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000}}}
	state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"MCP_STREAMABLE_HTTP"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "synthetic-mcp-key"}}}}
	fixture := newRuntimeFixture(t, state, key)
	defer fixture.close()
	steps := []struct {
		method, body string
		status       int
		contains     string
	}{
		{"POST", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"relay-test","version":"test"}}}`, 200, `"serverInfo"`},
		{"POST", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, 202, ""},
		{"POST", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, 200, `"fixture_scrape"`},
		{"POST", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"fixture_scrape","arguments":{"url":"https://example.com"}}}`, 200, "event: message\ndata:"},
		{"GET", "", 405, ""},
		{"DELETE", "", 204, ""},
		{"POST", `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`, 404, "session expired"},
	}
	var sessionID string
	client := fixture.server.Client()
	client.Timeout = 5 * time.Second
	for i, step := range steps {
		req := fixture.request(t, nil, step.method, resourceID, "/v2/mcp", strings.NewReader(step.body), "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if i > 0 {
			req.Header.Set("Mcp-Session-Id", sessionID)
			req.Header.Set("MCP-Protocol-Version", version)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != step.status || !strings.Contains(string(body), step.contains) {
			t.Fatalf("step %d: status=%d body=%s err=%v", i, resp.StatusCode, body, err)
		}
		if i == 0 {
			sessionID = resp.Header.Get("Mcp-Session-Id")
			if sessionID != session {
				t.Fatalf("session header lost: %q", sessionID)
			}
		}
		if (step.status == 202 || step.status == 204) && len(body) != 0 {
			t.Fatalf("step %d: expected empty body", i)
		}
		if i == 3 && resp.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatal("SSE content type lost")
		}
	}
}
