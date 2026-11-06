package httpapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) GetUserBudgets(w http.ResponseWriter, r *http.Request, userID adminapi.UserId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	if _, err := h.services.Identity.GetUserView(r.Context(), userID); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "user_not_found", "User not found")
		} else {
			writeProblem(w, 500, "internal_error", "Internal error")
		}
		return
	}
	states, err := h.services.Budget.UserStates(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	usageMeters, err := h.services.Usage.UsageMetersByCapability(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	assignment, err := h.services.Budget.GetAssignment(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, adminBudgetView(userID, h.services.Budget.Location.String(), states, usageMeters, assignment))
}

func (h *fullAdminHandler) ClearUserBudgetOverride(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, capability adminapi.BudgetCapability, params adminapi.ClearUserBudgetOverrideParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	view, err := h.services.Budget.ClearOverride(r.Context(), budget.ClearOverrideInput{
		UserID: userID, Capability: budget.Capability(capability), ExpectedRevision: int64(params.ExpectedRevision),
		ActorUserID: principal.UserID, Reason: params.Reason,
	})
	if errors.Is(err, budget.ErrRevisionConflict) {
		writeProblem(w, 409, "budget_revision_conflict", "Budget was changed or has no explicit override")
		return
	}
	if errors.Is(err, budget.ErrInvalidConfiguration) {
		writeProblem(w, 400, "invalid_budget", "Invalid budget configuration")
		return
	}
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	state, err := h.services.Budget.State(r.Context(), userID, budget.Capability(capability))
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	usageMeters, err := h.services.Usage.UsageMetersByCapability(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	state.Budget = view
	writeJSON(w, 200, adminBudgetCapabilityView(state, usageMeters[state.Budget.Capability]))
}

func (h *fullAdminHandler) PutUserBudget(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, capability adminapi.BudgetCapability, params adminapi.PutUserBudgetParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	if _, err := h.services.Identity.GetUserView(r.Context(), userID); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			writeProblem(w, 404, "user_not_found", "User not found")
		} else {
			writeProblem(w, 500, "internal_error", "Internal error")
		}
		return
	}
	var request adminapi.PutBudgetRequest
	if decodeStrictJSON(r, &request) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	current, err := h.services.Budget.Get(r.Context(), userID, budget.Capability(capability))
	if err != nil {
		writeProblem(w, 422, "invalid_budget", "Invalid budget configuration")
		return
	}
	scopes, err := budgetScopes(current, request)
	if err != nil {
		writeProblem(w, 422, "invalid_budget", "Invalid budget configuration")
		return
	}
	if request.Mode == adminapi.UNLIMITED {
		scopes = nil
	}
	_, err = h.services.Budget.Put(r.Context(), budget.PutBudgetInput{
		UserID: userID, Capability: budget.Capability(capability), ExpectedRevision: int64(request.ExpectedRevision),
		Mode: budget.Mode(request.Mode), Scopes: scopes, ActorUserID: principal.UserID, Reason: request.Reason,
	})
	if errors.Is(err, budget.ErrRevisionConflict) {
		writeProblem(w, 409, "budget_revision_conflict", "Budget was changed by another administrator")
		return
	}
	if errors.Is(err, budget.ErrInvalidConfiguration) {
		writeProblem(w, 422, "invalid_budget", "Invalid budget configuration")
		return
	}
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	states, err := h.services.Budget.UserStates(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	usageMeters, err := h.services.Usage.UsageMetersByCapability(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	for _, state := range states {
		if state.Budget.Capability == budget.Capability(capability) {
			writeJSON(w, 200, adminBudgetCapabilityView(state, usageMeters[state.Budget.Capability]))
			return
		}
	}
	writeProblem(w, 500, "internal_error", "Updated budget state is unavailable")
}

func (h *fullAdminHandler) ListUserBudgetAudit(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, capability adminapi.BudgetCapability, params adminapi.ListUserBudgetAuditParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	domainCapability := budget.Capability(capability)
	page, err := h.services.Budget.ListConfigAudit(r.Context(), budget.AuditQuery{UserID: userID, Capability: &domainCapability, PageSize: limit, Cursor: cursor})
	if err != nil {
		writeProblem(w, 400, "invalid_cursor", "Invalid budget audit query")
		return
	}
	items := make([]adminapi.BudgetAuditItem, 0, len(page.Items))
	for _, record := range page.Items {
		var after budget.BudgetView
		if json.Unmarshal(record.After, &after) != nil {
			writeProblem(w, 500, "internal_error", "Invalid persisted budget audit")
			return
		}
		items = append(items, adminapi.BudgetAuditItem{
			AuditId: strconv.Itoa(record.ID), UserId: record.UserID, Capability: adminapi.BudgetCapability(record.Capability),
			PreviousRevision: int(record.Revision - 1), Revision: int(record.Revision), Mode: adminapi.BudgetMode(after.Mode),
			Limits: adminLimitDefinitions(after), ChangedBy: record.ActorUserID, Reason: record.Reason, CreatedAt: record.CreatedAt,
		})
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, 200, adminapi.BudgetAuditPage{Items: items, NextCursor: next})
}

func budgetScopes(current budget.BudgetView, request adminapi.PutBudgetRequest) ([]budget.ScopeInput, error) {
	existing := map[budget.Period]string{}
	for _, scope := range current.Scopes {
		existing[scope.Period] = scope.ScopeKey
	}
	grouped := map[budget.Period][]budget.LimitSpec{}
	for _, limit := range request.Limits {
		value, err := strconv.ParseInt(limit.Limit, 10, 64)
		if err != nil || value < 0 {
			return nil, budget.ErrInvalidConfiguration
		}
		meter := budget.Meter(limit.Meter)
		if limit.Meter == adminapi.AUDIOSECONDS {
			if value > math.MaxInt64/1000 {
				return nil, budget.ErrInvalidConfiguration
			}
			meter, value = budget.MeterAudioMilliseconds, value*1000
		}
		period := budget.Period(limit.Period)
		grouped[period] = append(grouped[period], budget.LimitSpec{Meter: meter, Limit: value})
	}
	periods := make([]string, 0, len(grouped))
	for period := range grouped {
		periods = append(periods, string(period))
	}
	sort.Strings(periods)
	result := make([]budget.ScopeInput, 0, len(periods))
	for _, value := range periods {
		period := budget.Period(value)
		result = append(result, budget.ScopeInput{ScopeKey: existing[period], Period: period, Limits: grouped[period]})
	}
	return result, nil
}

func adminBudgetView(userID, timezone string, states []budget.EffectiveState, usageMeters usage.CapabilityMeters, assignment *budget.AssignmentView) adminapi.UserBudgetView {
	items := make([]adminapi.BudgetCapabilityView, 0, len(states))
	var asOf time.Time
	for _, state := range states {
		asOf = state.AsOf
		items = append(items, adminBudgetCapabilityView(state, usageMeters[state.Budget.Capability]))
	}
	view := adminapi.UserBudgetView{UserId: userID, Timezone: timezone, AsOf: asOf, Items: items}
	if assignment != nil {
		value := adminapi.BudgetTemplateAssignment{
			BudgetTemplateId: assignment.TemplateID, Name: assignment.TemplateName,
			TemplateRevision: int(assignment.TemplateRevision), AssignmentRevision: int(assignment.Revision), AssignedAt: assignment.AssignedAt,
		}
		view.TemplateAssignment = &value
	}
	return view
}

func adminBudgetCapabilityView(state budget.EffectiveState, usageMeters []usage.MeterSummary) adminapi.BudgetCapabilityView {
	effectiveFrom := state.AsOf
	if state.Budget.ActivatedAt != nil {
		effectiveFrom = *state.Budget.ActivatedAt
	}
	status := adminapi.BudgetStatusAVAILABLE
	switch state.Status {
	case budget.StatusExhausted:
		status = adminapi.BudgetStatusEXHAUSTED
	case budget.StatusReconciliation:
		status = adminapi.BudgetStatusPENDINGRECONCILIATION
	}
	return adminapi.BudgetCapabilityView{
		Capability: adminapi.BudgetCapability(state.Budget.Capability), Mode: adminapi.BudgetMode(state.Budget.Mode),
		Source: adminapi.BudgetSource(state.Budget.Source), Revision: int(state.Budget.Revision), Status: status,
		EffectiveFrom: effectiveFrom, AsOf: state.AsOf, InFlightRequests: int(state.InFlightRequests), Limits: adminLimitStates(state),
		UsageMeters: adminMeterQuantities(usageMeters),
	}
}

func adminLimitStates(state budget.EffectiveState) []adminapi.BudgetLimitState {
	starts := map[string]time.Time{}
	for _, scope := range state.Budget.Scopes {
		starts[scope.ScopeKey] = scope.EffectiveFrom
	}
	items := make([]adminapi.BudgetLimitState, 0, len(state.Limits))
	for _, limit := range state.Limits {
		meter, divisor := adminapi.PricingMeter(limit.Meter), int64(1)
		if limit.Meter == budget.MeterAudioMilliseconds {
			meter, divisor = adminapi.AUDIOSECONDS, 1000
		}
		items = append(items, adminapi.BudgetLimitState{
			Meter: meter, Period: adminapi.BudgetPeriod(limit.Period), Limit: decimalUnits(limit.Limit, divisor), Used: decimalUnits(limit.Used, divisor),
			Reserved: decimalUnits(limit.Reserved, divisor), Remaining: decimalUnits(limit.Remaining, divisor), Overage: decimalUnits(limit.Overage, divisor),
			ScopeStart: starts[limit.ScopeKey], ResetAt: limit.ResetAt,
		})
	}
	return items
}

func adminLimitDefinitions(view budget.BudgetView) []adminapi.BudgetLimitDefinition {
	items := []adminapi.BudgetLimitDefinition{}
	for _, scope := range view.Scopes {
		for _, limit := range scope.Limits {
			meter, divisor := adminapi.PricingMeter(limit.Meter), int64(1)
			if limit.Meter == budget.MeterAudioMilliseconds {
				meter, divisor = adminapi.AUDIOSECONDS, 1000
			}
			items = append(items, adminapi.BudgetLimitDefinition{Meter: meter, Period: adminapi.BudgetPeriod(scope.Period), Limit: decimalUnits(limit.Limit, divisor)})
		}
	}
	return items
}

func decimalUnits(value, divisor int64) string {
	if divisor == 1 {
		return strconv.FormatInt(value, 10)
	}
	whole, fraction := value/divisor, value%divisor
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "." + strings.TrimRight(strconv.FormatInt(fraction+divisor, 10)[1:], "0")
}
