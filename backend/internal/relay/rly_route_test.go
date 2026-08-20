package relay_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/relay/control"
	relaystate "measix/platform/internal/wire/relaystate"
	"crypto/ed25519"
	"crypto/rand"
	"time"
)

// RLY-ROUTE-001: Normal runtimePath + query preserved.
// RLY-ROUTE-002: Base URL with path not overwritten by leading slash.
// RLY-ROUTE-003: Absolute URI rejected.
// RLY-ROUTE-006: Method allowlist.
// RLY-ROUTE-007: allowedPathPrefixes boundary matching.
// RLY-ROUTE-009: Query preserved without changing target host.
// RLY-ROUTE-010: S0 does not perform generic path rewrite.

func newRouteFixture(t *testing.T, upstreamURL, allowedPath string, allowedMethods []string) (*runtimeFixture, string) {
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
		PrincipalState:          relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes:          []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID,
			AllowedMethods: allowedMethods, AllowedPathPrefixes: []string{allowedPath},
			TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE,
			TimeoutPolicy:   relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true,
			TransportCapabilities: []string{"HTTP_STREAMING_SSE"},
			Auth:                  relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), resourceID
}

// RLY-ROUTE-001: Normal runtimePath + query preserved.
func TestRLYROUTE001NormalPathAndQueryPreserved(t *testing.T) {
	var gotPath, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions?model=gpt-4&stream=true", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("normal path rejected: status=%d", resp.StatusCode)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path not preserved: %s", gotPath)
	}
	if gotQuery != "model=gpt-4&stream=true" {
		t.Fatalf("query not preserved: %s", gotQuery)
	}
}

// RLY-ROUTE-002: Base URL with path not overwritten by leading slash.
func TestRLYROUTE002BaseURLWithPathNotOverwritten(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	// Upstream URL has a base path prefix.
	fixture, resourceID := newRouteFixture(t, upstream.URL+"/api/v2", "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// The upstream should see the base path + runtime path.
	if !strings.HasPrefix(gotPath, "/api/v2/v1/chat/completions") {
		t.Fatalf("base URL path not preserved: %s", gotPath)
	}
}

// RLY-ROUTE-006: Method allowlist.
func TestRLYROUTE006MethodAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	// GET should be rejected.
	req := fixture.request(t, nil, http.MethodGet, resourceID, "/v1/chat/completions", nil, "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET not rejected by POST-only allowlist: status=%d", resp.StatusCode)
	}
}

// RLY-ROUTE-007: allowedPathPrefixes boundary matching — /v1/foo2 must not bypass /v1/foo.
func TestRLYROUTE007AllowedPathPrefixBoundary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/foo", []string{"POST"})
	defer fixture.close()
	// /v1/foo2 should NOT match /v1/foo prefix.
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/foo2", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("/v1/foo2 bypassed /v1/foo prefix: status=%d", resp.StatusCode)
	}
	// /v1/foo/bar should match.
	req2 := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/foo/bar", strings.NewReader(`{}`), "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("/v1/foo/bar rejected under /v1/foo prefix: status=%d", resp2.StatusCode)
	}
}

// RLY-ROUTE-009: Query preserved without changing target host.
func TestRLYROUTE009QueryPreservedNoHostChange(t *testing.T) {
	var gotHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions?param=value", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query request rejected: status=%d", resp.StatusCode)
	}
	// Host should be the upstream server, not the relay.
	expectedHost := strings.TrimPrefix(upstream.URL, "http://")
	if gotHost != expectedHost {
		t.Fatalf("host changed: got %s want %s", gotHost, expectedHost)
	}
}

// RLY-ROUTE-010: S0 does not perform generic path rewrite.
func TestRLYROUTE010NoGenericPathRewrite(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// Path must be exactly what was sent, no rewrite.
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path was rewritten: got %s", gotPath)
	}
}

// RLY-ROUTE-008: Adapter management endpoint not reachable via resource route.
func TestRLYROUTE008AdapterManagementNotReachable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	// Try to reach an adapter management endpoint.
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/admin/credentials", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("adapter management endpoint reachable: status=%d", resp.StatusCode)
	}
}

// RLY-ROUTE-003: Absolute URI in runtime path rejected.
func TestRLYROUTE003AbsoluteURIRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	// An absolute URI should not be accepted as a runtime path.
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	// Force the path to look like an absolute URI.
	req.URL.Path = "/runtime/v1/resources/" + resourceID + "/http://evil.example.com/"
	req.URL.RawPath = ""
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("absolute URI was not rejected: status=%d", resp.StatusCode)
	}
}

// RLY-ROUTE-004: Userinfo/scheme/host/port override rejected.
// The relay constructs the target URL from the upstream BaseUrl, not from the client request.
// So the client cannot override the target host. Verify the upstream receives the correct host.
func TestRLYROUTE004NoTargetOverride(t *testing.T) {
	var gotHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat/completions", []string{"POST"})
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	// Try to set a custom Host header — relay should strip it.
	req.Host = "evil.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	expectedHost := strings.TrimPrefix(upstream.URL, "http://")
	if gotHost != expectedHost {
		t.Fatalf("host override succeeded: got %s want %s", gotHost, expectedHost)
	}
}

// RLY-ROUTE-005: Path traversal variants rejected.
// The control layer's safePath validator already rejects "..", encoded traversals,
// and double-encoding variants. Here we test at the runtime routing layer.
func TestRLYROUTE005TraversalVariantsRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := newRouteFixture(t, upstream.URL, "/v1/chat", []string{"POST"})
	defer fixture.close()
	traversalPaths := []string{
		"/v1/chat/../../etc/passwd",
		"/v1/chat/%2e%2e/%2e%2e/etc/passwd",
		"/v1/chat/%252e%252e/%252e%252e/etc/passwd",
	}
	for _, path := range traversalPaths {
		req := fixture.request(t, nil, http.MethodPost, resourceID, path, strings.NewReader(`{}`), "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("traversal path not rejected: %s status=%d", path, resp.StatusCode)
		}
	}
}

// Ensure imports are used.
var _ = relayruntime.NewHandler
var _ = control.NewStore
var _ = relaystate.HashDescriptor
var _ = time.Now
