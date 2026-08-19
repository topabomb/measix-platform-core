package adminstatic_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/adminstatic"
)

func testFS() fs.FS {
	return fstest.MapFS{
		"index.html":             {Data: []byte("<html>admin</html>")},
		"assets/app.abcdef01.js": {Data: []byte("console.log('ok')")},
	}
}

func TestSPARouteFallsBackToIndex(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	w := httptest.NewRecorder()
	adminstatic.New(testFS()).ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "<html>admin</html>" {
		t.Fatalf("status/body=%d %q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index cache=%q", got)
	}
}

func TestFingerprintAssetServedAndMissingAssetDoesNotFallback(t *testing.T) {
	h := adminstatic.New(testFS())
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/admin/assets/app.abcdef01.js", http.StatusOK},
		{"/admin/assets/missing.js", http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Fatalf("%s status=%d want=%d", tc.path, w.Code, tc.want)
		}
		if tc.want == http.StatusOK && w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatalf("asset cache=%q", w.Header().Get("Cache-Control"))
		}
	}
}
