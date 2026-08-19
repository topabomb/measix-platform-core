package app_test

import (
	"github.com/topabomb/measix-platform-core/backend/internal/hub/app"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	a := app.New(config.Config{ListenAddr: ":0", PublicBaseURL: "https://example.test", DBPath: "test.db"})
	for _, p := range []string{"/live", "/ready"} {
		rr := httptest.NewRecorder()
		a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != 200 {
			t.Fatalf("%s=%d", p, rr.Code)
		}
	}
}
