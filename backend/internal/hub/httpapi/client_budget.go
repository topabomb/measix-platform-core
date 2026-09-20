package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/clientapi"
)

func (h *fullClientHandler) GetClientBudgets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	principal, ok := h.authenticateClient(w, r)
	if !ok {
		return
	}
	h.writeClientBudgets(w, r, principal.UserID)
}

func (h *fullClientHandler) GetPortalBudgets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	session, ok := h.authenticatePortal(w, r)
	if !ok {
		return
	}
	h.writeClientBudgets(w, r, session.UserId)
}

func (h *fullClientHandler) authenticateClient(w http.ResponseWriter, r *http.Request) (identity.AccessPrincipal, bool) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Unauthenticated")
		return identity.AccessPrincipal{}, false
	}
	principal, err := h.identity.AuthenticateAccess(r.Context(), token)
	if err != nil {
		writeIdentityError(w, err)
		return identity.AccessPrincipal{}, false
	}
	return principal, true
}

func (h *fullClientHandler) writeClientBudgets(w http.ResponseWriter, r *http.Request, userID string) {
	if h.budget == nil || h.usage == nil {
		writeProblem(w, http.StatusServiceUnavailable, "budget_service_unavailable", "Budget service unavailable")
		return
	}
	states, err := h.budget.UserStates(r.Context(), userID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	usageMeters, err := h.usage.UsageMetersByCapability(r.Context(), userID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, clientBudgetView(userID, h.budget.Location.String(), states, usageMeters))
}

func clientBudgetView(userID, timezone string, states []budget.EffectiveState, usageMeters usage.CapabilityMeters) clientapi.UserBudgetView {
	items := make([]clientapi.BudgetCapabilityView, 0, len(states))
	var asOf time.Time
	for _, state := range states {
		asOf = state.AsOf
		effectiveFrom := state.AsOf
		if state.Budget.ActivatedAt != nil {
			effectiveFrom = *state.Budget.ActivatedAt
		}
		status := clientapi.BudgetStatus("AVAILABLE")
		switch state.Status {
		case budget.StatusExhausted:
			status = "EXHAUSTED"
		case budget.StatusReconciliation:
			status = "PENDING_RECONCILIATION"
		}
		items = append(items, clientapi.BudgetCapabilityView{
			Capability: clientapi.BudgetCapability(state.Budget.Capability), Mode: clientapi.BudgetMode(state.Budget.Mode),
			Source: clientapi.BudgetSource(state.Budget.Source), Revision: int(state.Budget.Revision), Status: status,
			EffectiveFrom: effectiveFrom, AsOf: state.AsOf, InFlightRequests: int(state.InFlightRequests), Limits: clientLimitStates(state),
			UsageMeters: clientMeterQuantities(usageMeters[state.Budget.Capability]),
		})
	}
	return clientapi.UserBudgetView{UserId: userID, Timezone: timezone, AsOf: asOf, Items: items}
}

func clientLimitStates(state budget.EffectiveState) []clientapi.BudgetLimitState {
	starts := make(map[string]time.Time, len(state.Budget.Scopes))
	for _, scope := range state.Budget.Scopes {
		starts[scope.ScopeKey] = scope.EffectiveFrom
	}
	items := make([]clientapi.BudgetLimitState, 0, len(state.Limits))
	for _, limit := range state.Limits {
		meter, divisor := clientapi.UsageMeter(limit.Meter), int64(1)
		if limit.Meter == budget.MeterAudioMilliseconds {
			meter, divisor = "AUDIO_SECONDS", 1000
		}
		items = append(items, clientapi.BudgetLimitState{
			Meter: meter, Period: clientapi.BudgetPeriod(limit.Period), Limit: clientDecimalUnits(limit.Limit, divisor),
			Used: clientDecimalUnits(limit.Used, divisor), Reserved: clientDecimalUnits(limit.Reserved, divisor),
			Remaining: clientDecimalUnits(limit.Remaining, divisor), Overage: clientDecimalUnits(limit.Overage, divisor),
			ScopeStart: starts[limit.ScopeKey], ResetAt: limit.ResetAt,
		})
	}
	return items
}

func clientDecimalUnits(value, divisor int64) string {
	if divisor == 1 {
		return strconv.FormatInt(value, 10)
	}
	whole, fraction := value/divisor, value%divisor
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "." + strings.TrimRight(strconv.FormatInt(fraction+divisor, 10)[1:], "0")
}
