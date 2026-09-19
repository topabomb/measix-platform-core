package portalstatic_test

import (
	"measix/platform/internal/hub/portalstatic"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPortalStaticIsolation(t *testing.T) {
	h := portalstatic.New(fstest.MapFS{"index.html": {Data: []byte("Portal")}, "assets/app.js": {Data: []byte("export {}")}})
	for _, tc := range []struct {
		path   string
		status int
	}{{"/portal/", 200}, {"/portal", 308}, {"/portal/assets/app.js", 200}, {"/portal/assets/missing.js", 404}, {"/portal/assets/", 404}, {"/portal/../admin/", 404}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s status=%d", tc.path, w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("missing Portal security headers")
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self' data: blob:") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "media-src blob:;") {
			t.Fatal("phone previews must support document Blob media without external media sources")
		}
	}
}

func TestPortalRemoteProjectsStaticSiteWithoutForwardingAuthority(t *testing.T) {
	var seenPath, seenQuery, seenCookie, seenAuthorization, seenOrigin string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath, seenQuery = r.URL.Path, r.URL.RawQuery
		seenCookie, seenAuthorization, seenOrigin = r.Header.Get("Cookie"), r.Header.Get("Authorization"), r.Header.Get("Origin")
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Set-Cookie", "upstream=forbidden")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("export {}"))
	}))
	defer upstream.Close()
	h, err := portalstatic.NewRemote(upstream.URL+"/custom", upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/portal/assets/app.js?v=1", nil)
	r.Header.Set("Cookie", "measix_portal_session=secret")
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("Origin", "https://platform.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "export {}" || seenPath != "/custom/assets/app.js" || seenQuery != "v=1" {
		t.Fatalf("unexpected projection: status=%d path=%q query=%q body=%q", w.Code, seenPath, seenQuery, w.Body.String())
	}
	if seenCookie != "" || seenAuthorization != "" || seenOrigin != "" || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("Portal authority crossed the static upstream boundary")
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("Core security headers were not authoritative")
	}
}

func TestPortalRemoteRejectsRedirectAndInvalidConfigurationWithoutFallback(t *testing.T) {
	upstream := httptest.NewServer(http.RedirectHandler("https://other.example/", http.StatusFound))
	defer upstream.Close()
	h, err := portalstatic.NewRemote(upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/portal/", nil))
	if w.Code != http.StatusBadGateway || w.Header().Get("Location") != "" {
		t.Fatalf("redirect escaped Core: status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
	for _, raw := range []string{"", " ftp://portal.example", "ftp://portal.example", "https://user:pass@portal.example", "https://portal.example/path?mode=custom", "https://portal.example/path#fragment", "https://portal.example/a/../b"} {
		if _, err := portalstatic.ParseUpstream(raw); err == nil {
			t.Fatalf("invalid upstream accepted: %q", raw)
		}
	}
}
