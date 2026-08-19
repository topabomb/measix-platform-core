package relay_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	relayruntime "github.com/topabomb/measix-platform-core/backend/internal/relay/runtime"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaystate"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

type runtimeFixture struct {
	store         *control.Store
	server        *httptest.Server
	signer        *security.AccessSigner
	userID        string
	deviceID      string
	sessionID     string
	interactionID string
	now           time.Time
}

func newRuntimeFixture(t *testing.T, state relaycontrolapi.RuntimeControlState, privateKey ed25519.PrivateKey) *runtimeFixture {
	t.Helper()
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	deploymentID := state.DeploymentId
	signer, err := security.NewAccessSigner(privateKey, deploymentID, "i4-key", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	signer.Now = func() time.Time { return now }
	state.AuthKeys = []relaycontrolapi.PublicJwk{{
		Kty: relaycontrolapi.OKP, Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA,
		Use: relaycontrolapi.Sig, Kid: "i4-key", X: signer.PublicJWK()["x"],
	}}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	state.BundleHash = hash
	store := control.NewStore(func() time.Time { return now })
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	return &runtimeFixture{
		store: store, server: httptest.NewServer(relayruntime.NewHandler(store)), signer: signer,
		userID: platformid.New(platformid.User), deviceID: platformid.New(platformid.Device),
		sessionID: platformid.New(platformid.Session), interactionID: platformid.New(platformid.Interaction), now: now,
	}
}

func (f *runtimeFixture) close() { f.server.Close() }

func (f *runtimeFixture) request(t *testing.T, ctx context.Context, method, resourceID, path string, body io.Reader, contentType string) *http.Request {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	token, _, err := f.signer.Sign(f.userID, f.deviceID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, method, f.server.URL+"/runtime/v1/resources/"+resourceID+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Measix-Managed-Generation", "1")
	request.Header.Set("X-Measix-Interaction-Id", f.interactionID)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

func TestI4BinaryMultipartAndMCPRemainTransparent(t *testing.T) {
	binaryPayload := []byte{0x00, 0x01, 0x02, 0xfe, 0xff, 0x00}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Set-Cookie", "must-not-leak=1")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(binaryPayload)
		case "/v1/audio/transcriptions":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse multipart: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("form file: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer file.Close()
			payload, _ := io.ReadAll(file)
			if string(payload) != "voice-bytes" || r.FormValue("model") != "asr-model" {
				t.Errorf("multipart changed: file=%q model=%q", payload, r.FormValue("model"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"text":"ok"}`)
		case "/mcp":
			payload, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	deploymentID := platformid.New(platformid.Deployment)
	upstreamID := platformid.New(platformid.Upstream)
	ttsID := platformid.New(platformid.TTS)
	asrID := platformid.New(platformid.ASR)
	mcpID := platformid.New(platformid.MCP)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: 1, ActiveManagedGeneration: 1, DeploymentId: deploymentID,
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{
			{ResourceId: ttsID, RuntimeRouteId: "rte_11111111-1111-4111-8111-111111111111"},
			{ResourceId: asrID, RuntimeRouteId: "rte_22222222-2222-4222-8222-222222222222"},
			{ResourceId: mcpID, RuntimeRouteId: "rte_33333333-3333-4333-8333-333333333333"},
		},
		Routes: []relaycontrolapi.RuntimeRouteSpec{
			{RuntimeRouteId: "rte_11111111-1111-4111-8111-111111111111", UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/speech"}, TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy("HTTP_BINARY_STREAM"), TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: "rte_22222222-2222-4222-8222-222222222222", UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/transcriptions"}, TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy("HTTP_MULTIPART"), TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: "rte_33333333-3333-4333-8333-333333333333", UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy("MCP_STREAMABLE_HTTP"), TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
		},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true,
			TransportCapabilities: []string{"HTTP_BINARY_STREAM", "HTTP_MULTIPART", "MCP_STREAMABLE_HTTP"},
			Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	fixture := newRuntimeFixture(t, state, privateKey)
	defer fixture.close()

	binaryResponse, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, ttsID, "/v1/audio/speech", strings.NewReader(`{"input":"hi"}`), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	gotBinary, _ := io.ReadAll(binaryResponse.Body)
	binaryResponse.Body.Close()
	if binaryResponse.StatusCode != http.StatusOK || !bytes.Equal(gotBinary, binaryPayload) || binaryResponse.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("binary changed: status=%d type=%q body=%v", binaryResponse.StatusCode, binaryResponse.Header.Get("Content-Type"), gotBinary)
	}
	if binaryResponse.Header.Get("Set-Cookie") != "" {
		t.Fatal("upstream Set-Cookie leaked to client")
	}

	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	file, _ := writer.CreateFormFile("file", "voice.wav")
	_, _ = io.WriteString(file, "voice-bytes")
	_ = writer.WriteField("model", "asr-model")
	_ = writer.Close()
	multipartResponse, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, asrID, "/v1/audio/transcriptions", &multipartBody, writer.FormDataContentType()))
	if err != nil {
		t.Fatal(err)
	}
	multipartPayload, _ := io.ReadAll(multipartResponse.Body)
	multipartResponse.Body.Close()
	if multipartResponse.StatusCode != http.StatusOK || string(multipartPayload) != `{"text":"ok"}` {
		t.Fatalf("multipart response status=%d body=%s", multipartResponse.StatusCode, multipartPayload)
	}

	mcpPayload := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	mcpResponse, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, mcpID, "/mcp?session=abc", strings.NewReader(mcpPayload), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	mcpBody, _ := io.ReadAll(mcpResponse.Body)
	mcpResponse.Body.Close()
	if mcpResponse.StatusCode != http.StatusOK || string(mcpBody) != mcpPayload {
		t.Fatalf("MCP semantics changed: status=%d body=%s", mcpResponse.StatusCode, mcpBody)
	}
}

func TestI4CancellationPropagatesToUpstream(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "old-token")
	defer fixture.close()
	ctx, cancel := context.WithCancel(context.Background())
	request := fixture.request(t, ctx, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"stream":true}`), "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	response.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("client cancellation was not propagated upstream")
	}
}

func TestI4RequestCapturesControlStateAcrossAtomicApply(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var firstAuth string
	var firstOnce sync.Once
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstOnce.Do(func() {
			firstAuth = r.Header.Get("Authorization")
			close(started)
		})
		<-release
		_, _ = io.WriteString(w, "A")
	}))
	defer upstreamA.Close()
	var secondAuth string
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "B")
	}))
	defer upstreamB.Close()

	fixture, resourceID := singleRouteFixture(t, upstreamA.URL, "old-token")
	defer fixture.close()
	firstDone := make(chan string, 1)
	go func() {
		response, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader("first"), "application/json"))
		if err != nil {
			firstDone <- "error:" + err.Error()
			return
		}
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		firstDone <- string(payload)
	}()
	<-started

	current := fixture.store.Current()
	if current == nil {
		t.Fatal("missing current state")
	}
	state2 := relaycontrolapi.RuntimeControlState{
		ControlRevision: 2, ActiveManagedGeneration: 1, DeploymentId: current.DeploymentID,
		AuthKeys: []relaycontrolapi.PublicJwk{{Kty: relaycontrolapi.OKP, Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA, Use: relaycontrolapi.Sig, Kid: "i4-key", X: fixture.signer.PublicJWK()["x"]}},
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: "rte_44444444-4444-4444-8444-444444444444"}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: "rte_44444444-4444-4444-8444-444444444444", UpstreamId: "ups_55555555-5555-4555-8555-555555555555", AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy("HTTP_STREAMING_SSE"), TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: "ups_55555555-5555-4555-8555-555555555555", BaseUrl: upstreamB.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "new-token"}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	hash, err := relaystate.HashDescriptor(state2)
	if err != nil {
		t.Fatal(err)
	}
	state2.BundleHash = hash
	if _, err := fixture.store.Apply(state2); err != nil {
		t.Fatal(err)
	}
	close(release)
	if first := <-firstDone; first != "A" || firstAuth != "Bearer old-token" {
		t.Fatalf("in-flight request switched state: body=%q auth=%q", first, firstAuth)
	}

	second, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader("second"), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	secondBody, _ := io.ReadAll(second.Body)
	second.Body.Close()
	if string(secondBody) != "B" || secondAuth != "Bearer new-token" {
		t.Fatalf("new request did not use new state: body=%q auth=%q", secondBody, secondAuth)
	}
}

func singleRouteFixture(t *testing.T, upstreamURL, token string) (*runtimeFixture, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: 1, ActiveManagedGeneration: 1, DeploymentId: platformid.New(platformid.Deployment),
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: "rte_66666666-6666-4666-8666-666666666666"}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: "rte_66666666-6666-4666-8666-666666666666", UpstreamId: "ups_77777777-7777-4777-8777-777777777777", AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy("HTTP_STREAMING_SSE"), TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: "ups_77777777-7777-4777-8777-777777777777", BaseUrl: upstreamURL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": token}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), resourceID
}

func TestI4RuntimeURLKeepsBasePathAndQuery(t *testing.T) {
	seen := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL+"/adapter", "token")
	defer fixture.close()
	response, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions?x=1&y=two", strings.NewReader("{}"), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if got := <-seen; got != "/adapter/v1/chat/completions?x=1&y=two" {
		t.Fatalf("target URL changed: %q", got)
	}
}
