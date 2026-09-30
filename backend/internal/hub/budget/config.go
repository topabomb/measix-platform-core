package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"measix/platform/ent"
	"measix/platform/ent/budgetaudit"
	"measix/platform/ent/budgetlimit"
	"measix/platform/ent/userbudget"
	"measix/platform/pkg/platformid"
)

type configuredScope struct {
	key       string
	period    Period
	startedAt time.Time
	limits    map[Meter]int64
}

func (s *Service) Get(ctx context.Context, userID string, capability Capability) (BudgetView, error) {
	if err := validateSubject(userID, capability); err != nil {
		return BudgetView{}, err
	}
	row, err := s.Client.UserBudget.Query().
		Where(userbudget.UserIDEQ(userID), userbudget.CapabilityEQ(userbudget.Capability(capability))).
		Only(ctx)
	if ent.IsNotFound(err) {
		return defaultView(userID, capability), nil
	}
	if err != nil {
		return BudgetView{}, err
	}
	limits, err := s.Client.BudgetLimit.Query().
		Where(budgetlimit.UserBudgetIDEQ(row.ID), budgetlimit.EffectiveToIsNil()).
		All(ctx)
	if err != nil {
		return BudgetView{}, err
	}
	return viewFromRows(row, limits), nil
}

func (s *Service) Put(ctx context.Context, input PutBudgetInput) (BudgetView, error) {
	if err := validatePut(input); err != nil {
		return BudgetView{}, err
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return BudgetView{}, err
	}
	defer tx.Rollback()
	after, err := s.applyBudgetInTx(ctx, tx, budgetMutation{
		UserID: input.UserID, Capability: input.Capability, ExpectedRevision: input.ExpectedRevision, CheckRevision: true,
		Mode: input.Mode, Scopes: input.Scopes, Source: SourceExplicit,
		ActorUserID: input.ActorUserID, Reason: input.Reason, Action: "UPDATE",
	}, now)
	if err != nil {
		return BudgetView{}, err
	}
	if err := tx.Commit(); err != nil {
		return BudgetView{}, err
	}
	return after, nil
}

type budgetMutation struct {
	UserID           string
	Capability       Capability
	ExpectedRevision int64
	CheckRevision    bool
	Mode             Mode
	Scopes           []ScopeInput
	Source           Source
	ActorUserID      string
	Reason           string
	Action           string
}

func (s *Service) applyBudgetInTx(ctx context.Context, tx *ent.Tx, mutation budgetMutation, now time.Time) (BudgetView, error) {
	input := PutBudgetInput{
		UserID: mutation.UserID, Capability: mutation.Capability, ExpectedRevision: mutation.ExpectedRevision,
		Mode: mutation.Mode, Scopes: mutation.Scopes, ActorUserID: mutation.ActorUserID, Reason: mutation.Reason,
	}

	row, err := tx.UserBudget.Query().
		Where(userbudget.UserIDEQ(mutation.UserID), userbudget.CapabilityEQ(userbudget.Capability(mutation.Capability))).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return BudgetView{}, err
	}
	if ent.IsNotFound(err) {
		row = nil
	}
	currentRevision := int64(0)
	if row != nil {
		currentRevision = row.Revision
	}
	if mutation.CheckRevision && currentRevision != mutation.ExpectedRevision {
		return BudgetView{}, ErrRevisionConflict
	}

	var currentLimits []*ent.BudgetLimit
	var allLimits []*ent.BudgetLimit
	if row != nil {
		allLimits, err = tx.BudgetLimit.Query().Where(budgetlimit.UserBudgetIDEQ(row.ID)).All(ctx)
		if err != nil {
			return BudgetView{}, err
		}
		for _, limit := range allLimits {
			if limit.EffectiveTo == nil {
				currentLimits = append(currentLimits, limit)
				// SQLite can return the same wall-clock tick for consecutive
				// committed commands. Preserve immutable limit history with a
				// per-budget logical timestamp instead of producing a zero-width
				// version or mutating the prior row in place.
				if !limit.EffectiveFrom.Before(now) {
					now = limit.EffectiveFrom.Add(time.Nanosecond)
				}
			}
		}
	}
	before := defaultView(mutation.UserID, mutation.Capability)
	if row != nil {
		before = viewFromRows(row, currentLimits)
	}

	desired, replaceLimits, err := normalizeScopes(input, currentLimits, allLimits, now)
	if err != nil {
		return BudgetView{}, err
	}
	nextRevision := currentRevision + 1
	if row == nil {
		row, err = tx.UserBudget.Create().
			SetUserID(mutation.UserID).
			SetCapability(userbudget.Capability(mutation.Capability)).
			SetMode(userbudget.Mode(mutation.Mode)).
			SetSource(userbudget.Source(mutation.Source)).
			SetRevision(nextRevision).
			SetActivatedAt(now).
			SetUpdatedAt(now).
			SetUpdatedByUserID(mutation.ActorUserID).
			Save(ctx)
	} else {
		row, err = tx.UserBudget.UpdateOneID(row.ID).
			SetMode(userbudget.Mode(mutation.Mode)).
			SetSource(userbudget.Source(mutation.Source)).
			SetRevision(nextRevision).
			SetActivatedAt(now).
			SetUpdatedAt(now).
			SetUpdatedByUserID(mutation.ActorUserID).
			Save(ctx)
	}
	if err != nil {
		return BudgetView{}, err
	}

	if replaceLimits {
		if err := replaceActiveLimits(ctx, tx, row.ID, currentLimits, desired, mutation.ActorUserID, now); err != nil {
			return BudgetView{}, err
		}
	}
	updatedLimits, err := tx.BudgetLimit.Query().
		Where(budgetlimit.UserBudgetIDEQ(row.ID), budgetlimit.EffectiveToIsNil()).
		All(ctx)
	if err != nil {
		return BudgetView{}, err
	}
	after := viewFromRows(row, updatedLimits)
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return BudgetView{}, err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return BudgetView{}, err
	}
	action := mutation.Action
	if action == "" {
		action = "UPDATE"
	}
	if currentRevision == 0 && action == "UPDATE" {
		action = "CREATE"
	}
	if _, err := tx.BudgetAudit.Create().
		SetUserID(mutation.UserID).
		SetCapability(budgetaudit.Capability(mutation.Capability)).
		SetUserBudgetID(row.ID).
		SetBudgetRevision(nextRevision).
		SetActorUserID(mutation.ActorUserID).
		SetAction(budgetaudit.Action(action)).
		SetReason(strings.TrimSpace(mutation.Reason)).
		SetBeforeJSON(beforeJSON).
		SetAfterJSON(afterJSON).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return BudgetView{}, err
	}
	return after, nil
}

func validatePut(input PutBudgetInput) error {
	if err := validateSubject(input.UserID, input.Capability); err != nil {
		return err
	}
	if platformid.Validate(platformid.User, input.ActorUserID) != nil || strings.TrimSpace(input.Reason) == "" || input.ExpectedRevision < 0 {
		return ErrInvalidConfiguration
	}
	if input.Mode != ModeLimited && input.Mode != ModeUnlimited {
		return ErrInvalidConfiguration
	}
	if input.Mode == ModeUnlimited && len(input.Scopes) != 0 {
		return fmt.Errorf("%w: unlimited updates preserve existing scopes and must not replace them", ErrInvalidConfiguration)
	}
	return nil
}

func validateSubject(userID string, capability Capability) error {
	if platformid.Validate(platformid.User, userID) != nil || !validCapability(capability) {
		return ErrInvalidConfiguration
	}
	return nil
}

func normalizeScopes(input PutBudgetInput, rows, historicalRows []*ent.BudgetLimit, now time.Time) ([]configuredScope, bool, error) {
	current := groupLimitRows(rows)
	historical := latestScopesByPeriod(historicalRows)
	if input.Mode == ModeUnlimited {
		return nil, false, nil
	}
	if len(input.Scopes) == 0 {
		if len(current) == 0 {
			return nil, false, fmt.Errorf("%w: limited mode requires at least one limit", ErrInvalidConfiguration)
		}
		scopes := make([]configuredScope, 0, len(current))
		for _, scope := range current {
			scopes = append(scopes, scope)
		}
		return scopes, false, nil
	}

	desired := make([]configuredScope, 0, len(input.Scopes))
	seenScope := make(map[string]struct{}, len(input.Scopes))
	seenPeriodMeter := make(map[string]struct{})
	for _, candidate := range input.Scopes {
		if !validPeriod(candidate.Period) || len(candidate.Limits) == 0 {
			return nil, false, ErrInvalidConfiguration
		}
		key := strings.TrimSpace(candidate.ScopeKey)
		startedAt := now
		if key == "" {
			if prior, ok := historical[candidate.Period]; ok {
				key, startedAt = prior.key, prior.startedAt
			} else {
				key = newScopeKey()
			}
		} else {
			existing, ok := current[key]
			if !ok {
				return nil, false, fmt.Errorf("%w: unknown scope key", ErrInvalidConfiguration)
			}
			if existing.period == candidate.Period {
				startedAt = existing.startedAt
			} else {
				// A period change is a new accounting scope. The old key remains
				// attached to historical buckets and in-flight allocations.
				key = newScopeKey()
			}
		}
		if _, duplicate := seenScope[key]; duplicate {
			return nil, false, ErrInvalidConfiguration
		}
		seenScope[key] = struct{}{}
		limits := make(map[Meter]int64, len(candidate.Limits))
		for _, limit := range candidate.Limits {
			if !validMeter(limit.Meter) || !meterAllowed(input.Capability, limit.Meter) || limit.Limit < 0 {
				return nil, false, ErrInvalidConfiguration
			}
			if _, duplicate := limits[limit.Meter]; duplicate {
				return nil, false, ErrInvalidConfiguration
			}
			periodMeter := string(candidate.Period) + "\x00" + string(limit.Meter)
			if _, duplicate := seenPeriodMeter[periodMeter]; duplicate {
				return nil, false, fmt.Errorf("%w: duplicate period and meter", ErrInvalidConfiguration)
			}
			seenPeriodMeter[periodMeter] = struct{}{}
			limits[limit.Meter] = limit.Limit
		}
		desired = append(desired, configuredScope{key: key, period: candidate.Period, startedAt: startedAt, limits: limits})
	}
	return desired, true, nil
}

func latestScopesByPeriod(rows []*ent.BudgetLimit) map[Period]configuredScope {
	result := make(map[Period]configuredScope)
	latest := make(map[Period]time.Time)
	for _, row := range rows {
		period := Period(row.Period)
		if stamp, ok := latest[period]; ok && !row.EffectiveFrom.After(stamp) {
			continue
		}
		latest[period] = row.EffectiveFrom
		result[period] = configuredScope{key: row.ScopeKey, period: period, startedAt: row.ScopeStartedAt}
	}
	return result
}

func meterAllowed(capability Capability, meter Meter) bool {
	switch capability {
	case CapabilityModel:
		return meter == MeterRequests || meter == MeterInputTokens || meter == MeterOutputTokens || meter == MeterCachedTokens || meter == MeterTotalTokens
	case CapabilityTTS:
		return meter == MeterRequests || meter == MeterCharacters || meter == MeterAudioMilliseconds
	case CapabilityASR:
		return meter == MeterRequests || meter == MeterAudioMilliseconds
	case CapabilityMCP:
		return meter == MeterRequests
	case CapabilityImageGeneration:
		return meter == MeterRequests || meter == MeterRequestedImages
	default:
		return false
	}
}

func replaceActiveLimits(ctx context.Context, tx *ent.Tx, budgetID int, currentRows []*ent.BudgetLimit, desired []configuredScope, actor string, now time.Time) error {
	desiredLimits := make(map[string]struct {
		scope configuredScope
		meter Meter
		limit int64
	})
	for _, scope := range desired {
		for meter, limit := range scope.limits {
			desiredLimits[scope.key+"\x00"+string(meter)] = struct {
				scope configuredScope
				meter Meter
				limit int64
			}{scope: scope, meter: meter, limit: limit}
		}
	}
	for _, current := range currentRows {
		key := current.ScopeKey + "\x00" + string(current.Meter)
		next, retained := desiredLimits[key]
		if retained && Period(current.Period) == next.scope.period && current.LimitQuantity == next.limit {
			delete(desiredLimits, key)
			continue
		}
		if _, err := tx.BudgetLimit.UpdateOneID(current.ID).SetEffectiveTo(now).Save(ctx); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(desiredLimits))
	for key := range desiredLimits {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		next := desiredLimits[key]
		if _, err := tx.BudgetLimit.Create().
			SetUserBudgetID(budgetID).
			SetScopeKey(next.scope.key).
			SetPeriod(budgetlimit.Period(next.scope.period)).
			SetMeter(budgetlimit.Meter(next.meter)).
			SetLimitQuantity(next.limit).
			SetScopeStartedAt(next.scope.startedAt).
			SetEffectiveFrom(now).
			SetCreatedAt(now).
			SetCreatedByUserID(actor).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func groupLimitRows(rows []*ent.BudgetLimit) map[string]configuredScope {
	grouped := make(map[string]configuredScope)
	for _, row := range rows {
		scope := grouped[row.ScopeKey]
		if scope.key == "" {
			scope = configuredScope{key: row.ScopeKey, period: Period(row.Period), startedAt: row.ScopeStartedAt, limits: make(map[Meter]int64)}
		}
		scope.limits[Meter(row.Meter)] = row.LimitQuantity
		grouped[row.ScopeKey] = scope
	}
	return grouped
}

func viewFromRows(row *ent.UserBudget, rows []*ent.BudgetLimit) BudgetView {
	activatedAt := row.ActivatedAt
	view := BudgetView{
		UserID: row.UserID, Capability: Capability(row.Capability), Mode: Mode(row.Mode),
		Source: Source(row.Source), Revision: row.Revision, ActivatedAt: &activatedAt,
	}
	grouped := groupLimitRows(rows)
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		scope := grouped[key]
		meters := make([]string, 0, len(scope.limits))
		for meter := range scope.limits {
			meters = append(meters, string(meter))
		}
		sort.Strings(meters)
		scopeView := ScopeView{ScopeKey: key, Period: scope.period, EffectiveFrom: scope.startedAt}
		for _, meter := range meters {
			scopeView.Limits = append(scopeView.Limits, LimitSpec{Meter: Meter(meter), Limit: scope.limits[Meter(meter)]})
		}
		view.Scopes = append(view.Scopes, scopeView)
	}
	return view
}

func defaultView(userID string, capability Capability) BudgetView {
	return BudgetView{UserID: userID, Capability: capability, Mode: ModeUnlimited, Source: SourceDefault}
}

func newScopeKey() string { return "bgs_" + uuid.NewString() }

func validCapability(value Capability) bool {
	switch value {
	case CapabilityModel, CapabilityImageGeneration, CapabilityTTS, CapabilityASR, CapabilityMCP:
		return true
	default:
		return false
	}
}

func validPeriod(value Period) bool {
	switch value {
	case PeriodDay, PeriodWeek, PeriodMonth, PeriodLifetime:
		return true
	default:
		return false
	}
}

func validMeter(value Meter) bool {
	switch value {
	case MeterRequests, MeterRequestedImages, MeterInputTokens, MeterOutputTokens, MeterCachedTokens, MeterTotalTokens, MeterCharacters, MeterAudioMilliseconds:
		return true
	default:
		return false
	}
}
