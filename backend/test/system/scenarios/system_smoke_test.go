//go:build smoke

package scenarios

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// CAP-C4 / CAP-C6 system-smoke: a real runtime-relay process, the deterministic
// adapter and the client-facing Test Client prove the four required transports end
// to end. Relay control state is applied over the real Hub<->Relay internal boundary
// (PUT /internal/v1/control/state).
func TestSystemSmokeRealRelayFourTransports(t *testing.T) {
	env, err := harness.New()
	if err != nil {
		t.Fatal(err)
	}
	defer env.Cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	relayBin := filepath.Join(env.Root, "runtime-relay")
	if runtime.GOOS == "windows" {
		relayBin += ".exe"
	}
	if err := buildRelay(ctx, t, relayBin); err != nil {
		t.Fatalf("build relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	spoolPath := filepath.Join(env.Root, "relay-spool.db")
	tokenFile := filepath.Join(env.Root, "service.token")
	if err := os.WriteFile(tokenFile, []byte("relay-service-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(env.Root, "relay.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	proc, err := env.Start(ctx, logFile, relayBin,
		"--public-listen", fmt.Sprintf("127.0.0.1:%d", env.Ports.RelayPub),
		"--internal-listen", fmt.Sprintf("127.0.0.1:%d", env.Ports.RelayInt),
		"--spool", spoolPath,
		"--hub-usage-url", "http://127.0.0.1:1/internal/v1/usage/request-events:batch",
		"--hub-service-token-file", tokenFile,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stopProcess(proc)

	pubURL := fmt.Sprintf("http://127.0.0.1:%d", env.Ports.RelayPub)
	intURL := fmt.Sprintf("http://127.0.0.1:%d", env.Ports.RelayInt)
	if err := harness.WaitLive(ctx, intURL, 30*time.Second); err != nil {
		_ = proc.Cmd.Process.Kill()
		logContent, _ := os.ReadFile(filepath.Join(env.Root, "relay.log"))
		t.Fatalf("relay internal not live: %v\nrelay log:\n%s", err, logContent)
	}
	state, signer := buildState(t, ad.URL)
	if err := applyControl(ctx, env.Ports.RelayInt, state); err != nil {
		_ = proc.Cmd.Process.Kill()
		logContent, _ := os.ReadFile(filepath.Join(env.Root, "relay.log"))
		t.Fatalf("apply control: %v\nrelay log:\n%s", err, logContent)
	}
	// Relay becomes ready only after a control state is applied (fail-closed).
	if err := harness.WaitReady(ctx, pubURL, 30*time.Second); err != nil {
		_ = proc.Cmd.Process.Kill()
		logContent, _ := os.ReadFile(filepath.Join(env.Root, "relay.log"))
		t.Fatalf("relay not ready: %v\nrelay log:\n%s", err, logContent)
	}

	token := signer.sign(t, platformid.New(platformid.User), platformid.New(platformid.Device), platformid.New(platformid.Session))
	interaction := platformid.New(platformid.Interaction)
	c := client.New(client.Options{RuntimeBaseURL: pubURL, AccessToken: token, ManagedGeneration: 1, InteractionID: interaction})

	chatID, ttsID, asrID, mcpID := resourceIDs(state)

	// Model request/response (CAP-C4-001)
	body, _, err := c.ChatCompletion(ctx, chatID, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !bytes.Contains(body, []byte(`"role":"assistant"`)) {
		t.Fatalf("bad chat: %s", body)
	}

	// Model streaming (CAP-C4-002)
	chunks := 0
	if err := c.ChatCompletionStream(ctx, chatID, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func(data []byte) { chunks++ }); err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if chunks < 2 {
		t.Fatalf("stream chunks=%d", chunks)
	}

	// TTS binary (CAP-C4-010/011)
	ttsBody, ct, err := c.Speech(ctx, ttsID, "/v1/audio/speech", `{"model":"tts-1","input":"hi","voice":"alloy"}`)
	if err != nil {
		t.Fatalf("speech: %v", err)
	}
	if !bytes.Equal(ttsBody, ad.Bytes()) || !bytes.HasPrefix([]byte(ct), []byte("audio/mpeg")) {
		t.Fatalf("speech corrupt: len=%d ct=%q", len(ttsBody), ct)
	}

	// ASR multipart (CAP-C4-020)
	asrBody, _, err := c.Transcription(ctx, asrID, "/v1/audio/transcriptions", "whisper-test", "sample.wav", []byte("RIFF"))
	if err != nil {
		t.Fatalf("transcription: %v", err)
	}
	if !bytes.Contains(asrBody, []byte(`"text":"transcribed"`)) {
		t.Fatalf("bad transcription: %s", asrBody)
	}

	// MCP (CAP-C4-030)
	mcpBody, _, err := c.MCP(ctx, mcpID, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if err != nil {
		t.Fatalf("mcp: %v", err)
	}
	if !bytes.Contains(mcpBody, []byte(`"tools"`)) {
		t.Fatalf("bad mcp: %s", mcpBody)
	}

	// CAP-C4-040 stale generation no-forward
	old := client.New(client.Options{RuntimeBaseURL: pubURL, AccessToken: token, ManagedGeneration: 0, InteractionID: interaction})
	if _, _, err := old.ChatCompletion(ctx, chatID, "/v1/chat/completions", `{}`); err == nil {
		t.Fatal("expected 428 for stale generation")
	} else if p, ok := err.(client.ProblemError); !ok || p.Status != 428 {
		t.Fatalf("expected 428, got %v", err)
	}

	t.Log("system-smoke real relay four transports: PASS")
}

func resourceIDs(state relaycontrolapi.RuntimeControlState) (chat, tts, asr, mcp string) {
	for _, r := range state.ResourceRoutes {
		kind, err := platformid.KindOf(r.ResourceId)
		if err != nil {
			continue
		}
		switch kind {
		case platformid.Model:
			chat = r.ResourceId
		case platformid.TTS:
			tts = r.ResourceId
		case platformid.ASR:
			asr = r.ResourceId
		case platformid.MCP:
			mcp = r.ResourceId
		}
	}
	return
}

func buildState(t *testing.T, upstreamURL string) (relaycontrolapi.RuntimeControlState, *accessSigner) {
	t.Helper()
	now := time.Now().UTC()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := platformid.New(platformid.Deployment)
	signer := &accessSigner{privateKey: privateKey, deploymentID: deploymentID, kid: "i4-key", now: now}
	modelID := platformid.New(platformid.Model)
	ttsID := platformid.New(platformid.TTS)
	asrID := platformid.New(platformid.ASR)
	mcpID := platformid.New(platformid.MCP)
	upstreamID := platformid.New(platformid.Upstream)
	mkRoute := func() string { return platformid.New(platformid.Route) }
	modelRoute, ttsRoute, asrRoute, mcpRoute := mkRoute(), mkRoute(), mkRoute(), mkRoute()
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            deploymentID,
		AuthKeys:                []relaycontrolapi.PublicJwk{signer.publicJWK()},
		PrincipalState:          relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{
			{ResourceId: modelID, RuntimeRouteId: modelRoute},
			{ResourceId: ttsID, RuntimeRouteId: ttsRoute},
			{ResourceId: asrID, RuntimeRouteId: asrRoute},
			{ResourceId: mcpID, RuntimeRouteId: mcpRoute},
		},
		Routes: []relaycontrolapi.RuntimeRouteSpec{
			{RuntimeRouteId: modelRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: ttsRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/speech"}, TransportPolicy: relaycontrolapi.HTTPBINARYSTREAM, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: asrRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/transcriptions"}, TransportPolicy: relaycontrolapi.HTTPMULTIPART, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: mcpRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
		},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true,
			TransportCapabilities: []string{"HTTP_STREAMING_SSE", "HTTP_BINARY_STREAM", "HTTP_MULTIPART", "MCP_STREAMABLE_HTTP"},
			Auth:                  relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "server-secret"}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	state.BundleHash = hash
	return state, signer
}

func applyControl(ctx context.Context, port int, state relaycontrolapi.RuntimeControlState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("http://127.0.0.1:%d/internal/v1/control/state", port), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer relay-service-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apply control status=%d body=%s", resp.StatusCode, body)
	}
	return nil
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

func buildRelay(ctx context.Context, t *testing.T, target string) error {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	cmd := exec.CommandContext(ctx, "go", "build", "-o", target, "./cmd/runtime-relay")
	cmd.Dir = backendRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build relay: %v: %s", err, out)
	}
	return nil
}

func stopProcess(proc *harness.Process) {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return
	}
	_ = proc.Cmd.Process.Kill()
	select {
	case <-proc.Done:
	case <-time.After(3 * time.Second):
	}
}
