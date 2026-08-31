package app_test

import (
	"measix/platform/internal/hub/app"
	"measix/platform/internal/hub/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	a := app.New(config.Config{ListenAddr: ":0", DBPath: "test.db"})
	for _, p := range []string{"/live", "/ready"} {
		rr := httptest.NewRecorder()
		a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != 200 {
			t.Fatalf("%s=%d", p, rr.Code)
		}
	}
}
