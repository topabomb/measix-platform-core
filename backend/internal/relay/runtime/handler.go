package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	relaybudget "measix/platform/internal/relay/budget"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

type Handler struct {
	store         *control.Store
	recorder      UsageRecorder
	budget        relaybudget.Client
	baseTransport *http.Transport
	transports    sync.Map
}

const (
	runtimeMaxIdleConnections        = 256
	runtimeMaxIdleConnectionsPerHost = 128
)

func NewHandler(store *control.Store, recorder UsageRecorder, budgetClient relaybudget.Client) http.Handler {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	base = base.Clone()
	// Runtime responses are protocol payloads. Automatic gzip negotiation would
	// make net/http decompress and rewrite the provider response before the
	// Relay can forward it, violating byte-transparent transport.
	base.DisableCompression = true
	base.ForceAttemptHTTP2 = true
	if base.MaxIdleConns < runtimeMaxIdleConnections {
		base.MaxIdleConns = runtimeMaxIdleConnections
	}
	if base.MaxIdleConnsPerHost < runtimeMaxIdleConnectionsPerHost {
		base.MaxIdleConnsPerHost = runtimeMaxIdleConnectionsPerHost
	}
	return &Handler{store: store, recorder: recorder, budget: budgetClient, baseTransport: base}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := platformid.New(platformid.Request)
	admittedAt := h.store.Now()
	observer := &responseObserver{ResponseWriter: w}
	state := h.store.Current()
	if state == nil {
		writeProblem(observer, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime control unavailable", requestID, nil, false)
		return
	}

	resourceID, runtimePath, ok := runtimeTarget(r.URL.Path)
	validTarget := ok && isRuntimeResourceID(resourceID) && safeRuntimePath(runtimePath)
	if !isRuntimeResourceID(resourceID) {
		resourceID = ""
	}

	claims, err := h.authenticate(state, bearer(r))
	if err != nil {
		writeProblem(observer, http.StatusUnauthorized, "invalid_session", "Unauthorized", requestID, nil, false)
		return
	}
	interactionValue := r.Header.Get("X-Measix-Interaction-Id")
	var interactionID *string
	if platformid.Validate(platformid.Interaction, interactionValue) == nil {
		interactionID = &interactionValue
	}
	runtimeFailure := func(status int, code, title string, target *int) {
		writeProblem(observer, status, code, title, requestID, target, false)
	}

	if !validTarget {
		runtimeFailure(http.StatusNotFound, "route_not_found", "Route not found", nil)
		return
	}
	if _, deleted := state.DeletedUsers[claims.Subject]; deleted {
		runtimeFailure(http.StatusUnauthorized, "enterprise_identity_deleted", "Enterprise identity was deleted", nil)
		return
	}

	if _, disabled := state.DisabledUsers[claims.Subject]; disabled {
		runtimeFailure(http.StatusForbidden, "user_disabled", "User disabled", nil)
		return
	}
	if _, revoked := state.RevokedDevices[claims.DeviceID]; revoked {
		runtimeFailure(http.StatusForbidden, "device_revoked", "Device revoked", nil)
		return
	}
	if _, revoked := state.RevokedSessions[claims.SessionID]; revoked {
		runtimeFailure(http.StatusForbidden, "session_revoked", "Session revoked", nil)
		return
	}

	generation, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Measix-Managed-Generation")))
	if err != nil || generation < 0 {
		runtimeFailure(http.StatusBadRequest, "invalid_request", "Invalid managed generation", nil)
		return
	}
	if generation < state.ActiveManagedGeneration {
		target := state.ActiveManagedGeneration
		runtimeFailure(http.StatusPreconditionRequired, "managed_snapshot_required", "Managed snapshot required", &target)
		return
	}
	if generation > state.ActiveManagedGeneration {
		runtimeFailure(http.StatusConflict, "managed_generation_ahead", "Managed generation ahead", nil)
		return
	}
	if interactionID == nil {
		runtimeFailure(http.StatusBadRequest, "invalid_request", "Invalid interaction id", nil)
		return
	}

	resource, exists := state.Resources[resourceID]
	if !exists {
		runtimeFailure(http.StatusForbidden, "resource_not_allowed", "Resource not allowed", nil)
		return
	}
	route, exists := state.Routes[resource.RouteID]
	if !exists {
		runtimeFailure(http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime route unavailable", nil)
		return
	}
	upstream, exists := state.Upstreams[route.UpstreamID]
	if !exists {
		runtimeFailure(http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime upstream unavailable", nil)
		return
	}
	attr := usageAttribution{
		state: state, claims: claims, resourceID: resourceID, interactionID: interactionID,
		resource: resource, route: route, upstream: upstream, admittedAt: admittedAt, requestID: requestID,
	}
	_, methodAllowed := route.AllowedMethods[r.Method]
	pathAllowed := allowedPath(runtimePath, route.AllowedPathPrefixes)
	if resource.ModelMapping != nil {
		pathAllowed = runtimePath == resource.ModelMapping.ClientRuntimePath
	}
	if !methodAllowed || !pathAllowed {
		writeProblem(observer, http.StatusForbidden, "resource_not_allowed", "Route policy denied request", requestID, nil, false)
		return
	}
	if !upstream.Enabled {
		writeProblem(observer, http.StatusServiceUnavailable, "upstream_unavailable", "Upstream unavailable", requestID, nil, false)
		return
	}
	wantsUpgrade := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
	if (route.TransportPolicy == relaycontrolapi.WEBSOCKET && (!wantsUpgrade || r.Method != http.MethodGet)) || (route.TransportPolicy != relaycontrolapi.WEBSOCKET && r.Header.Get("Upgrade") != "") {
		writeProblem(observer, http.StatusBadRequest, "invalid_request", "Request does not match route transport", requestID, nil, false)
		return
	}

	maxRequestBytes := int64(state.OperationalLimits.MaxRequestBytes)
	if r.ContentLength > maxRequestBytes {
		writeProblem(observer, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", requestID, nil, false)
		return
	}
	outboundRuntimePath, err := prepareModelRequest(resource, r, runtimePath, maxRequestBytes)
	if err != nil {
		switch {
		case errors.Is(err, errObservedRequestTooLarge):
			writeProblem(observer, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", requestID, nil, false)
		case errors.Is(err, errUnsupportedContentEncoding):
			writeProblem(observer, http.StatusBadRequest, "unsupported_content_encoding", "Compressed model requests are not supported", requestID, nil, false)
		case errors.Is(err, errInvalidModelSelector):
			writeProblem(observer, http.StatusBadRequest, "invalid_model_selector", "Invalid model selector", requestID, nil, false)
		default:
			writeProblem(observer, http.StatusBadRequest, "invalid_model_request", "Invalid model request", requestID, nil, false)
		}
		return
	}
	observation, err := prepareUsageObservation(resource, r, runtimePath, maxRequestBytes)
	if err != nil {
		if errors.Is(err, errObservedRequestTooLarge) {
			writeProblem(observer, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", requestID, nil, false)
		} else if errors.Is(err, errInvalidImageGenerationRequest) {
			writeProblem(observer, http.StatusBadRequest, "invalid_request", "Invalid image generation request", requestID, nil, false)
		} else {
			writeProblem(observer, http.StatusBadRequest, "invalid_request", "Invalid request body", requestID, nil, false)
		}
		return
	}
	observer.observation = observation
	var body *countingBody
	if r.Body != nil {
		body = &countingBody{ReadCloser: http.MaxBytesReader(observer, r.Body, maxRequestBytes), observation: observation}
		r.Body = body
	}
	if h.budget == nil || h.recorder == nil {
		h.serveWithMeteringDegraded(observer, r, route, upstream, outboundRuntimePath, requestID, maxRequestBytes, "budget_or_metering_not_configured")
		return
	}
	admission := admissionRequest(attr, observation, r)
	decision, problem, err := h.budget.Admit(r.Context(), admission)
	if err != nil {
		h.serveWithMeteringDegraded(observer, r, route, upstream, outboundRuntimePath, requestID, maxRequestBytes, "budget_admission_unavailable")
		return
	}
	if problem != nil {
		writeBudgetProblem(observer, *problem, requestID)
		return
	}
	if !decision.Allowed {
		if decision.Code == usageingestapi.USAGEMETERUNAVAILABLE || decision.Code == usageingestapi.INFLIGHTLIMIT {
			h.serveWithMeteringDegraded(observer, r, route, upstream, outboundRuntimePath, requestID, maxRequestBytes, strings.ToLower(string(decision.Code)))
			return
		}
		writeAdmissionDenied(observer, decision, attr, requestID)
		status, code := admissionDeniedStatus(decision)
		_ = h.recordDenied(admission, attr, status, code)
		return
	}
	attr.budgetRevision = decision.Revision
	if err := h.recorder.PersistAdmission(admission, route.ID); err != nil {
		releaseUnforwarded(context.WithoutCancel(r.Context()), h, attr, "admission_journal_failed")
		h.serveWithMeteringDegraded(observer, r, route, upstream, outboundRuntimePath, requestID, maxRequestBytes, "admission_journal_failed")
		return
	}
	attr.startedAt = h.store.Now()
	const startRevision = 1
	eventHash := lifecycleHash(requestID, "start", startRevision, attr.startedAt)
	start := usageingestapi.BudgetLifecycleEvent{EventHash: eventHash, OccurredAt: attr.startedAt, Revision: startRevision}
	if err := h.recorder.MarkStarted(requestID, attr.startedAt); err != nil {
		releaseUnforwarded(context.WithoutCancel(r.Context()), h, attr, "start_journal_failed")
		h.serveWithMeteringDegraded(observer, r, route, upstream, outboundRuntimePath, requestID, maxRequestBytes, "start_journal_failed")
		return
	}
	if err := h.budget.Start(r.Context(), requestID, start); err != nil {
		releaseUnforwarded(context.WithoutCancel(r.Context()), h, attr, "budget_start_failed")
		h.serveWithMeteringDegraded(observer, r, route, upstream, outboundRuntimePath, requestID, maxRequestBytes, "budget_start_unavailable")
		return
	}
	attr.lifecycleRevision = startRevision

	result := &proxyResult{}
	// ReverseProxy aborts an interrupted response with http.ErrAbortHandler.
	// Meter in the unwind path as well, using the state captured at admission.
	defer func() {
		_ = h.recordSettlement(observer, body, observation, attr, true, result.UpstreamStatus, result.ErrorClass)
	}()
	h.serveProxy(observer, r, route, upstream, outboundRuntimePath, requestID, result, maxRequestBytes)
}

func (h *Handler) serveWithMeteringDegraded(w http.ResponseWriter, r *http.Request, route control.Route, upstream control.Upstream, runtimePath, requestID string, maxRequestBytes int64, reason string) {
	slog.Warn("runtime request proceeding while budget or metering is degraded", "event", "runtime.metering_degraded", "requestId", requestID, "reason", reason)
	h.serveProxy(w, r, route, upstream, runtimePath, requestID, &proxyResult{}, maxRequestBytes)
}

func lifecycleHash(requestID, action string, revision int, occurredAt time.Time) string {
	sum := sha256.Sum256([]byte(requestID + "|" + action + "|" + strconv.Itoa(revision) + "|" + occurredAt.UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("sha256:%x", sum[:])
}

func releaseUnforwarded(ctx context.Context, h *Handler, attr usageAttribution, reason string) {
	occurredAt := h.store.Now()
	revision := attr.lifecycleRevision + 1
	release := usageingestapi.BudgetReleaseRequest{
		EventHash:  lifecycleHash(attr.requestID, "release:"+reason, revision, occurredAt),
		OccurredAt: occurredAt, Reason: reason, Revision: revision,
	}
	if err := h.budget.Release(ctx, attr.requestID, release); err == nil {
		_ = h.recorder.AbortAdmission(attr.requestID)
	}
}

func writeAdmissionDenied(w http.ResponseWriter, decision usageingestapi.BudgetAdmissionDecision, attr usageAttribution, requestID string) {
	status, code := admissionDeniedStatus(decision)
	title := "Budget exhausted"
	if code == "usage_meter_unavailable" {
		title = "Required usage meter unavailable"
	} else if code == "in_flight_limit" {
		title = "Too many in-flight requests"
	}
	capability := usageingestapi.BudgetCapability(attr.resource.Kind)
	resourceID := attr.resourceID
	mode := decision.Mode
	blocking := decision.BlockingLimits
	asOf := decision.AsOf
	problem := usageingestapi.Problem{
		Type: "about:blank", Title: title, Status: status, Code: code,
		Budget: &usageingestapi.BudgetContext{
			Capability: &capability, ResourceId: &resourceID, Mode: &mode,
			BlockingLimits: &blocking, ResetAt: decision.ResetAt, AsOf: &asOf,
		},
	}
	writeBudgetProblem(w, problem, requestID)
}

func admissionDeniedStatus(decision usageingestapi.BudgetAdmissionDecision) (int, string) {
	switch decision.Code {
	case usageingestapi.USAGEMETERUNAVAILABLE:
		return http.StatusUnprocessableEntity, "usage_meter_unavailable"
	case usageingestapi.INFLIGHTLIMIT:
		return http.StatusTooManyRequests, "in_flight_limit"
	default:
		return http.StatusTooManyRequests, "budget_exhausted"
	}
}

func writeBudgetProblem(w http.ResponseWriter, problem usageingestapi.Problem, requestID string) {
	forwarded := false
	problem.Forwarded = &forwarded
	problem.RequestId = &requestID
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Measix-Request-Id", requestID)
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}

func writeProblem(w http.ResponseWriter, status int, code, title, requestID string, targetGeneration *int, forwarded bool) {
	w.Header().Set("Content-Type", "application/problem+json")
	if requestID != "" {
		w.Header().Set("X-Measix-Request-Id", requestID)
	}
	w.WriteHeader(status)
	problem := relaycontrolapi.Problem{Type: "about:blank", Title: title, Status: status, Code: code}
	if requestID != "" {
		problem.RequestId = &requestID
	}
	problem.TargetManagedGeneration = targetGeneration
	problem.Forwarded = &forwarded
	_ = json.NewEncoder(w).Encode(problem)
}
