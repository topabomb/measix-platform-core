package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func pricingRule(id string, effectiveFrom time.Time) PricingRuleRecord {
	return PricingRuleRecord{ID: id, Meter: "input_tokens", UnitSize: "1000", UnitPrice: "0.0015", Currency: "USD", EffectiveFrom: effectiveFrom}
}

// HUB-PRS-001: ReplacePricingSet replaces the whole set atomically and returns
// the new revision; a stale expectedRevision is rejected with a conflict.
func TestHUBPRS001ReplacePricingSetAtomicAndConflict(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(store.Client)
	ctx := context.Background()

	initialRev, _, err := service.PricingSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rules := []PricingRuleRecord{pricingRule(platformid.New(platformid.PricingRule), now)}

	newRev, stored, err := service.ReplacePricingSet(ctx, initialRev, rules)
	if err != nil {
		t.Fatal(err)
	}
	if newRev == initialRev {
		t.Fatal("revision did not change after replace")
	}
	if len(stored) != 1 || stored[0].ID != rules[0].ID {
		t.Fatalf("unexpected stored rules: %+v", stored)
	}

	// A stale expectedRevision must conflict.
	if _, _, err := service.ReplacePricingSet(ctx, initialRev, rules); !errors.Is(err, ErrPricingRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}

// HUB-PRS-002: ReplacePricingSet rejects invalid records (bad id, non-positive
// unit size, negative unit price, inverted effective range, duplicate ids).
func TestHUBPRS002ReplacePricingSetValidation(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(store.Client)
	ctx := context.Background()
	rev, _, err := service.PricingSet(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		mut  func(*PricingRuleRecord)
	}{
		{"bad id", func(r *PricingRuleRecord) { r.ID = "not-an-id" }},
		{"empty meter", func(r *PricingRuleRecord) { r.Meter = " " }},
		{"empty currency", func(r *PricingRuleRecord) { r.Currency = "" }},
		{"zero unit size", func(r *PricingRuleRecord) { r.UnitSize = "0" }},
		{"negative unit price", func(r *PricingRuleRecord) { r.UnitPrice = "-0.01" }},
		{"inverted effective range", func(r *PricingRuleRecord) { to := r.EffectiveFrom.Add(-time.Hour); r.EffectiveTo = &to }},
		{"bad upstream ref", func(r *PricingRuleRecord) { u := "ups_bad"; r.UpstreamID = &u }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := pricingRule(platformid.New(platformid.PricingRule), now)
			tc.mut(&rule)
			if _, _, err := service.ReplacePricingSet(ctx, rev, []PricingRuleRecord{rule}); !errors.Is(err, ErrInvalidBatch) {
				t.Fatalf("expected ErrInvalidBatch, got %v", err)
			}
		})
	}

	// Duplicate ids in the same batch are invalid.
	dup := pricingRule(platformid.New(platformid.PricingRule), now)
	if _, _, err := service.ReplacePricingSet(ctx, rev, []PricingRuleRecord{dup, dup}); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("expected ErrInvalidBatch for duplicate ids, got %v", err)
	}
}
