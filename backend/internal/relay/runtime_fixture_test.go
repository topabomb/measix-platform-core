package relay_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"measix/platform/internal/relay/control"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

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
		server:        httptest.NewServer(relayruntime.NewHandler(store)),
		signer:        signer,
		userID:        platformid.New(platformid.User),
		deviceID:      platformid.New(platformid.Device),
		sessionID:     platformid.New(platformid.Session),
		interactionID: platformid.New(platformid.Interaction),
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
