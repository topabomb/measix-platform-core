package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/common/observability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/system"
	"measix/platform/internal/hub/testutil"
)

func TestAdminSystemTelemetryAndEventsAreAuthenticatedAndBounded(t *testing.T) {
	st := testutil.OpenStore(t)
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	identity := testutil.NewIdentityService(t, st, now)
	if _, err := identity.Bootstrap(context.Background(), "Example", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	recorder := observability.NewRecorder(func() time.Time { return now })
	recorder.Record(observability.Outcome{Status: 200, Duration: 12 * time.Millisecond})
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "hub.jsonl"), []byte(`{"time":"2026-09-21T10:00:00Z","level":"WARN","event":"sample.failed","msg":"safe","token":"hidden"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	systemService := system.New(st, nil, "test")
	systemService.Telemetry = recorder
	systemService.DiagnosticsLogDir = logDir
	handler := httpapi.NewFull(httpapi.Services{Identity: identity, System: systemService})

	if got := doJSON(t, handler, http.MethodGet, "/api/admin/v1/system/telemetry?window=15m", nil, nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated telemetry=%d", got.Code)
	}
	cookie, _ := loginAdmin(t, handler)
	headers := map[string]string{"Cookie": cookie}
	telemetry := doJSON(t, handler, http.MethodGet, "/api/admin/v1/system/telemetry?window=15m", headers, nil)
	if telemetry.Code != http.StatusOK {
		t.Fatalf("telemetry=%d %s", telemetry.Code, telemetry.Body.String())
	}
	var observed struct {
		WindowMinutes int `json:"windowMinutes"`
		Hub           struct {
			Summary struct {
				RequestCount int `json:"requestCount"`
			} `json:"summary"`
		} `json:"hub"`
	}
	if err := json.Unmarshal(telemetry.Body.Bytes(), &observed); err != nil {
		t.Fatal(err)
	}
	if observed.WindowMinutes != 15 || observed.Hub.Summary.RequestCount != 1 {
		t.Fatalf("unexpected telemetry: %+v", observed)
	}

	events := doJSON(t, handler, http.MethodGet, "/api/admin/v1/system/events?service=HUB&limit=200", headers, nil)
	if events.Code != http.StatusOK {
		t.Fatalf("events=%d %s", events.Code, events.Body.String())
	}
	if strings.Contains(events.Body.String(), "hidden") {
		t.Fatalf("unsafe event response: %s", events.Body.String())
	}
}
