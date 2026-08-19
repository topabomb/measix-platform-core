package runtime

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

type Handler struct {
	store         *control.Store
	recorder      UsageRecorder
	baseTransport *http.Transport
	transports    sync.Map
}

func NewHandler(store *control.Store) http.Handler {
	return NewHandlerWithRecorder(store, nil)
}

func NewHandlerWithRecorder(store *control.Store, recorder UsageRecorder) http.Handler {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	return &Handler{store: store, recorder: recorder, baseTransport: base.Clone()}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := platformid.New(platformid.Request)
	startedAt := h.store.Now()
	observer := &responseObserver{ResponseWriter: w}
	state := h.store.Current()
	if state == nil {
		writeProblem(observer, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime control unavailable", requestID, nil, false)
		return
	}

	resourceID, runtimePath, ok := runtimeTarget(r.URL.Path)
	if !ok || !isRuntimeResourceID(resourceID) || !safeRuntimePath(runtimePath) {
		writeProblem(observer, http.StatusNotFound, "route_not_found", "Route not found", requestID, nil, false)
		return
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
	meterFailure := func(status int, code, title, errorClass string, target *int) {
		route, upstream, found := usageRoute(state, resourceID)
		writeProblem(observer, status, code, title, requestID, target, false)
		if found {
			h.recordUsage(observer, nil, usageAttribution{
				state: state, claims: claims, resourceID: resourceID, interactionID: interactionID,
				route: route, upstream: upstream, startedAt: startedAt, requestID: requestID,
			}, false, nil, errorClass)
		}
	}

	if _, disabled := state.DisabledUsers[claims.Subject]; disabled {
		meterFailure(http.StatusForbidden, "user_disabled", "User disabled", "USER_DISABLED", nil)
		return
	}
	if _, revoked := state.RevokedDevices[claims.DeviceID]; revoked {
		meterFailure(http.StatusForbidden, "device_revoked", "Device revoked", "DEVICE_REVOKED", nil)
		return
	}
	if _, revoked := state.RevokedSessions[claims.SessionID]; revoked {
		meterFailure(http.StatusUnauthorized, "invalid_session", "Session revoked", "SESSION_REVOKED", nil)
		return
	}

	generation, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Measix-Managed-Generation")))
	if err != nil || generation < 0 {
		meterFailure(http.StatusBadRequest, "invalid_request", "Invalid managed generation", "INVALID_GENERATION", nil)
		return
	}
	if generation < state.ActiveManagedGeneration {
		target := state.ActiveManagedGeneration
		meterFailure(http.StatusPreconditionRequired, "managed_snapshot_required", "Managed snapshot required", "MANAGED_SNAPSHOT_REQUIRED", &target)
		return
	}
	if generation > state.ActiveManagedGeneration {
		meterFailure(http.StatusConflict, "managed_generation_ahead", "Managed generation ahead", "MANAGED_GENERATION_AHEAD", nil)
		return
	}
	if interactionID == nil {
		meterFailure(http.StatusBadRequest, "invalid_request", "Invalid interaction id", "INVALID_INTERACTION", nil)
		return
	}

	routeID, exists := state.ResourceRoutes[resourceID]
	if !exists {
		writeProblem(observer, http.StatusForbidden, "resource_not_allowed", "Resource not allowed", requestID, nil, false)
		return
	}
	route, exists := state.Routes[routeID]
	if !exists {
		writeProblem(observer, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime route unavailable", requestID, nil, false)
		return
	}
	upstream, exists := state.Upstreams[route.UpstreamID]
	if !exists {
		writeProblem(observer, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime upstream unavailable", requestID, nil, false)
		return
	}
	attr := usageAttribution{
		state: state, claims: claims, resourceID: resourceID, interactionID: interactionID,
		route: route, upstream: upstream, startedAt: startedAt, requestID: requestID,
	}
	if _, allowed := route.AllowedMethods[r.Method]; !allowed || !allowedPath(runtimePath, route.AllowedPathPrefixes) {
		writeProblem(observer, http.StatusForbidden, "resource_not_allowed", "Route policy denied request", requestID, nil, false)
		h.recordUsage(observer, nil, attr, false, nil, "ROUTE_POLICY_DENIED")
		return
	}
	if !upstream.Enabled {
		writeProblem(observer, http.StatusServiceUnavailable, "upstream_unavailable", "Upstream unavailable", requestID, nil, false)
		h.recordUsage(observer, nil, attr, false, nil, "UPSTREAM_DISABLED")
		return
	}

	maxRequestBytes := int64(state.OperationalLimits.MaxRequestBytes)
	if r.ContentLength > maxRequestBytes {
		writeProblem(observer, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", requestID, nil, false)
		h.recordUsage(observer, nil, attr, false, nil, "REQUEST_TOO_LARGE")
		return
	}
	var body *countingBody
	if r.Body != nil {
		body = &countingBody{ReadCloser: http.MaxBytesReader(observer, r.Body, maxRequestBytes)}
		r.Body = body
	}

	result := h.serveProxy(observer, r, route, upstream, runtimePath, requestID)
	h.recordUsage(observer, body, attr, true, result.UpstreamStatus, result.ErrorClass)
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
