package health_test

import (
	"measix/platform/internal/common/health"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadiness(t *testing.T) {
	s := &health.State{}
	rr := httptest.NewRecorder()
	s.Ready(rr, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready status=%d", rr.Code)
	}
	s.SetReady(true)
	rr = httptest.NewRecorder()
	s.Ready(rr, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ready status=%d", rr.Code)
	}
}
