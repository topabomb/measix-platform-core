package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

func TestDashScopeHTTPASRThroughRelay(t *testing.T) {
	const path = "/api/v1/services/aigc/multimodal-generation/generation"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer synthetic-asr-key" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "wrong method, path, credential or content type", http.StatusBadRequest)
			return
		}
		var request struct {
			Model string `json:"model"`
			Input struct {
				Messages []struct {
					Content []struct {
						Type       string `json:"type"`
						InputAudio struct {
							Data string `json:"data"`
						} `json:"input_audio"`
					} `json:"content"`
				} `json:"messages"`
			} `json:"input"`
			Parameters struct {
				Format string `json:"format"`
			} `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "qwen-audio-3.0-asr-flash" ||
			len(request.Input.Messages) != 1 || len(request.Input.Messages[0].Content) != 1 ||
			request.Input.Messages[0].Content[0].Type != "input_audio" ||
			!strings.HasPrefix(request.Input.Messages[0].Content[0].InputAudio.Data, "data:audio/wav;base64,") || request.Parameters.Format != "wav" {
			http.Error(w, "wrong DashScope ASR body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"text":"识别成功"},"usage":{"duration":1}}`)
	}))
	defer upstream.Close()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resourceID, routeID, upstreamID := platformid.New(platformid.ASR), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
	state := minimalControlState(t, 1, 1)
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{path}, TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE,
		TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000}}}
	state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true,
		TransportCapabilities: []string{"HTTP_REQUEST_RESPONSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{
			Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "synthetic-asr-key"}}}}
	fixture := newRuntimeFixture(t, state, key)
	defer fixture.close()
	request := fixture.request(t, nil, http.MethodPost, resourceID, path, strings.NewReader(`{"model":"qwen-audio-3.0-asr-flash","input":{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"data:audio/wav;base64,UklGRg=="}}]}]},"parameters":{"format":"wav"}}`), "application/json")
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"text":"识别成功"`) {
		t.Fatalf("status=%d body=%s err=%v", response.StatusCode, body, err)
	}
}
