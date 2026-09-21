package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/internal/common/observability"
	"measix/platform/internal/wire/relaycontrolapi"
)

func TestControlStatusIncludesBoundedRelayTelemetry(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	recorder := observability.NewRecorder(func() time.Time { return now })
	recorder.Record(observability.Outcome{Status: http.StatusBadGateway, Duration: 15 * time.Millisecond, UpstreamError: true})
	handler := NewHandlerWithTelemetry(NewStore(nil), "service-token", "preview", nil, nil, recorder)
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/control/status", nil)
	request.Header.Set("Authorization", "Bearer service-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var status relaycontrolapi.ControlStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Telemetry == nil || status.Telemetry.Summary.RequestCount != 1 || status.Telemetry.Summary.UpstreamErrorCount == nil || *status.Telemetry.Summary.UpstreamErrorCount != 1 {
		t.Fatalf("unexpected telemetry: %+v", status.Telemetry)
	}
}
