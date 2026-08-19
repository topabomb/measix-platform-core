package httpapi

import (
	"errors"
	"net/http"
	"time"

	"measix/platform/ent"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) ListUsageRequests(w http.ResponseWriter, r *http.Request, params adminapi.ListUsageRequestsParams) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}
	rows, err := h.services.Usage.ListRequests(r.Context(), limit)
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
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
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

func (h *fullAdminHandler) UsageSummary(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	to := time.Now().UTC().Add(time.Nanosecond)
	from := time.Unix(0, 0).UTC()
	summary, err := h.services.Usage.Summary(r.Context(), from, to)
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
			Meter      string                                        `json:"meter"`
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
			Meter      string                                        `json:"meter"`
			Quantity   string                                        `json:"quantity"`
		}{Confidence: confidence, Meter: meter.Meter, Quantity: meter.Quantity})
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
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
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
	if _, _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
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
			Meter: rule.Meter, UnitSize: rule.UnitSize, UnitPrice: rule.UnitPrice, Currency: rule.Currency,
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

func requestUsageWire(row *ent.RequestUsage) adminapi.RequestUsageView {
	return adminapi.RequestUsageView{
		RequestId: row.RequestID, InteractionId: row.InteractionID, DeploymentId: row.DeploymentID,
		UserId: row.UserID, DeviceId: row.DeviceID, ResourceId: row.ResourceID, RuntimeRouteId: row.RuntimeRouteID,
		UpstreamId: row.UpstreamID, ManagedGeneration: int(row.ManagedGeneration), ControlRevision: int(row.ControlRevision),
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, Forwarded: row.Forwarded,
		HttpStatus: row.HTTPStatus, UpstreamHttpStatus: row.UpstreamHTTPStatus,
		RequestBytes: int(row.RequestBytes), ResponseBytes: int(row.ResponseBytes), DurationMs: int(row.DurationMs), ErrorClass: row.ErrorClass,
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
			Meter: row.Meter, UnitSize: row.UnitSize, UnitPrice: row.UnitPrice, Currency: row.Currency,
			EffectiveFrom: row.EffectiveFrom,
		})
	}
	return adminapi.PricingSet{PricingRevision: revision, Rules: items}
}
