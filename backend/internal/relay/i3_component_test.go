package relay_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	relayruntime "github.com/topabomb/measix-platform-core/backend/internal/relay/runtime"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func TestI3ControlApplyAndRuntimeAdmission(t *testing.T) {
	var upstreamCalls atomic.Int32
	var upstreamAuth atomic.Value
	var upstreamRequestID atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		upstreamAuth.Store(r.Header.Get("Authorization"))
		upstreamRequestID.Store(r.Header.Get("X-Measix-Request-Id"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: first\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = io.WriteString(w, "data: done\n\n")
	}))
	defer upstream.Close()

	deploymentID := platformid.New(platformid.Deployment)
	userID := platformid.New(platformid.User)
	deviceID := platformid.New(platformid.Device)
	sessionID := platformid.New(platformid.Session)
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	interactionID := platformid.New(platformid.Interaction)

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := security.NewAccessSigner(privateKey, deploymentID, "i3-key", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	signer.Now = func() time.Time { return now }

	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 7,
		DeploymentId:            deploymentID,
		AuthKeys: []relaycontrolapi.PublicJwk{{
			Kty: relaycontrolapi.OKP, Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA,
			Use: relaycontrolapi.Sig, Kid: "i3-key", X: signer.PublicJWK()["x"],
		}},
		PrincipalState: relaycontrolapi.PrincipalState{
			DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
		},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID,
			AllowedMethods: []string{http.MethodPost}, AllowedPathPrefixes: []string{"/v1/chat/completions"},
			TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE,
			TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true,
			TransportCapabilities: []string{"HTTP_STREAMING_SSE"},
			Auth: relaycontrolapi.RuntimeUpstreamAuth{
				Type: relaycontrolapi.BEARER,
				AdditionalProperties: map[string]interface{}{"token": "upstream-secret"},
			},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	state.BundleHash, err = control.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}

	store := control.NewStore(func() time.Time { return now })
	internal := httptest.NewServer(control.NewHandler(store, "relay-service-token"))
	defer internal.Close()
	applyControl(t, internal.URL, "relay-service-token", state, http.StatusOK)

	statusRequest, _ := http.NewRequest(http.MethodGet, internal.URL+"/internal/v1/control/status", nil)
	statusRequest.Header.Set("Authorization", "Bearer relay-service-token")
	statusResponse, err := http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResponse.Body.Close()
	var status relaycontrolapi.ControlStatus
	if err := json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if statusResponse.StatusCode != http.StatusOK || !status.Ready || status.AppliedControlRevision != 1 || status.ActiveManagedGeneration != 7 || status.BundleHash != string(state.BundleHash) {
		t.Fatalf("unexpected control status: status=%d body=%+v", statusResponse.StatusCode, status)
	}

	public := httptest.NewServer(relayruntime.NewHandler(store))
	defer public.Close()
	accessToken, _, err := signer.Sign(userID, deviceID, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	requestBody := []byte(`{"model":"model-x","stream":true}`)
	request, _ := http.NewRequest(http.MethodPost, public.URL+"/runtime/v1/resources/"+resourceID+"/v1/chat/completions", bytes.NewReader(requestBody))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("X-Measix-Managed-Generation", "7")
	request.Header.Set("X-Measix-Interaction-Id", interactionID)
	request.Header.Set("X-Measix-Request-Id", "client-must-not-control-this")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "data: done") {
		t.Fatalf("runtime status=%d body=%q", response.StatusCode, body)
	}
	if got, _ := upstreamAuth.Load().(string); got != "Bearer upstream-secret" {
		t.Fatalf("upstream Authorization=%q", got)
	}
	if got, _ := upstreamRequestID.Load().(string); !strings.HasPrefix(got, "req_") {
		t.Fatalf("upstream request id=%q", got)
	}
	if got := response.Header.Get("X-Measix-Request-Id"); !strings.HasPrefix(got, "req_") {
		t.Fatalf("client request id=%q", got)
	}

	callsBefore := upstreamCalls.Load()
	stale, _ := http.NewRequest(http.MethodPost, public.URL+"/runtime/v1/resources/"+resourceID+"/v1/chat/completions", strings.NewReader("must-not-be-forwarded"))
	stale.Header.Set("Authorization", "Bearer "+accessToken)
	stale.Header.Set("X-Measix-Managed-Generation", "6")
	stale.Header.Set("X-Measix-Interaction-Id", interactionID)
	staleResponse, err := http.DefaultClient.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	defer staleResponse.Body.Close()
	if staleResponse.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("stale generation status=%d", staleResponse.StatusCode)
	}
	if upstreamCalls.Load() != callsBefore {
		t.Fatal("stale generation request reached upstream")
	}
}

func TestI3ControlRevisionConflictKeepsCurrentState(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state := minimalControlState(t, 9, 3)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	conflict := state
	conflict.ActiveManagedGeneration = 4
	conflict.BundleHash, _ = control.HashDescriptor(conflict)
	if _, err := store.Apply(conflict); !control.IsRevisionHashConflict(err) {
		t.Fatalf("conflict err=%v", err)
	}
	current := store.Status()
	if current.AppliedControlRevision != 9 || current.ActiveManagedGeneration != 3 || current.BundleHash != string(state.BundleHash) {
		t.Fatalf("conflict replaced current state: %+v", current)
	}
}

func applyControl(t *testing.T, baseURL, serviceToken string, state relaycontrolapi.RuntimeControlState, want int) {
	t.Helper()
	body, _ := json.Marshal(state)
	request, _ := http.NewRequest(http.MethodPut, baseURL+"/internal/v1/control/state", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+serviceToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("apply status=%d want=%d body=%s", response.StatusCode, want, payload)
	}
}

func minimalControlState(t *testing.T, revision, generation int) relaycontrolapi.RuntimeControlState {
	t.Helper()
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: revision, ActiveManagedGeneration: generation,
		DeploymentId: platformid.New(platformid.Deployment),
		AuthKeys: []relaycontrolapi.PublicJwk{},
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{}, Routes: []relaycontrolapi.RuntimeRouteSpec{}, Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	var err error
	state.BundleHash, err = control.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
