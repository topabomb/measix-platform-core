package usage

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

// HUB-USG-004: UNKNOWN/PARTIAL must not fabricate a precise cost.
// When completeness is UNKNOWN, cost must be UNKNOWN regardless of
// pricing rules. When completeness is PARTIAL, cost must be PARTIAL.
func TestHUBUSG004UnknownPartialDoNotFabricateCost(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, upstreamID := seedUsageParents(t, store.Client, now)
	resourceID := platformid.New(platformid.Model)
	service := NewService(store.Client)

	// Create a pricing rule
	_, err := service.CreatePricingRule(context.Background(), PricingRuleInput{
		ResourceID: &resourceID, UpstreamID: &upstreamID,
		Meter: "input_tokens", UnitSizeDecimal: "1000", UnitPriceDecimal: "0.002", Currency: "USD",
		EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	// UNKNOWN must not produce a cost
	unknown, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "input_tokens",
		QuantityDecimal: "1500", Completeness: CompletenessUnknown, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.State != CostUnknown || unknown.AmountDecimal != "" {
		t.Fatalf("UNKNOWN fabricated cost: %+v", unknown)
	}

	// PARTIAL must produce a PARTIAL cost (amount known but state is PARTIAL)
	partial, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "input_tokens",
		QuantityDecimal: "1500", Completeness: CompletenessPartial, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.State != CostPartial || partial.AmountDecimal != "0.003" {
		t.Fatalf("PARTIAL cost not correct: %+v", partial)
	}

	// COMPLETE must produce a KNOWN cost
	complete, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "input_tokens",
		QuantityDecimal: "1500", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if complete.State != CostKnown || complete.AmountDecimal != "0.003" {
		t.Fatalf("COMPLETE cost not correct: %+v", complete)
	}
}

// HUB-USG-006: when meter or price is missing, Cost must be UNKNOWN.
func TestHUBUSG006MissingMeterOrPriceGivesUnknownCost(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, upstreamID := seedUsageParents(t, store.Client, now)
	resourceID := platformid.New(platformid.Model)
	service := NewService(store.Client)

	// No pricing rules created — meter missing
	result, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "nonexistent_meter",
		QuantityDecimal: "100", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CostUnknown || result.AmountDecimal != "" {
		t.Fatalf("missing meter should give UNKNOWN cost: %+v", result)
	}

	// Create a rule for one meter but query a different meter
	_, err = service.CreatePricingRule(context.Background(), PricingRuleInput{
		ResourceID: &resourceID, UpstreamID: &upstreamID,
		Meter: "input_tokens", UnitSizeDecimal: "1000", UnitPriceDecimal: "0.002", Currency: "USD",
		EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	result2, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "output_tokens",
		QuantityDecimal: "100", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result2.State != CostUnknown || result2.AmountDecimal != "" {
		t.Fatalf("missing price for meter should give UNKNOWN: %+v", result2)
	}
}

// HUB-USG-007: decimal quantity/cost must have no binary floating point errors.
// Use values that would produce rounding errors with float64.
func TestHUBUSG007DecimalArithmeticNoFloatErrors(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, upstreamID := seedUsageParents(t, store.Client, now)
	resourceID := platformid.New(platformid.Model)
	service := NewService(store.Client)

	_, err := service.CreatePricingRule(context.Background(), PricingRuleInput{
		ResourceID: &resourceID, UpstreamID: &upstreamID,
		Meter: "input_tokens", UnitSizeDecimal: "3", UnitPriceDecimal: "0.1", Currency: "USD",
		EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	// cost = (6/3) * 0.1 = 0.2
	// Use quantity=10, unitSize=4, unitPrice=0.1 → (10/4)*0.1 = 2.5*0.1 = 0.25
	// Actually let's use something simpler: quantity=10, unitSize=2, unitPrice=0.1
	// → (10/2)*0.1 = 5*0.1 = 0.5
	// Let's use quantity=10, unitSize=5, unitPrice=0.1 → (10/5)*0.1 = 2*0.1 = 0.2
	_, err = service.CreatePricingRule(context.Background(), PricingRuleInput{
		ResourceID: &resourceID, UpstreamID: &upstreamID,
		Meter: "simple_tokens", UnitSizeDecimal: "5", UnitPriceDecimal: "0.1", Currency: "USD",
		EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "simple_tokens",
		QuantityDecimal: "10", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AmountDecimal != "0.2" {
		t.Fatalf("expected exact 0.2, got %s", result.AmountDecimal)
	}

	// Test with large numbers that would overflow float64 precision
	// quantity=999999999999, unitSize=1, unitPrice=0.001
	// cost = 999999999999 * 0.001 = 999999999.999
	_, err = service.CreatePricingRule(context.Background(), PricingRuleInput{
		ResourceID: &resourceID, UpstreamID: &upstreamID,
		Meter: "large_tokens", UnitSizeDecimal: "1", UnitPriceDecimal: "0.001", Currency: "USD",
		EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	result2, err := service.CalculateCost(context.Background(), CostInput{
		UpstreamID: upstreamID, ResourceID: resourceID, Meter: "large_tokens",
		QuantityDecimal: "999999999999", Completeness: CompletenessComplete, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result2.AmountDecimal != "999999999.999" {
		t.Fatalf("expected exact 999999999.999, got %s", result2.AmountDecimal)
	}
}
