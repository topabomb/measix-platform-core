package usage

import (
	"context"
	"math/big"
	"testing"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/requestusage"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func TestSummaryPricesSettledModelTokensWithoutChargingCacheTwice(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	deploymentID, userID, upstreamID := seedUsageParents(t, store.Client, now)
	modelID := platformid.New(platformid.Model)
	requestID := createUsageRequestRow(t, store.Client, requestRowInput{
		DeploymentID: deploymentID, UserID: userID, ResourceID: modelID, ResourceKind: "MODEL",
		Protocol: "OPENAI_CHAT_COMPLETIONS", UpstreamID: upstreamID, CompletedAt: now,
		Forwarded: true, HTTPStatus: 200, Completeness: "EXACT",
	})
	for meter, quantity := range map[string]int64{
		"REQUESTS": 1, "INPUT_TOKENS": 100, "CACHED_TOKENS": 40,
		"OUTPUT_TOKENS": 20, "TOTAL_TOKENS": 120,
	} {
		createSemanticUsageRow(t, store.Client, requestID, meter, quantity, "EXACT", now)
	}
	service := NewService(store.Client)
	service.Now = func() time.Time { return now.Add(time.Second) }
	for meter, price := range map[string]string{"INPUT_TOKENS": "2", "CACHED_TOKENS": "0.04", "OUTPUT_TOKENS": "8"} {
		if _, err := service.CreatePricingRule(ctx, PricingRuleInput{
			Meter: meter, UnitSizeDecimal: "1000000", UnitPriceDecimal: price, Currency: "CNY",
			EffectiveFrom: now.Add(-time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := service.Summary(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Cost.State != CostKnown || summary.Cost.Amount != "0.0002816" || summary.Cost.Currency != "CNY" {
		t.Fatalf("priced summary = %+v", summary.Cost)
	}
	request, err := service.GetRequest(ctx, requestID)
	if err != nil || request.Cost.State != CostKnown || request.Cost.Amount != summary.Cost.Amount {
		t.Fatalf("request cost = %+v, err=%v", request.Cost, err)
	}
	from, to := now.Add(-time.Hour), now.Add(time.Hour)
	trend, err := service.Trend(ctx, Filter{From: &from, To: &to}, time.UTC)
	if err != nil || len(trend.Points) != 1 || trend.Points[0].Cost.Amount != summary.Cost.Amount {
		t.Fatalf("trend cost = %+v, err=%v", trend, err)
	}
	distribution, err := service.Distribution(ctx, Filter{From: &from, To: &to})
	if err != nil || len(distribution.Items) != 1 || distribution.Items[0].Cost.Amount != summary.Cost.Amount {
		t.Fatalf("distribution cost = %+v, err=%v", distribution, err)
	}
}

func TestPriceRequestCurrentMeterFamiliesAndIncompleteCosts(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	rule := func(meter, price, currency string) *ent.PricingRule {
		return &ent.PricingRule{ID: meter, Meter: meter, UnitSize: "100", UnitPriceDecimal: price, Currency: currency, EffectiveFrom: now.Add(-time.Hour)}
	}
	meter := func(name, quantity string, confidence Completeness) MeterSummary {
		return MeterSummary{Meter: name, Quantity: quantity, Confidence: confidence}
	}
	tests := []struct {
		name       string
		kind       ResourceKind
		meters     []MeterSummary
		rules      []*ent.PricingRule
		wantState  CostState
		wantAmount string
	}{
		{"total token alternative", ResourceKindModel, []MeterSummary{meter("TOTAL_TOKENS", "120", CompletenessComplete)}, []*ent.PricingRule{rule("TOTAL_TOKENS", "2", "CNY")}, CostKnown, "2.4"},
		{"input only leaves output unpriced", ResourceKindModel, []MeterSummary{meter("INPUT_TOKENS", "100", CompletenessComplete), meter("OUTPUT_TOKENS", "20", CompletenessComplete)}, []*ent.PricingRule{rule("INPUT_TOKENS", "2", "CNY")}, CostPartial, "2"},
		{"image count", ResourceKindImage, []MeterSummary{meter("REQUESTED_IMAGES", "2", CompletenessComplete)}, []*ent.PricingRule{rule("REQUESTED_IMAGES", "3", "CNY")}, CostKnown, "0.06"},
		{"tts characters", ResourceKindTTS, []MeterSummary{meter("CHARACTERS", "20", CompletenessComplete)}, []*ent.PricingRule{rule("CHARACTERS", "2", "CNY")}, CostKnown, "0.4"},
		{"asr audio seconds", ResourceKindASR, []MeterSummary{meter("AUDIO_SECONDS", "12.5", CompletenessComplete)}, []*ent.PricingRule{rule("AUDIO_SECONDS", "4", "CNY")}, CostKnown, "0.5"},
		{"mcp per call", ResourceKindMCP, []MeterSummary{meter("REQUESTS", "1", CompletenessComplete)}, []*ent.PricingRule{rule("REQUESTS", "2", "CNY")}, CostKnown, "0.02"},
		{"image count plus call fee", ResourceKindImage, []MeterSummary{meter("REQUESTED_IMAGES", "2", CompletenessComplete), meter("REQUESTS", "1", CompletenessComplete)}, []*ent.PricingRule{rule("REQUESTED_IMAGES", "3", "CNY"), rule("REQUESTS", "2", "CNY")}, CostKnown, "0.08"},
		{"model output only with exact zero input", ResourceKindModel, []MeterSummary{meter("INPUT_TOKENS", "0.0", CompletenessComplete), meter("OUTPUT_TOKENS", "20", CompletenessComplete)}, []*ent.PricingRule{rule("OUTPUT_TOKENS", "8", "CNY")}, CostKnown, "1.6"},
		{"unknown input with output price", ResourceKindModel, []MeterSummary{meter("INPUT_TOKENS", "0", CompletenessUnknown), meter("OUTPUT_TOKENS", "20", CompletenessComplete)}, []*ent.PricingRule{rule("OUTPUT_TOKENS", "8", "CNY")}, CostPartial, "1.6"},
		{"cached detail unknown leaves output subtotal", ResourceKindModel, []MeterSummary{meter("INPUT_TOKENS", "100", CompletenessComplete), meter("CACHED_TOKENS", "0", CompletenessUnknown), meter("OUTPUT_TOKENS", "20", CompletenessComplete)}, []*ent.PricingRule{rule("INPUT_TOKENS", "2", "CNY"), rule("CACHED_TOKENS", "1", "CNY"), rule("OUTPUT_TOKENS", "8", "CNY")}, CostPartial, "1.6"},
		{"invalid cached subset leaves output subtotal", ResourceKindModel, []MeterSummary{meter("INPUT_TOKENS", "100", CompletenessComplete), meter("CACHED_TOKENS", "120", CompletenessComplete), meter("OUTPUT_TOKENS", "20", CompletenessComplete)}, []*ent.PricingRule{rule("INPUT_TOKENS", "2", "CNY"), rule("CACHED_TOKENS", "1", "CNY"), rule("OUTPUT_TOKENS", "8", "CNY")}, CostPartial, "1.6"},
		{"unknown measured amount", ResourceKindImage, []MeterSummary{meter("REQUESTED_IMAGES", "0", CompletenessUnknown)}, []*ent.PricingRule{rule("REQUESTED_IMAGES", "3", "CNY")}, CostUnknown, ""},
		{"missing price", ResourceKindImage, []MeterSummary{meter("REQUESTED_IMAGES", "2", CompletenessComplete)}, nil, CostUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := priceRequest(RequestView{ResourceKind: tt.kind, ResourceID: "resource", UpstreamID: "upstream", CompletedAt: now, Forwarded: true, SemanticMeters: tt.meters}, tt.rules)
			if err != nil || got.State != tt.wantState || got.Amount != tt.wantAmount {
				t.Fatalf("price = %+v, err=%v; want %s %s", got, err, tt.wantState, tt.wantAmount)
			}
		})
	}
}

func TestPricingScopeEffectiveTimeCurrencyAndRounding(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	resourceID, upstreamID := "mdl_a", "ups_a"
	global := &ent.PricingRule{ID: "pr_global", Meter: "REQUESTS", UnitSize: "1", UnitPriceDecimal: "1", Currency: "CNY", EffectiveFrom: now.Add(-time.Hour)}
	upstream := &ent.PricingRule{ID: "pr_upstream", Meter: "REQUESTS", UpstreamID: &upstreamID, UnitSize: "1", UnitPriceDecimal: "2", Currency: "CNY", EffectiveFrom: now.Add(-time.Hour)}
	resource := &ent.PricingRule{ID: "pr_resource", Meter: "REQUESTS", ResourceID: &resourceID, UnitSize: "1", UnitPriceDecimal: "3", Currency: "USD", EffectiveFrom: now.Add(-time.Hour)}
	expires := now.Add(time.Hour)
	resource.EffectiveTo = &expires
	future := &ent.PricingRule{ID: "pr_future", Meter: "REQUESTS", ResourceID: &resourceID, UpstreamID: &upstreamID, UnitSize: "1", UnitPriceDecimal: "9", Currency: "CNY", EffectiveFrom: now.Add(2 * time.Hour)}
	view := RequestView{ResourceKind: ResourceKindMCP, ResourceID: resourceID, UpstreamID: upstreamID,
		CompletedAt: now, Forwarded: true, SemanticMeters: []MeterSummary{{Meter: "REQUESTS", Quantity: "1", Confidence: CompletenessComplete}}}
	got, err := priceRequest(view, []*ent.PricingRule{global, upstream, resource, future})
	if err != nil || got.State != CostKnown || got.Amount != "3" || got.Currency != "USD" || len(got.Lines) != 1 || got.Lines[0].PricingRuleID != resource.ID {
		t.Fatalf("scoped price = %+v, err=%v", got, err)
	}
	view.CompletedAt = now.Add(90 * time.Minute)
	got, err = priceRequest(view, []*ent.PricingRule{global, upstream, resource, future})
	if err != nil || got.Amount != "2" || got.Currency != "CNY" {
		t.Fatalf("expired resource price = %+v, err=%v", got, err)
	}
	view.CompletedAt = now.Add(3 * time.Hour)
	got, err = priceRequest(view, []*ent.PricingRule{global, upstream, resource, future})
	if err != nil || got.Amount != "9" || got.Currency != "CNY" {
		t.Fatalf("future price = %+v, err=%v", got, err)
	}
	third, err := exactDecimal(big.NewRat(1, 3))
	if err != nil || third != "0.333333333333" {
		t.Fatalf("repeating unit price = %q, err=%v", third, err)
	}
}

func TestCostAccumulatorKeepsCurrenciesAndUnknownPortionSeparate(t *testing.T) {
	var costs costAccumulator
	for _, item := range []CostBreakdown{
		{CostSummary: CostSummary{State: CostKnown, Amounts: []CurrencyAmount{{Currency: "CNY", Amount: "2"}}}},
		{CostSummary: CostSummary{State: CostKnown, Amounts: []CurrencyAmount{{Currency: "USD", Amount: "1.5"}}}},
		{CostSummary: CostSummary{State: CostUnknown}, MissingPricing: true},
	} {
		if err := costs.add(item); err != nil {
			t.Fatal(err)
		}
	}
	got, err := costs.summary()
	if err != nil || got.State != CostPartial || got.Amount != "" || len(got.Amounts) != 2 || got.Amounts[0].Currency != "CNY" || got.Amounts[1].Currency != "USD" || got.MissingPricingRequests != 1 {
		t.Fatalf("multi-currency partial cost = %+v, err=%v", got, err)
	}
}

func TestCostAccumulatorRoundsOnlyAfterSumming(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	rule := &ent.PricingRule{ID: "pr_thirds", Meter: "REQUESTS", UnitSize: "3", UnitPriceDecimal: "1", Currency: "CNY", EffectiveFrom: now.Add(-time.Hour)}
	view := RequestView{ResourceKind: ResourceKindMCP, CompletedAt: now, Forwarded: true,
		SemanticMeters: []MeterSummary{{Meter: "REQUESTS", Quantity: "1", Confidence: CompletenessComplete}}}
	var costs costAccumulator
	for i := 0; i < 3; i++ {
		priced, err := priceRequest(view, []*ent.PricingRule{rule})
		if err != nil {
			t.Fatal(err)
		}
		if priced.Amount != "0.333333333333" {
			t.Fatalf("request amount = %q", priced.Amount)
		}
		if err := costs.add(priced); err != nil {
			t.Fatal(err)
		}
	}
	got, err := costs.summary()
	if err != nil || got.State != CostKnown || got.Amount != "1" {
		t.Fatalf("exact aggregate = %+v, err=%v", got, err)
	}
}

func TestCostAnalysisUsesCurrentSettlementRevisionAndCombinedFilters(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	deploymentID, userID, upstreamID := seedUsageParents(t, store.Client, now)
	resourceID := platformid.New(platformid.ImageGeneration)
	requestID := createUsageRequestRow(t, store.Client, requestRowInput{
		DeploymentID: deploymentID, UserID: userID, ResourceID: resourceID, ResourceKind: "IMAGE_GENERATION",
		Protocol: "OPENAI_IMAGES_GENERATIONS", UpstreamID: upstreamID, CompletedAt: now,
		Forwarded: true, HTTPStatus: 200, Completeness: "EXACT",
	})
	createSemanticUsageRow(t, store.Client, requestID, "REQUESTED_IMAGES", 9, "EXACT", now)
	if _, err := store.Client.RequestUsage.Update().Where(requestusage.RequestIDEQ(requestID)).SetSettlementRevision(2).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.SemanticUsage.Create().SetID(requestID + ":2:REQUESTED_IMAGES").SetRequestID(requestID).
		SetSettlementRevision(2).SetSourceEventID(requestID + ":2").SetMeter("REQUESTED_IMAGES").SetQuantityUnits(2).
		SetQuantityDecimal("2").SetCompleteness("EXACT").SetSource("test").SetOccurredAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	service := NewService(store.Client)
	service.Now = func() time.Time { return now.Add(time.Hour) }
	if _, err := service.CreatePricingRule(ctx, PricingRuleInput{Meter: "REQUESTED_IMAGES", UnitSizeDecimal: "1", UnitPriceDecimal: "3", Currency: "CNY", EffectiveFrom: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	from, to := now.Add(-time.Minute), now.Add(time.Minute)
	filter := Filter{From: &from, To: &to, UserID: userID, ResourceID: resourceID, UpstreamID: upstreamID, Status: RequestStatusSuccess, Completeness: CompletenessComplete}
	got, err := service.Summary(ctx, filter)
	if err != nil || got.Cost.Amount != "6" || got.Cost.PricedRequests != 1 {
		t.Fatalf("corrected and filtered cost = %+v, err=%v", got.Cost, err)
	}
	request, err := service.GetRequest(ctx, requestID)
	if err != nil || request.Cost.Amount != got.Cost.Amount {
		t.Fatalf("corrected request cost = %+v, err=%v", request.Cost, err)
	}
	filter.ResourceID = platformid.New(platformid.ImageGeneration)
	got, err = service.Summary(ctx, filter)
	if err != nil || got.Cost.State != CostUnknown || got.Cost.PricedRequests != 0 || len(got.Cost.Amounts) != 0 {
		t.Fatalf("empty filtered cost = %+v, err=%v", got.Cost, err)
	}
}

func TestCostAnalysisScansPastBatchBoundary(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	deploymentID, userID, upstreamID := seedUsageParents(t, store.Client, now)
	resourceID := platformid.New(platformid.MCP)
	for i := 0; i < 501; i++ {
		requestID := createUsageRequestRow(t, store.Client, requestRowInput{
			DeploymentID: deploymentID, UserID: userID, ResourceID: resourceID, ResourceKind: "MCP",
			Protocol: "MCP", UpstreamID: upstreamID, CompletedAt: now.Add(time.Duration(i) * time.Millisecond),
			Forwarded: true, HTTPStatus: 200, Completeness: "EXACT",
		})
		createSemanticUsageRow(t, store.Client, requestID, "REQUESTS", 1, "EXACT", now)
	}
	service := NewService(store.Client)
	service.Now = func() time.Time { return now.Add(time.Hour) }
	if _, err := service.CreatePricingRule(ctx, PricingRuleInput{Meter: "REQUESTS", UnitSizeDecimal: "1", UnitPriceDecimal: "2", Currency: "CNY", EffectiveFrom: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var costs costAccumulator
	if err := service.analyzeCosts(ctx, Filter{}, func(_ RequestView, priced CostBreakdown) error { return costs.add(priced) }); err != nil {
		t.Fatal(err)
	}
	got, err := costs.summary()
	if err != nil || got.State != CostKnown || got.PricedRequests != 501 || got.Amount != "1002" {
		t.Fatalf("501 priced requests = %+v, err=%v", got, err)
	}
}
