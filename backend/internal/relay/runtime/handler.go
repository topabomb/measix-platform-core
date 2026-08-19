package runtime

import (
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

const runtimePrefix = "/runtime/v1/resources/"

type Handler struct {
	store     *control.Store
	transport http.RoundTripper
}

func NewHandler(store *control.Store) http.Handler {
	return &Handler{store: store, transport: http.DefaultTransport}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	state := h.store.Current()
	if state == nil {
		writeProblem(w, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime control unavailable", "", nil)
		return
	}

	resourceID, runtimePath, ok := runtimeTarget(r.URL.Path)
	if !ok || !isRuntimeResourceID(resourceID) || !safeRuntimePath(runtimePath) {
		writeProblem(w, http.StatusNotFound, "route_not_found", "Route not found", "", nil)
		return
	}

	claims, err := h.authenticate(state, bearer(r))
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "invalid_session", "Unauthorized", "", nil)
		return
	}
	if _, disabled := state.DisabledUsers[claims.Subject]; disabled {
		writeProblem(w, http.StatusForbidden, "user_disabled", "User disabled", "", nil)
		return
	}
	if _, revoked := state.RevokedDevices[claims.DeviceID]; revoked {
		writeProblem(w, http.StatusForbidden, "device_revoked", "Device revoked", "", nil)
		return
	}
	if _, revoked := state.RevokedSessions[claims.SessionID]; revoked {
		writeProblem(w, http.StatusUnauthorized, "invalid_session", "Session revoked", "", nil)
		return
	}

	generation, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Measix-Managed-Generation")))
	if err != nil || generation < 0 {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid managed generation", "", nil)
		return
	}
	if generation < state.ActiveManagedGeneration {
		target := state.ActiveManagedGeneration
		writeProblem(w, http.StatusPreconditionRequired, "managed_snapshot_required", "Managed snapshot required", "", &target)
		return
	}
	if generation > state.ActiveManagedGeneration {
		writeProblem(w, http.StatusConflict, "managed_generation_ahead", "Managed generation ahead", "", nil)
		return
	}
	if platformid.Validate(platformid.Interaction, r.Header.Get("X-Measix-Interaction-Id")) != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid interaction id", "", nil)
		return
	}

	routeID, exists := state.ResourceRoutes[resourceID]
	if !exists {
		writeProblem(w, http.StatusForbidden, "resource_not_allowed", "Resource not allowed", "", nil)
		return
	}
	route, exists := state.Routes[routeID]
	if !exists {
		writeProblem(w, http.StatusServiceUnavailable, "runtime_control_unavailable", "Runtime route unavailable", "", nil)
		return
	}
	if _, allowed := route.AllowedMethods[r.Method]; !allowed || !allowedPath(runtimePath, route.AllowedPathPrefixes) {
		writeProblem(w, http.StatusForbidden, "resource_not_allowed", "Route policy denied request", "", nil)
		return
	}
	upstream, exists := state.Upstreams[route.UpstreamID]
	if !exists || !upstream.Enabled {
		writeProblem(w, http.StatusServiceUnavailable, "upstream_unavailable", "Upstream unavailable", "", nil)
		return
	}
	if r.ContentLength > int64(state.OperationalLimits.MaxRequestBytes) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", "", nil)
		return
	}

	requestID := platformid.New(platformid.Request)
	target := targetURL(upstream.BaseURL, runtimePath, r.URL.RawQuery)
	outbound := r.Clone(r.Context())
	outbound.URL = target
	outbound.RequestURI = ""
	outbound.Host = ""
	outbound.Header = r.Header.Clone()
	sanitizeOutboundHeaders(outbound.Header)
	outbound.Header.Set("X-Measix-Request-Id", requestID)
	applyUpstreamAuth(outbound, upstream.Auth)

	response, err := h.transport.RoundTrip(outbound)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "upstream_unavailable", "Upstream unavailable", requestID, nil)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 && response.Header.Get("Location") != "" {
		writeProblem(w, http.StatusBadGateway, "upstream_protocol_error", "Upstream redirect is not allowed", requestID, nil)
		return
	}

	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("X-Measix-Request-Id", requestID)
	w.WriteHeader(response.StatusCode)
	writer := io.Writer(w)
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		writer = flushWriter{w: w}
	}
	_, _ = io.Copy(writer, response.Body)
}

func (h *Handler) authenticate(state *control.State, value string) (*security.AccessClaims, error) {
	claims := &security.AccessClaims{}
	token, err := jwt.ParseWithClaims(
		value,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodEdDSA {
				return nil, jwt.ErrSignatureInvalid
			}
			kid, _ := token.Header["kid"].(string)
			key, ok := state.AuthKeys[kid]
			if !ok || len(key) != ed25519.PublicKeySize {
				return nil, jwt.ErrTokenUnverifiable
			}
			return key, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(state.DeploymentID),
		jwt.WithAudience("runtime"),
		jwt.WithTimeFunc(h.store.Now),
	)
	if err != nil || !token.Valid || claims.DeploymentID != state.DeploymentID {
		return nil, jwt.ErrTokenInvalidClaims
	}
	if platformid.Validate(platformid.User, claims.Subject) != nil || platformid.Validate(platformid.Device, claims.DeviceID) != nil || platformid.Validate(platformid.Session, claims.SessionID) != nil {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

func bearer(r *http.Request) string {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

func runtimeTarget(path string) (resourceID, runtimePath string, ok bool) {
	if !strings.HasPrefix(path, runtimePrefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(path, runtimePrefix)
	separator := strings.IndexByte(remainder, '/')
	if separator <= 0 {
		return "", "", false
	}
	return remainder[:separator], remainder[separator:], true
}

func isRuntimeResourceID(value string) bool {
	kind, err := platformid.KindOf(value)
	if err != nil {
		return false
	}
	switch kind {
	case platformid.Model, platformid.TTS, platformid.ASR, platformid.MCP:
		return true
	default:
		return false
	}
}

func safeRuntimePath(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") || strings.Contains(value, "\\") {
		return false
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func allowedPath(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix) || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func targetURL(base *url.URL, runtimePath, rawQuery string) *url.URL {
	target := *base
	target.Path = strings.TrimRight(base.Path, "/") + runtimePath
	target.RawPath = ""
	target.RawQuery = rawQuery
	target.Fragment = ""
	return &target
}

func sanitizeOutboundHeaders(header http.Header) {
	connectionTokens := strings.Split(header.Get("Connection"), ",")
	for _, token := range connectionTokens {
		header.Del(strings.TrimSpace(token))
	}
	for key := range header {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "cookie" || lower == "host" || strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "x-measix-") || hopByHop(lower) {
			header.Del(key)
		}
	}
}

func applyUpstreamAuth(request *http.Request, auth control.UpstreamAuth) {
	switch auth.Type {
	case relaycontrolapi.BEARER:
		request.Header.Set("Authorization", "Bearer "+auth.Token)
	case relaycontrolapi.STATICHEADER:
		request.Header.Set(auth.HeaderName, auth.Value)
	case relaycontrolapi.BASIC:
		request.SetBasicAuth(auth.Username, auth.Password)
	}
}

func copyResponseHeaders(target, source http.Header) {
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower == "set-cookie" || strings.HasPrefix(lower, "x-measix-") || hopByHop(lower) {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func hopByHop(lower string) bool {
	switch lower {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

type flushWriter struct{ w http.ResponseWriter }

func (w flushWriter) Write(value []byte) (int, error) {
	n, err := w.w.Write(value)
	if flusher, ok := w.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

func writeProblem(w http.ResponseWriter, status int, code, title, requestID string, targetGeneration *int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	problem := relaycontrolapi.Problem{Type: "about:blank", Title: title, Status: status, Code: code}
	if requestID != "" {
		problem.RequestId = &requestID
	}
	problem.TargetManagedGeneration = targetGeneration
	forwarded := false
	problem.Forwarded = &forwarded
	_ = json.NewEncoder(w).Encode(problem)
}
