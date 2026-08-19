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
	baseTransport *http.Transport
	transports    sync.Map
}

func NewHandler(store *control.Store) http.Handler {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	return &Handler{store: store, baseTransport: base.Clone()}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := platformid.New(platformid.Request)
	state := h.store.Current()
	if state == nil {
		writeProblem(w, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime control unavailable", requestID, nil, false)
		return
	}

	resourceID, runtimePath, ok := runtimeTarget(r.URL.Path)
	if !ok || !isRuntimeResourceID(resourceID) || !safeRuntimePath(runtimePath) {
		writeProblem(w, http.StatusNotFound, "route_not_found", "Route not found", requestID, nil, false)
		return
	}

	claims, err := h.authenticate(state, bearer(r))
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "invalid_session", "Unauthorized", requestID, nil, false)
		return
	}
	if _, disabled := state.DisabledUsers[claims.Subject]; disabled {
		writeProblem(w, http.StatusForbidden, "user_disabled", "User disabled", requestID, nil, false)
		return
	}
	if _, revoked := state.RevokedDevices[claims.DeviceID]; revoked {
		writeProblem(w, http.StatusForbidden, "device_revoked", "Device revoked", requestID, nil, false)
		return
	}
	if _, revoked := state.RevokedSessions[claims.SessionID]; revoked {
		writeProblem(w, http.StatusUnauthorized, "invalid_session", "Session revoked", requestID, nil, false)
		return
	}

	generation, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Measix-Managed-Generation")))
	if err != nil || generation < 0 {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid managed generation", requestID, nil, false)
		return
	}
	if generation < state.ActiveManagedGeneration {
		target := state.ActiveManagedGeneration
		writeProblem(w, http.StatusPreconditionRequired, "managed_snapshot_required", "Managed snapshot required", requestID, &target, false)
		return
	}
	if generation > state.ActiveManagedGeneration {
		writeProblem(w, http.StatusConflict, "managed_generation_ahead", "Managed generation ahead", requestID, nil, false)
		return
	}
	if platformid.Validate(platformid.Interaction, r.Header.Get("X-Measix-Interaction-Id")) != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid interaction id", requestID, nil, false)
		return
	}

	routeID, exists := state.ResourceRoutes[resourceID]
	if !exists {
		writeProblem(w, http.StatusForbidden, "resource_not_allowed", "Resource not allowed", requestID, nil, false)
		return
	}
	route, exists := state.Routes[routeID]
	if !exists {
		writeProblem(w, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime route unavailable", requestID, nil, false)
		return
	}
	if _, allowed := route.AllowedMethods[r.Method]; !allowed || !allowedPath(runtimePath, route.AllowedPathPrefixes) {
		writeProblem(w, http.StatusForbidden, "resource_not_allowed", "Route policy denied request", requestID, nil, false)
		return
	}
	upstream, exists := state.Upstreams[route.UpstreamID]
	if !exists || !upstream.Enabled {
		writeProblem(w, http.StatusServiceUnavailable, "upstream_unavailable", "Upstream unavailable", requestID, nil, false)
		return
	}

	maxRequestBytes := int64(state.OperationalLimits.MaxRequestBytes)
	if r.ContentLength > maxRequestBytes {
		writeProblem(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", requestID, nil, false)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	}

	h.serveProxy(w, r, route, upstream, runtimePath, requestID)
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
