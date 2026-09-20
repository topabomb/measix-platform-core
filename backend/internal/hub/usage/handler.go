package usage

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/wire/usageingestapi"
)

const maxUsageBatchBody = 2 << 20

type Handler struct {
	usageingestapi.Unimplemented
	usage        *Service
	budget       *budget.Service
	serviceToken string
}

func NewHandler(usageService *Service, budgetService *budget.Service, serviceToken string) *Handler {
	return &Handler{usage: usageService, budget: budgetService, serviceToken: serviceToken}
}

func (h *Handler) AdmitBudget(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	var input usageingestapi.BudgetAdmissionRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	known, err := budgetMeterValues(pointerValues(input.KnownUsage))
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid budget admission")
		return
	}
	supported := make([]budget.Meter, 0, len(input.SupportedMeters))
	for _, meter := range input.SupportedMeters {
		supported = append(supported, budgetMeter(meter))
	}
	decision, err := h.budget.Admit(r.Context(), budget.AdmitInput{
		RequestID: input.RequestId, RequestHash: input.RequestHash, DeploymentID: input.DeploymentId, UserID: input.UserId,
		InteractionID: input.InteractionId, DeviceID: input.DeviceId, Capability: budget.Capability(input.ResourceKind),
		ResourceID: input.ResourceId, ClientProtocol: budget.ClientProtocol(input.ClientProtocol), UpstreamID: input.UpstreamId,
		ManagedGeneration: int64(input.ManagedGeneration), ControlRevision: int64(input.ControlRevision), AdmittedAt: input.AdmittedAt,
		KnownQuantities: known, SupportedMeters: supported,
	})
	if err != nil {
		status, code := http.StatusUnprocessableEntity, "invalid_request"
		if errors.Is(err, budget.ErrIdentityDeleted) {
			status, code = http.StatusUnauthorized, "enterprise_identity_deleted"
		} else if errors.Is(err, budget.ErrRequestConflict) {
			status, code = http.StatusConflict, "request_conflict"
		}
		writeProblem(w, status, code, "Budget admission failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toWireDecision(decision))
}

func (h *Handler) MarkBudgetRequestStarted(w http.ResponseWriter, r *http.Request, requestID usageingestapi.RequestId) {
	if !h.authorize(w, r) {
		return
	}
	var input usageingestapi.BudgetLifecycleEvent
	if !decodeJSON(w, r, &input) {
		return
	}
	err := h.budget.Start(r.Context(), budget.LifecycleInput{RequestID: requestID, Revision: int64(input.Revision), OccurredAt: input.OccurredAt, EventHash: input.EventHash})
	if err != nil {
		writeLifecycleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReleaseBudgetRequest(w http.ResponseWriter, r *http.Request, requestID usageingestapi.RequestId) {
	if !h.authorize(w, r) {
		return
	}
	var input usageingestapi.BudgetReleaseRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	err := h.budget.Release(r.Context(), budget.ReleaseInput{LifecycleInput: budget.LifecycleInput{
		RequestID: requestID, Revision: int64(input.Revision), OccurredAt: input.OccurredAt, EventHash: input.EventHash,
	}, Reason: input.Reason})
	if err != nil {
		writeLifecycleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) IngestUsageSettlements(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	var batch usageingestapi.UsageSettlementBatch
	if !decodeJSON(w, r, &batch) {
		return
	}
	ack, err := h.usage.IngestSettlements(r.Context(), batch)
	if err != nil {
		switch {
		case errors.Is(err, ErrSettlementConflict), errors.Is(err, budget.ErrSettlementRevisionConflict):
			writeProblem(w, http.StatusConflict, "settlement_conflict", "Usage settlement conflicts with an existing revision")
		case errors.Is(err, ErrInvalidBatch), errors.Is(err, ErrAttributionMismatch), errors.Is(err, budget.ErrInvalidSettlement), errors.Is(err, budget.ErrRequestNotStarted):
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_usage_settlement", "Invalid usage settlement")
		default:
			writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ack)
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.usage == nil || h.budget == nil || h.serviceToken == "" {
		writeProblem(w, http.StatusServiceUnavailable, "service_unavailable", "Usage and budget service unavailable")
		return false
	}
	if !validServiceBearer(r.Header.Get("Authorization"), h.serviceToken) {
		writeProblem(w, http.StatusUnauthorized, "invalid_service_credential", "Unauthorized")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid request")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUsageBatchBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || ensureJSONEOF(decoder) != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid request")
		return false
	}
	return true
}

func toWireDecision(input budget.AdmissionDecision) usageingestapi.BudgetAdmissionDecision {
	blocking := make([]usageingestapi.BudgetLimitState, 0, len(input.BlockingLimits))
	for _, limit := range input.BlockingLimits {
		meter, divisor := usageingestapi.UsageMeter(limit.Meter), int64(1)
		if limit.Meter == budget.MeterAudioMilliseconds {
			meter, divisor = usageingestapi.AUDIOSECONDS, 1000
		}
		blocking = append(blocking, usageingestapi.BudgetLimitState{
			Meter: meter, Period: usageingestapi.BudgetPeriod(limit.Period), Limit: ceilDiv(limit.Limit, divisor),
			Used: ceilDiv(limit.Used, divisor), Reserved: ceilDiv(limit.Reserved, divisor), ResetAt: limit.ResetAt,
		})
	}
	unavailable := make([]usageingestapi.UsageMeter, 0, len(input.Unavailable))
	for _, meter := range input.Unavailable {
		if meter == budget.MeterAudioMilliseconds {
			unavailable = append(unavailable, usageingestapi.AUDIOSECONDS)
		} else {
			unavailable = append(unavailable, usageingestapi.UsageMeter(meter))
		}
	}
	result := usageingestapi.BudgetAdmissionDecision{
		RequestId: input.RequestID, Allowed: input.Allowed, Code: usageingestapi.BudgetAdmissionDecisionCode(input.Code),
		Mode: usageingestapi.BudgetMode(input.Mode), Source: usageingestapi.BudgetSource(input.Source), Revision: int(input.Revision),
		InFlightRequests: int(input.InFlightRequests), BlockingLimits: blocking, ResetAt: input.ResetAt, AsOf: input.AsOf,
	}
	if len(unavailable) > 0 {
		result.UnavailableMeters = &unavailable
	}
	return result
}

func pointerValues(values *[]usageingestapi.MeterValue) []usageingestapi.MeterValue {
	if values == nil {
		return nil
	}
	return *values
}

func ceilDiv(value, divisor int64) int64 {
	if value == 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

func writeLifecycleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, budget.ErrRequestNotFound):
		writeProblem(w, http.StatusNotFound, "request_not_found", "Budget request not found")
	case errors.Is(err, budget.ErrLifecycleRevisionConflict), errors.Is(err, budget.ErrInvalidTransition), errors.Is(err, budget.ErrAlreadyForwarded):
		writeProblem(w, http.StatusConflict, "lifecycle_conflict", "Budget lifecycle conflict")
	default:
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

func validServiceBearer(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	actual := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return len(actual) == len(expected) && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func writeProblem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(usageingestapi.Problem{Type: "about:blank", Title: title, Status: status, Code: code})
}
