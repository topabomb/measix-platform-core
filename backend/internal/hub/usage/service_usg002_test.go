package usage

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

// HUB-USG-002: an invalid batch must not be partially committed.
// If any event in the batch is invalid, no row should be persisted.
func TestHUBUSG002InvalidBatchDoesNotPartiallyCommit(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	userID, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }

	validEvent := validRequestUsageEvent(now, userID, upstreamID)
	// Create a second valid event with a different requestId
	validEvent2 := validRequestUsageEvent(now, userID, upstreamID)
	validEvent2.RequestId = platformid.New(platformid.Request)

	// Corrupt the second event's resourceId to make it invalid
	invalidEvent := validEvent2
	invalidEvent.ResourceId = "not_a_valid_resource_id"

	// Batch has one valid + one invalid event
	_, err := service.Ingest(context.Background(), usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{validEvent, invalidEvent}})
	if err == nil {
		t.Fatal("expected error for invalid batch, got nil")
	}

	// Verify that NO rows were committed (not even the valid one)
	count, err := store.Client.RequestUsage.Query().Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid batch partially committed %d rows, expected 0", count)
	}

	// Now verify the valid event alone succeeds
	ack, err := service.Ingest(context.Background(), usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{validEvent}})
	if err != nil {
		t.Fatalf("valid event alone should succeed: %v", err)
	}
	if ack.AcceptedCount != 1 {
		t.Fatalf("expected accepted=1, got %d", ack.AcceptedCount)
	}
}
