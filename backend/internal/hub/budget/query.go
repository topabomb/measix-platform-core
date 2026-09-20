package budget

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/budgetallocation"
	"measix/platform/ent/budgetaudit"
	"measix/platform/ent/budgetbucket"
	"measix/platform/ent/budgetlimit"
	"measix/platform/ent/budgetreconciliation"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/predicate"
	"measix/platform/ent/userbudget"
	"measix/platform/pkg/platformid"
)

// State returns enforcement state from budget buckets, never by scanning usage
// history. The snapshot time is owned by Hub and is returned to every consumer.
func (s *Service) State(ctx context.Context, userID string, capability Capability) (EffectiveState, error) {
	if err := validateSubject(userID, capability); err != nil {
		return EffectiveState{}, err
	}
	states, err := s.UserStatesBatch(ctx, []string{userID})
	if err != nil {
		return EffectiveState{}, err
	}
	for _, state := range states[userID] {
		if state.Budget.Capability == capability {
			return state, nil
		}
	}
	return EffectiveState{}, fmt.Errorf("missing %s budget projection for %s", capability, userID)
}

// UserStates returns all four capability states with one asOf and one database
// snapshot so Admin, Client and Portal cannot observe mixed revisions.
func (s *Service) UserStates(ctx context.Context, userID string) ([]EffectiveState, error) {
	if platformid.Validate(platformid.User, userID) != nil {
		return nil, ErrInvalidConfiguration
	}
	states, err := s.UserStatesBatch(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	return states[userID], nil
}

// UserStatesBatch projects current states for a bounded Admin page in one
// database snapshot. It is the same owner calculation used by State and
// UserStates; callers never reconstruct limits from usage history.
func (s *Service) UserStatesBatch(ctx context.Context, userIDs []string) (map[string][]EffectiveState, error) {
	if len(userIDs) == 0 || len(userIDs) > 100 {
		return nil, ErrInvalidConfiguration
	}
	unique := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if platformid.Validate(platformid.User, userID) != nil {
			return nil, ErrInvalidConfiguration
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		unique = append(unique, userID)
	}
	asOf := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	budgetRows, err := tx.UserBudget.Query().Where(userbudget.UserIDIn(unique...)).All(ctx)
	if err != nil {
		return nil, err
	}
	budgetsBySubject := make(map[string]*ent.UserBudget, len(budgetRows))
	budgetIDs := make([]int, 0, len(budgetRows))
	for _, row := range budgetRows {
		budgetsBySubject[subjectKey(row.UserID, Capability(row.Capability))] = row
		budgetIDs = append(budgetIDs, row.ID)
	}
	limitsByBudget := make(map[int][]*ent.BudgetLimit, len(budgetRows))
	var limits []*ent.BudgetLimit
	if len(budgetIDs) > 0 {
		limits, err = tx.BudgetLimit.Query().Where(
			budgetlimit.UserBudgetIDIn(budgetIDs...),
			budgetlimit.EffectiveFromLTE(asOf),
			budgetlimit.Or(budgetlimit.EffectiveToIsNil(), budgetlimit.EffectiveToGT(asOf)),
		).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, limit := range limits {
			limitsByBudget[limit.UserBudgetID] = append(limitsByBudget[limit.UserBudgetID], limit)
		}
	}

	type limitWindow struct {
		limit  *ent.BudgetLimit
		window periodWindow
	}
	windows := make([]limitWindow, 0, len(limits))
	bucketPredicates := make([]predicate.BudgetBucket, 0, len(limits))
	for _, limit := range limits {
		window, err := windowFor(Period(limit.Period), asOf, limit.ScopeStartedAt, s.Location)
		if err != nil {
			return nil, err
		}
		windows = append(windows, limitWindow{limit: limit, window: window})
		bucketPredicates = append(bucketPredicates, budgetbucket.And(
			budgetbucket.ScopeKeyEQ(limit.ScopeKey),
			budgetbucket.PeriodStartEQ(window.Start),
			budgetbucket.MeterEQ(budgetbucket.Meter(limit.Meter)),
		))
	}
	bucketsByLimit := make(map[string]*ent.BudgetBucket, len(bucketPredicates))
	// Each exact bucket predicate contributes three bind parameters. Chunk the
	// page-wide lookup below SQLite's default variable limit while keeping the
	// query bounded to the current accounting windows.
	for offset := 0; offset < len(bucketPredicates); offset += 200 {
		end := min(offset+200, len(bucketPredicates))
		buckets, err := tx.BudgetBucket.Query().Where(budgetbucket.Or(bucketPredicates[offset:end]...)).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, bucket := range buckets {
			bucketsByLimit[bucketKey(bucket.ScopeKey, bucket.PeriodStart, Meter(bucket.Meter))] = bucket
		}
	}

	inFlight := make(map[string]int64)
	reconciliations := make(map[string]bool)
	requests, err := tx.BudgetRequest.Query().Where(
		budgetrequest.UserIDIn(unique...),
		budgetrequest.StateIn(budgetrequest.State(RequestAdmitted), budgetrequest.State(RequestStarted), budgetrequest.State(RequestReconciliation)),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		key := subjectKey(request.UserID, Capability(request.Capability))
		inFlight[key]++
		if RequestState(request.State) == RequestReconciliation {
			reconciliations[key] = true
		}
	}

	windowByLimit := make(map[int]periodWindow, len(windows))
	for _, value := range windows {
		windowByLimit[value.limit.ID] = value.window
	}
	capabilities := []Capability{CapabilityModel, CapabilityTTS, CapabilityASR, CapabilityMCP}
	result := make(map[string][]EffectiveState, len(unique))
	for _, userID := range unique {
		states := make([]EffectiveState, 0, len(capabilities))
		for _, capability := range capabilities {
			key := subjectKey(userID, capability)
			row := budgetsBySubject[key]
			var currentLimits []*ent.BudgetLimit
			view := defaultView(userID, capability)
			if row != nil {
				currentLimits = limitsByBudget[row.ID]
				view = viewFromRows(row, currentLimits)
			}
			sort.Slice(currentLimits, func(i, j int) bool {
				if currentLimits[i].Period != currentLimits[j].Period {
					return currentLimits[i].Period < currentLimits[j].Period
				}
				return currentLimits[i].Meter < currentLimits[j].Meter
			})
			state := EffectiveState{
				Budget: view, Status: StatusUnlimited, AsOf: asOf,
				InFlightRequests: inFlight[key], HasReconciliation: reconciliations[key],
			}
			for _, limit := range currentLimits {
				window := windowByLimit[limit.ID]
				bucket := bucketsByLimit[bucketKey(limit.ScopeKey, window.Start, Meter(limit.Meter))]
				used, reserved := int64(0), int64(0)
				if bucket != nil {
					used, reserved = bucket.SettledQuantity, bucket.ReservedQuantity
				}
				remaining := max(limit.LimitQuantity-used-reserved, 0)
				overage := max(used-limit.LimitQuantity, 0)
				limitState := EffectiveLimitState{
					ScopeKey: limit.ScopeKey, Meter: Meter(limit.Meter), Period: Period(limit.Period),
					Limit: limit.LimitQuantity, Used: used, Reserved: reserved,
					Remaining: remaining, Overage: overage, ResetAt: window.End,
				}
				state.Limits = append(state.Limits, limitState)
				if view.Mode == ModeLimited && used+reserved >= limit.LimitQuantity {
					state.BlockingLimits = append(state.BlockingLimits, limitState)
				}
			}
			switch {
			case state.HasReconciliation:
				state.Status = StatusReconciliation
			case view.Mode == ModeUnlimited:
				state.Status = StatusUnlimited
			case len(state.BlockingLimits) > 0:
				state.Status = StatusExhausted
			default:
				state.Status = StatusAvailable
			}
			state.ResetAt = effectiveOverallResetAt(state.BlockingLimits)
			states = append(states, state)
		}
		result[userID] = states
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func subjectKey(userID string, capability Capability) string {
	return userID + "\x00" + string(capability)
}

func bucketKey(scopeKey string, periodStart time.Time, meter Meter) string {
	return scopeKey + "\x00" + periodStart.UTC().Format(time.RFC3339Nano) + "\x00" + string(meter)
}

func (s *Service) ListConfigAudit(ctx context.Context, query AuditQuery) (AuditPage, error) {
	if platformid.Validate(platformid.User, query.UserID) != nil || (query.Capability != nil && !validCapability(*query.Capability)) {
		return AuditPage{}, ErrInvalidConfiguration
	}
	pageSize, err := normalizedPageSize(query.PageSize)
	if err != nil {
		return AuditPage{}, err
	}
	cursor, err := decodePageCursor(query.Cursor)
	if err != nil {
		return AuditPage{}, err
	}
	builder := s.Client.BudgetAudit.Query().Where(
		budgetaudit.UserIDEQ(query.UserID),
		budgetaudit.ActionIn(budgetaudit.ActionCREATE, budgetaudit.ActionUPDATE),
	)
	if query.Capability != nil {
		builder.Where(budgetaudit.CapabilityEQ(budgetaudit.Capability(*query.Capability)))
	}
	if cursor != nil {
		builder.Where(budgetaudit.Or(
			budgetaudit.CreatedAtLT(cursor.Time),
			budgetaudit.And(budgetaudit.CreatedAtEQ(cursor.Time), budgetaudit.IDLT(cursor.ID)),
		))
	}
	rows, err := builder.
		Order(ent.Desc(budgetaudit.FieldCreatedAt), ent.Desc(budgetaudit.FieldID)).
		Limit(pageSize + 1).
		All(ctx)
	if err != nil {
		return AuditPage{}, err
	}
	page := AuditPage{}
	for _, row := range rows[:min(len(rows), pageSize)] {
		var before json.RawMessage
		if row.BeforeJSON != nil {
			before = append(before, (*row.BeforeJSON)...)
		}
		page.Items = append(page.Items, AuditRecord{
			ID: row.ID, UserID: row.UserID, Capability: Capability(row.Capability), Revision: row.BudgetRevision,
			ActorUserID: row.ActorUserID, Action: string(row.Action), Reason: row.Reason,
			Before: before, After: append(json.RawMessage(nil), row.AfterJSON...), CreatedAt: row.CreatedAt,
		})
	}
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		page.NextCursor, err = encodePageCursor(pageCursor{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return AuditPage{}, err
		}
	}
	return page, nil
}

func (s *Service) ListOpenReconciliations(ctx context.Context, query ReconciliationQuery) (ReconciliationPage, error) {
	if query.UserID != nil && platformid.Validate(platformid.User, *query.UserID) != nil {
		return ReconciliationPage{}, ErrInvalidConfiguration
	}
	if query.Capability != nil && !validCapability(*query.Capability) {
		return ReconciliationPage{}, ErrInvalidConfiguration
	}
	pageSize, err := normalizedPageSize(query.PageSize)
	if err != nil {
		return ReconciliationPage{}, err
	}
	cursor, err := decodePageCursor(query.Cursor)
	if err != nil {
		return ReconciliationPage{}, err
	}
	builder := s.Client.BudgetReconciliation.Query().Where(budgetreconciliation.StateEQ(budgetreconciliation.StateOPEN))
	requestPredicates := make([]predicate.BudgetRequest, 0, 2)
	if query.UserID != nil || query.Capability != nil {
		if query.UserID != nil {
			requestPredicates = append(requestPredicates, budgetrequest.UserIDEQ(*query.UserID))
		}
		if query.Capability != nil {
			requestPredicates = append(requestPredicates, budgetrequest.CapabilityEQ(budgetrequest.Capability(*query.Capability)))
		}
		builder.Where(budgetreconciliation.HasRequestWith(requestPredicates...))
	}
	if cursor != nil {
		builder.Where(budgetreconciliation.Or(
			budgetreconciliation.OpenedAtLT(cursor.Time),
			budgetreconciliation.And(budgetreconciliation.OpenedAtEQ(cursor.Time), budgetreconciliation.IDLT(cursor.ID)),
		))
	}
	rows, err := builder.
		Order(ent.Desc(budgetreconciliation.FieldOpenedAt), ent.Desc(budgetreconciliation.FieldID)).
		Limit(pageSize + 1).
		All(ctx)
	if err != nil {
		return ReconciliationPage{}, err
	}
	visible := rows[:min(len(rows), pageSize)]
	requestIDs := make([]string, 0, len(visible))
	for _, row := range visible {
		requestIDs = append(requestIDs, row.RequestID)
	}
	requestsByID := make(map[string]*ent.BudgetRequest, len(requestIDs))
	allocationsByRequest := make(map[string][]ReconciliationAllocation, len(requestIDs))
	if len(requestIDs) > 0 {
		requests, err := s.Client.BudgetRequest.Query().Where(budgetrequest.IDIn(requestIDs...)).All(ctx)
		if err != nil {
			return ReconciliationPage{}, err
		}
		for _, request := range requests {
			requestsByID[request.ID] = request
		}
		allocations, err := s.Client.BudgetAllocation.Query().Where(budgetallocation.RequestIDIn(requestIDs...)).All(ctx)
		if err != nil {
			return ReconciliationPage{}, err
		}
		for _, allocation := range allocations {
			allocationsByRequest[allocation.RequestID] = append(allocationsByRequest[allocation.RequestID], ReconciliationAllocation{
				ScopeKey: allocation.ScopeKey, Period: Period(allocation.Period), Meter: Meter(allocation.Meter),
				Reserved: allocation.ReservedQuantity, Observed: allocation.SettledQuantity,
				ReservationReleased: allocation.ReservationReleased, Resolved: allocation.Resolved,
			})
		}
	}
	page := ReconciliationPage{}
	for _, row := range visible {
		request := requestsByID[row.RequestID]
		if request == nil {
			return ReconciliationPage{}, fmt.Errorf("reconciliation %d references missing request %s", row.ID, row.RequestID)
		}
		allocations := allocationsByRequest[row.RequestID]
		sort.Slice(allocations, func(i, j int) bool {
			if allocations[i].Period != allocations[j].Period {
				return allocations[i].Period < allocations[j].Period
			}
			return allocations[i].Meter < allocations[j].Meter
		})
		page.Items = append(page.Items, ReconciliationRecord{
			ID: row.ID, RequestID: row.RequestID, UserID: request.UserID, Capability: Capability(request.Capability),
			ResourceID: request.ResourceID, ClientProtocol: ClientProtocol(request.ClientProtocol), RequestState: RequestState(request.State),
			Reason: row.Reason, LastSettlementRevision: request.LastSettlementRevision, Allocations: allocations,
			AdmittedAt: request.AdmittedAt, StartedAt: request.StartedAt, OpenedAt: row.OpenedAt, UpdatedAt: request.UpdatedAt,
		})
	}
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		page.NextCursor, err = encodePageCursor(pageCursor{Time: last.OpenedAt, ID: last.ID})
		if err != nil {
			return ReconciliationPage{}, err
		}
	}
	return page, nil
}

func (s *Service) GetOpenReconciliation(ctx context.Context, requestID string) (ReconciliationRecord, error) {
	if platformid.Validate(platformid.Request, requestID) != nil {
		return ReconciliationRecord{}, ErrInvalidConfiguration
	}
	reconciliation, err := s.Client.BudgetReconciliation.Query().Where(
		budgetreconciliation.RequestIDEQ(requestID), budgetreconciliation.StateEQ(budgetreconciliation.StateOPEN),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return ReconciliationRecord{}, ErrReconciliationNotOpen
	}
	if err != nil {
		return ReconciliationRecord{}, err
	}
	request, err := s.Client.BudgetRequest.Get(ctx, requestID)
	if err != nil {
		return ReconciliationRecord{}, err
	}
	rows, err := s.Client.BudgetAllocation.Query().Where(budgetallocation.RequestIDEQ(requestID)).All(ctx)
	if err != nil {
		return ReconciliationRecord{}, err
	}
	allocations := make([]ReconciliationAllocation, 0, len(rows))
	for _, allocation := range rows {
		allocations = append(allocations, ReconciliationAllocation{
			ScopeKey: allocation.ScopeKey, Period: Period(allocation.Period), Meter: Meter(allocation.Meter),
			Reserved: allocation.ReservedQuantity, Observed: allocation.SettledQuantity,
			ReservationReleased: allocation.ReservationReleased, Resolved: allocation.Resolved,
		})
	}
	sort.Slice(allocations, func(i, j int) bool {
		if allocations[i].Period != allocations[j].Period {
			return allocations[i].Period < allocations[j].Period
		}
		return allocations[i].Meter < allocations[j].Meter
	})
	return ReconciliationRecord{
		ID: reconciliation.ID, RequestID: request.ID, UserID: request.UserID, Capability: Capability(request.Capability),
		ResourceID: request.ResourceID, ClientProtocol: ClientProtocol(request.ClientProtocol), RequestState: RequestState(request.State),
		Reason: reconciliation.Reason, LastSettlementRevision: request.LastSettlementRevision, Allocations: allocations,
		AdmittedAt: request.AdmittedAt, StartedAt: request.StartedAt, OpenedAt: reconciliation.OpenedAt, UpdatedAt: request.UpdatedAt,
	}, nil
}

type pageCursor struct {
	Time time.Time `json:"time"`
	ID   int       `json:"id"`
}

func normalizedPageSize(value int) (int, error) {
	if value == 0 {
		return 50, nil
	}
	if value < 1 || value > 100 {
		return 0, ErrInvalidConfiguration
	}
	return value, nil
}

func encodePageCursor(cursor pageCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodePageCursor(value string) (*pageCursor, error) {
	if value == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	var cursor pageCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.ID <= 0 || cursor.Time.IsZero() {
		return nil, ErrInvalidConfiguration
	}
	return &cursor, nil
}

func effectiveOverallResetAt(blockers []EffectiveLimitState) *time.Time {
	if len(blockers) == 0 {
		return nil
	}
	var latest time.Time
	for _, blocker := range blockers {
		if blocker.ResetAt == nil {
			return nil
		}
		if blocker.ResetAt.After(latest) {
			latest = *blocker.ResetAt
		}
	}
	return &latest
}
