package client_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"measix/platform/internal/relay/control"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
)

// TestClient must only use the client-facing runtime topology
// (/runtime/v1/resources/{resourceId}{runtimePath} + generation + interaction headers)
// and must not need upstreamId/runtimeRouteId/base URL/Secret.

func TestCAPC4001ClientChatNonStream(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	body, ct, err := c.ChatCompletion(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type=%q", ct)
	}
	if !bytes.Contains(body, []byte(`"role":"assistant"`)) {
		t.Fatalf("bad body: %s", body)
	}
	fact := env.adapter.LastRequest("/v1/chat/completions")
	if fact == nil || fact.XMeasixRequestId == "" {
		t.Fatalf("relay did not inject request correlation: %+v", fact)
	}
}

func TestCAPC4002ClientChatStream(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	var chunks int
	var done bool
	err := c.ChatCompletionStream(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func(data []byte) {
		if bytes.Contains(data, []byte("[DONE]")) {
			done = true
		} else {
			chunks++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if chunks < 2 || !done {
		t.Fatalf("chunks=%d done=%v", chunks, done)
	}
}

func TestCAPC4010ClientTTSBinary(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	body, ct, err := c.Speech(context.Background(), env.resourceIDs.tts, "/v1/audio/speech", `{"input":"hello","voice":"alloy"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct, "audio/mpeg") {
		t.Fatalf("content-type=%q", ct)
	}
	if !bytes.Equal(body, env.adapter.Bytes()) {
		t.Fatalf("binary corrupted: len=%d", len(body))
	}
}

func TestCAPC4020ClientASRMultipart(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	body, _, err := c.Transcription(context.Background(), env.resourceIDs.asr, "/v1/audio/transcriptions", "whisper-test", "sample.wav", []byte("RIFF-test-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"text":"transcribed"`)) {
		t.Fatalf("bad transcription: %s", body)
	}
	fact := env.adapter.LastRequest("/v1/audio/transcriptions")
	if fact == nil || fact.MultipartFields["model"] != "whisper-test" || !bytes.Contains(fact.MultipartFiles["file"], []byte("RIFF-test-bytes")) {
		t.Fatalf("multipart not preserved: %+v", fact)
	}
}

func TestCAPC4030ClientMCP(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})

	// CAP-C4-030: MCP Streamable HTTP requires a real JSON-RPC minimal flow:
	// initialize → tools/list → tools/call. The deterministic Adapter must
	// parse the JSON-RPC method and respond appropriately for each step.

	// Step 1: initialize
	initBody, _, err := c.MCP(context.Background(), env.resourceIDs.mcp, "/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"measix-test-client","version":"1.0.0"}}}`)
	if err != nil {
		t.Fatalf("mcp initialize: %v", err)
	}
	if !bytes.Contains(initBody, []byte(`"protocolVersion"`)) {
		t.Fatalf("bad MCP initialize response: %s", initBody)
	}

	// Step 2: tools/list
	listBody, _, err := c.MCP(context.Background(), env.resourceIDs.mcp, "/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if err != nil {
		t.Fatalf("mcp tools/list: %v", err)
	}
	if !bytes.Contains(listBody, []byte(`"tools"`)) {
		t.Fatalf("bad MCP tools/list response: %s", listBody)
	}

	// Step 3: tools/call — prove the Adapter executes the tool and returns content
	callBody, _, err := c.MCP(context.Background(), env.resourceIDs.mcp, "/mcp",
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"tool-a","arguments":{"query":"hello"}}}`)
	if err != nil {
		t.Fatalf("mcp tools/call: %v", err)
	}
	if !bytes.Contains(callBody, []byte(`"content"`)) || !bytes.Contains(callBody, []byte(`"tool-a executed"`)) {
		t.Fatalf("bad MCP tools/call response: %s", callBody)
	}
}

// CAP-C4-022: a client cancel mid-stream must be propagated by the Relay to the
// upstream, so the upstream observes cancellation (no orphaned upstream work).
func TestCAPC4022ClientStreamCancelPropagates(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	ctx, cancel := context.WithCancel(context.Background())
	var observedErr error
	var errCh = make(chan error, 1)
	go func() {
		errCh <- c.ChatCompletionStream(ctx, env.resourceIDs.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) {})
	}()
	// Wait for at least one chunk to flow, then cancel the client.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if env.adapter.Cancelled() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case observedErr = <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("client stream did not return after cancel")
	}
	// The adapter must observe cancellation of the upstream request.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if env.adapter.Cancelled() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("adapter did not observe cancellation: err=%v", observedErr)
}

// CAP-C4-040: old generation must be rejected before any upstream body forward.
func TestCAPC4040OldGenerationNoForward(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: 0, InteractionID: env.interactionID})
	_, _, err := c.ChatCompletion(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{"model":"gpt-test"}`)
	if err == nil {
		t.Fatal("expected 428 for stale generation")
	}
	problem, ok := err.(client.ProblemError)
	if !ok {
		t.Fatalf("expected ProblemError, got %T", err)
	}
	if problem.Status != 428 || problem.Forwarded == nil || *problem.Forwarded {
		t.Fatalf("unexpected problem: status=%d code=%s forwarded=%v", problem.Status, problem.Code, problem.Forwarded)
	}
	if fact := env.adapter.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatalf("adapter received body despite stale generation: %+v", fact)
	}
}

// CAP-C4-041: invalid JWT must be rejected without forward.
func TestCAPC4041InvalidJWTReturns401NoForward(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: "not-a-token", ManagedGeneration: env.generation, InteractionID: env.interactionID})
	_, _, err := c.ChatCompletion(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{}`)
	if err == nil {
		t.Fatal("expected 401")
	}
	problem, ok := err.(client.ProblemError)
	if !ok || problem.Status != 401 {
		t.Fatalf("expected 401 problem, got %v", err)
	}
	if fact := env.adapter.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatalf("adapter received body despite invalid JWT")
	}
}

// CAP-C4-042: a revoked session must be rejected (401) before any upstream forward.
func TestCAPC4042RevokedSessionRejectedNoForward(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	env.revokeSession()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	_, _, err := c.ChatCompletion(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{}`)
	if err == nil {
		t.Fatal("expected 401 for revoked session")
	}
	problem, ok := err.(client.ProblemError)
	if !ok || problem.Status != 401 || problem.Code != "invalid_session" {
		t.Fatalf("expected 401 invalid_session, got %v", err)
	}
	if fact := env.adapter.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatalf("adapter received body despite revoked session: %+v", fact)
	}
}

// CAP-C4-042: a disabled user must be rejected (403) before any upstream forward.
func TestCAPC4042DisabledUserRejectedNoForward(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	env.disableUser()
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	_, _, err := c.ChatCompletion(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{}`)
	if err == nil {
		t.Fatal("expected 403 for disabled user")
	}
	problem, ok := err.(client.ProblemError)
	if !ok || problem.Status != 403 || problem.Code != "user_disabled" {
		t.Fatalf("expected 403 user_disabled, got %v", err)
	}
	if fact := env.adapter.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatalf("adapter received body despite disabled user: %+v", fact)
	}
}

// CAP-C4-043: an unknown resource id must be rejected (403 resource_not_allowed)
// before any upstream forward.
func TestCAPC4043UnknownResourceRejectedNoForward(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	unknown := platformid.New(platformid.Model) // a different, unbound model id
	c := client.New(client.Options{RuntimeBaseURL: env.relayURL, AccessToken: env.token, ManagedGeneration: env.generation, InteractionID: env.interactionID})
	_, _, err := c.ChatCompletion(context.Background(), unknown, "/v1/chat/completions", `{}`)
	if err == nil {
		t.Fatal("expected 403 for unknown resource")
	}
	problem, ok := err.(client.ProblemError)
	if !ok || problem.Status != 403 || problem.Code != "resource_not_allowed" {
		t.Fatalf("expected 403 resource_not_allowed, got %v", err)
	}
	if fact := env.adapter.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatalf("adapter received body despite unknown resource: %+v", fact)
	}
}

// CAP-C4-045: client-supplied internal X-Measix-* headers must not reach the
// upstream; the relay replaces them with its own request correlation.
func TestCAPC4045ClientInternalHeaderSpoofStripped(t *testing.T) {
	env := newEnv(t)
	defer env.close()
	c := client.New(client.Options{
		RuntimeBaseURL: env.relayURL, AccessToken: env.token,
		ManagedGeneration: env.generation, InteractionID: env.interactionID,
		SpoofHeaders: map[string]string{
			"X-Measix-Request-Id": "forged-request",
			"X-Measix-Internal":   "secret-bypass",
			"X-Forwarded-For":     "203.0.113.7",
		},
	})
	if _, _, err := c.ChatCompletion(context.Background(), env.resourceIDs.model, "/v1/chat/completions", `{}`); err != nil {
		t.Fatal(err)
	}
	fact := env.adapter.LastRequest("/v1/chat/completions")
	if fact == nil {
		t.Fatal("request not captured")
	}
	// The relay must strip the forged headers and set its own correlation id.
	if fact.XMeasixRequestId == "" || fact.XMeasixRequestId == "forged-request" {
		t.Fatalf("relay did not replace spoofed request-id: %q", fact.XMeasixRequestId)
	}
	if fact.Headers["X-Measix-Internal"] != "" {
		t.Fatalf("internal header leaked upstream: %q", fact.Headers["X-Measix-Internal"])
	}
	if fact.Headers["X-Forwarded-For"] != "" {
		t.Fatalf("client-controlled x-forwarded-for reached upstream: %q", fact.Headers["X-Forwarded-For"])
	}
}

type resourceIDs struct{ model, tts, asr, mcp string }

type env struct {
	adapter       *adapter.Adapter
	relayURL      string
	token         string
	generation    int
	interactionID string
	resourceIDs   resourceIDs
	srv           *httptest.Server
	store         *control.Store
	state         relaycontrolapi.RuntimeControlState
	userID        string
	sessionID     string
	deviceID      string
}

// revokeSession marks the signed session revoked and re-applies control state.
func (e *env) revokeSession() {
	e.state.PrincipalState.RevokedSessionIds = append(e.state.PrincipalState.RevokedSessionIds, e.sessionID)
	e.applyState()
}

// disableUser marks the signed user disabled and re-applies control state.
func (e *env) disableUser() {
	e.state.PrincipalState.DisabledUserIds = append(e.state.PrincipalState.DisabledUserIds, e.userID)
	e.applyState()
}

func (e *env) applyState() {
	e.state.ControlRevision++
	hash, err := relaystate.HashDescriptor(e.state)
	if err != nil {
		panic(err)
	}
	e.state.BundleHash = hash
	if _, err := e.store.Apply(e.state); err != nil {
		panic(err)
	}
}

func newEnv(t *testing.T) *env {
	t.Helper()
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := platformid.New(platformid.Deployment)
	userID := platformid.New(platformid.User)
	deviceID := platformid.New(platformid.Device)
	sessionID := platformid.New(platformid.Session)
	interactionID := platformid.New(platformid.Interaction)

	signer := &accessSigner{privateKey: privateKey, deploymentID: deploymentID, kid: "i4-key", now: now}
	token := signer.sign(t, userID, deviceID, sessionID)

	ad := adapter.New()
	ids := resourceIDs{
		model: platformid.New(platformid.Model),
		tts:   platformid.New(platformid.TTS),
		asr:   platformid.New(platformid.ASR),
		mcp:   platformid.New(platformid.MCP),
	}
	upstreamID := platformid.New(platformid.Upstream)
	modelRoute := platformid.New(platformid.Route)
	ttsRoute := platformid.New(platformid.Route)
	asrRoute := platformid.New(platformid.Route)
	mcpRoute := platformid.New(platformid.Route)

	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            deploymentID,
		AuthKeys:                []relaycontrolapi.PublicJwk{signer.publicJWK()},
		PrincipalState:          relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{
			{ResourceId: ids.model, RuntimeRouteId: modelRoute},
			{ResourceId: ids.tts, RuntimeRouteId: ttsRoute},
			{ResourceId: ids.asr, RuntimeRouteId: asrRoute},
			{ResourceId: ids.mcp, RuntimeRouteId: mcpRoute},
		},
		Routes: []relaycontrolapi.RuntimeRouteSpec{
			{RuntimeRouteId: modelRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: ttsRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/speech"}, TransportPolicy: relaycontrolapi.HTTPBINARYSTREAM, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: asrRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/transcriptions"}, TransportPolicy: relaycontrolapi.HTTPMULTIPART, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: mcpRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
		},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: ad.URL, Enabled: true,
			TransportCapabilities: []string{"HTTP_STREAMING_SSE", "HTTP_BINARY_STREAM", "HTTP_MULTIPART", "MCP_STREAMABLE_HTTP"},
			Auth:                  relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "server-side-secret"}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	state.BundleHash = hash
	store := control.NewStore(func() time.Time { return now })
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(relayruntime.NewHandler(store))
	return &env{
		adapter: ad, relayURL: srv.URL, token: token, generation: 1, interactionID: interactionID,
		resourceIDs: ids, srv: srv, store: store, state: state,
		userID: userID, sessionID: sessionID, deviceID: deviceID,
	}
}

func (e *env) close() {
	e.srv.Close()
	e.adapter.Close()
}

type accessSigner struct {
	privateKey   ed25519.PrivateKey
	deploymentID string
	kid          string
	now          time.Time
}

func (s *accessSigner) sign(t *testing.T, userID, deviceID, sessionID string) string {
	t.Helper()
	claims := struct {
		DeploymentID string `json:"deploymentId"`
		DeviceID     string `json:"deviceId"`
		SessionID    string `json:"sessionId"`
		jwt.RegisteredClaims
	}{
		DeploymentID: s.deploymentID, DeviceID: deviceID, SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.deploymentID, Subject: userID,
			Audience: jwt.ClaimStrings{"client", "runtime"},
			IssuedAt: jwt.NewNumericDate(s.now), ExpiresAt: jwt.NewNumericDate(s.now.Add(10 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.kid
	value, err := token.SignedString(s.privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func (s *accessSigner) publicJWK() relaycontrolapi.PublicJwk {
	publicKey := s.privateKey.Public().(ed25519.PublicKey)
	return relaycontrolapi.PublicJwk{
		Kty: relaycontrolapi.OKP, Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA, Use: relaycontrolapi.Sig,
		Kid: s.kid, X: base64.RawURLEncoding.EncodeToString(publicKey),
	}
}
