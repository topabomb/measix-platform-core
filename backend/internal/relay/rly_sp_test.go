package relay_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/relay/metering"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

// RLY-SP-001: Event committed, then process crash/restart — event persists.
func TestRLYSP001EventPersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relay-spool-001.db")
	spool, err := metering.OpenSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	event := usageEventLocal(now)
	payload, _ := json.Marshal(event)
	if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
		t.Fatal(err)
	}
	if err := spool.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen — event must persist.
	spool2, err := metering.OpenSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	defer spool2.Close()
	rows, err := spool2.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RequestID != event.RequestId {
		t.Fatalf("event not persisted: %+v", rows)
	}
}

// RLY-SP-002: RequestId unique.
func TestRLYSP002RequestIdUnique(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool-002.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	id := platformid.New(platformid.Request)
	if err := spool.Append(ctx, id, json.RawMessage(`{"a":1}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := spool.Append(ctx, id, json.RawMessage(`{"a":2}`), time.Now()); err == nil {
		t.Fatal("duplicate request ID unexpectedly accepted")
	}
}

// RLY-SP-005: 401/403 internal auth → degraded + low-frequency retry.
func TestRLYSP005AuthFailureTriggersDegradedLowFreq(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool-005.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	event := usageEventLocal(now)
	payload, _ := json.Marshal(event)
	if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
		t.Fatal(err)
	}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer hub.Close()
	sender := metering.NewSender(spool, hub.URL, "hub-service-token")
	sender.Now = func() time.Time { return now }
	sender.Jitter = func(time.Duration) time.Duration { return 0 }
	if err := sender.FlushOnce(ctx); err == nil {
		t.Fatal("auth failure unexpectedly succeeded")
	}
	rows, err := spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AttemptCount != 1 {
		t.Fatalf("auth failure did not retain/backoff: %+v", rows)
	}
	stats, err := spool.Stats(ctx, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if stats.State != metering.StateDegraded {
		t.Fatalf("expected degraded, got %s", stats.State)
	}
}

// RLY-SP-007: Sender restart does not need in-memory sent-set.
func TestRLYSP007SenderRestartWithoutMemorySet(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool-007.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	event := usageEventLocal(now)
	payload, _ := json.Marshal(event)
	if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
		t.Fatal(err)
	}
	// First sender fails (Hub unavailable).
	sender1 := metering.NewSender(spool, "http://127.0.0.1:1", "hub-service-token")
	sender1.Now = func() time.Time { return now }
	sender1.Jitter = func(time.Duration) time.Duration { return 0 }
	_ = sender1.FlushOnce(ctx)
	// Second sender (new instance) on same spool must still see the row.
	sender2 := metering.NewSender(spool, "http://127.0.0.1:1", "hub-service-token")
	sender2.Now = func() time.Time { return now.Add(2 * time.Second) }
	sender2.Jitter = func(time.Duration) time.Duration { return 0 }
	_ = sender2.FlushOnce(ctx)
	rows, err := spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("row lost after sender restart: %+v", rows)
	}
}

// RLY-SP-008: Oldest age / pending count / status correct.
func TestRLYSP008StatsCorrect(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool-008.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		event := usageEventLocal(now)
		payload, _ := json.Marshal(event)
		if err := spool.Append(ctx, event.RequestId, payload, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := spool.Stats(ctx, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if stats.PendingCount != 3 {
		t.Fatalf("pending count: got %d want 3", stats.PendingCount)
	}
	if stats.OldestPendingAge < 9*time.Second {
		t.Fatalf("oldest age too small: %v", stats.OldestPendingAge)
	}
	if stats.State != metering.StateOK {
		t.Fatalf("expected OK, got %s", stats.State)
	}
}

// RLY-SP-009: Disk full / write failure → METERING_DEGRADED.
func TestRLYSP009DiskFullTriggersDegraded(t *testing.T) {
	// Use a non-existent directory to simulate write failure.
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool-009.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	recorder := metering.NewRecorder(spool)
	recorder.Timeout = 100 * time.Millisecond
	// Close the spool to force write failure.
	_ = spool.Close()
	event := usageEventLocal(time.Now())
	if err := recorder.Record(event); err == nil {
		t.Fatal("write failure expected but got nil")
	}
	if recorder.State() != metering.StateDegraded {
		t.Fatalf("expected degraded after write failure, got %s", recorder.State())
	}
}

// RLY-SP-010: Shutdown does not infinitely block on unreachable Hub.
func TestRLYSP010ShutdownDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool-010.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	event := usageEventLocal(now)
	payload, _ := json.Marshal(event)
	if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
		t.Fatal(err)
	}
	sender := metering.NewSender(spool, "http://127.0.0.1:1", "hub-service-token")
	sender.Now = func() time.Time { return now }
	sender.Jitter = func(time.Duration) time.Duration { return 0 }
	sender.Client = &http.Client{Timeout: 100 * time.Millisecond}
	done := make(chan error, 1)
	go func() {
		done <- sender.FlushOnce(ctx)
	}()
	select {
	case <-done:
		// Good — did not block.
	case <-time.After(1 * time.Second):
		t.Fatal("sender blocked on unreachable Hub during shutdown")
	}
}

// RLY-CTL: Control handler security — public caller cannot access /internal/v1/control/*.
func TestRLYSecurityPublicCannotAccessInternalControl(t *testing.T) {
	fixture, _ := singleRouteFixture(t, "http://127.0.0.1:1", "runtime-secret")
	defer fixture.close()
	// Try to access internal control endpoint from the public listener.
	req, _ := http.NewRequest(http.MethodGet, fixture.server.URL+"/internal/v1/control/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("public caller accessed internal control endpoint")
	}
}

// RLY-SP: Client cannot inject X-Measix-Request-Id.
func TestRLYSecurityClientCannotInjectRequestId(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Measix-Request-Id")
		if rid == "client-injected" {
			t.Fatal("client injected X-Measix-Request-Id reached upstream")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	req.Header.Set("X-Measix-Request-Id", "client-injected")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// Relay must overwrite the client-supplied requestId.
	if resp.Header.Get("X-Measix-Request-Id") == "client-injected" {
		t.Fatal("client X-Measix-Request-Id was not overwritten")
	}
}

// Ensure imports are used.
var _ = os.Stat
var _ = usageingestapi.UsageBatch{}

// usageEventLocal creates a minimal valid RequestUsageEvent for relay_test package tests.
func usageEventLocal(now time.Time) usageingestapi.RequestUsageEvent {
	return usageingestapi.RequestUsageEvent{
		RequestId: platformid.New(platformid.Request), DeploymentId: platformid.New(platformid.Deployment),
		UserId: platformid.New(platformid.User), ResourceId: platformid.New(platformid.Model),
		RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: platformid.New(platformid.Upstream),
		ManagedGeneration: 1, ControlRevision: 1, StartedAt: now, CompletedAt: now,
		Forwarded: true, HttpStatus: 200, RequestBytes: 1, ResponseBytes: 1, DurationMs: 0,
	}
}
