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

func TestPortalRemoteConditionalGetAndHead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, conditional := range []string{"", "If-None-Match", "If-Modified-Since"} {
			t.Run(method+"/"+conditional, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
						t.Error("credentials crossed static boundary")
					}
					w.Header().Set("ETag", `"asset-v1"`)
					w.Header().Set("Last-Modified", "Mon, 05 Oct 2026 00:00:00 GMT")
					w.Header().Set("Set-Cookie", "forbidden=true")
					if conditional != "" && r.Header.Get(conditional) != "" {
						w.WriteHeader(http.StatusNotModified)
						return
					}
					_, _ = w.Write([]byte("asset"))
				}))
				defer upstream.Close()
				h, err := portalstatic.NewRemote(upstream.URL, upstream.Client())
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest(method, "/portal/assets/app.js", nil)
				r.Header.Set("Authorization", "Bearer synthetic")
				r.Header.Set("Cookie", "synthetic=true")
				if conditional == "If-None-Match" {
					r.Header.Set(conditional, `"asset-v1"`)
				}
				if conditional == "If-Modified-Since" {
					r.Header.Set(conditional, "Mon, 05 Oct 2026 00:00:00 GMT")
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				wantStatus, wantBody := http.StatusOK, "asset"
				if conditional != "" {
					wantStatus, wantBody = http.StatusNotModified, ""
				}
				if method == http.MethodHead {
					wantBody = ""
				}
				if w.Code != wantStatus || w.Body.String() != wantBody || w.Header().Get("ETag") != `"asset-v1"` || w.Header().Get("Last-Modified") == "" {
					t.Fatalf("conditional projection: status=%d body=%q headers=%v", w.Code, w.Body.String(), w.Header())
				}
				if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
					t.Fatal("static upstream overrode Core authority")
				}
			})
		}
	}
}
