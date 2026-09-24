package usage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/ent/semanticusage"
	"measix/platform/ent/usageevent"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestSettlementLedgerIsAtomicIdempotentAndConvertsAudioOnce(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	deploymentID, userID, upstreamID := seedUsageParents(t, store.Client, now)
	budgetService, err := budget.NewService(store.Client, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = func() time.Time { return now }
	service := NewService(store.Client, budgetService)
	service.Now = func() time.Time { return now.Add(time.Second) }

	requestID := platformid.New(platformid.Request)
	resourceID := platformid.New(platformid.ASR)
	interactionID := platformid.New(platformid.Interaction)
	decision, err := budgetService.Admit(ctx, budget.AdmitInput{
		RequestID: requestID, RequestHash: testHash("admission"), DeploymentID: deploymentID, UserID: userID,
		InteractionID: &interactionID, Capability: budget.CapabilityASR, ResourceID: resourceID,
		ClientProtocol: budget.ProtocolOpenAIAudioTranscriptions, UpstreamID: upstreamID,
		ManagedGeneration: 1, ControlRevision: 1, AdmittedAt: now,
		KnownQuantities: []budget.MeterQuantity{{Meter: budget.MeterRequests, Quantity: 1}},
		SupportedMeters: []budget.Meter{budget.MeterRequests, budget.MeterAudioMilliseconds},
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("admit: %+v %v", decision, err)
	}
	if err := budgetService.Start(ctx, budget.LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: now.Add(time.Millisecond), EventHash: testHash("start")}); err != nil {
		t.Fatal(err)
	}

	one, three := int64(1), int64(3)
	settlement := usageingestapi.UsageSettlement{
		RequestId: requestID, Revision: 1, EventHash: testHash("settlement"), SourceEventId: requestID + ":1",
		OccurredAt: now.Add(time.Second), Completeness: usageingestapi.EXACT, State: usageingestapi.SETTLED,
		Request: usageingestapi.RequestUsageFact{
			DeploymentId: deploymentID, UserId: userID, InteractionId: &interactionID, ResourceId: resourceID,
			ResourceKind: usageingestapi.ResourceKindASR, ClientProtocol: usageingestapi.OPENAIAUDIOTRANSCRIPTIONS,
			RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: upstreamID, ManagedGeneration: 1, ControlRevision: 1,
			StartedAt: now.Add(time.Millisecond), CompletedAt: now.Add(time.Second), Forwarded: true, HttpStatus: 200,
			RequestBytes: 100, ResponseBytes: 20, DurationMs: 999,
		},
		Meters: []usageingestapi.MeterValue{
			meter(usageingestapi.REQUESTS, 1, 1),
			{Meter: usageingestapi.AUDIOSECONDS, Completeness: usageingestapi.EXACT, Numerator: &one, Denominator: &three},
		},
	}
	ack, err := service.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{settlement}})
	if err != nil || ack.AcceptedCount != 1 {
		t.Fatalf("ingest: %+v %v", ack, err)
	}
	ack, err = service.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{settlement}})
	if err != nil || ack.DuplicateCount != 1 {
		t.Fatalf("duplicate: %+v %v", ack, err)
	}
	row, err := store.Client.SemanticUsage.Query().Where(semanticusage.RequestIDEQ(requestID), semanticusage.MeterEQ("AUDIO_SECONDS")).Only(ctx)
	if err != nil || row.QuantityUnits != 334 || row.QuantityDecimal != "0.333333333" {
		t.Fatalf("audio projection: %+v %v", row, err)
	}
	summary, err := service.Summary(ctx, Filter{UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Meters) != 2 || summary.Meters[0].Meter != "AUDIO_SECONDS" || summary.Meters[0].Quantity != "0.334" {
		t.Fatalf("summary must expose aggregated internal milliseconds as seconds: %+v", summary.Meters)
	}
	users, err := service.ListUsers(ctx, Filter{}, budgetService, "", 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users.Items) != 1 || users.Items[0].UserID != userID || len(users.Items[0].Budget) != 5 ||
		len(users.Items[0].Meters) != 2 || users.Items[0].Meters[0].Meter != "AUDIO_SECONDS" || users.Items[0].Meters[0].Quantity != "0.334" {
		t.Fatalf("batched user projection = %+v", users)
	}
	retained := users.Items[0].UsageMeters[budget.CapabilityASR]
	if len(retained) != 2 || retained[0].Meter != "AUDIO_SECONDS" || retained[0].Quantity != "0.334" {
		t.Fatalf("retained capability usage = %+v", users.Items[0].UsageMeters)
	}
	byCapability, err := service.UsageMetersByCapability(ctx, userID)
	if err != nil || len(byCapability[budget.CapabilityASR]) != 2 {
		t.Fatalf("capability usage projection = %+v err=%v", byCapability, err)
	}
	trend, err := service.Trend(ctx, Filter{UserID: userID}, time.UTC)
	if err != nil || len(trend.Points) == 0 {
		t.Fatalf("batched trend: %+v err=%v", trend, err)
	}
	latestPoint := trend.Points[len(trend.Points)-1]
	if latestPoint.RequestCount != 1 || latestPoint.ForwardedRequestCount != 1 || len(latestPoint.Meters) != 2 {
		t.Fatalf("batched trend point = %+v", latestPoint)
	}
	distribution, err := service.Distribution(ctx, Filter{UserID: userID})
	if err != nil || len(distribution.Items) != 1 || distribution.Items[0].RequestCount != 1 || len(distribution.Items[0].Meters) != 2 {
		t.Fatalf("batched distribution = %+v err=%v", distribution, err)
	}

	conflict := settlement
	conflict.EventHash = testHash("different")
	if _, err := service.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{conflict}}); err == nil {
		t.Fatal("same revision with different content was accepted")
	}

	correction := settlement
	correction.Revision = 2
	correction.EventHash = testHash("correction-with-mutated-route")
	correction.SourceEventId = requestID + ":2"
	correction.Request.RuntimeRouteId = platformid.New(platformid.Route)
	if _, err := service.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{correction}}); !errors.Is(err, ErrAttributionMismatch) {
		t.Fatalf("correction changed immutable request attribution: %v", err)
	}
	if count, err := store.Client.UsageEvent.Query().Where(usageevent.RequestIDEQ(requestID)).Count(ctx); err != nil || count != 1 {
		t.Fatalf("rejected correction was not atomic: count=%d err=%v", count, err)
	}
}

func TestProvenUnforwardedDenialRequiresFullAttributionAndAppearsBlocked(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	deploymentID, userID, upstreamID := seedUsageParents(t, store.Client, now)
	budgetService, err := budget.NewService(store.Client, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = func() time.Time { return now }
	if _, err := budgetService.Put(ctx, budget.PutBudgetInput{
		UserID: userID, Capability: budget.CapabilityModel, ExpectedRevision: 0, Mode: budget.ModeLimited,
		Scopes:      []budget.ScopeInput{{Period: budget.PeriodLifetime, Limits: []budget.LimitSpec{{Meter: budget.MeterRequests, Limit: 0}}}},
		ActorUserID: userID, Reason: "deny all model requests",
	}); err != nil {
		t.Fatal(err)
	}
	requestID := platformid.New(platformid.Request)
	resourceID := platformid.New(platformid.Model)
	decision, err := budgetService.Admit(ctx, budget.AdmitInput{
		RequestID: requestID, RequestHash: testHash("denied-admission"), DeploymentID: deploymentID, UserID: userID,
		Capability: budget.CapabilityModel, ResourceID: resourceID, ClientProtocol: budget.ProtocolOpenAIResponses,
		UpstreamID: upstreamID, ManagedGeneration: 2, ControlRevision: 3, AdmittedAt: now,
		KnownQuantities: []budget.MeterQuantity{{Meter: budget.MeterRequests, Quantity: 1}}, SupportedMeters: []budget.Meter{budget.MeterRequests},
	})
	if err != nil || decision.Allowed || decision.Code != budget.DecisionBudgetExhausted {
		t.Fatalf("denial decision = %+v err=%v", decision, err)
	}
	service := NewService(store.Client, budgetService)
	service.Now = func() time.Time { return now.Add(time.Second) }
	event := usageingestapi.UsageSettlement{
		RequestId: requestID, Revision: 1, EventHash: testHash("denied-settlement"), SourceEventId: requestID + ":1",
		OccurredAt: now, State: usageingestapi.SETTLED, Completeness: usageingestapi.EXACT, Meters: []usageingestapi.MeterValue{},
		Request: usageingestapi.RequestUsageFact{
			DeploymentId: deploymentID, UserId: userID, ResourceId: resourceID, ResourceKind: usageingestapi.ResourceKindMODEL,
			ClientProtocol: usageingestapi.OPENAIRESPONSES, RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: upstreamID,
			ManagedGeneration: 2, ControlRevision: 3, StartedAt: now, CompletedAt: now, Forwarded: false,
			HttpStatus: 429, RequestBytes: 40, ResponseBytes: 0, DurationMs: 0,
		},
	}
	ack, err := service.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{event}})
	if err != nil || ack.AcceptedCount != 1 {
		t.Fatalf("denied ingest = %+v err=%v", ack, err)
	}
	summary, err := service.Summary(ctx, Filter{UserID: userID, Status: RequestStatusBlocked})
	if err != nil || summary.RequestCount != 1 || summary.ForwardedRequestCount != 0 || len(summary.Meters) != 0 {
		t.Fatalf("blocked summary = %+v err=%v", summary, err)
	}
	invalid := event
	invalid.RequestId = platformid.New(platformid.Request)
	invalid.SourceEventId = invalid.RequestId + ":1"
	invalid.EventHash = testHash("missing-route")
	invalid.Request.RuntimeRouteId = ""
	if _, err := service.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{invalid}}); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("missing denial attribution error = %v", err)
	}
}

func TestInvalidSettlementBatchDoesNotPartiallyCommit(t *testing.T) {
	store := testutil.OpenStore(t)
	budgetService, err := budget.NewService(store.Client, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store.Client, budgetService)
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	valid := structurallyValidSettlement(now)
	invalid := structurallyValidSettlement(now.Add(time.Second))
	invalid.Request.ResourceId = "invalid-resource-id"

	if _, err := service.IngestSettlements(context.Background(), usageingestapi.UsageSettlementBatch{
		Events: []usageingestapi.UsageSettlement{valid, invalid},
	}); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("invalid batch error = %v", err)
	}
	if count, err := store.Client.UsageEvent.Query().Count(context.Background()); err != nil || count != 0 {
		t.Fatalf("invalid batch partially committed: count=%d err=%v", count, err)
	}
}

func TestLateSettlementForDeletedPrincipalIsAcknowledgedAndDiscarded(t *testing.T) {
	store := testutil.OpenStore(t)
	budgetService, err := budget.NewService(store.Client, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store.Client, budgetService)
	event := structurallyValidSettlement(time.Now().UTC())
	if _, err := store.Client.DeletedPrincipal.Create().SetID(event.Request.UserId).SetDeletedAt(time.Now().UTC()).Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	ack, err := service.IngestSettlements(context.Background(), usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{event}})
	if err != nil || ack.DiscardedCount != 1 || ack.AcceptedCount != 0 || ack.DuplicateCount != 0 {
		t.Fatalf("late deleted settlement ack=%+v err=%v", ack, err)
	}
	if count, err := store.Client.UsageEvent.Query().Count(context.Background()); err != nil || count != 0 {
		t.Fatalf("deleted usage fact was recreated: count=%d err=%v", count, err)
	}
	if exists, err := store.Client.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(event.Request.UserId)).Exist(context.Background()); err != nil || !exists {
		t.Fatalf("deleted principal tombstone changed: exists=%v err=%v", exists, err)
	}
}

func structurallyValidSettlement(now time.Time) usageingestapi.UsageSettlement {
	one := int64(1)
	return usageingestapi.UsageSettlement{
		RequestId: platformid.New(platformid.Request), Revision: 1, EventHash: testHash(now.String()),
		SourceEventId: platformid.New(platformid.Request) + ":1", OccurredAt: now,
		Completeness: usageingestapi.EXACT, State: usageingestapi.SETTLED,
		Request: usageingestapi.RequestUsageFact{
			DeploymentId: platformid.New(platformid.Deployment), UserId: platformid.New(platformid.User),
			ResourceId: platformid.New(platformid.Model), ResourceKind: usageingestapi.ResourceKindMODEL,
			ClientProtocol: usageingestapi.OPENAICHATCOMPLETIONS, RuntimeRouteId: platformid.New(platformid.Route),
			UpstreamId: platformid.New(platformid.Upstream), ManagedGeneration: 1, ControlRevision: 1,
			StartedAt: now.Add(-time.Second), CompletedAt: now, Forwarded: true, HttpStatus: 200,
			RequestBytes: 10, ResponseBytes: 20, DurationMs: 1000,
		},
		Meters: []usageingestapi.MeterValue{{
			Meter: usageingestapi.REQUESTS, Completeness: usageingestapi.EXACT, Numerator: &one, Denominator: &one,
		}},
	}
}

func seedUsageParents(t *testing.T, client *ent.Client, now time.Time) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	deploymentID := platformid.New(platformid.Deployment)
	if _, err := client.Deployment.Create().SetID(deploymentID).SetName("Usage").SetStatus("ACTIVE").SetTimezone("Asia/Shanghai").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	userID := platformid.New(platformid.User)
	if _, err := client.User.Create().SetID(userID).SetUsername(userID).SetDisplayName("Usage User").SetRole("MEMBER").SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID := platformid.New(platformid.Upstream)
	if _, err := client.Upstream.Create().SetID(upstreamID).SetName("Usage upstream").SetConfigRevision(1).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	return deploymentID, userID, upstreamID
}

func meter(name usageingestapi.UsageMeter, value, denominator int64) usageingestapi.MeterValue {
	return usageingestapi.MeterValue{Meter: name, Completeness: usageingestapi.EXACT, Numerator: &value, Denominator: &denominator}
}

func testHash(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}
