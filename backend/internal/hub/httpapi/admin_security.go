package httpapi

import (
	"net/http"

	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) DisableUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.DisableUserParams) {
	admin, _, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.DisableUser(r.Context(), admin.ID, params.IdempotencyKey, userID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) EnableUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.EnableUserParams) {
	admin, _, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.EnableUser(r.Context(), admin.ID, params.IdempotencyKey, userID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) RevokeDevice(w http.ResponseWriter, r *http.Request, deviceID adminapi.DeviceId, params adminapi.RevokeDeviceParams) {
	admin, _, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.RevokeDevice(r.Context(), admin.ID, params.IdempotencyKey, deviceID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}
