package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
)

// Run provider-shaped requests through the real authenticated Relay boundary.
// The synthetic credential must replace the platform bearer at the upstream.
func TestProviderProtocolsThroughRelay(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	target, err := url.Parse(a.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, body, header, contains string }{
		{"responses", "/v1/responses", `{"model":"test-model","input":[{"role":"user","content":"hello"}],"stream":true,"store":false}`, "Authorization", "response.completed"},
		{"gemini-model", "/v1beta/models/gemini-test:streamGenerateContent?alt=sse", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, "x-goog-api-key", `"finishReason":"STOP"`},
		{"claude-model", "/v1/messages", `{"model":"claude-test","max_tokens":100,"messages":[{"role":"user","content":"hello"}],"stream":true}`, "x-api-key", "event: message_stop"},
		{"mimo-speech", "/v1/chat/completions", `{"model":"mimo-v2.5-tts","messages":[{"role":"assistant","content":"hello"}],"audio":{"voice":"mimo_default","format":"pcm16"},"stream":true}`, "api-key", `"audio":{"data":"AAABAP//AAA="}`},
		{"gemini-speech", "/v1beta/models/gemini-2.5-flash-preview-tts:generateContent", `{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"responseModalities":["AUDIO"],"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}}}`, "x-goog-api-key", `"data":"AAABAP//AAA="`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := httputil.NewSingleHostReverseProxy(target)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expected := "synthetic-provider-key"
				if tc.header == "Authorization" {
					expected = "Bearer " + expected
				}
				if r.Header.Get(tc.header) != expected || (tc.header != "Authorization" && r.Header.Get("Authorization") != "") {
					http.Error(w, "credential boundary violated", http.StatusUnauthorized)
					return
				}
				proxy.ServeHTTP(w, r)
			}))
			defer upstream.Close()
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			state := minimalControlState(t, 1, 1)
			resourceID, routeID, upstreamID := platformid.New(platformid.Model), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
			if strings.Contains(tc.name, "speech") {
				resourceID = platformid.New(platformid.TTS)
			}
			state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}}
			state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{strings.Split(tc.path, "?")[0]}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000}}}
			auth := relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.STATICHEADER, AdditionalProperties: map[string]interface{}{"headerName": tc.header, "value": "synthetic-provider-key"}}
			if tc.header == "Authorization" {
				auth = relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "synthetic-provider-key"}}
			}
			state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: auth}}
			fixture := newRuntimeFixture(t, state, key)
			defer fixture.close()
			req := fixture.request(t, nil, http.MethodPost, resourceID, tc.path, strings.NewReader(tc.body), "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			resp, err := fixture.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != 200 || !strings.Contains(string(body), tc.contains) {
				t.Fatalf("status=%d body=%s err=%v", resp.StatusCode, body, err)
			}
			fact := a.LastRequest(strings.Split(tc.path, "?")[0])
			if fact == nil || fact.XMeasixRequestId == "" {
				t.Fatal("missing upstream request correlation")
			}
		})
	}
}
