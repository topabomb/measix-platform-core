package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// IMAGE is a declared managed input modality. Relay must preserve the native
// provider's image parts and conversation history, without requiring file upload
// APIs or translating bodies. This proves transport, not image recognition.
func TestManagedImageInputsPreserveNativePayload(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jhZkAAAAASUVORK5CYII="
	for _, tc := range []struct{ name, path, body string }{
		{"chat", "/v1/chat/completions", `{"model":"vision","stream":true,"messages":[{"role":"assistant","content":"Previous answer"},{"role":"user","content":[{"type":"text","text":"Describe this image"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` + png + `"}}]}]}`},
		{"responses", "/v1/responses", `{"model":"vision","stream":true,"store":false,"input":[{"role":"assistant","content":"Previous answer"},{"role":"user","content":[{"type":"input_text","text":"Describe this image"},{"type":"input_image","image_url":"data:image/png;base64,` + png + `"}]}]}`},
		{"gemini", "/v1beta/models/vision:streamGenerateContent?alt=sse", `{"contents":[{"role":"model","parts":[{"text":"Previous answer"}]},{"role":"user","parts":[{"text":"Describe this image"},{"inlineData":{"mimeType":"image/png","data":"` + png + `"}}]}]}`},
		{"claude", "/v1/messages", `{"model":"vision","max_tokens":128,"stream":true,"messages":[{"role":"assistant","content":"Previous answer"},{"role":"user","content":[{"type":"text","text":"Describe this image"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + png + `"}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != tc.body || r.URL.RequestURI() != tc.path {
					t.Error("native image payload/history/path changed in transit")
					http.Error(w, "payload mismatch", 400)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"synthetic\":true}\n\n")
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
			state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE}}}
			fixture := newRuntimeFixture(t, state, key)
			defer fixture.close()
			request := fixture.request(t, nil, http.MethodPost, resourceID, tc.path, strings.NewReader(tc.body), "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 200 || string(body) != "data: {\"synthetic\":true}\n\n" {
				t.Fatalf("response status=%d body=%s err=%v", response.StatusCode, body, err)
			}
		})
	}
}
