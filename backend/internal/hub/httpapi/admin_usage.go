package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) UsageTrend(w http.ResponseWriter, r *http.Request, params adminapi.UsageTrendParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	filter, err := usageFilterFromParams(params.From, params.To, params.UserId, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status), strPtr(params.ClientProtocol))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	if params.Completeness != nil {
		filter.Completeness = usage.Completeness(*params.Completeness)
	}
	trend, err := h.services.Usage.Trend(r.Context(), filter, h.services.Budget.Location)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	points := make([]adminapi.UsageTrendPoint, 0, len(trend.Points))
	for _, point := range trend.Points {
		points = append(points, adminapi.UsageTrendPoint{
			Date: openapi_types.Date{Time: point.Date}, RequestCount: point.RequestCount,
			ForwardedRequestCount: point.ForwardedRequestCount, SemanticMeters: adminMeterQuantities(point.Meters),
		})
	}
	writeJSON(w, http.StatusOK, adminapi.UsageTrend{From: trend.From, To: trend.To, Timezone: trend.Timezone, Points: points})
}

func (h *fullAdminHandler) UsageDistribution(w http.ResponseWriter, r *http.Request, params adminapi.UsageDistributionParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	filter, err := usageFilterFromParams(params.From, params.To, params.UserId, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status), strPtr(params.ClientProtocol))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	if params.Completeness != nil {
		filter.Completeness = usage.Completeness(*params.Completeness)
	}
	distribution, err := h.services.Usage.Distribution(r.Context(), filter)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]adminapi.UsageDistributionItem, 0, len(distribution.Items))
	for _, item := range distribution.Items {
		resourceID, resourceName := item.ResourceID, item.ResourceName
		items = append(items, adminapi.UsageDistributionItem{
			ResourceKind: adminapi.ResourceKind(item.ResourceKind), ClientProtocol: adminapi.UsageClientProtocol(item.ClientProtocol),
			ResourceId: &resourceID, ResourceDisplayName: &resourceName, RequestCount: item.RequestCount,
			SemanticMeters: adminMeterQuantities(item.Meters),
		})
	}
	writeJSON(w, http.StatusOK, adminapi.UsageDistribution{From: distribution.From, To: distribution.To, Items: items})
}

func (h *fullAdminHandler) ListUsageUsers(w http.ResponseWriter, r *http.Request, params adminapi.ListUsageUsersParams) {
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
	filter, err := usageFilterFromParams(params.From, params.To, nil, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status), strPtr(params.ClientProtocol))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	if params.Completeness != nil {
		filter.Completeness = usage.Completeness(*params.Completeness)
	}
	page, err := h.services.Usage.ListUsers(r.Context(), filter, h.services.Budget, usage.UserBudgetFilter(valueOrEmptyString(strPtr(params.BudgetStatus))), limit, cursor)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter or cursor")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]adminapi.UserUsageView, 0, len(page.Items))
	for _, item := range page.Items {
		assignment, err := h.services.Budget.GetAssignment(r.Context(), item.UserID)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
			return
		}
		items = append(items, adminapi.UserUsageView{
			UserId: item.UserID, UserDisplayName: item.DisplayName, RequestCount: item.RequestCount,
			SemanticMeters: adminMeterQuantities(item.Meters),
			Budget:         adminBudgetView(item.UserID, h.services.Budget.Location.String(), item.Budget, item.UsageMeters, assignment),
		})
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, http.StatusOK, adminapi.UserUsagePage{Items: items, NextCursor: next})
}

func (h *fullAdminHandler) ListUsageReconciliations(w http.ResponseWriter, r *http.Request, params adminapi.ListUsageReconciliationsParams) {
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
	page, err := h.services.Budget.ListOpenReconciliations(r.Context(), budget.ReconciliationQuery{PageSize: limit, Cursor: cursor})
	if errors.Is(err, budget.ErrInvalidConfiguration) {
		writeProblem(w, http.StatusBadRequest, "invalid_reconciliation_query", "Invalid reconciliation query")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	requestIDs := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		requestIDs = append(requestIDs, item.RequestID)
	}
	requestsByID, err := h.services.Usage.GetRequests(r.Context(), requestIDs)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]adminapi.ReconciliationView, 0, len(page.Items))
	for _, item := range page.Items {
		view := reconciliationWire(item, adminapi.ReconciliationViewStatePENDING, nil, nil, nil)
		if request, ok := requestsByID[item.RequestID]; ok {
			requestView := requestUsageWire(request)
			view.Request = &requestView
		}
		items = append(items, view)
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, http.StatusOK, adminapi.ReconciliationPage{Items: items, NextCursor: next})
}

func (h *fullAdminHandler) ResolveUsageReconciliation(w http.ResponseWriter, r *http.Request, requestID adminapi.RequestId, params adminapi.ResolveUsageReconciliationParams) {
	principal, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.ResolveReconciliationRequest
	if decodeStrictJSON(r, &request) != nil || request.ExpectedState != adminapi.ResolveReconciliationRequestExpectedStatePENDING {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Expected state must be PENDING")
		return
	}
	record, err := h.services.Budget.GetOpenReconciliation(r.Context(), requestID)
	if errors.Is(err, budget.ErrReconciliationNotOpen) {
		writeProblem(w, http.StatusConflict, "reconciliation_not_pending", "Reconciliation is no longer pending")
		return
	}
	if errors.Is(err, budget.ErrInvalidConfiguration) || errors.Is(err, budget.ErrRequestNotFound) {
		writeProblem(w, http.StatusNotFound, "reconciliation_not_found", "Reconciliation not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	if request.Action != adminapi.RELEASEUNCERTAIN {
		err = budget.ErrInvalidConfiguration
	}
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_reconciliation", "Invalid reconciliation action")
		return
	}
	if err := h.services.Budget.Resolve(r.Context(), budget.ResolveInput{
		RequestID: requestID, Action: budget.ResolutionReleaseUncertain, Revision: record.LastSettlementRevision + 1,
		ActorUserID: principal.UserID, Reason: request.Reason,
	}); errors.Is(err, budget.ErrReconciliationNotOpen) || errors.Is(err, budget.ErrSettlementRevisionConflict) {
		writeProblem(w, http.StatusConflict, "reconciliation_not_pending", "Reconciliation is no longer pending")
		return
	} else if errors.Is(err, budget.ErrInvalidConfiguration) || errors.Is(err, budget.ErrInvalidSettlement) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_reconciliation", "Invalid reconciliation resolution")
		return
	} else if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	resolvedAt, resolvedBy, reason := h.services.Budget.Now().UTC(), principal.UserID, request.Reason
	writeJSON(w, http.StatusOK, reconciliationWire(record, adminapi.ReconciliationViewStateRESOLVED, &resolvedAt, &resolvedBy, &reason))
}

func (h *fullAdminHandler) ListUsageRequests(w http.ResponseWriter, r *http.Request, params adminapi.ListUsageRequestsParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, after, valid := pageParams(w, r, params.Limit, params.Cursor)
	if !valid {
		return
	}
	filter, err := usageFilterFromParams(params.From, params.To, params.UserId, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status), strPtr(params.ClientProtocol))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	if params.Completeness != nil {
		filter.Completeness = usage.Completeness(*params.Completeness)
	}
	filter.After = after
	rows, err := h.services.Usage.ListRequests(r.Context(), filter, limit+1)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, 400, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	rows, next := pageResult(r, rows, limit, func(row usage.RequestView) string {
		return row.CompletedAt.UTC().Format(time.RFC3339Nano) + "|" + row.RequestID
	})
	items := make([]adminapi.RequestUsageView, 0, len(rows))
	for _, row := range rows {
		items = append(items, requestUsageWire(row))
	}
	writeJSON(w, http.StatusOK, adminapi.RequestUsagePage{Items: items, NextCursor: next})
}

func (h *fullAdminHandler) GetUsageRequest(w http.ResponseWriter, r *http.Request, requestID adminapi.RequestId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	row, err := h.services.Usage.GetRequest(r.Context(), requestID)
	if errors.Is(err, usage.ErrRequestNotFound) {
		writeProblem(w, http.StatusNotFound, "usage_request_not_found", "Usage request not found")
		return
	}
	if err != nil {
		writeProblem(w, 500, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, requestUsageWire(row))
}

func (h *fullAdminHandler) UsageSummary(w http.ResponseWriter, r *http.Request, params adminapi.UsageSummaryParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	filter, err := usageFilterFromParams(params.From, params.To, params.UserId, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status), strPtr(params.ClientProtocol))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	if params.Completeness != nil {
		filter.Completeness = usage.Completeness(*params.Completeness)
	}
	summary, err := h.services.Usage.Summary(r.Context(), filter)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, 400, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	wire := adminapi.UsageSummary{
		From: summary.From, To: summary.To,
		RequestCount: summary.RequestCount, ForwardedRequestCount: summary.ForwardedRequestCount,
		RequestCompleteness: adminapi.RequestCompletenessCounts{Exact: summary.RequestCompleteness.Exact, Partial: summary.RequestCompleteness.Partial, Unknown: summary.RequestCompleteness.Unknown},
		RequestBytes:        int(summary.RequestBytes), ResponseBytes: int(summary.ResponseBytes),
		SemanticMeters: []adminapi.MeterQuantity{},
	}
	for _, meter := range summary.Meters {
		confidence := adminapi.MeterQuantityConfidenceUNKNOWN
		switch meter.Confidence {
		case usage.CompletenessComplete:
			confidence = adminapi.MeterQuantityConfidenceEXACT
		case usage.CompletenessPartial:
			confidence = adminapi.MeterQuantityConfidencePARTIAL
		}
		wire.SemanticMeters = append(wire.SemanticMeters, adminapi.MeterQuantity{
			Confidence: confidence, Meter: adminapi.PricingMeter(meter.Meter), Quantity: meter.Quantity,
		})
	}
	wire.Cost.Status = adminapi.UsageSummaryCostStatusUNKNOWN
	if summary.Cost.State == usage.CostKnown || summary.Cost.State == usage.CostPartial {
		amount, currency := summary.Cost.Amount, summary.Cost.Currency
		wire.Cost.Amount, wire.Cost.Currency = &amount, &currency
		if summary.Cost.State == usage.CostKnown {
			wire.Cost.Status = adminapi.UsageSummaryCostStatusKNOWN
		} else {
			wire.Cost.Status = adminapi.UsageSummaryCostStatusPARTIAL
		}
	}
	writeJSON(w, http.StatusOK, wire)
}

func (h *fullAdminHandler) GetPricing(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	revision, rows, err := h.services.Usage.PricingSet(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, pricingSetWire(revision, rows))
}

func (h *fullAdminHandler) PutPricing(w http.ResponseWriter, r *http.Request, params adminapi.PutPricingParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.PutPricingRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	rules := make([]usage.PricingRuleRecord, 0, len(request.Rules))
	for _, rule := range request.Rules {
		resourceID := rule.ResourceId
		var upstreamID *string
		if rule.UpstreamId != nil {
			value := string(*rule.UpstreamId)
			upstreamID = &value
		}
		rules = append(rules, usage.PricingRuleRecord{
			ID: rule.PricingRuleId, ResourceID: resourceID, UpstreamID: upstreamID,
			Meter: string(rule.Meter), UnitSize: rule.UnitSize, UnitPrice: rule.UnitPrice, Currency: rule.Currency,
			EffectiveFrom: rule.EffectiveFrom,
		})
	}
	revision, rows, err := h.services.Usage.ReplacePricingSet(r.Context(), request.ExpectedPricingRevision, rules)
	if errors.Is(err, usage.ErrPricingRevisionConflict) {
		writeProblem(w, http.StatusConflict, "pricing_revision_conflict", "Pricing revision conflict")
		return
	}
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_pricing", "Invalid pricing")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, pricingSetWire(revision, rows))
}

func usageFilterFromParams(from, to *time.Time, userID, resourceID, resourceKind, upstreamID, status, clientProtocol *string) (usage.Filter, error) {
	filter := usage.Filter{From: from, To: to}
	if userID != nil {
		filter.UserID = *userID
	}
	if resourceID != nil {
		filter.ResourceID = *resourceID
	}
	if upstreamID != nil {
		filter.UpstreamID = *upstreamID
	}
	if resourceKind != nil {
		filter.ResourceKind = usage.ResourceKind(*resourceKind)
	}
	if status != nil {
		filter.Status = usage.RequestStatus(*status)
	}
	if clientProtocol != nil {
		filter.ClientProtocol = *clientProtocol
	}
	return filter, nil
}

func strPtr[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

func valueOrEmptyString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func requestUsageWire(row usage.RequestView) adminapi.RequestUsageView {
	semantic := make([]adminapi.MeterQuantity, 0, len(row.SemanticMeters))
	for _, meter := range row.SemanticMeters {
		semantic = append(semantic, adminapi.MeterQuantity{Meter: adminapi.PricingMeter(meter.Meter), Quantity: meter.Quantity, Confidence: adminMeterConfidence(meter.Confidence)})
	}
	result := adminapi.RequestUsageView{
		RequestId: row.RequestID, InteractionId: row.InteractionID, DeploymentId: row.DeploymentID,
		UserId: row.UserID, DeviceId: row.DeviceID, ResourceId: row.ResourceID, RuntimeRouteId: row.RuntimeRouteID,
		ResourceKind: adminapi.ResourceKind(row.ResourceKind), ClientProtocol: adminapi.UsageClientProtocol(row.ClientProtocol),
		ResourceDisplayName: row.ResourceDisplayName,
		UserDisplayName:     row.UserDisplayName, DeviceName: row.DeviceName,
		UpstreamId: row.UpstreamID, ManagedGeneration: row.ManagedGeneration, ControlRevision: row.ControlRevision,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, Forwarded: row.Forwarded,
		HttpStatus: row.HTTPStatus, UpstreamHttpStatus: row.UpstreamHTTPStatus,
		RequestBytes: row.RequestBytes, ResponseBytes: row.ResponseBytes, DurationMs: row.DurationMs, ErrorClass: row.ErrorClass,
		RequestCompleteness: adminapi.RequestUsageViewRequestCompleteness(row.RequestCompleteness),
		SettlementState:     adminapi.RequestUsageViewSettlementState(row.SettlementState), SemanticMeters: semantic,
	}
	if row.Budget != nil {
		result.Budget = adminBudgetContext(*row.Budget)
	}
	return result
}

func adminMeterConfidence(value usage.Completeness) adminapi.MeterQuantityConfidence {
	switch value {
	case usage.CompletenessComplete:
		return adminapi.MeterQuantityConfidenceEXACT
	case usage.CompletenessPartial:
		return adminapi.MeterQuantityConfidencePARTIAL
	default:
		return adminapi.MeterQuantityConfidenceUNKNOWN
	}
}

func adminMeterQuantities(meters []usage.MeterSummary) []adminapi.MeterQuantity {
	items := make([]adminapi.MeterQuantity, 0, len(meters))
	for _, meter := range meters {
		items = append(items, adminapi.MeterQuantity{
			Meter: adminapi.PricingMeter(meter.Meter), Quantity: meter.Quantity,
			Confidence: adminMeterConfidence(meter.Confidence),
		})
	}
	return items
}

func reconciliationWire(record budget.ReconciliationRecord, state adminapi.ReconciliationViewState, resolvedAt *time.Time, resolvedBy, reason *string) adminapi.ReconciliationView {
	reservationByMeter := make(map[budget.Meter]int64)
	observedByMeter := make(map[budget.Meter]int64)
	for _, allocation := range record.Allocations {
		if !allocation.ReservationReleased && allocation.Reserved > reservationByMeter[allocation.Meter] {
			reservationByMeter[allocation.Meter] = allocation.Reserved
		}
		if allocation.Resolved && allocation.Observed > observedByMeter[allocation.Meter] {
			observedByMeter[allocation.Meter] = allocation.Observed
		}
	}
	return adminapi.ReconciliationView{
		RequestId: record.RequestID, UserId: record.UserID, Capability: adminapi.BudgetCapability(record.Capability), State: state,
		ResourceId: record.ResourceID, ClientProtocol: adminapi.UsageClientProtocol(record.ClientProtocol), ReconciliationReason: record.Reason,
		Forwarded: record.Forwarded, HttpStatus: record.HTTPStatus, UpstreamHttpStatus: record.UpstreamHTTPStatus,
		ErrorClass: record.ErrorClass, AdmittedAt: record.AdmittedAt, StartedAt: record.StartedAt,
		Reservation: reconciliationMeterWire(reservationByMeter), Observed: reconciliationMeterWire(observedByMeter),
		CreatedAt: record.OpenedAt, UpdatedAt: record.UpdatedAt, ResolvedAt: resolvedAt, ResolvedBy: resolvedBy, ResolutionReason: reason,
		Completeness: func() *adminapi.UsageCompleteness {
			if record.Completeness == nil {
				return nil
			}
			value := adminapi.UsageCompleteness(*record.Completeness)
			return &value
		}(),
	}
}

func reconciliationMeterWire(values map[budget.Meter]int64) []adminapi.MeterQuantity {
	meters := make([]string, 0, len(values))
	for meter := range values {
		meters = append(meters, string(meter))
	}
	sort.Strings(meters)
	items := make([]adminapi.MeterQuantity, 0, len(meters))
	for _, name := range meters {
		meter, divisor := budget.Meter(name), int64(1)
		wireMeter := adminapi.PricingMeter(meter)
		if meter == budget.MeterAudioMilliseconds {
			wireMeter, divisor = adminapi.AUDIOSECONDS, 1000
		}
		items = append(items, adminapi.MeterQuantity{
			Meter: wireMeter, Quantity: decimalUnits(values[meter], divisor), Confidence: adminapi.MeterQuantityConfidenceEXACT,
		})
	}
	return items
}

func adminBudgetContext(decision budget.AdmissionDecision) *adminapi.BudgetContext {
	blockers := make([]adminapi.BudgetLimitState, 0, len(decision.BlockingLimits))
	for _, limit := range decision.BlockingLimits {
		meter, divisor := adminapi.PricingMeter(limit.Meter), int64(1)
		if limit.Meter == budget.MeterAudioMilliseconds {
			meter, divisor = adminapi.AUDIOSECONDS, 1000
		}
		remaining := limit.Limit - limit.Used - limit.Reserved
		overage := int64(0)
		if remaining < 0 {
			overage, remaining = -remaining, 0
		}
		blockers = append(blockers, adminapi.BudgetLimitState{
			Meter: meter, Period: adminapi.BudgetPeriod(limit.Period), Limit: decimalUnits(limit.Limit, divisor),
			Used: decimalUnits(limit.Used, divisor), Reserved: decimalUnits(limit.Reserved, divisor),
			Remaining: decimalUnits(remaining, divisor), Overage: decimalUnits(overage, divisor),
			ScopeStart: limit.ScopeStart, ResetAt: limit.ResetAt,
		})
	}
	return &adminapi.BudgetContext{Capability: adminapi.BudgetCapability(decision.Capability), Mode: adminapi.BudgetMode(decision.Mode), Revision: int(decision.Revision), Blockers: blockers, ResetAt: decision.ResetAt, AsOf: decision.AsOf}
}

func pricingSetWire(revision int, rows []usage.PricingRuleRecord) adminapi.PricingSet {
	items := make([]adminapi.PricingRule, 0, len(rows))
	for _, row := range rows {
		var upstreamID *adminapi.UpstreamId
		if row.UpstreamID != nil {
			value := adminapi.UpstreamId(*row.UpstreamID)
			upstreamID = &value
		}
		items = append(items, adminapi.PricingRule{
			PricingRuleId: row.ID, ResourceId: row.ResourceID, UpstreamId: upstreamID,
			Meter: adminapi.PricingMeter(row.Meter), UnitSize: row.UnitSize, UnitPrice: row.UnitPrice, Currency: row.Currency,
			EffectiveFrom: row.EffectiveFrom,
		})
	}
	return adminapi.PricingSet{PricingRevision: revision, Rules: items}
}
