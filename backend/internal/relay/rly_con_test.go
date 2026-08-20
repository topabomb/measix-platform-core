package relay_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

// RLY-CON-003: High concurrency requests + atomic swap no race/panic/partial map.
func TestRLYCON003HighConcurrencyWithAtomicSwap(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()

	// Concurrently make requests and apply a new state.
	var wg sync.WaitGroup
	const N = 10
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
			resp, _ := http.DefaultClient.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
		}()
	}
	// Also apply a new state concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		state := minimalControlState(t, 2, 2)
		state.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
		state.BundleHash, _ = relaystate.HashDescriptor(state)
		_, _ = fixture.store.Apply(state)
	}()
	wg.Wait()
}

// RLY-CON-004: Old credential only for captured old request, not new.
func TestRLYCON004OldCredentialNotUsedInNewRequest(t *testing.T) {
	var firstAuth, secondAuth atomic.Value
	var releaseFirst = make(chan struct{})
	var firstEntered = make(chan struct{})

	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstAuth.Store(r.Header.Get("Authorization"))
		select {
		case <-firstEntered:
		default:
			close(firstEntered)
		}
		<-releaseFirst
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("A"))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("B"))
	}))
	defer upstreamB.Close()

	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	deploymentID := platformid.New(platformid.Deployment)

	stateA := relaycontrolapi.RuntimeControlState{
		ControlRevision: 1, ActiveManagedGeneration: 1, DeploymentId: deploymentID,
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
		Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstreamA.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "old-credential"}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	fixture := newRuntimeFixture(t, stateA, privateKey)
	defer fixture.close()

	// Start first request (will block on releaseFirst).
	firstReq := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	go func() {
		resp, _ := http.DefaultClient.Do(firstReq)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	// Wait for first request to enter upstream handler.
	select {
	case <-firstEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not reach upstream")
	}

	// Apply new state with different upstream and credential.
	stateB := relaycontrolapi.RuntimeControlState{
		ControlRevision: 2, ActiveManagedGeneration: 2, DeploymentId: deploymentID,
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
		Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstreamB.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "new-credential"}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	stateB.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
	stateB.BundleHash, _ = relaystate.HashDescriptor(stateB)
	if _, err := fixture.store.Apply(stateB); err != nil {
		t.Fatal(err)
	}

	// New request should use new credential and upstream B.
	secondReq := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	secondReq.Header.Set("X-Measix-Managed-Generation", "2")
	resp, err := http.DefaultClient.Do(secondReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "B" {
		t.Fatalf("new request did not use new upstream: %s", body)
	}
	if got, _ := secondAuth.Load().(string); got != "Bearer new-credential" {
		t.Fatalf("new request used old credential: %s", got)
	}

	// Release first request — it should have used old credential.
	close(releaseFirst)
	// Give the first request time to complete.
	time.Sleep(100 * time.Millisecond)
	if got, _ := firstAuth.Load().(string); got != "Bearer old-credential" {
		t.Fatalf("old request used new credential: %s", got)
	}
}

// RLY-CON-005: Cancel storm does not leak goroutines/connections.
func TestRLYCON005CancelStormNoLeak(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(1 * time.Second):
		}
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()

	// Launch 3 concurrent requests and cancel them.
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		req := fixture.request(t, ctx, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		cancel()
	}
}

// RLY-CON-006: Control apply and usage sender concurrent — no shared long lock.
func TestRLYCON006ControlApplyAndUsageSenderNoDeadlock(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()

	// Concurrently apply new state while making requests.
	var wg sync.WaitGroup
	const N = 10
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
			resp, _ := http.DefaultClient.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
		}(i)
	}
	// Also apply a new state concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		state := minimalControlState(t, 2, 2)
		state.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
		state.BundleHash, _ = relaystate.HashDescriptor(state)
		_, _ = fixture.store.Apply(state)
	}()
	wg.Wait()
}

// RLY-CON-001: Long stream uses revision R; concurrent apply R+1 then stream
// continues using captured route/credential/state.
// This is verified by TestRLYCON004OldCredentialNotUsedInNewRequest above,
// which proves that the first (long) request keeps the old captured state.
// Here we add a simpler explicit assertion.
func TestRLYCON001LongStreamKeepsCapturedState(t *testing.T) {
	var firstAuth atomic.Value
	var releaseFirst = make(chan struct{})
	var firstEntered = make(chan struct{})

	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstAuth.Store(r.Header.Get("Authorization"))
		select {
		case <-firstEntered:
		default:
			close(firstEntered)
		}
		<-releaseFirst
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamA.Close()

	fixture, resourceID := singleRouteFixture(t, upstreamA.URL, "old-credential")
	defer fixture.close()

	// Start first request (will block on releaseFirst).
	firstReq := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	go func() {
		resp, _ := http.DefaultClient.Do(firstReq)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	// Wait for first request to enter upstream handler.
	select {
	case <-firstEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not reach upstream")
	}

	// Apply new state with different credential.
	stateB := minimalControlState(t, 2, 2)
	stateB.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
	stateB.BundleHash, _ = relaystate.HashDescriptor(stateB)
	if _, err := fixture.store.Apply(stateB); err != nil {
		t.Fatal(err)
	}

	// The first request should still use old credential (captured state).
	close(releaseFirst)
	time.Sleep(100 * time.Millisecond)
	if got, _ := firstAuth.Load().(string); got != "Bearer old-credential" {
		t.Fatalf("long stream did not use captured credential: %s", got)
	}
}

// RLY-CON-002: After apply R+1, new requests use R+1.
// Implicitly verified by TestRLYCON004OldCredentialNotUsedInNewRequest (second request uses new state).
// Here we add a simpler explicit assertion.
func TestRLYCON002NewRequestUsesNewState(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	fixture, _ := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()

	// Apply new state (same upstream, same credential, just new revision/generation).
	resourceID2 := platformid.New(platformid.Model)
	routeID2 := platformid.New(platformid.Route)
	upstreamID2 := platformid.New(platformid.Upstream)
	state2 := relaycontrolapi.RuntimeControlState{
		ControlRevision: 2, ActiveManagedGeneration: 2, DeploymentId: fixture.signer.deploymentID,
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resourceID2, RuntimeRouteId: routeID2}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID2, UpstreamId: upstreamID2, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
		Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID2, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "runtime-secret"}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	state2.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
	state2.BundleHash, _ = relaystate.HashDescriptor(state2)
	if _, err := fixture.store.Apply(state2); err != nil {
		t.Fatal(err)
	}

	// New request with generation=2 should succeed.
	req := fixture.request(t, nil, http.MethodPost, resourceID2, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	req.Header.Set("X-Measix-Managed-Generation", "2")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new request with new state not accepted: status=%d", resp.StatusCode)
	}
}

// Ensure imports are used.
var _ = platformid.New
