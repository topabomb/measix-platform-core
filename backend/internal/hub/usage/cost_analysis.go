package usage

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/semanticusage"
)

type CurrencyAmount struct {
	Currency string
	Amount   string
}

type CostLine struct {
	Meter, Quantity, UnitSize, UnitPrice, Currency, Amount, PricingRuleID string
}

// CostSummary is an estimate from the current pricing set and the settled
// semantic ledger. Amounts are never converted or summed across currencies.
type CostBreakdown struct {
	CostSummary
	MissingPricing bool
	UnknownMeter   bool
	rawAmounts     map[string]*big.Rat
}

func selectPricingRule(rules []*ent.PricingRule, meter, resourceID, upstreamID string, at time.Time) *ent.PricingRule {
	var best *ent.PricingRule
	bestSpecificity := -1
	for _, rule := range rules {
		if rule.Meter != meter || rule.EffectiveFrom.After(at) || rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo) {
			continue
		}
		specificity := 0
		if rule.ResourceID != nil {
			if *rule.ResourceID != resourceID {
				continue
			}
			specificity += 2
		}
		if rule.UpstreamID != nil {
			if *rule.UpstreamID != upstreamID {
				continue
			}
			specificity++
		}
		if best == nil || specificity > bestSpecificity || specificity == bestSpecificity && (rule.EffectiveFrom.After(best.EffectiveFrom) || rule.EffectiveFrom.Equal(best.EffectiveFrom) && rule.ID < best.ID) {
			best, bestSpecificity = rule, specificity
		}
	}
	return best
}

func priceRequest(view RequestView, rules []*ent.PricingRule) (CostBreakdown, error) {
	result := CostBreakdown{CostSummary: CostSummary{State: CostUnknown}}
	if !view.Forwarded {
		result.State = CostKnown // Proven never forwarded: no upstream charge.
		result.PricedRequests = 1
		return result, nil
	}
	meters := make(map[string]MeterSummary, len(view.SemanticMeters))
	for _, meter := range view.SemanticMeters {
		meters[meter.Meter] = meter
	}
	selected := make(map[string]*ent.PricingRule)
	for _, meter := range []string{"REQUESTS", "REQUESTED_IMAGES", "CHARACTERS", "AUDIO_SECONDS", "INPUT_TOKENS", "OUTPUT_TOKENS", "CACHED_TOKENS", "TOTAL_TOKENS"} {
		selected[meter] = selectPricingRule(rules, meter, view.ResourceID, view.UpstreamID, view.CompletedAt.UTC())
	}
	amounts := map[string]*big.Rat{}
	priced := false
	partial := false
	add := func(meter string, quantity *big.Rat) error {
		rule := selected[meter]
		if rule == nil {
			return nil
		}
		unit, ok := decimalRat(rule.UnitSize)
		if !ok || unit.Sign() <= 0 {
			return fmt.Errorf("invalid persisted pricing unit size")
		}
		price, ok := decimalRat(rule.UnitPriceDecimal)
		if !ok || price.Sign() < 0 {
			return fmt.Errorf("invalid persisted pricing unit price")
		}
		amount := new(big.Rat).Mul(new(big.Rat).Quo(quantity, unit), price)
		quantityDecimal, err := exactDecimal(quantity)
		if err != nil {
			return err
		}
		amountDecimal, err := exactDecimal(amount)
		if err != nil {
			return err
		}
		result.Lines = append(result.Lines, CostLine{Meter: meter, Quantity: quantityDecimal, UnitSize: rule.UnitSize,
			UnitPrice: rule.UnitPriceDecimal, Currency: rule.Currency, Amount: amountDecimal, PricingRuleID: rule.ID})
		if amounts[rule.Currency] == nil {
			amounts[rule.Currency] = new(big.Rat)
		}
		amounts[rule.Currency].Add(amounts[rule.Currency], amount)
		priced = true
		return nil
	}
	charge := func(meter string, quantity *big.Rat) error {
		if selected[meter] == nil {
			return nil
		}
		entry, ok := meters[meter]
		if !ok || entry.Confidence == CompletenessUnknown {
			result.UnknownMeter = true
			return nil
		}
		if entry.Confidence == CompletenessPartial {
			partial = true
			result.UnknownMeter = true
		}
		if quantity == nil {
			var valid bool
			quantity, valid = decimalRat(entry.Quantity)
			if !valid || quantity.Sign() < 0 {
				return fmt.Errorf("invalid settled quantity for %s", meter)
			}
		}
		return add(meter, quantity)
	}
	if err := charge("REQUESTS", nil); err != nil {
		return result, err
	}
	switch view.ResourceKind {
	case ResourceKindModel:
		components := selected["INPUT_TOKENS"] != nil || selected["OUTPUT_TOKENS"] != nil || selected["CACHED_TOKENS"] != nil
		if components {
			input, hasInput := meters["INPUT_TOKENS"]
			output, hasOutput := meters["OUTPUT_TOKENS"]
			missingComponent := func(meter MeterSummary, exists bool) bool {
				if !exists || meter.Confidence != CompletenessComplete {
					result.UnknownMeter = true
					return true
				}
				quantity, valid := decimalRat(meter.Quantity)
				return !valid || quantity.Sign() != 0
			}
			if selected["INPUT_TOKENS"] == nil && missingComponent(input, hasInput) {
				result.MissingPricing = true
			}
			if selected["OUTPUT_TOKENS"] == nil && missingComponent(output, hasOutput) {
				result.MissingPricing = true
			}
			if selected["CACHED_TOKENS"] != nil && selected["INPUT_TOKENS"] != nil {
				cached, ok := meters["CACHED_TOKENS"]
				if !ok || cached.Confidence == CompletenessUnknown || !hasInput || input.Confidence == CompletenessUnknown {
					result.UnknownMeter = true
				} else {
					allInput, validInput := decimalRat(input.Quantity)
					cachedInput, validCached := decimalRat(cached.Quantity)
					if !validInput || !validCached || allInput.Sign() < 0 || cachedInput.Sign() < 0 || cachedInput.Cmp(allInput) > 0 {
						result.UnknownMeter = true
					} else {
						if input.Confidence == CompletenessPartial || cached.Confidence == CompletenessPartial {
							partial = true
							result.UnknownMeter = true
						}
						if err := add("INPUT_TOKENS", new(big.Rat).Sub(allInput, cachedInput)); err != nil {
							return result, err
						}
						if err := add("CACHED_TOKENS", cachedInput); err != nil {
							return result, err
						}
					}
				}
			} else {
				if err := charge("INPUT_TOKENS", nil); err != nil {
					return result, err
				}
				if err := charge("CACHED_TOKENS", nil); err != nil {
					return result, err
				}
			}
			if err := charge("OUTPUT_TOKENS", nil); err != nil {
				return result, err
			}
		} else if err := charge("TOTAL_TOKENS", nil); err != nil {
			return result, err
		}
	case ResourceKindImage:
		if err := charge("REQUESTED_IMAGES", nil); err != nil {
			return result, err
		}
	case ResourceKindTTS, ResourceKindASR:
		for _, meter := range []string{"CHARACTERS", "AUDIO_SECONDS"} {
			if err := charge(meter, nil); err != nil {
				return result, err
			}
		}
	}
	if !priced && !result.UnknownMeter {
		result.MissingPricing = true
	}
	if priced {
		result.rawAmounts = amounts
		currencies := make([]string, 0, len(amounts))
		for currency := range amounts {
			currencies = append(currencies, currency)
		}
		sort.Strings(currencies)
		for _, currency := range currencies {
			decimal, err := exactDecimal(amounts[currency])
			if err != nil {
				return result, err
			}
			result.Amounts = append(result.Amounts, CurrencyAmount{Currency: currency, Amount: decimal})
		}
		if len(result.Amounts) == 1 {
			result.Currency, result.Amount = result.Amounts[0].Currency, result.Amounts[0].Amount
		}
		result.State = CostKnown
		if partial || result.MissingPricing || result.UnknownMeter {
			result.State = CostPartial
		}
	} else if partial || result.MissingPricing || result.UnknownMeter {
		result.State = CostUnknown
	}
	switch result.State {
	case CostKnown:
		result.PricedRequests = 1
	case CostPartial:
		result.PartialRequests = 1
	default:
		result.UnknownRequests = 1
	}
	if result.MissingPricing {
		result.MissingPricingRequests = 1
	}
	if result.UnknownMeter {
		result.UnknownMeterRequests = 1
	}
	return result, nil
}

type costAccumulator struct {
	amounts                      map[string]*big.Rat
	priced, partial, unknown     int
	missingPricing, unknownMeter int
}

func (a *costAccumulator) add(value CostBreakdown) error {
	if a.amounts == nil {
		a.amounts = map[string]*big.Rat{}
	}
	switch value.State {
	case CostKnown:
		a.priced++
	case CostPartial:
		a.partial++
	default:
		a.unknown++
	}
	if value.MissingPricing {
		a.missingPricing++
	}
	if value.UnknownMeter {
		a.unknownMeter++
	}
	if value.rawAmounts != nil {
		for currency, amount := range value.rawAmounts {
			if a.amounts[currency] == nil {
				a.amounts[currency] = new(big.Rat)
			}
			a.amounts[currency].Add(a.amounts[currency], amount)
		}
	} else {
		for _, item := range value.Amounts {
			amount, ok := decimalRat(item.Amount)
			if !ok {
				return fmt.Errorf("invalid priced amount")
			}
			if a.amounts[item.Currency] == nil {
				a.amounts[item.Currency] = new(big.Rat)
			}
			a.amounts[item.Currency].Add(a.amounts[item.Currency], amount)
		}
	}
	return nil
}

func (a *costAccumulator) summary() (CostSummary, error) {
	result := CostSummary{State: CostUnknown, PricedRequests: a.priced, PartialRequests: a.partial, UnknownRequests: a.unknown,
		MissingPricingRequests: a.missingPricing, UnknownMeterRequests: a.unknownMeter}
	if a.priced+a.partial+a.unknown == 0 {
		return result, nil
	}
	if a.unknown == 0 && a.partial == 0 {
		result.State = CostKnown
	} else if a.priced > 0 || a.partial > 0 {
		result.State = CostPartial
	}
	currencies := make([]string, 0, len(a.amounts))
	for currency := range a.amounts {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	for _, currency := range currencies {
		amount, err := exactDecimal(a.amounts[currency])
		if err != nil {
			return result, err
		}
		result.Amounts = append(result.Amounts, CurrencyAmount{Currency: currency, Amount: amount})
	}
	if len(result.Amounts) == 1 {
		result.Currency, result.Amount = result.Amounts[0].Currency, result.Amounts[0].Amount
	}
	return result, nil
}

// analyzeCosts walks the filtered ledger in bounded batches and joins only the
// current settlement revision. The callback can build summary, day, or resource
// buckets without loading retained usage history into memory.
func (s *Service) analyzeCosts(ctx context.Context, filter Filter, visit func(RequestView, CostBreakdown) error) error {
	rules, err := s.Client.PricingRule.Query().All(ctx)
	if err != nil {
		return err
	}
	lastID := 0
	for {
		q := s.Client.RequestUsage.Query().Where(requestFilterPreds(filter)...)
		if lastID != 0 {
			q = q.Where(requestusage.IDGT(lastID))
		}
		rows, err := q.Order(ent.Asc(requestusage.FieldID)).Limit(500).All(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.RequestID)
		}
		semantic, err := s.Client.SemanticUsage.Query().Where(semanticusage.RequestIDIn(ids...)).All(ctx)
		if err != nil {
			return err
		}
		meters := make(map[string][]MeterSummary, len(rows))
		revisions := make(map[string]int64, len(rows))
		for _, row := range rows {
			revisions[row.RequestID] = row.SettlementRevision
		}
		for _, meter := range semantic {
			if meter.SettlementRevision != revisions[meter.RequestID] {
				continue
			}
			meters[meter.RequestID] = append(meters[meter.RequestID], MeterSummary{Meter: meter.Meter, Quantity: meter.QuantityDecimal, Confidence: Completeness(meter.Completeness)})
		}
		for _, row := range rows {
			view := requestView(row)
			view.SemanticMeters = meters[row.RequestID]
			priced, err := priceRequest(view, rules)
			if err != nil {
				return err
			}
			if err := visit(view, priced); err != nil {
				return err
			}
		}
		lastID = rows[len(rows)-1].ID
		if len(rows) < 500 {
			return nil
		}
	}
}
