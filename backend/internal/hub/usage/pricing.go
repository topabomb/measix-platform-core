package usage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/pricingrule"
	"measix/platform/ent/semanticusage"
	"measix/platform/pkg/platformid"
)

type Completeness string

const (
	CompletenessUnknown  Completeness = "UNKNOWN"
	CompletenessPartial  Completeness = "PARTIAL"
	CompletenessComplete Completeness = "COMPLETE"
)

type CostState string

const (
	CostUnknown CostState = "UNKNOWN"
	CostPartial CostState = "PARTIAL"
	CostKnown   CostState = "KNOWN"
)

type SemanticInput struct {
	RequestID       *string
	UpstreamID      string
	ResourceID      string
	SourceEventID   string
	Meter           string
	QuantityDecimal string
	Completeness    Completeness
	ProviderCost    string
	Currency        string
	Source          string
	OccurredAt      time.Time
}

type PricingRuleInput struct {
	ResourceID       *string
	UpstreamID       *string
	Meter            string
	UnitSizeDecimal  string
	UnitPriceDecimal string
	Currency         string
	EffectiveFrom    time.Time
	EffectiveTo      *time.Time
}

type CostInput struct {
	UpstreamID      string
	ResourceID      string
	Meter           string
	QuantityDecimal string
	Completeness    Completeness
	OccurredAt      time.Time
}

type CostResult struct {
	State         CostState
	AmountDecimal string
	Currency      string
	PricingRuleID string
}

func (s *Service) RecordSemantic(ctx context.Context, input SemanticInput) (string, bool, error) {
	if s.Client == nil || platformid.Validate(platformid.Upstream, input.UpstreamID) != nil || strings.TrimSpace(input.Meter) == "" || strings.TrimSpace(input.Source) == "" || input.OccurredAt.IsZero() || !validCompleteness(input.Completeness) {
		return "", false, ErrInvalidBatch
	}
	if input.ResourceID != "" && !runtimeResourceID(input.ResourceID) {
		return "", false, ErrInvalidBatch
	}
	if input.RequestID != nil && platformid.Validate(platformid.Request, *input.RequestID) != nil {
		return "", false, ErrInvalidBatch
	}
	quantity, ok := decimalRat(input.QuantityDecimal)
	if !ok || quantity.Sign() < 0 {
		return "", false, ErrInvalidBatch
	}
	if input.ProviderCost != "" {
		providerCost, ok := decimalRat(input.ProviderCost)
		if !ok || providerCost.Sign() < 0 || strings.TrimSpace(input.Currency) == "" {
			return "", false, ErrInvalidBatch
		}
	}

	if input.SourceEventID != "" {
		existing, err := s.Client.SemanticUsage.Query().Where(
			semanticusage.UpstreamIDEQ(input.UpstreamID),
			semanticusage.SourceEventIDEQ(input.SourceEventID),
		).Only(ctx)
		if err == nil {
			return existing.ID, true, nil
		}
		if !ent.IsNotFound(err) {
			return "", false, err
		}
	}

	id := platformid.New(platformid.UsageEvent)
	builder := s.Client.SemanticUsage.Create().
		SetID(id).
		SetNillableRequestID(input.RequestID).
		SetUpstreamID(input.UpstreamID).
		SetMeter(strings.TrimSpace(input.Meter)).
		SetQuantityDecimal(input.QuantityDecimal).
		SetCompleteness(string(input.Completeness)).
		SetSource(strings.TrimSpace(input.Source)).
		SetOccurredAt(input.OccurredAt.UTC())
	if input.ResourceID != "" {
		builder.SetResourceID(input.ResourceID)
	}
	if input.SourceEventID != "" {
		builder.SetSourceEventID(input.SourceEventID)
	}
	if input.ProviderCost != "" {
		builder.SetProviderCost(input.ProviderCost).SetCurrency(strings.TrimSpace(input.Currency))
	}
	row, err := builder.Save(ctx)
	if err != nil {
		if input.SourceEventID != "" && ent.IsConstraintError(err) {
			existing, lookupErr := s.Client.SemanticUsage.Query().Where(
				semanticusage.UpstreamIDEQ(input.UpstreamID),
				semanticusage.SourceEventIDEQ(input.SourceEventID),
			).Only(ctx)
			if lookupErr == nil {
				return existing.ID, true, nil
			}
		}
		return "", false, err
	}
	return row.ID, false, nil
}

func (s *Service) CreatePricingRule(ctx context.Context, input PricingRuleInput) (string, error) {
	if s.Client == nil || strings.TrimSpace(input.Meter) == "" || strings.TrimSpace(input.Currency) == "" || input.EffectiveFrom.IsZero() {
		return "", ErrInvalidBatch
	}
	if input.ResourceID != nil && !runtimeResourceID(*input.ResourceID) {
		return "", ErrInvalidBatch
	}
	if input.UpstreamID != nil && platformid.Validate(platformid.Upstream, *input.UpstreamID) != nil {
		return "", ErrInvalidBatch
	}
	if input.EffectiveTo != nil && !input.EffectiveTo.After(input.EffectiveFrom) {
		return "", ErrInvalidBatch
	}
	unitSize, ok := decimalRat(input.UnitSizeDecimal)
	if !ok || unitSize.Sign() <= 0 {
		return "", ErrInvalidBatch
	}
	unitPrice, ok := decimalRat(input.UnitPriceDecimal)
	if !ok || unitPrice.Sign() < 0 {
		return "", ErrInvalidBatch
	}

	id := platformid.New(platformid.PricingRule)
	row, err := s.Client.PricingRule.Create().
		SetID(id).
		SetNillableResourceID(input.ResourceID).
		SetNillableUpstreamID(input.UpstreamID).
		SetMeter(strings.TrimSpace(input.Meter)).
		SetUnitSize(input.UnitSizeDecimal).
		SetUnitPriceDecimal(input.UnitPriceDecimal).
		SetCurrency(strings.TrimSpace(input.Currency)).
		SetEffectiveFrom(input.EffectiveFrom.UTC()).
		SetNillableEffectiveTo(input.EffectiveTo).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return row.ID, nil
}

func (s *Service) CalculateCost(ctx context.Context, input CostInput) (CostResult, error) {
	if s.Client == nil || platformid.Validate(platformid.Upstream, input.UpstreamID) != nil || !runtimeResourceID(input.ResourceID) || strings.TrimSpace(input.Meter) == "" || input.OccurredAt.IsZero() || !validCompleteness(input.Completeness) {
		return CostResult{}, ErrInvalidBatch
	}
	quantity, ok := decimalRat(input.QuantityDecimal)
	if !ok || quantity.Sign() < 0 {
		return CostResult{}, ErrInvalidBatch
	}
	if input.Completeness == CompletenessUnknown {
		return CostResult{State: CostUnknown}, nil
	}

	rules, err := s.Client.PricingRule.Query().Where(pricingrule.MeterEQ(strings.TrimSpace(input.Meter))).All(ctx)
	if err != nil {
		return CostResult{}, err
	}
	at := input.OccurredAt.UTC()
	type candidate struct {
		rule        *ent.PricingRule
		specificity int
	}
	candidates := make([]candidate, 0, len(rules))
	for _, rule := range rules {
		if rule.EffectiveFrom.After(at) || rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo) {
			continue
		}
		specificity := 0
		if rule.ResourceID != nil {
			if *rule.ResourceID != input.ResourceID {
				continue
			}
			specificity += 2
		}
		if rule.UpstreamID != nil {
			if *rule.UpstreamID != input.UpstreamID {
				continue
			}
			specificity++
		}
		candidates = append(candidates, candidate{rule: rule, specificity: specificity})
	}
	if len(candidates) == 0 {
		return CostResult{State: CostUnknown}, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].specificity != candidates[j].specificity {
			return candidates[i].specificity > candidates[j].specificity
		}
		return candidates[i].rule.EffectiveFrom.After(candidates[j].rule.EffectiveFrom)
	})
	rule := candidates[0].rule
	unitSize, ok := decimalRat(rule.UnitSize)
	if !ok || unitSize.Sign() <= 0 {
		return CostResult{}, errors.New("invalid persisted pricing unit size")
	}
	unitPrice, ok := decimalRat(rule.UnitPriceDecimal)
	if !ok || unitPrice.Sign() < 0 {
		return CostResult{}, errors.New("invalid persisted pricing unit price")
	}
	amount := new(big.Rat).Mul(new(big.Rat).Quo(quantity, unitSize), unitPrice)
	decimal, err := exactDecimal(amount)
	if err != nil {
		return CostResult{}, err
	}
	state := CostKnown
	if input.Completeness == CompletenessPartial {
		state = CostPartial
	}
	return CostResult{State: state, AmountDecimal: decimal, Currency: rule.Currency, PricingRuleID: rule.ID}, nil
}

func validCompleteness(value Completeness) bool {
	return value == CompletenessUnknown || value == CompletenessPartial || value == CompletenessComplete
}

func runtimeResourceID(value string) bool {
	kind, err := platformid.KindOf(value)
	if err != nil {
		return false
	}
	return kind == platformid.Model || kind == platformid.TTS || kind == platformid.ASR || kind == platformid.MCP
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	r := new(big.Rat)
	_, ok := r.SetString(value)
	return r, ok
}

func exactDecimal(value *big.Rat) (string, error) {
	if value == nil {
		return "", errors.New("nil decimal")
	}
	denominator := new(big.Int).Set(value.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	zero := big.NewInt(0)
	one := big.NewInt(1)
	countFactor := func(factor *big.Int) int {
		count := 0
		quotient, remainder := new(big.Int), new(big.Int)
		for {
			quotient.QuoRem(denominator, factor, remainder)
			if remainder.Cmp(zero) != 0 {
				return count
			}
			denominator.Set(quotient)
			count++
		}
	}
	count2 := countFactor(two)
	count5 := countFactor(five)
	if denominator.Cmp(one) != 0 {
		return "", fmt.Errorf("cost is not a finite decimal")
	}
	scale := count2
	if count5 > scale {
		scale = count5
	}
	numerator := new(big.Int).Set(value.Num())
	if scale > count2 {
		numerator.Mul(numerator, new(big.Int).Exp(two, big.NewInt(int64(scale-count2)), nil))
	}
	if scale > count5 {
		numerator.Mul(numerator, new(big.Int).Exp(five, big.NewInt(int64(scale-count5)), nil))
	}
	negative := numerator.Sign() < 0
	if negative {
		numerator.Abs(numerator)
	}
	digits := numerator.String()
	if scale == 0 {
		if negative {
			return "-" + digits, nil
		}
		return digits, nil
	}
	for len(digits) <= scale {
		digits = "0" + digits
	}
	point := len(digits) - scale
	result := digits[:point] + "." + digits[point:]
	result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	if result == "" {
		result = "0"
	}
	if negative && result != "0" {
		result = "-" + result
	}
	return result, nil
}
