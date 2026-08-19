package relay_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
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
}

func newRuntimeFixture(t *testing.T, state relaycontrolapi.RuntimeControlState, privateKey ed25519.PrivateKey) *runtimeFixture {
	t.Helper()
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	signer, err := security.NewAccessSigner(privateKey, state.DeploymentId, "i4-key", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	signer.Now = func() time.Time { return now }
	state.AuthKeys = []relaycontrolapi.PublicJwk{{
		Kty: relaycontrolapi.OKP,
		Crv: relaycontrolapi.Ed25519,
		Alg: relaycontrolapi.EdDSA,
		Use: relaycontrolapi.Sig,
		Kid: "i4-key",
		X:   signer.PublicJWK()["x"],
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
