package relay_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	relaybudget "measix/platform/internal/relay/budget"
	"measix/platform/internal/relay/control"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

type captureUsageRecorder struct {
	mu     sync.Mutex
	events []usageingestapi.RequestUsageFact
}

func (r *captureUsageRecorder) PersistAdmission(usageingestapi.BudgetAdmissionRequest, string) error {
	return nil
}
func (r *captureUsageRecorder) MarkStarted(string, time.Time) error { return nil }
func (r *captureUsageRecorder) AbortAdmission(string) error         { return nil }
func (r *captureUsageRecorder) RecordDenied(_ usageingestapi.BudgetAdmissionRequest, _ string, event usageingestapi.UsageSettlement) error {
	return r.Record(event)
}
func (r *captureUsageRecorder) Record(event usageingestapi.UsageSettlement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event.Request)
	return nil
}
func (r *captureUsageRecorder) snapshot() []usageingestapi.RequestUsageFact {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]usageingestapi.RequestUsageFact(nil), r.events...)
}
func (r *captureUsageRecorder) waitFor(t *testing.T, count int) []usageingestapi.RequestUsageFact {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if events := r.snapshot(); len(events) >= count {
			return events
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d completed usage facts, got %d", count, len(r.snapshot()))
	return nil
}

type allowBudgetClient struct{}

func (*allowBudgetClient) Admit(_ context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	return usageingestapi.BudgetAdmissionDecision{RequestId: input.RequestId, Allowed: true, Code: usageingestapi.ALLOWED, Mode: usageingestapi.UNLIMITED, Source: usageingestapi.DEFAULT, AsOf: input.AdmittedAt}, nil, nil
}
func (*allowBudgetClient) Start(context.Context, string, usageingestapi.BudgetLifecycleEvent) error {
	return nil
}
func (*allowBudgetClient) Release(context.Context, string, usageingestapi.BudgetReleaseRequest) error {
	return nil
}

var _ relaybudget.Client = (*allowBudgetClient)(nil)

type testAccessClaims struct {
	DeploymentID string `json:"deploymentId"`
	DeviceID     string `json:"deviceId"`
	SessionID    string `json:"sessionId"`
	jwt.RegisteredClaims
}

type testAccessSigner struct {
	privateKey   ed25519.PrivateKey
	deploymentID string
	kid          string
	now          time.Time
}

func (s *testAccessSigner) sign(t *testing.T, userID, deviceID, sessionID string) string {
	t.Helper()
	claims := testAccessClaims{
		DeploymentID: s.deploymentID,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.deploymentID,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"client", "runtime"},
			IssuedAt:  jwt.NewNumericDate(s.now),
			ExpiresAt: jwt.NewNumericDate(s.now.Add(10 * time.Minute)),
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

func (s *testAccessSigner) publicJWK() relaycontrolapi.PublicJwk {
	publicKey := s.privateKey.Public().(ed25519.PublicKey)
	return relaycontrolapi.PublicJwk{
		Kty: relaycontrolapi.OKP,
		Crv: relaycontrolapi.Ed25519,
		Alg: relaycontrolapi.EdDSA,
		Use: relaycontrolapi.Sig,
		Kid: s.kid,
		X:   base64.RawURLEncoding.EncodeToString(publicKey),
	}
}

type runtimeFixture struct {
	store         *control.Store
	server        *httptest.Server
	signer        *testAccessSigner
	userID        string
	deviceID      string
	sessionID     string
	interactionID string
}

func newRuntimeFixture(t *testing.T, state relaycontrolapi.RuntimeControlState, privateKey ed25519.PrivateKey) *runtimeFixture {
	t.Helper()
	completeTestMeterProfiles(&state)
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	signer := &testAccessSigner{privateKey: privateKey, deploymentID: state.DeploymentId, kid: "i4-key", now: now}
	state.AuthKeys = []relaycontrolapi.PublicJwk{signer.publicJWK()}
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
		store:         store,
		server:        httptest.NewServer(relayruntime.NewHandler(store, &captureUsageRecorder{}, &allowBudgetClient{})),
		signer:        signer,
		userID:        platformid.New(platformid.User),
		deviceID:      platformid.New(platformid.Device),
		sessionID:     platformid.New(platformid.Session),
		interactionID: platformid.New(platformid.Interaction),
	}
}

// Every runtime resource in the production control contract carries its meter
// profile. Older transport-focused fixtures declare only resource/route IDs;
// complete those fixtures here so the tests exercise the current contract.
func completeTestMeterProfiles(state *relaycontrolapi.RuntimeControlState) {
	if state == nil {
		return
	}
	for index := range state.ResourceRoutes {
		resource := &state.ResourceRoutes[index]
		if resource.ResourceKind != "" && resource.ClientProtocol != "" {
			if resource.ResourceKind == "MODEL" && resource.LlmProfile == nil {
				resource.LlmProfile = testLlmProfile(resource.ClientProtocol)
			}
			continue
		}
		kind, err := platformid.KindOf(resource.ResourceId)
		if err != nil {
			continue
		}
		path, transport := "", relaycontrolapi.RuntimeRouteSpecTransportPolicy("")
		for _, route := range state.Routes {
			if route.RuntimeRouteId == resource.RuntimeRouteId {
				if len(route.AllowedPathPrefixes) > 0 {
					path = route.AllowedPathPrefixes[0]
				}
				transport = route.TransportPolicy
				break
			}
		}
		switch kind {
		case platformid.Model:
			resource.ResourceKind = "MODEL"
			switch {
			case strings.Contains(path, "responses"):
				resource.ClientProtocol = "OPENAI_RESPONSES"
			case strings.Contains(path, "generateContent"):
				resource.ClientProtocol = "GOOGLE_GENERATE_CONTENT"
			case strings.Contains(path, "messages"):
				resource.ClientProtocol = "ANTHROPIC_MESSAGES"
			default:
				resource.ClientProtocol = "OPENAI_CHAT_COMPLETIONS"
			}
		case platformid.TTS:
			resource.ResourceKind = "TTS"
			switch {
			case strings.Contains(path, "generateContent"):
				resource.ClientProtocol = "GEMINI_GENERATE_CONTENT_TTS"
			case strings.Contains(path, "chat/completions"):
				resource.ClientProtocol = "MIMO_CHAT_COMPLETIONS_TTS"
			default:
				resource.ClientProtocol = "OPENAI_AUDIO_SPEECH"
			}
		case platformid.ASR:
			resource.ResourceKind = "ASR"
			encoding, sampleRate := relaycontrolapi.RuntimeAudioProfileEncoding("WAV_PCM16_LE"), 16000
			switch {
			case transport == relaycontrolapi.WEBSOCKET && strings.Contains(path, "api-ws"):
				resource.ClientProtocol, encoding = "DASHSCOPE_REALTIME_ASR", "PCM16_LE"
			case transport == relaycontrolapi.WEBSOCKET:
				resource.ClientProtocol, encoding, sampleRate = "OPENAI_REALTIME_TRANSCRIPTION", "PCM16_LE", 24000
			case strings.Contains(path, "aigc"):
				resource.ClientProtocol = "DASHSCOPE_HTTP_ASR"
			default:
				resource.ClientProtocol = "OPENAI_AUDIO_TRANSCRIPTIONS"
			}
			resource.AudioProfile = &relaycontrolapi.RuntimeAudioProfile{Encoding: encoding, Channels: 1, SampleRates: []relaycontrolapi.RuntimeAudioProfileSampleRates{relaycontrolapi.RuntimeAudioProfileSampleRates(sampleRate)}}
		case platformid.MCP:
			resource.ResourceKind = "MCP"
			resource.ClientProtocol = "MCP_STREAMABLE_HTTP"
		}
		if resource.ResourceKind == "MODEL" && resource.LlmProfile == nil {
			resource.LlmProfile = testLlmProfile(resource.ClientProtocol)
		}
	}
}

func testLlmProfile(protocol relaycontrolapi.ResourceRouteClientProtocol) *relaycontrolapi.RuntimeLlmProfile {
	return &relaycontrolapi.RuntimeLlmProfile{
		GeminiThoughtsMayBeAbsent:       protocol == relaycontrolapi.GOOGLEGENERATECONTENT,
		AnthropicCacheFieldsMayBeAbsent: protocol == relaycontrolapi.ANTHROPICMESSAGES,
	}
}

func (f *runtimeFixture) close() { f.server.Close() }

func (f *runtimeFixture) request(t *testing.T, ctx context.Context, method, resourceID, path string, body io.Reader, contentType string) *http.Request {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, method, f.server.URL+"/runtime/v1/resources/"+resourceID+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+f.signer.sign(t, f.userID, f.deviceID, f.sessionID))
	request.Header.Set("X-Measix-Managed-Generation", "1")
	request.Header.Set("X-Measix-Interaction-Id", f.interactionID)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

func singleRouteFixture(t *testing.T, upstreamURL, token string) (*runtimeFixture, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            platformid.New(platformid.Deployment),
		PrincipalState: relaycontrolapi.PrincipalState{
			DisabledUserIds:   []string{},
			DeletedUserIds:    []string{},
			RevokedDeviceIds:  []string{},
			RevokedSessionIds: []string{},
		},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId:      routeID,
			UpstreamId:          upstreamID,
			AllowedMethods:      []string{"POST"},
			AllowedPathPrefixes: []string{"/v1/chat/completions"},
			TransportPolicy:     relaycontrolapi.HTTPSTREAMINGSSE,
			TimeoutPolicy:       relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId:            upstreamID,
			BaseUrl:               upstreamURL,
			Enabled:               true,
			TransportCapabilities: []string{"HTTP_STREAMING_SSE"},
			Auth: relaycontrolapi.RuntimeUpstreamAuth{
				Type:                 relaycontrolapi.BEARER,
				AdditionalProperties: map[string]interface{}{"token": token},
			},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), resourceID
}
