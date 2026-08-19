package usage

import (
	"context"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func TestHUBI5SemanticUsageDedupeAndCompleteness(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)

	input := SemanticInput{
		UpstreamID: upstreamID,
		ResourceID: platformid.New(platformid.Model),
		SourceEventID: "provider-event-42",
		Meter: "input_tokens",
		QuantityDecimal: "1234",
		Completeness: CompletenessPartial,
		Source: "provider_response",
		OccurredAt: now,
	}
	first, duplicate, err := service.RecordSemantic(context.Background(), input)
	if err != nil || duplicate || first == "" {
		t.Fatalf("unexpected first semantic result id=%q duplicate=%v err=%v", first, duplicate, err)
	}
	second, duplicate, err := service.RecordSemantic(context.Background(), input)
	if err != nil || !duplicate || second != first {
		t.Fatalf("semantic dedupe failed id=%q duplicate=%v err=%v", second, duplicate, err)
	}
}

func TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, upstreamID := seedUsageParents(t, store.Client, now)
	resourceID := platformid.New(platformid.Model)
	service := NewService(store.Client)

	if _, err := service.CreatePricingRule(context.Background(), PricingRuleInput{
		UpstreamID: &upstreamID,
		Meter: "input_tokens", UnitSizeDecimal: "1000", UnitPriceDecimal: "0.0015", Currency: "USD",
		EffectiveFrom: now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePricingRule(context.Background(), PricingRuleInput{
		ResourceID: &resourceID, UpstreamID: &upstreamID,
		Meter: "input_tokens", UnitSizeDecimal: "1000", UnitPriceDecimal: "0.002", Currency: "USD",
		EffectiveFrom: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	cost, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "input_tokens",
		QuantityDecimal: "1500", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cost.State != CostKnown || cost.AmountDecimal != "0.003" || cost.Currency != "USD" {
		t.Fatalf("unexpected exact cost: %+v", cost)
	}

	partial, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "input_tokens",
		QuantityDecimal: "1500", Completeness: CompletenessPartial, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.State != CostPartial || partial.AmountDecimal != "0.003" {
		t.Fatalf("partial usage was not preserved: %+v", partial)
	}

	unknown, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "output_tokens",
		QuantityDecimal: "1", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.State != CostUnknown || unknown.AmountDecimal != "" {
		t.Fatalf("missing price was presented as known cost: %+v", unknown)
	}
}
