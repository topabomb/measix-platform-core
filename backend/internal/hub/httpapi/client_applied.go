package httpapi

import (
	"errors"
	"net/http"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/wire/clientapi"
)

func (h *clientHandler) ReportManagedApplied(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, 401, "unauthorized", "Unauthorized")
		return
	}
	var report clientapi.ManagedAppliedReport
	if err := decodeStrictJSON(r, &report); err != nil {
		writeProblem(w, 400, "invalid_request", "Invalid application report")
		return
	}
	err := h.identity.ReportManagedApplied(r.Context(), token, report.ManagedGeneration, report.SnapshotHash)
	switch {
	case errors.Is(err, identity.ErrAppliedRelease):
		writeProblem(w, 422, "applied_release_mismatch", "Application report does not match a published release")
	case errors.Is(err, identity.ErrAppliedRegression):
		writeProblem(w, 409, "applied_generation_regression", "Application report is older than the current session report")
	case err != nil:
		writeIdentityError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
