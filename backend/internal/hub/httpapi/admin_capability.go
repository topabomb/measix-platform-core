package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"measix/platform/ent"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) GetDraft(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	view, err := h.services.Capability.GetDraft(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, adminapi.Draft{DraftId: view.DraftID, DraftRevision: view.DraftRevision, Content: view.Content})
}

func (h *fullAdminHandler) PutDraft(w http.ResponseWriter, r *http.Request, params adminapi.PutDraftParams) {
	admin, _, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.PutDraftRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	view, err := h.services.Capability.PutDraft(r.Context(), admin.ID, request.ExpectedDraftRevision, request.Content)
	if errors.Is(err, capability.ErrRevisionConflict) {
		writeProblem(w, http.StatusConflict, "stale_draft_revision", "Draft revision conflict")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_draft", "Invalid draft")
		return
	}
	writeJSON(w, http.StatusOK, adminapi.Draft{DraftId: view.DraftID, DraftRevision: view.DraftRevision, Content: view.Content})
}

func (h *fullAdminHandler) ValidateDraft(w http.ResponseWriter, r *http.Request, params adminapi.ValidateDraftParams) {
	if _, _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.ValidateDraftRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	result, err := h.services.Capability.ValidateDraft(r.Context(), request.ExpectedDraftRevision)
	if errors.Is(err, capability.ErrRevisionConflict) {
		writeProblem(w, http.StatusConflict, "stale_draft_revision", "Draft revision conflict")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, adminapi.ValidateDraftResponse{Valid: result.Valid, Errors: result.Errors, Warnings: result.Warnings})
}

func (h *fullAdminHandler) PublishDraft(w http.ResponseWriter, r *http.Request, params adminapi.PublishDraftParams) {
	admin, _, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.PublishDraftRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	result, err := h.services.RuntimeControl.Publish(r.Context(), runtimecontrol.PublishRequest{
		AdminUserID: admin.ID, IdempotencyKey: params.IdempotencyKey,
		ExpectedDraftRevision: request.ExpectedDraftRevision, AcknowledgedWarnings: request.AcknowledgedWarningCodes,
	})
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) ListReleases(w http.ResponseWriter, r *http.Request, params adminapi.ListReleasesParams) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}
	rows, err := h.services.Identity.Client.ManagedRelease.Query().Order(ent.Desc(managedrelease.FieldManagedGeneration)).Limit(limit).All(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items := make([]adminapi.Release, 0, len(rows))
	for _, row := range rows {
		items = append(items, releaseWire(row))
	}
	writeJSON(w, http.StatusOK, adminapi.ReleasePage{Items: items})
}

func (h *fullAdminHandler) GetRelease(w http.ResponseWriter, r *http.Request, releaseID adminapi.ReleaseId) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	row, err := h.services.Identity.Client.ManagedRelease.Get(r.Context(), releaseID)
	if ent.IsNotFound(err) {
		writeProblem(w, http.StatusNotFound, "release_not_found", "Release not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, releaseWire(row))
}

func (h *fullAdminHandler) GetActivation(w http.ResponseWriter, r *http.Request, activationID adminapi.ActivationId) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	row, err := h.services.Identity.Client.Activation.Get(r.Context(), activationID)
	if ent.IsNotFound(err) {
		writeProblem(w, http.StatusNotFound, "activation_not_found", "Activation not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	result := runtimecontrol.ActivationResult{
		ActivationID: row.ID, Kind: row.Kind, State: row.State, DesiredControlRevision: int(row.ControlRevision),
		BundleHash: row.BundleHash, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt, ErrorCode: row.ErrorCode,
	}
	if row.SubjectID != nil {
		result.ReleaseID = *row.SubjectID
	}
	if row.TargetGeneration != nil {
		result.TargetManagedGeneration = int(*row.TargetGeneration)
	}
	writeJSON(w, http.StatusOK, activationWire(result))
}

func (h *fullAdminHandler) RepublishRelease(w http.ResponseWriter, r *http.Request, releaseID adminapi.ReleaseId, params adminapi.RepublishReleaseParams) {
	admin, _, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.Republish(r.Context(), admin.ID, params.IdempotencyKey, releaseID)
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func activationWire(result runtimecontrol.ActivationResult) adminapi.Activation {
	updated := result.CreatedAt
	if result.CompletedAt != nil {
		updated = *result.CompletedAt
	}
	wire := adminapi.Activation{
		ActivationId: result.ActivationID, Kind: adminapi.ActivationKind(result.Kind), State: adminapi.ActivationState(result.State),
		DesiredControlRevision: result.DesiredControlRevision, CreatedAt: result.CreatedAt, UpdatedAt: updated, ErrorCode: result.ErrorCode,
	}
	if result.BundleHash != "" {
		hash := adminapi.Sha256Hash(result.BundleHash)
		wire.BundleHash = &hash
	}
	if result.ReleaseID != "" {
		id := adminapi.ReleaseId(result.ReleaseID)
		wire.ReleaseId = &id
	}
	if result.TargetManagedGeneration > 0 {
		value := result.TargetManagedGeneration
		wire.TargetManagedGeneration = &value
	}
	return wire
}

func releaseWire(row *ent.ManagedRelease) adminapi.Release {
	return adminapi.Release{
		ReleaseId: row.ID, ManagedGeneration: int(row.ManagedGeneration), SnapshotHash: row.SnapshotHash,
		Status: adminapi.ReleaseStatus(row.Status), CreatedAt: row.CreatedAt,
	}
}

func writeRuntimeControlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, runtimecontrol.ErrIdempotencyConflict):
		writeProblem(w, http.StatusConflict, "idempotency_conflict", "Idempotency conflict")
	case errors.Is(err, runtimecontrol.ErrActivationInProgress):
		writeProblem(w, http.StatusConflict, "activation_in_progress", "Activation in progress")
	case errors.Is(err, capability.ErrRevisionConflict):
		writeProblem(w, http.StatusConflict, "stale_draft_revision", "Draft revision conflict")
	case errors.Is(err, capability.ErrInvalidDraft):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_draft", "Invalid draft")
	default:
		writeProblem(w, http.StatusServiceUnavailable, "runtime_activation_failed", "Runtime activation failed")
	}
}

var _ = json.Valid
