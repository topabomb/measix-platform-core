package httpapi

import (
	"errors"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/clientapi"
)

func (h *fullClientHandler) GetPortalUsageSummary(w http.ResponseWriter, r *http.Request, params clientapi.GetPortalUsageSummaryParams) {
	_, filter, ok := h.portalUsageFilter(w, r, params.From, params.To, params.ResourceKind, params.ClientProtocol)
	if !ok {
		return
	}
	summary, err := h.usage.Summary(r.Context(), filter)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, clientapi.UsageSummary{
		From: summary.From, To: summary.To, RequestCount: summary.RequestCount,
		ForwardedRequestCount: summary.ForwardedRequestCount, RequestBytes: int(summary.RequestBytes),
		ResponseBytes: int(summary.ResponseBytes), SemanticMeters: clientMeterQuantities(summary.Meters),
	})
}

func (h *fullClientHandler) GetPortalUsageTrend(w http.ResponseWriter, r *http.Request, params clientapi.GetPortalUsageTrendParams) {
	_, filter, ok := h.portalUsageFilter(w, r, params.From, params.To, params.ResourceKind, params.ClientProtocol)
	if !ok {
		return
	}
	trend, err := h.usage.Trend(r.Context(), filter, h.budget.Location)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	points := make([]clientapi.UsageTrendPoint, 0, len(trend.Points))
	for _, point := range trend.Points {
		points = append(points, clientapi.UsageTrendPoint{
			Date: openapi_types.Date{Time: point.Date}, RequestCount: point.RequestCount,
			SemanticMeters: clientMeterQuantities(point.Meters),
		})
	}
	writeJSON(w, http.StatusOK, clientapi.UsageTrend{From: trend.From, To: trend.To, Timezone: trend.Timezone, Points: points})
}

func (h *fullClientHandler) GetPortalUsageDistribution(w http.ResponseWriter, r *http.Request, params clientapi.GetPortalUsageDistributionParams) {
	_, filter, ok := h.portalUsageFilter(w, r, params.From, params.To, params.ResourceKind, params.ClientProtocol)
	if !ok {
		return
	}
	distribution, err := h.usage.Distribution(r.Context(), filter)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]clientapi.UsageDistributionItem, 0, len(distribution.Items))
	for _, item := range distribution.Items {
		resourceID, resourceName := item.ResourceID, item.ResourceName
		items = append(items, clientapi.UsageDistributionItem{
			ResourceKind: clientapi.ResourceKind(item.ResourceKind), ClientProtocol: clientapi.UsageClientProtocol(item.ClientProtocol),
			ResourceId: &resourceID, ResourceDisplayName: &resourceName, RequestCount: item.RequestCount,
			SemanticMeters: clientMeterQuantities(item.Meters),
		})
	}
	writeJSON(w, http.StatusOK, clientapi.UsageDistribution{From: distribution.From, To: distribution.To, Items: items})
}

func (h *fullClientHandler) ListPortalUsageRequests(w http.ResponseWriter, r *http.Request, params clientapi.ListPortalUsageRequestsParams) {
	_, filter, ok := h.portalUsageFilter(w, r, params.From, params.To, params.ResourceKind, params.ClientProtocol)
	if !ok {
		return
	}
	limit, after, valid := pageParams(w, r, params.Limit, params.Cursor)
	if !valid {
		return
	}
	filter.After = after
	rows, err := h.usage.ListRequests(r.Context(), filter, limit+1)
	if errors.Is(err, usage.ErrInvalidBatch) {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", "Invalid usage filter or cursor")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	rows, next := pageResult(r, rows, limit, func(row usage.RequestView) string {
		return row.CompletedAt.UTC().Format(time.RFC3339Nano) + "|" + row.RequestID
	})
	items := make([]clientapi.RequestUsageView, 0, len(rows))
	for _, row := range rows {
		items = append(items, clientRequestUsageWire(row))
	}
	writeJSON(w, http.StatusOK, clientapi.RequestUsagePage{Items: items, NextCursor: next})
}

func (h *fullClientHandler) GetPortalUsageRequest(w http.ResponseWriter, r *http.Request, requestID clientapi.RequestId) {
	w.Header().Set("Cache-Control", "no-store")
	session, ok := h.authenticatePortal(w, r)
	if !ok {
		return
	}
	if h.usage == nil {
		writeProblem(w, http.StatusServiceUnavailable, "usage_service_unavailable", "Usage service unavailable")
		return
	}
	row, err := h.usage.GetRequest(r.Context(), requestID)
	if errors.Is(err, usage.ErrRequestNotFound) || err == nil && row.UserID != session.UserId {
		writeProblem(w, http.StatusNotFound, "usage_request_not_found", "Usage request not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, clientRequestUsageWire(row))
}

func (h *fullClientHandler) portalUsageFilter(w http.ResponseWriter, r *http.Request, from, to *time.Time, resourceKind *clientapi.ResourceKind, protocol *clientapi.UsageClientProtocol) (clientapi.PortalSession, usage.Filter, bool) {
	w.Header().Set("Cache-Control", "no-store")
	session, ok := h.authenticatePortal(w, r)
	if !ok {
		return clientapi.PortalSession{}, usage.Filter{}, false
	}
	if h.usage == nil || h.budget == nil {
		writeProblem(w, http.StatusServiceUnavailable, "usage_service_unavailable", "Usage service unavailable")
		return clientapi.PortalSession{}, usage.Filter{}, false
	}
	filter := usage.Filter{From: from, To: to, UserID: session.UserId}
	if resourceKind != nil {
		filter.ResourceKind = usage.ResourceKind(*resourceKind)
	}
	if protocol != nil {
		filter.ClientProtocol = string(*protocol)
	}
	return session, filter, true
}

func clientRequestUsageWire(row usage.RequestView) clientapi.RequestUsageView {
	status := row.HTTPStatus
	resourceID, resourceName := row.ResourceID, row.ResourceDisplayName
	result := clientapi.RequestUsageView{
		RequestId: row.RequestID, InteractionId: row.InteractionID, ResourceId: &resourceID, ResourceDisplayName: &resourceName,
		ResourceKind: clientapi.ResourceKind(row.ResourceKind), ClientProtocol: clientapi.UsageClientProtocol(row.ClientProtocol),
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, Forwarded: row.Forwarded, HttpStatus: &status,
		RequestBytes: row.RequestBytes, ResponseBytes: row.ResponseBytes, DurationMs: row.DurationMs, ErrorClass: row.ErrorClass,
		RequestCompleteness: clientapi.UsageCompleteness(row.RequestCompleteness),
		SettlementState:     clientapi.RequestUsageViewSettlementState(row.SettlementState),
		SemanticMeters:      clientMeterQuantities(row.SemanticMeters),
	}
	if row.Budget != nil {
		result.Budget = clientBudgetContext(*row.Budget)
	}
	return result
}

func clientMeterQuantities(meters []usage.MeterSummary) []clientapi.MeterQuantity {
	items := make([]clientapi.MeterQuantity, 0, len(meters))
	for _, meter := range meters {
		items = append(items, clientapi.MeterQuantity{
			Meter: clientapi.UsageMeter(meter.Meter), Quantity: meter.Quantity,
			Completeness: clientapi.UsageCompleteness(meter.Confidence),
		})
	}
	return items
}

func clientBudgetContext(decision budget.AdmissionDecision) *clientapi.BudgetContext {
	blockers := make([]clientapi.BudgetLimitState, 0, len(decision.BlockingLimits))
	for _, limit := range decision.BlockingLimits {
		meter, divisor := clientapi.UsageMeter(limit.Meter), int64(1)
		if limit.Meter == budget.MeterAudioMilliseconds {
			meter, divisor = clientapi.AUDIOSECONDS, 1000
		}
		remaining := limit.Limit - limit.Used - limit.Reserved
		overage := int64(0)
		if remaining < 0 {
			overage, remaining = -remaining, 0
		}
		blockers = append(blockers, clientapi.BudgetLimitState{
			Meter: meter, Period: clientapi.BudgetPeriod(limit.Period), Limit: clientDecimalUnits(limit.Limit, divisor),
			Used: clientDecimalUnits(limit.Used, divisor), Reserved: clientDecimalUnits(limit.Reserved, divisor),
			Remaining: clientDecimalUnits(remaining, divisor), Overage: clientDecimalUnits(overage, divisor),
			ScopeStart: limit.ScopeStart, ResetAt: limit.ResetAt,
		})
	}
	return &clientapi.BudgetContext{
		Capability: clientapi.BudgetCapability(decision.Capability), Mode: clientapi.BudgetMode(decision.Mode),
		Revision: int(decision.Revision), Blockers: blockers, ResetAt: decision.ResetAt, AsOf: decision.AsOf,
	}
}
