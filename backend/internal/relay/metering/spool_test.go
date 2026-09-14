package metering_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"measix/platform/internal/relay/metering"
)

func TestSpoolPersistsAcrossRestartAndDeletesOnlyAckedRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relay-spool.db")
	spool, err := metering.OpenSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := spool.Append(ctx, "req_550e8400-e29b-41d4-a716-446655440000", json.RawMessage(`{"forwarded":true}`), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := spool.Close(); err != nil {
		t.Fatal(err)
	}

	spool, err = metering.OpenSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	rows, err := spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RequestID != "req_550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected persisted rows: %+v", rows)
	}
	if err := spool.Ack(ctx, []string{rows[0].RequestID}); err != nil {
		t.Fatal(err)
	}
	rows, err = spool.Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("acked row remains: %+v", rows)
	}
}

func TestSpoolRejectsDuplicateRequestID(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "relay-spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	id := "req_550e8400-e29b-41d4-a716-446655440000"
	if err := spool.Append(ctx, id, json.RawMessage(`{"a":1}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := spool.Append(ctx, id, json.RawMessage(`{"a":2}`), time.Now()); err == nil {
		t.Fatal("duplicate request id unexpectedly accepted")
	}
}

func TestSpoolDueUsesChronologicalTimestampOrder(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	instant := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if err := spool.Append(ctx, "req_test", json.RawMessage("{}"), instant); err != nil {
		t.Fatal(err)
	}
	if err := spool.MarkFailed(ctx, []string{"req_test"}, instant, "retry"); err != nil {
		t.Fatal(err)
	}
	rows, err := spool.Due(ctx, instant.Add(500*time.Millisecond), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("due retry lost at fractional second: rows=%v err=%v", rows, err)
	}
}
