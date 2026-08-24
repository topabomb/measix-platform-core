package httpapi

import (
	"errors"
	"net/http"
	"time"

	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) ListUsageRequests(w http.ResponseWriter, r *http.Request, params adminapi.ListUsageRequestsParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}
	filter, err := usageFilterFromParams(params.From, params.To, params.UserId, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	rows, err := h.services.Usage.ListRequests(r.Context(), filter, limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]adminapi.RequestUsageView, 0, len(rows))
	for _, row := range rows {
		items = append(items, requestUsageWire(row))
	}
	writeJSON(w, http.StatusOK, adminapi.RequestUsagePage{Items: items})
}

func (h *fullAdminHandler) GetUsageRequest(w http.ResponseWriter, r *http.Request, requestID adminapi.RequestId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	row, err := h.services.Usage.GetRequest(r.Context(), requestID)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "usage_request_not_found", "Usage request not found")
		return
	}
	writeJSON(w, http.StatusOK, requestUsageWire(row))
}

func (h *fullAdminHandler) UsageSummary(w http.ResponseWriter, r *http.Request, params adminapi.UsageSummaryParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	filter, err := usageFilterFromParams(params.From, params.To, params.UserId, params.ResourceId, strPtr(params.ResourceKind), params.UpstreamId, strPtr(params.Status))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_usage_filter", err.Error())
		return
	}
	if params.Completeness != nil {
		switch *params.Completeness {
		case adminapi.UsageSummaryParamsCompletenessKNOWN:
			filter.Completeness = usage.CompletenessComplete
		case adminapi.UsageSummaryParamsCompletenessPARTIAL:
			filter.Completeness = usage.CompletenessPartial
		case adminapi.UsageSummaryParamsCompletenessUNKNOWN:
			filter.Completeness = usage.CompletenessUnknown
		}
	}
	summary, err := h.services.Usage.Summary(r.Context(), filter)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	wire := adminapi.UsageSummary{
		From: summary.From, To: summary.To,
		RequestCount: summary.RequestCount, ForwardedRequestCount: summary.ForwardedRequestCount,
		RequestBytes: int(summary.RequestBytes), ResponseBytes: int(summary.ResponseBytes),
		SemanticMeters: []struct {
			Confidence adminapi.UsageSummarySemanticMetersConfidence `json:"confidence"`
			Meter      adminapi.PricingMeter                          `json:"meter"`
			Quantity   string                                        `json:"quantity"`
		}{},
	}
	for _, meter := range summary.Meters {
		confidence := adminapi.UsageSummarySemanticMetersConfidenceUNKNOWN
		switch meter.Confidence {
		case usage.CompletenessComplete:
			confidence = adminapi.UsageSummarySemanticMetersConfidenceEXACT
		case usage.CompletenessPartial:
			confidence = adminapi.UsageSummarySemanticMetersConfidencePARTIAL
		}
		wire.SemanticMeters = append(wire.SemanticMeters, struct {
			Confidence adminapi.UsageSummarySemanticMetersConfidence `json:"confidence"`
			Meter      adminapi.PricingMeter                          `json:"meter"`
			Quantity   string                                        `json:"quantity"`
		}{Confidence: confidence, Meter: adminapi.PricingMeter(meter.Meter), Quantity: meter.Quantity})
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

func usageFilterFromParams(from, to *time.Time, userID, resourceID, resourceKind, upstreamID, status *string) (usage.Filter, error) {
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
	return filter, nil
}

func strPtr[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

func requestUsageWire(row usage.RequestView) adminapi.RequestUsageView {
	return adminapi.RequestUsageView{
		RequestId: row.RequestID, InteractionId: row.InteractionID, DeploymentId: row.DeploymentID,
		UserId: row.UserID, DeviceId: row.DeviceID, ResourceId: row.ResourceID, RuntimeRouteId: row.RuntimeRouteID,
		UpstreamId: row.UpstreamID, ManagedGeneration: row.ManagedGeneration, ControlRevision: row.ControlRevision,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, Forwarded: row.Forwarded,
		HttpStatus: row.HTTPStatus, UpstreamHttpStatus: row.UpstreamHTTPStatus,
		RequestBytes: row.RequestBytes, ResponseBytes: row.ResponseBytes, DurationMs: row.DurationMs, ErrorClass: row.ErrorClass,
	}
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
