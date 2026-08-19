package relay_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func TestRLYI4TransportsStreamWithoutProtocolTranslation(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte{0x49, 0x44, 0x33, 0x01, 0x02})
		case "/v1/audio/transcriptions":
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
				http.Error(w, "multipart required", http.StatusBadRequest)
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("model") != "whisper-test" {
				http.Error(w, "invalid multipart", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"text":"ok"}`))
		case "/mcp":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	fixture, ids := multiTransportFixture(t, upstream.URL)
	defer fixture.close()

	t.Run("TTS binary", func(t *testing.T) {
		request := fixture.request(t, nil, http.MethodPost, ids.tts, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`), "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "audio/mpeg" || !bytes.Equal(body, []byte{0x49, 0x44, 0x33, 0x01, 0x02}) {
			t.Fatalf("unexpected binary response: status=%d type=%q body=%v", response.StatusCode, response.Header.Get("Content-Type"), body)
		}
	})

	t.Run("ASR multipart", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("model", "whisper-test")
		part, err := writer.CreateFormFile("file", "sample.wav")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("RIFF-test"))
		_ = writer.Close()
		request := fixture.request(t, nil, http.MethodPost, ids.asr, "/v1/audio/transcriptions", &body, writer.FormDataContentType())
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("unexpected ASR status: %d", response.StatusCode)
		}
	})

	t.Run("MCP Streamable HTTP", func(t *testing.T) {
		request := fixture.request(t, nil, http.MethodPost, ids.mcp, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"tools"`)) {
			t.Fatalf("unexpected MCP response: status=%d body=%s", response.StatusCode, body)
		}
	})

	if gotAuth != "Bearer runtime-secret" {
		t.Fatalf("Relay did not inject service-side upstream credential: %q", gotAuth)
	}
}

type transportIDs struct{ tts, asr, mcp string }

func multiTransportFixture(t *testing.T, upstreamURL string) (*runtimeFixture, transportIDs) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ids := transportIDs{tts: platformid.New(platformid.TTS), asr: platformid.New(platformid.ASR), mcp: platformid.New(platformid.MCP)}
	upstreamID := platformid.New(platformid.Upstream)
	ttsRoute := platformid.New(platformid.Route)
	asrRoute := platformid.New(platformid.Route)
	mcpRoute := platformid.New(platformid.Route)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            platformid.New(platformid.Deployment),
		PrincipalState:          relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{
			{ResourceId: ids.tts, RuntimeRouteId: ttsRoute},
			{ResourceId: ids.asr, RuntimeRouteId: asrRoute},
			{ResourceId: ids.mcp, RuntimeRouteId: mcpRoute},
		},
		Routes: []relaycontrolapi.RuntimeRouteSpec{
			{RuntimeRouteId: ttsRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/speech"}, TransportPolicy: relaycontrolapi.HTTPBINARYSTREAM, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: asrRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/transcriptions"}, TransportPolicy: relaycontrolapi.HTTPMULTIPART, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: mcpRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
		},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true,
			TransportCapabilities: []string{"HTTP_BINARY_STREAM", "HTTP_MULTIPART", "MCP_STREAMABLE_HTTP"},
			Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "runtime-secret"}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), ids
}
