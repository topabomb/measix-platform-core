package metering_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"measix/platform/internal/relay/metering"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestRLYSPHubAckDeletesOnlyAcknowledgedBatch(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		event := usageEvent(now)
		payload, _ := json.Marshal(event)
		if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
			t.Fatal(err)
		}
	}

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer hub-service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var batch usageingestapi.UsageBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "bad batch", http.StatusUnprocessableEntity)
			return
		}
		_ = json.NewEncoder(w).Encode(usageingestapi.UsageBatchAck{AcceptedCount: len(batch.Events), DuplicateCount: 0})
	}))
	defer hub.Close()

	sender := metering.NewSender(spool, hub.URL+"/internal/v1/usage/request-events:batch", "hub-service-token")
	sender.Now = func() time.Time { return now }
	if err := sender.FlushOnce(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("acked events remain in spool: %+v", rows)
	}
}

func TestRLYSPHubOutageKeepsRowsAndRecordsBackoff(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	event := usageEvent(now)
	payload, _ := json.Marshal(event)
	if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
		t.Fatal(err)
	}

	sender := metering.NewSender(spool, "http://127.0.0.1:1/internal/v1/usage/request-events:batch", "hub-service-token")
	sender.Now = func() time.Time { return now }
	sender.Jitter = func(time.Duration) time.Duration { return 0 }
	if err := sender.FlushOnce(ctx); err == nil {
		t.Fatal("Hub outage unexpectedly succeeded")
	}
	rows, err := spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AttemptCount != 1 {
		t.Fatalf("outage did not retain/backoff row: %+v", rows)
	}
	stats, err := spool.Stats(ctx, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if stats.PendingCount != 1 || stats.State != metering.StateDegraded {
		t.Fatalf("unexpected degraded stats: %+v", stats)
	}
}

func TestRLYSPPoison422IsolatedWithoutDroppingGoodRows(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	good := usageEvent(now)
	poison := usageEvent(now)
	for _, event := range []usageingestapi.RequestUsageEvent{good, poison} {
		payload, _ := json.Marshal(event)
		if err := spool.Append(ctx, event.RequestId, payload, now); err != nil {
			t.Fatal(err)
		}
	}

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch usageingestapi.UsageBatch
		_ = json.NewDecoder(r.Body).Decode(&batch)
		for _, event := range batch.Events {
			if event.RequestId == poison.RequestId {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(usageingestapi.UsageBatchAck{AcceptedCount: len(batch.Events)})
	}))
	defer hub.Close()

	sender := metering.NewSender(spool, hub.URL, "hub-service-token")
	sender.Now = func() time.Time { return now }
	sender.Jitter = func(time.Duration) time.Duration { return 0 }
	if err := sender.FlushOnce(ctx); err == nil {
		t.Fatal("poison batch should report degraded flush")
	}
	rows, err := spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RequestID != poison.RequestId || rows[0].AttemptCount != 1 {
		t.Fatalf("poison isolation failed: %+v", rows)
	}
}

func usageEvent(now time.Time) usageingestapi.RequestUsageEvent {
	return usageingestapi.RequestUsageEvent{
		RequestId: platformid.New(platformid.Request), DeploymentId: platformid.New(platformid.Deployment),
		UserId: platformid.New(platformid.User), ResourceId: platformid.New(platformid.Model),
		RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: platformid.New(platformid.Upstream),
		ManagedGeneration: 1, ControlRevision: 1, StartedAt: now, CompletedAt: now,
		Forwarded: true, HttpStatus: 200, RequestBytes: 1, ResponseBytes: 1, DurationMs: 0,
	}
}
