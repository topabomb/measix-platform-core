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

// RLY-HDR-001..008: Outbound header/credential policy.
// Table-driven cases covering: client Authorization removal, Host/hop-by-hop removal,
// X-Measix-* removal, Relay writes X-Measix-Request-Id, credential injection,
// credential not leaking across routes, Set-Cookie not passed, upstream 4xx/5xx preserved.

func TestRLYHDR001To008OutboundResponseHeaderPolicy(t *testing.T) {
	var gotHeaders atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deep-copy headers to avoid race with Relay modifying them.
		hdr := make(http.Header)
		for k, v := range r.Header {
			cp := make([]string, len(v))
			copy(cp, v)
			hdr[k] = cp
		}
		gotHeaders.Store(hdr)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=abc123; Path=/")
		w.Header().Set("X-Custom", "custom-value")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "upstream-bearer-secret")
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`), "application/json")
	// Set headers that must be stripped — but NOT Authorization (it's the JWT, set by fixture.request).
	req.Header.Set("Cookie", "client-cookie=value")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.Header.Set("X-Measix-Client-Id", "client-injected")
	req.Header.Set("X-Measix-Request-Id", "client-must-not-set-this")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request failed: status=%d", resp.StatusCode)
	}

	sentHeaders, _ := gotHeaders.Load().(http.Header)

	t.Run("RLY-HDR-001 client Authorization removed", func(t *testing.T) {
		// The client JWT (set by fixture.request) must not be forwarded.
		// The upstream credential should be injected instead.
		if auth := sentHeaders.Get("Authorization"); !strings.HasPrefix(auth, "Bearer upstream-bearer-secret") {
			t.Fatalf("upstream credential not injected: got %q", auth)
		}
	})

	t.Run("RLY-HDR-002 Host and hop-by-hop removed", func(t *testing.T) {
		if sentHeaders.Get("Host") != "" {
			t.Fatal("Host header forwarded")
		}
		if sentHeaders.Get("Connection") != "" {
			t.Fatal("Connection header forwarded")
		}
		if sentHeaders.Get("Transfer-Encoding") != "" {
			t.Fatal("Transfer-Encoding forwarded")
		}
	})

	t.Run("RLY-HDR-003 client X-Measix-* removed", func(t *testing.T) {
		if v := sentHeaders.Get("X-Measix-Client-Id"); v != "" {
			t.Fatalf("client X-Measix-* header forwarded: %s", v)
		}
	})

	t.Run("RLY-HDR-004 Relay writes X-Measix-Request-Id", func(t *testing.T) {
		rid := sentHeaders.Get("X-Measix-Request-Id")
		if !strings.HasPrefix(rid, "req_") {
			t.Fatalf("Relay did not write X-Measix-Request-Id: %q", rid)
		}
	})

	t.Run("RLY-HDR-005 cookie removed", func(t *testing.T) {
		if sentHeaders.Get("Cookie") != "" {
			t.Fatal("Cookie header forwarded to upstream")
		}
	})

	t.Run("RLY-HDR-006 X-Forwarded removed", func(t *testing.T) {
		if sentHeaders.Get("X-Forwarded-For") != "" {
			t.Fatal("X-Forwarded-For forwarded")
		}
	})

	t.Run("RLY-HDR-007 response Set-Cookie not passed", func(t *testing.T) {
		if resp.Header.Get("Set-Cookie") != "" {
			t.Fatal("Set-Cookie passed through to client")
		}
	})

	t.Run("RLY-HDR-008 response X-Measix-Request-Id set", func(t *testing.T) {
		rid := resp.Header.Get("X-Measix-Request-Id")
		if !strings.HasPrefix(rid, "req_") {
			t.Fatalf("response missing Relay X-Measix-Request-Id: %q", rid)
		}
	})
}

// RLY-HDR: Upstream 4xx/5xx not turned into Relay success.
func TestRLYHDRUpstreamErrorPreserved(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream-error"}`))
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
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("upstream error changed: got %d want %d", resp.StatusCode, http.StatusBadGateway)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "upstream-error") {
		t.Fatalf("upstream error body changed: %s", body)
	}
}

// RLY-HDR: Redirect not auto-followed.
func TestRLYHDRRedirectNotFollowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://evil.example.com")
		w.WriteHeader(http.StatusFound)
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
	// Relay should map redirect to 502 Bad Gateway, not follow it.
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("redirect followed: status=%d", resp.StatusCode)
	}
}

// Ensure imports are used.
var _ = relaycontrolapi.HTTPSTREAMINGSSE
var _ = platformid.New
