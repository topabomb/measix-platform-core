package usage

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"measix/platform/pkg/platformid"
)

type PricingRuleRecord struct {
	ID            string
	ResourceID    *string
	UpstreamID    *string
	Meter         string
	UnitSize      string
	UnitPrice     string
	Currency      string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

func (s *Service) PricingSet(ctx context.Context) (int, []PricingRuleRecord, error) {
	rows, err := s.Client.PricingRule.Query().All(ctx)
	if err != nil {
		return 0, nil, err
	}
	rules := make([]PricingRuleRecord, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, PricingRuleRecord{
			ID: row.ID, ResourceID: row.ResourceID, UpstreamID: row.UpstreamID,
			Meter: row.Meter, UnitSize: row.UnitSize, UnitPrice: row.UnitPriceDecimal, Currency: row.Currency,
			EffectiveFrom: row.EffectiveFrom, EffectiveTo: row.EffectiveTo,
		})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return pricingRevision(rules), rules, nil
}

func (s *Service) ReplacePricingSet(ctx context.Context, expectedRevision int, rules []PricingRuleRecord) (int, []PricingRuleRecord, error) {
	currentRevision, _, err := s.PricingSet(ctx)
	if err != nil {
		return 0, nil, err
	}
	if currentRevision != expectedRevision {
		return 0, nil, ErrPricingRevisionConflict
	}
	seen := map[string]struct{}{}
	for _, rule := range rules {
		if err := validatePricingRecord(rule); err != nil {
			return 0, nil, err
		}
		if _, exists := seen[rule.ID]; exists {
			return 0, nil, ErrInvalidBatch
		}
		seen[rule.ID] = struct{}{}
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	if _, err := tx.PricingRule.Delete().Exec(ctx); err != nil {
		return 0, nil, err
	}
	for _, rule := range rules {
		builder := tx.PricingRule.Create().
			SetID(rule.ID).
			SetNillableResourceID(rule.ResourceID).
			SetNillableUpstreamID(rule.UpstreamID).
			SetMeter(strings.TrimSpace(rule.Meter)).
			SetUnitSize(rule.UnitSize).
			SetUnitPriceDecimal(rule.UnitPrice).
			SetCurrency(strings.TrimSpace(rule.Currency)).
			SetEffectiveFrom(rule.EffectiveFrom.UTC()).
			SetNillableEffectiveTo(rule.EffectiveTo)
		if _, err := builder.Save(ctx); err != nil {
			return 0, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return s.PricingSet(ctx)
}

func validatePricingRecord(rule PricingRuleRecord) error {
	if platformid.Validate(platformid.PricingRule, rule.ID) != nil || strings.TrimSpace(rule.Meter) == "" || strings.TrimSpace(rule.Currency) == "" || rule.EffectiveFrom.IsZero() {
		return ErrInvalidBatch
	}
	if rule.ResourceID != nil && !runtimeResourceID(*rule.ResourceID) {
		return ErrInvalidBatch
	}
	if rule.UpstreamID != nil && platformid.Validate(platformid.Upstream, *rule.UpstreamID) != nil {
		return ErrInvalidBatch
	}
	unitSize, ok := decimalRat(rule.UnitSize)
	if !ok || unitSize.Sign() <= 0 {
		return ErrInvalidBatch
	}
	unitPrice, ok := decimalRat(rule.UnitPrice)
	if !ok || unitPrice.Sign() < 0 {
		return ErrInvalidBatch
	}
	if rule.EffectiveTo != nil && !rule.EffectiveTo.After(rule.EffectiveFrom) {
		return ErrInvalidBatch
	}
	return nil
}

func pricingRevision(rules []PricingRuleRecord) int {
	if len(rules) == 0 {
		return 0
	}
	ordered := append([]PricingRuleRecord(nil), rules...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	payload, _ := json.Marshal(ordered)
	sum := sha256.Sum256(payload)
	value := binary.BigEndian.Uint32(sum[:4]) & 0x7fffffff
	if value == 0 {
		value = 1
	}
	return int(value)
}
