package relay_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// RLY-ADM-001: Generation == active → can continue.
// RLY-ADM-002: Generation old/new/missing/invalid → rejected.
// RLY-ADM-003: Old generation returns 428 stable Problem.
// RLY-ADM-004: Unknown/disabled resource rejected.
// RLY-ADM-005: ResourceID resolves to current route/upstream.
// RLY-ADM-006: S0 does not produce per-user ACL implicit second authorization rule.

// RLY-ADM-001: Generation == active → can continue.
func TestRLYADM001GenerationEqualActiveCanContinue(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generation==active rejected: status=%d", resp.StatusCode)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream not called exactly once: %d", upstreamCalls.Load())
	}
}

// RLY-ADM-002 + 003: Old/new/missing/invalid generation → rejected.
func TestRLYADM002GenerationMismatchRejected(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()

	t.Run("old generation returns 428", func(t *testing.T) {
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("X-Measix-Managed-Generation", "0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPreconditionRequired {
			t.Fatalf("old generation not 428: status=%d", resp.StatusCode)
		}
		if upstreamCalls.Load() != 0 {
			t.Fatal("old generation reached upstream")
		}
		// Verify Problem details
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "managed_snapshot_required") {
			t.Fatalf("428 problem missing code: %s", body)
		}
	})

	t.Run("new generation returns 409", func(t *testing.T) {
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("X-Measix-Managed-Generation", "99")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("new generation not 409: status=%d", resp.StatusCode)
		}
	})

	t.Run("missing generation returns 400", func(t *testing.T) {
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("X-Measix-Managed-Generation", "")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("missing generation not 400: status=%d", resp.StatusCode)
		}
	})

	t.Run("invalid generation returns 400", func(t *testing.T) {
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("X-Measix-Managed-Generation", "abc")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid generation not 400: status=%d", resp.StatusCode)
		}
	})
}

// RLY-ADM-004: Unknown/disabled resource rejected.
func TestRLYADM004UnknownResourceRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, _ := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	// Use a random resource ID not in the route map.
	unknownResource := platformid.New(platformid.Model)
	req := fixture.request(t, nil, http.MethodPost, unknownResource, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unknown resource not rejected: status=%d", resp.StatusCode)
	}
}

// RLY-ADM-005: ResourceID resolves to current route/upstream.
// This is implicitly verified by RLY-ADM-001 (the upstream receives the request).
// Here we add an explicit check that the correct upstream is reached.
func TestRLYADM005ResourceIDResolvesToCorrectRoute(t *testing.T) {
	var reachedUpstream atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reachedUpstream.Store(true)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"route":"matched"}`))
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resource not routed correctly: status=%d", resp.StatusCode)
	}
	if !reachedUpstream.Load().(bool) {
		t.Fatal("upstream not reached")
	}
}

// RLY-ADM-006: S0 does not produce per-user ACL implicit second authorization rule.
// Verify that any user with a valid token and matching generation can access the resource
// (no per-user ACL check exists beyond disabled/revoked).
func TestRLYADM006NoPerUserACLInS0(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	// Two different users should both be able to access the same resource.
	for i := 0; i < 2; i++ {
		userID := platformid.New(platformid.User)
		deviceID := platformid.New(platformid.Device)
		sessionID := platformid.New(platformid.Session)
		token := fixture.signer.sign(t, userID, deviceID, sessionID)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("user %d (iteration %d) rejected without ACL: status=%d", i, i, resp.StatusCode)
		}
	}
}

// Ensure relaycontrolapi is used.
var _ = relaycontrolapi.HTTPSTREAMINGSSE
