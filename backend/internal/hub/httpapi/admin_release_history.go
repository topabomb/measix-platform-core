package httpapi

import (
	"errors"
	"measix/platform/ent"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"net/http"
)

func writeHistoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, capability.ErrCleanupSelection):
		writeProblem(w, 400, "invalid_cleanup_selection", "Choose one valid cleanup mode or retention rule")
	case errors.Is(err, capability.ErrCleanupStale):
		writeProblem(w, 409, "stale_release_cleanup_preview", "History changed; preview again before cleaning")
	case errors.Is(err, capability.ErrRetentionRevision):
		writeProblem(w, 409, "stale_release_retention_revision", "Retention rule changed; refresh before saving")
	case errors.Is(err, capability.ErrCleanupBusy):
		writeProblem(w, 409, "activation_in_progress", "Wait for runtime activation to settle")
	default:
		writeIdentityError(w, err)
	}
}
func (h *fullAdminHandler) GetReleaseRetention(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	out, err := h.services.Capability.GetReleaseRetention(r.Context())
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) UpdateReleaseRetention(w http.ResponseWriter, r *http.Request, params adminapi.UpdateReleaseRetentionParams) {
	actor, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var req adminapi.UpdateReleaseRetentionRequest
	if decodeStrictJSON(r, &req) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	out, err := h.services.Capability.UpdateReleaseRetention(r.Context(), actor.UserID, req)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) PreviewReleaseCleanup(w http.ResponseWriter, r *http.Request, params adminapi.PreviewReleaseCleanupParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var req adminapi.ReleaseCleanupSelection
	if decodeStrictJSON(r, &req) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	out, err := h.services.Capability.PreviewReleaseCleanup(r.Context(), req)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) ExecuteReleaseCleanup(w http.ResponseWriter, r *http.Request, params adminapi.ExecuteReleaseCleanupParams) {
	actor, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var req adminapi.ExecuteReleaseCleanupRequest
	if decodeStrictJSON(r, &req) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid request")
		return
	}
	out, err := h.services.Capability.ExecuteReleaseCleanup(r.Context(), actor.UserID, req)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) ListReleaseHistoryAudit(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	out, err := h.services.Capability.ListReleaseHistoryAudit(r.Context())
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) GetUpstreamReferences(w http.ResponseWriter, r *http.Request, id adminapi.UpstreamId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	out, err := h.services.Upstream.GetReferences(r.Context(), id)
	if ent.IsNotFound(err) {
		writeProblem(w, 404, "upstream_not_found", "Upstream not found")
		return
	}
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
