package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/system"
	"measix/platform/internal/hub/testutil"
)

func TestPublicSystemHealthDoesNotBuildAdminReadModel(t *testing.T) {
	st := testutil.OpenStore(t)
	router := chi.NewRouter()
	httpapi.RegisterFull(router, httpapi.Services{System: system.New(st, nil, "test")})
	// No managed state or Relay: the public probe must not query diagnostics.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/admin/v1/system/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("public health traversed Admin state: %d %s", rr.Code, rr.Body.String())
	}
}
