package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// These strict peers verify the Relay boundary, not a model's intelligence or
// the Android encoder. Provider state (IDs, reasoning and signatures) must make
// both HTTP trips unchanged. Protocol sources are recorded in the verification plan.
func TestModelToolRoundTripsPreserveProviderState(t *testing.T) {
	for _, tc := range []struct {
		name, path, header, first, result, call, final string
	}{
		{
			name: "chat", path: "/v1/chat/completions", header: "Authorization",
			first:  `{"model":"test","stream":true,"messages":[{"role":"user","content":"read status"}],"tools":[{"type":"function","function":{"name":"read_status","parameters":{"type":"object","properties":{}}}}]}`,
			result: `{"model":"test","stream":true,"messages":[{"role":"user","content":"read status"},{"role":"assistant","content":null,"reasoning_content":"Read the current status.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_status","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ready"}],"tools":[{"type":"function","function":{"name":"read_status","parameters":{"type":"object","properties":{}}}}]}`,
			call:   "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"Read the current status.\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read_status\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n",
			final:  "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ready\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
		},
		{
			name: "responses", path: "/v1/responses", header: "Authorization",
			first:  `{"model":"test","stream":true,"store":false,"input":[{"role":"user","content":"read status"}],"tools":[{"type":"function","name":"read_status","parameters":{"type":"object","properties":{}}}]}`,
			result: `{"model":"test","stream":true,"store":false,"input":[{"role":"user","content":"read status"},{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque-reasoning"},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_status","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"ready"}],"tools":[{"type":"function","name":"read_status","parameters":{"type":"object","properties":{}}}]}`,
			call:   "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[{\"type\":\"reasoning\",\"id\":\"rs_1\",\"summary\":[],\"encrypted_content\":\"opaque-reasoning\"},{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read_status\",\"arguments\":\"{}\"}]}}\n\n",
			final:  "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ready\"}]}]}}\n\n",
		},
		{
			name: "gemini", path: "/v1beta/models/test:streamGenerateContent?alt=sse", header: "x-goog-api-key",
			first:  `{"contents":[{"role":"user","parts":[{"text":"read status"}]}],"tools":[{"functionDeclarations":[{"name":"read_status","parameters":{"type":"object","properties":{}}}]}]}`,
			result: `{"contents":[{"role":"user","parts":[{"text":"read status"}]},{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"read_status","args":{}},"thoughtSignature":"c2lnbmF0dXJl"}]},{"role":"user","parts":[{"functionResponse":{"id":"call_1","name":"read_status","response":{"status":"ready"}}}]}],"tools":[{"functionDeclarations":[{"name":"read_status","parameters":{"type":"object","properties":{}}}]}]}`,
			call:   "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"id\":\"call_1\",\"name\":\"read_status\",\"args\":{}},\"thoughtSignature\":\"c2lnbmF0dXJl\"}]},\"finishReason\":\"STOP\"}]}\n\n",
			final:  "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ready\"}]},\"finishReason\":\"STOP\"}]}\n\n",
		},
		{
			name: "claude", path: "/v1/messages", header: "x-api-key",
			first:  `{"model":"test","stream":true,"max_tokens":100,"messages":[{"role":"user","content":"read status"}],"tools":[{"name":"read_status","input_schema":{"type":"object","properties":{}}}]}`,
			result: `{"model":"test","stream":true,"max_tokens":100,"messages":[{"role":"user","content":"read status"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_status","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ready"}]}],"tools":[{"name":"read_status","input_schema":{"type":"object","properties":{}}}]}`,
			call:   "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"read_status\",\"input\":{}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			final:  "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ready\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := "synthetic-key"
				if tc.header == "Authorization" {
					key = "Bearer " + key
				}
				if r.Header.Get(tc.header) != key || (tc.header != "Authorization" && r.Header.Get("Authorization") != "") {
					http.Error(w, "credential boundary", 401)
					return
				}
				if r.Method != "POST" || r.URL.RequestURI() != tc.path || (tc.name == "claude" && r.Header.Get("anthropic-version") != "2023-06-01") {
					http.Error(w, "request metadata changed", 400)
					return
				}
				body, _ := io.ReadAll(r.Body)
				n := calls.Add(1)
				want, reply := tc.first, tc.call
				if n == 2 {
					want, reply = tc.result, tc.final
				}
				if n > 2 {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Retry-After", "3")
					w.WriteHeader(429)
					io.WriteString(w, `{"error":{"message":"synthetic quota exhausted"}}`)
					return
				}
				if string(body) != want {
					http.Error(w, "tool state changed", 400)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, reply)
			}))
			defer upstream.Close()
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			state := minimalControlState(t, 1, 1)
			resourceID, routeID, upstreamID := platformid.New(platformid.Model), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
			state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}}
			state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{strings.Split(tc.path, "?")[0]}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000}}}
			auth := relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.STATICHEADER, AdditionalProperties: map[string]interface{}{"headerName": tc.header, "value": "synthetic-key"}}
			if tc.header == "Authorization" {
				auth = relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "synthetic-key"}}
			}
			state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: auth}}
			fixture := newRuntimeFixture(t, state, key)
			defer fixture.close()
			for i, body := range []string{tc.first, tc.result, tc.result} {
				req := fixture.request(t, nil, http.MethodPost, resourceID, tc.path, strings.NewReader(body), "application/json")
				req.Header.Set("anthropic-version", "2023-06-01")
				resp, err := fixture.server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				got, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				want, status := tc.call, 200
				if i == 1 {
					want = tc.final
				}
				if i == 2 {
					want, status = `{"error":{"message":"synthetic quota exhausted"}}`, 429
					if resp.Header.Get("Retry-After") != "3" {
						t.Fatal("lost retry hint")
					}
				}
				if readErr != nil || resp.StatusCode != status || string(got) != want {
					t.Fatalf("turn=%d status=%d body=%s err=%v", i, resp.StatusCode, got, readErr)
				}
			}
			if calls.Load() != 3 {
				t.Fatalf("unexpected replay: %d calls", calls.Load())
			}
		})
	}
}
