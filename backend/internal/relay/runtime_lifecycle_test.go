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

	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaystate"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func TestRLYI4CancellationPropagatesToUpstream(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	ctx, cancel := context.WithCancel(context.Background())
	request := fixture.request(t, ctx, http.MethodPost, resourceID, "/v1/chat/completions", http.NoBody, "application/json")
	done := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream did not observe cancellation")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("client request unexpectedly succeeded after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled client request did not return")
	}
}

func TestRLYI4InFlightRequestKeepsCapturedControlState(t *testing.T) {
	enteredA := make(chan struct{})
	releaseA := make(chan struct{})
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(enteredA)
		<-releaseA
		_, _ = w.Write([]byte("A"))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("B"))
	}))
	defer upstreamB.Close()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	deploymentID := platformid.New(platformid.Deployment)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            deploymentID,
		PrincipalState:          relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes:          []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"},
			AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE,
			TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstreamA.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	fixture := newRuntimeFixture(t, state, privateKey)
	defer fixture.close()

	first := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", http.NoBody, "application/json")
	firstBody := make(chan string, 1)
	go func() {
		response, err := http.DefaultClient.Do(first)
		if err != nil {
			firstBody <- "ERR"
			return
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		firstBody <- string(body)
	}()
	select {
	case <-enteredA:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not reach upstream A")
	}

	state.ControlRevision = 2
	state.Upstreams[0].BaseUrl = upstreamB.URL
	state.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	state.BundleHash = hash
	if _, err := fixture.store.Apply(state); err != nil {
		t.Fatal(err)
	}
	close(releaseA)
	if got := <-firstBody; got != "A" {
		t.Fatalf("in-flight request switched control state: got %q", got)
	}

	second := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", http.NoBody, "application/json")
	response, err := http.DefaultClient.Do(second)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "B" {
		t.Fatalf("new request did not use new control state: %q", body)
	}
}
