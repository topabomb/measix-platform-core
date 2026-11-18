package httpapi

import (
	"testing"
	"time"

	"measix/platform/internal/hub/usage"
)

func TestAdminCostWirePreservesCurrencyBucketsAndPricingLines(t *testing.T) {
	wire := adminCostWire(usage.CostSummary{
		State: usage.CostPartial, PartialRequests: 1, MissingPricingRequests: 1,
		Amounts: []usage.CurrencyAmount{{Currency: "CNY", Amount: "2"}, {Currency: "USD", Amount: "1"}},
		Lines:   []usage.CostLine{{Meter: "INPUT_TOKENS", Quantity: "100", UnitSize: "1000000", UnitPrice: "2", Currency: "CNY", Amount: "0.0002", PricingRuleID: "prc_1"}},
	})
	if wire.Status != "PARTIAL" || wire.Amount != nil || wire.Currency != nil || len(wire.Amounts) != 2 || wire.Amounts[1].Currency != "USD" || wire.Lines == nil || len(*wire.Lines) != 1 || (*wire.Lines)[0].PricingRuleId != "prc_1" || wire.MissingPricingRequests != 1 {
		t.Fatalf("admin cost wire = %+v", wire)
	}
}

func TestPricingWirePreservesExclusiveEffectiveEnd(t *testing.T) {
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	wire := pricingSetWire(1, []usage.PricingRuleRecord{{ID: "prc_1", Meter: "REQUESTS", UnitSize: "1", UnitPrice: "2", Currency: "CNY", EffectiveFrom: end.Add(-time.Hour), EffectiveTo: &end}})
	if len(wire.Rules) != 1 || wire.Rules[0].EffectiveTo == nil || !wire.Rules[0].EffectiveTo.Equal(end) {
		t.Fatalf("pricing effective end = %+v", wire.Rules)
	}
}
