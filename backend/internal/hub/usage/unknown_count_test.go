package usage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
)

func TestUnknownRequestCountUsesWholeRequestCompleteness(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	userID, upstreamID := seedUsageParents(t, st.Client, now)
	service := NewService(st.Client)
	count, err := service.UnknownRequestCount(ctx)
	if err != nil || count != 0 {
		t.Fatalf("empty count=%d err=%v", count, err)
	}
	// Missing, exact, partial, and mixed exact/unknown requests. Multiple unknown
	// meters still count as one request; unlinked provider records count as none.
	for i, states := range [][]Completeness{nil, {CompletenessComplete}, {CompletenessPartial}, {CompletenessComplete, CompletenessUnknown, CompletenessUnknown}} {
		event := validRequestUsageEvent(now, userID, upstreamID)
		if _, err := service.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event}}); err != nil {
			t.Fatal(err)
		}
		for j, state := range states {
			_, _, err := service.RecordSemantic(ctx, SemanticInput{RequestID: &event.RequestId, UpstreamID: upstreamID, ResourceID: event.ResourceId, SourceEventID: fmt.Sprintf("%d-%d", i, j), Meter: "INPUT_TOKENS", QuantityDecimal: "1", Completeness: state, Source: "provider_response", OccurredAt: now})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, _, err := service.RecordSemantic(ctx, SemanticInput{UpstreamID: upstreamID, SourceEventID: "unlinked", Meter: "INPUT_TOKENS", QuantityDecimal: "1", Completeness: CompletenessUnknown, Source: "provider_response", OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	count, err = service.UnknownRequestCount(ctx)
	if err != nil || count != 2 {
		t.Fatalf("count=%d want 2, err=%v", count, err)
	}
	for _, tc := range []struct {
		filter Filter
		want   RequestCompletenessCounts
	}{
		{Filter{}, RequestCompletenessCounts{Exact: 1, Partial: 1, Unknown: 2}},
		{Filter{Completeness: CompletenessUnknown}, RequestCompletenessCounts{Unknown: 2}},
		{Filter{UserID: "usr_00000000-0000-4000-8000-000000000001"}, RequestCompletenessCounts{}},
	} {
		summary, err := service.Summary(ctx, tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		if summary.RequestCompleteness != tc.want || summary.RequestCount != tc.want.Exact+tc.want.Partial+tc.want.Unknown {
			t.Fatalf("summary count=%d completeness=%+v want=%+v", summary.RequestCount, summary.RequestCompleteness, tc.want)
		}
	}
}
