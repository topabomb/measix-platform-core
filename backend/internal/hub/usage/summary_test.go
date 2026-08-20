package usage

import (
	"context"
	"strconv"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

// HUB-SUM-001: Summary aggregates request counts/bytes and semantic meters with
// EXACT/PARTIAL/UNKNOWN completeness, and reports cost as KNOWN/PARTIAL/UNKNOWN.
func TestHUBSUM001SummaryAggregatesCompletenessAndCost(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	userID, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }
	ctx := context.Background()
	resourceID := platformid.New(platformid.Model)

	// Two forwarded request rows.
	if _, err := service.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{
		validRequestUsageEvent(now, userID, upstreamID),
		validRequestUsageEvent(now, userID, upstreamID),
	}}); err != nil {
		t.Fatal(err)
	}

	// Semantic meters with mixed completeness. Unique source event ids so the
	// deterministic dedupe does not collapse records.
	for i, tc := range []struct {
		meter        string
		quantity     string
		completeness Completeness
	}{
		{"input_tokens", "1000", CompletenessComplete},
		{"input_tokens", "500", CompletenessPartial},
		{"output_tokens", "250", CompletenessUnknown},
	} {
		if _, _, err := service.RecordSemantic(ctx, SemanticInput{
			UpstreamID: upstreamID, ResourceID: resourceID, SourceEventID: "e-" + tc.meter + "-" + strconv.Itoa(i),
			Meter: tc.meter, QuantityDecimal: tc.quantity, Completeness: tc.completeness,
			Source: "provider_response", OccurredAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)
	sum, err := service.Summary(ctx, Filter{From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if sum.RequestCount != 2 || sum.ForwardedRequestCount != 2 {
		t.Fatalf("unexpected request counts: %+v", sum)
	}
	if sum.RequestBytes != 256 || sum.ResponseBytes != 1024 {
		t.Fatalf("unexpected bytes: req=%d resp=%d", sum.RequestBytes, sum.ResponseBytes)
	}
	if len(sum.Meters) != 2 {
		t.Fatalf("expected 2 meters, got %d", len(sum.Meters))
	}
	var input, output *MeterSummary
	for i := range sum.Meters {
		if sum.Meters[i].Meter == "input_tokens" {
			input = &sum.Meters[i]
		}
		if sum.Meters[i].Meter == "output_tokens" {
			output = &sum.Meters[i]
		}
	}
	if input == nil || input.Quantity != "1500" || input.Confidence != CompletenessPartial {
		t.Fatalf("unexpected input_tokens meter: %+v", input)
	}
	if output == nil || output.Quantity != "250" || output.Confidence != CompletenessUnknown {
		t.Fatalf("unexpected output_tokens meter: %+v", output)
	}
	// No provider cost was recorded → cost is UNKNOWN.
	if sum.Cost.State != CostUnknown {
		t.Fatalf("expected UNKNOWN cost, got %+v", sum.Cost)
	}
}

// HUB-SUM-002: Summary honours the completeness filter and time range, and
// returns ErrInvalidBatch for an inverted range.
func TestHUBSUM002SummaryFilterAndInvalidRange(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }
	ctx := context.Background()
	resourceID := platformid.New(platformid.Model)

	if _, _, err := service.RecordSemantic(ctx, SemanticInput{
		UpstreamID: upstreamID, ResourceID: resourceID, SourceEventID: "e-1",
		Meter: "input_tokens", QuantityDecimal: "100", Completeness: CompletenessComplete,
		Source: "provider_response", OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)

	// Completeness=EXACT filter should include the complete-only row.
	exactOnly, err := service.Summary(ctx, Filter{From: &from, To: &to, Completeness: CompletenessComplete})
	if err != nil {
		t.Fatal(err)
	}
	if len(exactOnly.Meters) != 1 {
		t.Fatalf("expected 1 meter under EXACT filter, got %d", len(exactOnly.Meters))
	}

	// Inverted range must be rejected.
	badFrom := now.Add(time.Hour)
	badTo := now.Add(-time.Hour)
	if _, err := service.Summary(ctx, Filter{From: &badFrom, To: &badTo}); err == nil {
		t.Fatal("expected ErrInvalidBatch for inverted range")
	}
}
