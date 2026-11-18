package httpapi

import (
	"errors"
	"net/http"

	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.DeleteUserParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.DeleteUserRequest
	if decodeStrictJSON(r, &request) != nil {
		writeProblem(w, 400, "invalid_request", "Invalid deletion request")
		return
	}
	result, err := h.services.RuntimeControl.DeleteUser(r.Context(), admin.UserID, params.IdempotencyKey, userID, runtimecontrol.DeleteUserInput{
		ConfirmationUsername: request.ConfirmationUsername, Reason: request.Reason,
	})
	switch {
	case errors.Is(err, runtimecontrol.ErrDeleteConfirmation):
		writeProblem(w, 409, "delete_confirmation_mismatch", "Confirmation username does not match")
		return
	case errors.Is(err, runtimecontrol.ErrDeleteSelf):
		writeProblem(w, 409, "cannot_delete_current_admin", "Sign in as another administrator to delete this account")
		return
	case errors.Is(err, runtimecontrol.ErrDeleteLastAdmin):
		writeProblem(w, 409, "cannot_delete_last_admin", "The last administrator cannot be deleted")
		return
	case errors.Is(err, runtimecontrol.ErrDeleteInFlight):
		writeProblem(w, 409, "user_deletion_in_flight", "Wait for active enterprise requests to finish before deleting this user")
		return
	case err != nil:
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) DisableUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.DisableUserParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.DisableUser(r.Context(), admin.UserID, params.IdempotencyKey, userID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) EnableUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.EnableUserParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.EnableUser(r.Context(), admin.UserID, params.IdempotencyKey, userID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) RevokeDevice(w http.ResponseWriter, r *http.Request, deviceID adminapi.DeviceId, params adminapi.RevokeDeviceParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.RevokeDevice(r.Context(), admin.UserID, params.IdempotencyKey, deviceID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}
