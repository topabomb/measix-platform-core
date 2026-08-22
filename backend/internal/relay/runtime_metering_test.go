package relay_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/usageingestapi"
)

type captureUsageRecorder struct {
	mu     sync.Mutex
	events []usageingestapi.RequestUsageEvent
}

func (r *captureUsageRecorder) Record(event usageingestapi.RequestUsageEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *captureUsageRecorder) snapshot() []usageingestapi.RequestUsageEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]usageingestapi.RequestUsageEvent(nil), r.events...)
}

func TestRLYI5RuntimeWritesCapturedRequestUsage(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	fixture.server.Close()
	recorder := &captureUsageRecorder{}
	fixture.server = httptest.NewServer(relayruntime.NewHandlerWithRecorder(fixture.store, recorder))
	defer fixture.close()

	request := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"hello":"world"}`), "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected runtime status: %d body=%s", response.StatusCode, responseBody)
	}
	events := recorder.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected one usage event, got %d", len(events))
	}
	event := events[0]
	if !event.Forwarded || event.HttpStatus != http.StatusCreated || event.UpstreamHttpStatus == nil || *event.UpstreamHttpStatus != http.StatusCreated {
		t.Fatalf("unexpected forwarded event: %+v", event)
	}
	if event.ResourceId != resourceID || event.RuntimeRouteId == "" || event.UpstreamId == "" || event.ControlRevision != 1 || event.ManagedGeneration != 1 {
		t.Fatalf("event lost captured control attribution: %+v", event)
	}
	if event.RequestBytes == 0 || event.ResponseBytes == 0 || event.UserId != fixture.userID || event.DeviceId == nil || *event.DeviceId != fixture.deviceID {
		t.Fatalf("event lost request attribution/counts: %+v", event)
	}

	stale := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"sideEffect":true}`), "application/json")
	stale.Header.Set("X-Measix-Managed-Generation", "0")
	response, err = http.DefaultClient.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	staleBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusPreconditionRequired || upstreamCalls.Load() != 1 {
		t.Fatalf("stale generation forwarded unexpectedly: status=%d calls=%d body=%s", response.StatusCode, upstreamCalls.Load(), staleBody)
	}
	events = recorder.snapshot()
	if len(events) != 2 || events[1].Forwarded || events[1].HttpStatus != http.StatusPreconditionRequired {
		t.Fatalf("428 usage fact missing/incorrect: %+v", events)
	}
}
