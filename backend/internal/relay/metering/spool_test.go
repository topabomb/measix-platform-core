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
	id := "req_550e8400-e29b-41d4-a716-446655440000"
	if err := appendPending(ctx, spool, id, json.RawMessage(`{"requestId":"req_550e8400-e29b-41d4-a716-446655440000"}`), time.Unix(1, 0)); err != nil {
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
	now := time.Now().UTC()
	if err := appendPending(ctx, spool, id, json.RawMessage(`{"requestId":"req_550e8400-e29b-41d4-a716-446655440000","a":1}`), now); err != nil {
		t.Fatal(err)
	}
	if err := spool.AppendSettlement(ctx, id, json.RawMessage(`{"requestId":"req_550e8400-e29b-41d4-a716-446655440000","a":2}`), now); err == nil {
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
	if err := appendPending(ctx, spool, "req_test", json.RawMessage(`{"requestId":"req_test"}`), instant); err != nil {
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

func TestSpoolPurgesEveryStateOwnedByDeletedUser(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Now().UTC()
	deletedRequest := "req_deleted"
	if err := spool.SaveAdmission(ctx, deletedRequest, json.RawMessage(`{"requestId":"req_deleted","userId":"usr_deleted"}`), now); err != nil {
		t.Fatal(err)
	}
	if err := spool.MarkStarted(ctx, deletedRequest, now); err != nil {
		t.Fatal(err)
	}
	if err := spool.AppendSettlement(ctx, deletedRequest, json.RawMessage(`{"requestId":"req_deleted"}`), now); err != nil {
		t.Fatal(err)
	}
	if err := spool.SaveAdmission(ctx, "req_retained", json.RawMessage(`{"requestId":"req_retained","userId":"usr_retained"}`), now); err != nil {
		t.Fatal(err)
	}
	if err := spool.PurgeUsers(ctx, []string{"usr_deleted"}); err != nil {
		t.Fatal(err)
	}
	if rows, err := spool.Pending(ctx, 10); err != nil || len(rows) != 0 {
		t.Fatalf("deleted settlement remains: rows=%+v err=%v", rows, err)
	}
	if rows, err := spool.Recoverable(ctx, 10); err != nil || len(rows) != 1 || rows[0].RequestID != "req_retained" {
		t.Fatalf("unrelated request was not preserved: rows=%+v err=%v", rows, err)
	}
}

func appendPending(ctx context.Context, spool *metering.Spool, requestID string, settlement json.RawMessage, now time.Time) error {
	if err := spool.SaveAdmission(ctx, requestID, json.RawMessage(`{"admission":true}`), now); err != nil {
		return err
	}
	if err := spool.MarkStarted(ctx, requestID, now); err != nil {
		return err
	}
	return spool.AppendSettlement(ctx, requestID, settlement, now)
}
