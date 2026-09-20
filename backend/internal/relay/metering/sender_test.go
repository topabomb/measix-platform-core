package metering_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/relay/metering"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestRecordDeniedSpoolHubIngestAndBlockedSummary(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	store := testutil.OpenStore(t)
	deploymentID := platformid.New(platformid.Deployment)
	userID := platformid.New(platformid.User)
	upstreamID := platformid.New(platformid.Upstream)
	if _, err := store.Client.Deployment.Create().SetID(deploymentID).SetName("Denied integration").SetStatus("ACTIVE").SetTimezone("UTC").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.User.Create().SetID(userID).SetUsername(userID).SetDisplayName("Denied User").SetRole("MEMBER").SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.Upstream.Create().SetID(upstreamID).SetName("Denied upstream").SetConfigRevision(1).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	budgetService, err := budget.NewService(store.Client, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = func() time.Time { return now }
	if _, err := budgetService.Put(ctx, budget.PutBudgetInput{
		UserID: userID, Capability: budget.CapabilityModel, ExpectedRevision: 0, Mode: budget.ModeLimited,
		Scopes:      []budget.ScopeInput{{Period: budget.PeriodLifetime, Limits: []budget.LimitSpec{{Meter: budget.MeterRequests, Limit: 0}}}},
		ActorUserID: userID, Reason: "integration denial",
	}); err != nil {
		t.Fatal(err)
	}
	requestID := platformid.New(platformid.Request)
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	decision, err := budgetService.Admit(ctx, budget.AdmitInput{
		RequestID: requestID, RequestHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeploymentID: deploymentID, UserID: userID, Capability: budget.CapabilityModel, ResourceID: resourceID,
		ClientProtocol: budget.ProtocolOpenAIResponses, UpstreamID: upstreamID, ManagedGeneration: 1, ControlRevision: 1,
		AdmittedAt: now, KnownQuantities: []budget.MeterQuantity{{Meter: budget.MeterRequests, Quantity: 1}}, SupportedMeters: []budget.Meter{budget.MeterRequests},
	})
	if err != nil || decision.Allowed {
		t.Fatalf("expected durable denial: %+v err=%v", decision, err)
	}
	one := int64(1)
	knownUsage := []usageingestapi.MeterValue{{Meter: usageingestapi.REQUESTS, Completeness: usageingestapi.EXACT, Numerator: &one, Denominator: &one}}
	admission := usageingestapi.BudgetAdmissionRequest{
		RequestId: requestID, RequestHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeploymentId: deploymentID, UserId: userID, ResourceId: resourceID, ResourceKind: usageingestapi.ResourceKindMODEL,
		ClientProtocol: usageingestapi.OPENAIRESPONSES, UpstreamId: upstreamID, ManagedGeneration: 1, ControlRevision: 1,
		AdmittedAt: now, KnownUsage: &knownUsage, SupportedMeters: []usageingestapi.UsageMeter{usageingestapi.REQUESTS},
	}
	settlement := usageingestapi.UsageSettlement{
		RequestId: requestID, Revision: 1, EventHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SourceEventId: requestID + ":1", OccurredAt: now, State: usageingestapi.SETTLED, Completeness: usageingestapi.EXACT,
		Meters: []usageingestapi.MeterValue{}, Request: usageingestapi.RequestUsageFact{
			DeploymentId: deploymentID, UserId: userID, ResourceId: resourceID, ResourceKind: usageingestapi.ResourceKindMODEL,
			ClientProtocol: usageingestapi.OPENAIRESPONSES, RuntimeRouteId: routeID, UpstreamId: upstreamID,
			ManagedGeneration: 1, ControlRevision: 1, StartedAt: now, CompletedAt: now, Forwarded: false,
			HttpStatus: 429, RequestBytes: 20, ResponseBytes: 0, DurationMs: 0,
		},
	}
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "denied-spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	if err := metering.NewRecorder(spool).RecordDenied(admission, routeID, settlement); err != nil {
		t.Fatal(err)
	}
	usageService := usage.NewService(store.Client, budgetService)
	usageService.Now = func() time.Time { return now.Add(time.Second) }
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var batch usageingestapi.UsageSettlementBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "invalid batch", http.StatusUnprocessableEntity)
			return
		}
		ack, err := usageService.IngestSettlements(r.Context(), batch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		_ = json.NewEncoder(w).Encode(ack)
	}))
	defer hub.Close()
	sender := metering.NewSender(spool, hub.URL, "private-token")
	sender.Now = func() time.Time { return now.Add(time.Second) }
	if err := sender.FlushOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if pending, err := spool.Pending(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("denied event remained in spool: %+v err=%v", pending, err)
	}
	summary, err := usageService.Summary(ctx, usage.Filter{UserID: userID, Status: usage.RequestStatusBlocked})
	if err != nil || summary.RequestCount != 1 || summary.ForwardedRequestCount != 0 || len(summary.Meters) != 0 {
		t.Fatalf("blocked summary = %+v err=%v", summary, err)
	}
}

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
		if err := appendPending(ctx, spool, event.RequestId, payload, now); err != nil {
			t.Fatal(err)
		}
	}

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer hub-service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var batch usageingestapi.UsageSettlementBatch
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
	if err := appendPending(ctx, spool, event.RequestId, payload, now); err != nil {
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
	for _, event := range []usageingestapi.UsageSettlement{good, poison} {
		payload, _ := json.Marshal(event)
		if err := appendPending(ctx, spool, event.RequestId, payload, now); err != nil {
			t.Fatal(err)
		}
	}

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch usageingestapi.UsageSettlementBatch
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

func usageEvent(now time.Time) usageingestapi.UsageSettlement {
	one := int64(1)
	requestID := platformid.New(platformid.Request)
	return usageingestapi.UsageSettlement{
		RequestId: requestID, Revision: 1, EventHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceEventId: requestID + ":1", OccurredAt: now, Completeness: usageingestapi.EXACT, State: usageingestapi.SETTLED,
		Meters: []usageingestapi.MeterValue{{Meter: usageingestapi.REQUESTS, Completeness: usageingestapi.EXACT, Numerator: &one, Denominator: &one}},
		Request: usageingestapi.RequestUsageFact{
			DeploymentId: platformid.New(platformid.Deployment), UserId: platformid.New(platformid.User),
			ResourceId: platformid.New(platformid.Model), ResourceKind: usageingestapi.ResourceKindMODEL,
			ClientProtocol: usageingestapi.OPENAIRESPONSES, RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: platformid.New(platformid.Upstream),
			ManagedGeneration: 1, ControlRevision: 1, StartedAt: now, CompletedAt: now,
			Forwarded: true, HttpStatus: 200, RequestBytes: 1, ResponseBytes: 1, DurationMs: 0,
		},
	}
}
func TestSenderRejectsMalformedAck(t *testing.T) {
	for _, body := range []string{`{"acceptedCount":-1,"duplicateCount":2}`, `{"acceptedCount":1,"duplicateCount":0}{}`} {
		t.Run(body, func(t *testing.T) {
			ctx := context.Background()
			spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "spool.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer spool.Close()
			now := time.Now().UTC()
			e := usageEvent(now)
			payload, _ := json.Marshal(e)
			if err := appendPending(ctx, spool, e.RequestId, payload, now); err != nil {
				t.Fatal(err)
			}
			hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer hub.Close()
			sender := metering.NewSender(spool, hub.URL, "private-token")
			if err := sender.FlushOnce(ctx); err == nil {
				t.Fatal("invalid ACK accepted")
			}
			rows, err := spool.Pending(ctx, 100)
			if err != nil || len(rows) != 1 {
				t.Fatalf("unacknowledged row lost: %d %v", len(rows), err)
			}
		})
	}
}
