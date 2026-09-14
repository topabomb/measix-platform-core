package portalstatic_test

import (
	"measix/platform/internal/hub/portalstatic"
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
