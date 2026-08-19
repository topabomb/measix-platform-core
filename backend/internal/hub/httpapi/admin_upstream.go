package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) CreateSecret(w http.ResponseWriter, r *http.Request, params adminapi.CreateSecretParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.CreateSecretRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	view, err := h.services.Upstream.CreateSecret(r.Context(), admin.UserID, request.Name, request.Value)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_secret", "Invalid secret")
		return
	}
	writeJSON(w, http.StatusCreated, adminapi.Secret{SecretId: view.SecretID, Name: view.Name, SecretVersion: view.SecretVersion})
}

func (h *fullAdminHandler) ReplaceSecret(w http.ResponseWriter, r *http.Request, secretID adminapi.SecretId, params adminapi.ReplaceSecretParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.ReplaceSecretRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	view, err := h.services.Upstream.ReplaceSecret(r.Context(), admin.UserID, secretID, request.ExpectedSecretVersion, request.Value)
	if errors.Is(err, upstream.ErrRevisionConflict) {
		writeProblem(w, http.StatusConflict, "secret_revision_conflict", "Secret revision conflict")
		return
	}
	if errors.Is(err, upstream.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "secret_not_found", "Secret not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_secret", "Invalid secret")
		return
	}
	writeJSON(w, http.StatusOK, adminapi.Secret{SecretId: view.SecretID, Name: view.Name, SecretVersion: view.SecretVersion})
}

func (h *fullAdminHandler) ListUpstreams(w http.ResponseWriter, r *http.Request, params adminapi.ListUpstreamsParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := h.services.Upstream.ListUpstreams(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]adminapi.Upstream, 0, len(rows))
	for _, row := range rows {
		items = append(items, upstreamWire(row))
	}
	writeJSON(w, http.StatusOK, adminapi.UpstreamPage{Items: items})
}

func (h *fullAdminHandler) CreateUpstream(w http.ResponseWriter, r *http.Request, params adminapi.CreateUpstreamParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.CreateUpstreamRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	view, err := h.services.Upstream.CreateUpstream(r.Context(), admin.UserID, request.Config)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_upstream", "Invalid upstream")
		return
	}
	writeJSON(w, http.StatusCreated, upstreamWire(view))
}

func (h *fullAdminHandler) GetUpstream(w http.ResponseWriter, r *http.Request, upstreamID adminapi.UpstreamId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	view, err := h.services.Upstream.GetUpstream(r.Context(), upstreamID)
	if errors.Is(err, upstream.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "upstream_not_found", "Upstream not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, upstreamWire(view))
}

func (h *fullAdminHandler) UpdateUpstream(w http.ResponseWriter, r *http.Request, upstreamID adminapi.UpstreamId, params adminapi.UpdateUpstreamParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.UpdateUpstreamRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	view, err := h.services.Upstream.UpdateUpstream(r.Context(), admin.UserID, upstreamID, request.ExpectedConfigRevision, request.Config)
	if errors.Is(err, upstream.ErrRevisionConflict) {
		writeProblem(w, http.StatusConflict, "upstream_revision_conflict", "Upstream revision conflict")
		return
	}
	if errors.Is(err, upstream.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "upstream_not_found", "Upstream not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_upstream", "Invalid upstream")
		return
	}
	writeJSON(w, http.StatusOK, upstreamWire(view))
}

func (h *fullAdminHandler) TestUpstream(w http.ResponseWriter, r *http.Request, upstreamID adminapi.UpstreamId, params adminapi.TestUpstreamParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	view, err := h.services.Upstream.GetUpstream(r.Context(), upstreamID)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "upstream_not_found", "Upstream not found")
		return
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(view.Config.TimeoutDefaults.ConnectMs+view.Config.TimeoutDefaults.ResponseHeaderMs)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, view.Config.BaseUrl, nil)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_upstream", "Invalid upstream")
		return
	}
	response, err := http.DefaultClient.Do(request)
	latency := int(time.Since(started).Milliseconds())
	result := adminapi.UpstreamTestResult{Reachable: err == nil, LatencyMs: &latency, VerifiedCapabilities: []string{}, Warnings: []string{}}
	if response != nil {
		_ = response.Body.Close()
		if response.StatusCode >= 500 {
			result.Warnings = append(result.Warnings, "upstream_http_5xx")
		}
	}
	if err == nil {
		result.VerifiedCapabilities = append(result.VerifiedCapabilities, view.Config.TransportCapabilities...)
	} else {
		result.Warnings = append(result.Warnings, "upstream_unreachable")
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *fullAdminHandler) ApplyUpstream(w http.ResponseWriter, r *http.Request, upstreamID adminapi.UpstreamId, params adminapi.ApplyUpstreamParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.ApplyUpstream(r.Context(), admin.UserID, params.IdempotencyKey, upstreamID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func upstreamWire(view upstream.UpstreamView) adminapi.Upstream {
	return adminapi.Upstream{
		UpstreamId: view.UpstreamID, Name: view.Name, ConfigRevision: view.ConfigRevision,
		ActiveConfigRevision: view.ActiveConfigRevision, Status: adminapi.UpstreamStatus(view.Status), Config: &view.Config,
	}
}

var _ = runtimecontrol.ErrRelayAckMismatch
