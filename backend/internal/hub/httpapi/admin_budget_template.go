package httpapi

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"

	"measix/platform/ent"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) ListBudgetTemplates(w http.ResponseWriter, r *http.Request, params adminapi.ListBudgetTemplatesParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, cursor, query := 50, "", ""
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	if params.Query != nil {
		query = *params.Query
	}
	page, err := h.services.Budget.ListTemplates(r.Context(), query, limit, cursor)
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	items := make([]adminapi.BudgetTemplate, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, adminBudgetTemplate(item))
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, 200, adminapi.BudgetTemplatePage{Items: items, NextCursor: next})
}

func (h *fullAdminHandler) CreateBudgetTemplate(w http.ResponseWriter, r *http.Request, params adminapi.CreateBudgetTemplateParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.CreateBudgetTemplateRequest
	if decodeStrictJSON(r, &request) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	rules, err := domainTemplateRules(request.Rules)
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	view, err := h.services.Budget.CreateTemplate(r.Context(), budget.CreateTemplateInput{
		Name: request.Name, Description: request.Description, Rules: rules,
		ActorUserID: principal.UserID, Reason: request.Reason,
	})
	if ent.IsConstraintError(err) {
		writeProblem(w, 409, "budget_template_conflict", "Budget template already exists")
		return
	}
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	writeJSON(w, 201, adminBudgetTemplate(view))
}

func (h *fullAdminHandler) ListBudgetTemplateAudit(w http.ResponseWriter, r *http.Request, templateID adminapi.BudgetTemplateId, params adminapi.ListBudgetTemplateAuditParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, cursor := 50, ""
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	page, err := h.services.Budget.ListTemplateAudit(r.Context(), templateID, limit, cursor)
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	items := make([]adminapi.BudgetTemplateAuditItem, 0, len(page.Items))
	for _, item := range page.Items {
		wire := adminapi.BudgetTemplateAuditItem{
			AuditId: int64(item.ID), BudgetTemplateId: item.TemplateID,
			Action: adminapi.BudgetTemplateAuditItemAction(item.Action), TemplateRevision: int(item.TemplateRevision),
			AssignmentRevision: int(item.AssignmentRevision), ChangedBy: item.ActorUserID,
			Reason: item.Reason, CreatedAt: item.CreatedAt,
		}
		if item.UserID != nil {
			value := adminapi.UserId(*item.UserID)
			wire.UserId = &value
		}
		wire.Before = adminTemplateAuditSnapshot(item.Before)
		wire.After = adminTemplateAuditSnapshot(item.After)
		items = append(items, wire)
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, 200, adminapi.BudgetTemplateAuditPage{Items: items, NextCursor: next})
}

func (h *fullAdminHandler) GetBudgetTemplate(w http.ResponseWriter, r *http.Request, templateID adminapi.BudgetTemplateId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	view, err := h.services.Budget.GetTemplate(r.Context(), templateID)
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	writeJSON(w, 200, adminBudgetTemplate(view))
}

func (h *fullAdminHandler) UpdateBudgetTemplate(w http.ResponseWriter, r *http.Request, templateID adminapi.BudgetTemplateId, params adminapi.UpdateBudgetTemplateParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.UpdateBudgetTemplateRequest
	if decodeStrictJSON(r, &request) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	rules, err := domainTemplateRules(request.Rules)
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	view, err := h.services.Budget.UpdateTemplate(r.Context(), budget.UpdateTemplateInput{
		TemplateID: templateID, ExpectedRevision: int64(request.ExpectedRevision), Name: request.Name,
		Description: request.Description, Rules: rules, ActorUserID: principal.UserID, Reason: request.Reason,
	})
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	writeJSON(w, 200, adminBudgetTemplate(view))
}

func (h *fullAdminHandler) DeleteBudgetTemplate(w http.ResponseWriter, r *http.Request, templateID adminapi.BudgetTemplateId, params adminapi.DeleteBudgetTemplateParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	err = h.services.Budget.DeleteTemplate(r.Context(), budget.DeleteTemplateInput{
		TemplateID: templateID, ExpectedRevision: int64(params.ExpectedRevision), ActorUserID: principal.UserID, Reason: params.Reason,
	})
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *fullAdminHandler) ListBudgetTemplateUsers(w http.ResponseWriter, r *http.Request, templateID adminapi.BudgetTemplateId, params adminapi.ListBudgetTemplateUsersParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, cursor, query := 50, "", ""
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	if params.Query != nil {
		query = *params.Query
	}
	page, err := h.services.Budget.ListTemplateUsers(r.Context(), templateID, query, limit, cursor)
	if err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	items := make([]adminapi.User, 0, len(page.UserIDs))
	for _, userID := range page.UserIDs {
		view, err := h.services.Identity.GetUserView(r.Context(), userID)
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		items = append(items, userWire(view))
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, 200, adminapi.UserPage{Items: items, NextCursor: next})
}

func (h *fullAdminHandler) AssignUserBudgetTemplate(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.AssignUserBudgetTemplateParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	if _, err := h.services.Identity.GetUserView(r.Context(), userID); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.AssignBudgetTemplateRequest
	if decodeStrictJSON(r, &request) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	if _, err := h.services.Budget.AssignTemplate(r.Context(), budget.AssignTemplateInput{
		UserID: userID, TemplateID: request.BudgetTemplateId, ExpectedRevision: int64(request.ExpectedAssignmentRevision),
		ActorUserID: principal.UserID, Reason: request.Reason,
	}); err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	h.writeAdminUserBudget(w, r, userID)
}

func (h *fullAdminHandler) UnassignUserBudgetTemplate(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.UnassignUserBudgetTemplateParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	if _, err := h.services.Budget.UnassignTemplate(r.Context(), budget.UnassignTemplateInput{
		UserID: userID, ExpectedRevision: int64(params.ExpectedAssignmentRevision), ActorUserID: principal.UserID, Reason: params.Reason,
	}); err != nil {
		writeBudgetTemplateError(w, err)
		return
	}
	h.writeAdminUserBudget(w, r, userID)
}

func (h *fullAdminHandler) writeAdminUserBudget(w http.ResponseWriter, r *http.Request, userID string) {
	states, err := h.services.Budget.UserStates(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	meters, err := h.services.Usage.UsageMetersByCapability(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	assignment, err := h.services.Budget.GetAssignment(r.Context(), userID)
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	writeJSON(w, 200, adminBudgetView(userID, h.services.Budget.Location.String(), states, meters, assignment))
}

func domainTemplateRules(values []adminapi.BudgetTemplateRule) ([]budget.TemplateRule, error) {
	rules := make([]budget.TemplateRule, 0, len(values))
	for _, value := range values {
		scopes, err := domainLimitScopes(value.Limits)
		if err != nil {
			return nil, err
		}
		rules = append(rules, budget.TemplateRule{Capability: budget.Capability(value.Capability), Mode: budget.Mode(value.Mode), Scopes: scopes})
	}
	return rules, nil
}

func domainLimitScopes(values []adminapi.BudgetLimitDefinition) ([]budget.ScopeInput, error) {
	grouped := make(map[budget.Period][]budget.LimitSpec)
	for _, value := range values {
		quantity, err := parseAdminMeterQuantity(value.Meter, value.Limit)
		if err != nil {
			return nil, err
		}
		grouped[budget.Period(value.Period)] = append(grouped[budget.Period(value.Period)], quantity)
	}
	periods := make([]string, 0, len(grouped))
	for period := range grouped {
		periods = append(periods, string(period))
	}
	sort.Strings(periods)
	scopes := make([]budget.ScopeInput, 0, len(periods))
	for _, period := range periods {
		scopes = append(scopes, budget.ScopeInput{Period: budget.Period(period), Limits: grouped[budget.Period(period)]})
	}
	return scopes, nil
}

func parseAdminMeterQuantity(meter adminapi.PricingMeter, value string) (budget.LimitSpec, error) {
	quantity, err := strconv.ParseInt(value, 10, 64)
	if err != nil || quantity < 0 {
		return budget.LimitSpec{}, budget.ErrInvalidConfiguration
	}
	domainMeter := budget.Meter(meter)
	if meter == adminapi.AUDIOSECONDS {
		if quantity > math.MaxInt64/1000 {
			return budget.LimitSpec{}, budget.ErrInvalidConfiguration
		}
		domainMeter, quantity = budget.MeterAudioMilliseconds, quantity*1000
	}
	return budget.LimitSpec{Meter: domainMeter, Limit: quantity}, nil
}

func adminMeterQuantity(meter budget.Meter, value int64) (adminapi.PricingMeter, string) {
	if meter == budget.MeterAudioMilliseconds {
		return adminapi.AUDIOSECONDS, decimalUnits(value, 1000)
	}
	return adminapi.PricingMeter(meter), decimalUnits(value, 1)
}

func adminBudgetTemplate(view budget.TemplateView) adminapi.BudgetTemplate {
	return adminapi.BudgetTemplate{
		BudgetTemplateId: view.TemplateID, Name: view.Name, Description: view.Description, Revision: int(view.Revision), Rules: adminTemplateRules(view.Rules),
		AssignedUserCount: view.AssignedUserCount, CreatedAt: view.CreatedAt, UpdatedAt: view.UpdatedAt,
	}
}

func adminTemplateRules(values []budget.TemplateRule) []adminapi.BudgetTemplateRule {
	rules := make([]adminapi.BudgetTemplateRule, 0, len(values))
	for _, rule := range values {
		limits := make([]adminapi.BudgetLimitDefinition, 0)
		for _, scope := range rule.Scopes {
			for _, limit := range scope.Limits {
				meter, quantity := adminMeterQuantity(limit.Meter, limit.Limit)
				limits = append(limits, adminapi.BudgetLimitDefinition{Meter: meter, Period: adminapi.BudgetPeriod(scope.Period), Limit: quantity})
			}
		}
		rules = append(rules, adminapi.BudgetTemplateRule{Capability: adminapi.BudgetCapability(rule.Capability), Mode: adminapi.BudgetMode(rule.Mode), Limits: limits})
	}
	return rules
}

func adminTemplateAuditSnapshot(value *budget.TemplateAuditSnapshot) *adminapi.BudgetTemplateAuditSnapshot {
	if value == nil {
		return nil
	}
	return &adminapi.BudgetTemplateAuditSnapshot{Name: value.Name, Description: value.Description, Rules: adminTemplateRules(value.Rules)}
}

func writeBudgetTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, budget.ErrTemplateNotFound), errors.Is(err, budget.ErrAssignmentNotFound), errors.Is(err, identity.ErrNotFound):
		writeProblem(w, 404, "budget_template_not_found", "Budget template or assignment not found")
	case errors.Is(err, budget.ErrRevisionConflict):
		writeProblem(w, 409, "budget_revision_conflict", "Budget configuration was changed by another administrator")
	case errors.Is(err, budget.ErrTemplateAssigned):
		writeProblem(w, 409, "budget_template_assigned", "Budget template is still assigned to users")
	case errors.Is(err, budget.ErrInvalidConfiguration):
		writeProblem(w, 400, "invalid_budget_template", "Invalid budget template configuration")
	default:
		writeProblem(w, 500, "internal_error", "Internal error")
	}
}
