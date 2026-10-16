package usage

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"measix/platform/ent/pricingrule"
	"measix/platform/pkg/platformid"
)

type Completeness string

const (
	CompletenessUnknown  Completeness = "UNKNOWN"
	CompletenessPartial  Completeness = "PARTIAL"
	CompletenessComplete Completeness = "EXACT"
)

type CostState string

const (
	CostUnknown CostState = "UNKNOWN"
	CostPartial CostState = "PARTIAL"
	CostKnown   CostState = "KNOWN"
)

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

func (s *Service) CreatePricingRule(ctx context.Context, input PricingRuleInput) (string, error) {
	if s.Client == nil || !validMeter(input.Meter) || strings.TrimSpace(input.Currency) == "" || input.EffectiveFrom.IsZero() {
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
	if s.Client == nil || platformid.Validate(platformid.Upstream, input.UpstreamID) != nil || !runtimeResourceID(input.ResourceID) || !validMeter(input.Meter) || input.OccurredAt.IsZero() || !validCompleteness(input.Completeness) {
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
	rule := selectPricingRule(rules, strings.TrimSpace(input.Meter), input.ResourceID, input.UpstreamID, input.OccurredAt.UTC())
	if rule == nil {
		return CostResult{State: CostUnknown}, nil
	}
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

// validMeter returns true if the meter string is one of the standard meters
// defined in architecture s0-control-protocol §13.
// Standard meters for S0.1 required profile:
//
//	INPUT_TOKENS, OUTPUT_TOKENS, CACHED_TOKENS, CHARACTERS, AUDIO_SECONDS, REQUESTED_IMAGES, REQUESTS
func validMeter(value string) bool {
	switch strings.TrimSpace(value) {
	case "INPUT_TOKENS", "OUTPUT_TOKENS", "CACHED_TOKENS", "TOTAL_TOKENS", "CHARACTERS", "AUDIO_SECONDS", "REQUESTED_IMAGES", "REQUESTS":
		return true
	}
	return false
}

func runtimeResourceID(value string) bool {
	kind, err := platformid.KindOf(value)
	if err != nil {
		return false
	}
	return kind == platformid.Model || kind == platformid.ImageGeneration || kind == platformid.TTS || kind == platformid.ASR || kind == platformid.MCP
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
		return strings.TrimRight(strings.TrimRight(value.FloatString(12), "0"), "."), nil
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
