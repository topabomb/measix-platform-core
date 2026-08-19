package usage

import (
	"context"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/ent"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/usageingestapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func TestHUBI5RequestUsageBatchIsIdempotent(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	userID, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }
	event := validRequestUsageEvent(now, userID, upstreamID)

	ack, err := service.Ingest(context.Background(), usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event, event}})
	if err != nil {
		t.Fatal(err)
	}
	if ack.AcceptedCount != 1 || ack.DuplicateCount != 1 {
		t.Fatalf("unexpected first ack: %+v", ack)
	}
	ack, err = service.Ingest(context.Background(), usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event}})
	if err != nil {
		t.Fatal(err)
	}
	if ack.AcceptedCount != 0 || ack.DuplicateCount != 1 {
		t.Fatalf("unexpected replay ack: %+v", ack)
	}
	count, err := store.Client.RequestUsage.Query().Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("request usage dedupe failed: %d rows", count)
	}
}

func TestHUBI5RequestUsageRejectsInvalidIdentity(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Now().UTC()
	userID, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)
	event := validRequestUsageEvent(now, userID, upstreamID)
	event.RequestId = "req_invalid"
	if _, err := service.Ingest(context.Background(), usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event}}); err == nil {
		t.Fatal("invalid requestId was accepted")
	}
	count, _ := store.Client.RequestUsage.Query().Count(context.Background())
	if count != 0 {
		t.Fatalf("invalid batch wrote rows: %d", count)
	}
}

func seedUsageParents(t *testing.T, client *ent.Client, now time.Time) (string, string) {
	t.Helper()
	ctx := context.Background()
	userID := platformid.New(platformid.User)
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("usage-test-" + userID).
		SetDisplayName("Usage Test").
		SetRole("MEMBER").
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID := platformid.New(platformid.Upstream)
	if _, err := client.Upstream.Create().
		SetID(upstreamID).
		SetName("usage-test-upstream").
		SetConfigRevision(1).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	return userID, upstreamID
}

func validRequestUsageEvent(now time.Time, userID, upstreamID string) usageingestapi.RequestUsageEvent {
	interaction := platformid.New(platformid.Interaction)
	upstreamStatus := 200
	return usageingestapi.RequestUsageEvent{
		RequestId:          platformid.New(platformid.Request),
		InteractionId:      &interaction,
		DeploymentId:       platformid.New(platformid.Deployment),
		UserId:             userID,
		ResourceId:         platformid.New(platformid.Model),
		RuntimeRouteId:     platformid.New(platformid.Route),
		UpstreamId:         upstreamID,
		ManagedGeneration:  3,
		ControlRevision:    7,
		StartedAt:          now.Add(-250 * time.Millisecond),
		CompletedAt:        now,
		Forwarded:          true,
		HttpStatus:         200,
		UpstreamHttpStatus: &upstreamStatus,
		RequestBytes:       128,
		ResponseBytes:      512,
		DurationMs:         250,
	}
}
