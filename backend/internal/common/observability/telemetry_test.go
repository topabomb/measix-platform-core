package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestRecorderBoundsWindowAndComputesP95WithoutSamples(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	recorder := NewRecorder(func() time.Time { return now })
	for i := 0; i < 80; i++ {
		recorder.Record(Outcome{Status: 200, Duration: time.Duration(i+1) * time.Millisecond})
		now = now.Add(time.Minute)
	}
	now = now.Add(-time.Minute)
	snapshot := recorder.Snapshot(60 * time.Minute)
	if len(snapshot.Buckets) != 60 {
		t.Fatalf("bucket count=%d", len(snapshot.Buckets))
	}
	if snapshot.Buckets[len(snapshot.Buckets)-1].DurationP95Ms == 0 {
		t.Fatal("missing p95")
	}
}

func TestSafeLoggerAttachesIdentityRedactsAndBounds(t *testing.T) {
	var output bytes.Buffer
	logger := NewLogger(&output, "hub", "preview")
	logger.Error("failed", "event", "test.failed", "token", "secret-value", "error", errors.New(strings.Repeat("x", 800)), "username", "alice")
	var row map[string]any
	if err := json.Unmarshal(output.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row["service"] != "hub" || row["buildVersion"] != "preview" || row["event"] != "test.failed" {
		t.Fatalf("missing fixed fields: %#v", row)
	}
	if row["token"] != "[redacted]" || row["username"] != "[redacted]" {
		t.Fatalf("sensitive field leaked: %#v", row)
	}
	if len(row["error"].(string)) > maxFieldLength+3 {
		t.Fatal("error was not bounded")
	}
	if _, ok := any(safeHandler{}).(slog.Handler); !ok {
		t.Fatal("safe handler does not implement slog.Handler")
	}
}

func TestSafeErrorUsesStableCodesWithoutLeakingSecrets(t *testing.T) {
	value := SafeError(errors.New("open database: token=private-value https://private.example/path"))
	if value != "internal_error" || strings.Contains(value, "private-value") || strings.Contains(value, "private.example") {
		t.Fatalf("unsafe error code: %q", value)
	}
	if value := SafeError(fmt.Errorf("listen failed: %w", syscall.EADDRINUSE)); value != "address_in_use" {
		t.Fatalf("address code=%q", value)
	}
}

func TestHTTPMiddlewareRecordsRouteTemplateInsteadOfPathValue(t *testing.T) {
	var output bytes.Buffer
	recorder := NewRecorder(nil)
	router := chi.NewRouter()
	router.Use(HTTPMiddleware(recorder, Logger{Log: NewLogger(&output, "hub", "preview")}))
	router.Get("/items/{itemID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/items/private-item-123", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}

	var row map[string]any
	if err := json.Unmarshal(output.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row["route"] != "/items/{itemID}" {
		t.Fatalf("route=%v", row["route"])
	}
	if strings.Contains(output.String(), "private-item-123") {
		t.Fatal("request path value leaked into bounded-cardinality logs")
	}
	if snapshot := recorder.Snapshot(15 * time.Minute); snapshot.Summary.RequestCount != 1 {
		t.Fatalf("request count=%d", snapshot.Summary.RequestCount)
	}
}

func TestHTTPMiddlewareSuppressesSuccessfulControlPollingButKeepsFailures(t *testing.T) {
	var output bytes.Buffer
	recorder := NewRecorder(nil)
	router := chi.NewRouter()
	router.Use(HTTPMiddleware(recorder, Logger{Log: NewLogger(&output, "relay", "preview")}))
	status := http.StatusOK
	router.Get("/internal/v1/control/status", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/internal/v1/control/status", nil))
	if output.Len() != 0 || recorder.Snapshot(15*time.Minute).Summary.RequestCount != 0 {
		t.Fatal("successful control polling must not dominate diagnostics")
	}
	status = http.StatusServiceUnavailable
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/internal/v1/control/status", nil))
	if !strings.Contains(output.String(), "internal/v1/control/status") || recorder.Snapshot(15*time.Minute).Summary.ServerErrorCount != 1 {
		t.Fatal("failed control polling must remain observable")
	}
}
