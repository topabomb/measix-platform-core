package usage

import (
	"context"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"testing"
	"time"
)

func TestUsageFiltersApplyToRequestsAndSemanticTotals(t *testing.T) {
	st := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	userA, up := seedUsageParents(t, st.Client, now)
	userB, _ := seedUsageParents(t, st.Client, now)
	s := NewService(st.Client)
	s.Now = func() time.Time { return now }
	ctx := context.Background()
	a := validRequestUsageEvent(now, userA, up)
	b := validRequestUsageEvent(now, userB, up)
	if _, err := s.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{a, b}}); err != nil {
		t.Fatal(err)
	}
	for i, event := range []usageingestapi.RequestUsageEvent{a, b} {
		quantity := "3"
		if i == 1 {
			quantity = "100"
		}
		if _, _, err := s.RecordSemantic(ctx, SemanticInput{RequestID: &event.RequestId, UpstreamID: up, ResourceID: event.ResourceId, Meter: "INPUT_TOKENS", QuantityDecimal: quantity, Completeness: CompletenessComplete, Source: "test", OccurredAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	from, to := now.Add(-time.Hour), now.Add(time.Hour)
	summary, err := s.Summary(ctx, Filter{From: &from, To: &to, UserID: userA})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Meters) != 1 || summary.Meters[0].Quantity != "3" {
		t.Fatalf("user filter leaked unrelated totals: %+v", summary.Meters)
	}
	from = now.Add(time.Minute)
	rows, err := s.ListRequests(ctx, Filter{From: &from, To: &to}, 50)
	if err != nil || len(rows) != 0 {
		t.Fatalf("time filter ignored: len=%d err=%v", len(rows), err)
	}
	rows, err = s.ListRequests(ctx, Filter{Completeness: CompletenessUnknown}, 50)
	if err != nil || len(rows) != 0 {
		t.Fatalf("completeness filter ignored: len=%d err=%v", len(rows), err)
	}
}

func TestSummaryCompletenessUsesWholeRequest(t *testing.T) {
	st := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	user, up := seedUsageParents(t, st.Client, now)
	svc := NewService(st.Client)
	svc.Now = func() time.Time { return now }
	ctx := context.Background()
	event := validRequestUsageEvent(now, user, up)
	if _, err := svc.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event}}); err != nil {
		t.Fatal(err)
	}
	for i, confidence := range []Completeness{CompletenessComplete, CompletenessPartial} {
		if _, _, err := svc.RecordSemantic(ctx, SemanticInput{RequestID: &event.RequestId, UpstreamID: up, ResourceID: event.ResourceId, Meter: "INPUT_TOKENS", QuantityDecimal: "3", Completeness: confidence, Source: []string{"first", "second"}[i], OccurredAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for _, userFilter := range []string{"", user} {
		sum, err := svc.Summary(ctx, Filter{UserID: userFilter, Completeness: CompletenessPartial})
		if err != nil {
			t.Fatal(err)
		}
		if sum.RequestCount != 1 || len(sum.Meters) != 1 || sum.Meters[0].Quantity != "6" {
			t.Fatalf("partial request must include all its meters: %+v", sum)
		}
		exact, err := svc.Summary(ctx, Filter{UserID: userFilter, Completeness: CompletenessComplete})
		if err != nil {
			t.Fatal(err)
		}
		if exact.RequestCount != 0 || len(exact.Meters) != 0 {
			t.Fatalf("exact filter leaked a partial request: %+v", exact)
		}
	}
}
