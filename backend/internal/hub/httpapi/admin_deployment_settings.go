package httpapi

import (
	"errors"
	"net/http"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) GetDeploymentSettings(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	settings, err := h.identity.DeploymentSettings(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, deploymentSettingsWire(settings))
}

func (h *fullAdminHandler) UpdateDeploymentSettings(w http.ResponseWriter, r *http.Request, params adminapi.UpdateDeploymentSettingsParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.UpdateDeploymentSettingsRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	settings, err := h.identity.UpdateDeploymentSettings(r.Context(), request.Name, request.PublicOrigin, request.ExpectedUpdatedAt, admin.UserID)
	if errors.Is(err, identity.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid deployment settings")
		return
	}
	if errors.Is(err, identity.ErrConflict) {
		writeProblem(w, http.StatusConflict, "revision_conflict", "Deployment settings changed; reload and try again")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, deploymentSettingsWire(settings))
}

func deploymentSettingsWire(settings identity.DeploymentSettingsView) adminapi.DeploymentSettings {
	return adminapi.DeploymentSettings{
		DeploymentId: settings.DeploymentID,
		Name:         settings.Name,
		Timezone:     settings.Timezone,
		PublicOrigin: settings.PublicOrigin,
		UpdatedAt:    settings.UpdatedAt.Round(0).In(time.UTC),
	}
}
