package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) GetDraft(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
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
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.PutDraftRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	view, err := h.services.Capability.PutDraft(r.Context(), admin.UserID, request.ExpectedDraftRevision, request.Content)
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
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
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

func (h *fullAdminHandler) PreviewDraft(w http.ResponseWriter, r *http.Request, params adminapi.PreviewDraftParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.PreviewDraftRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	preview, err := h.services.Capability.PreviewDraft(r.Context(), request.ExpectedDraftRevision)
	if errors.Is(err, capability.ErrRevisionConflict) {
		writeProblem(w, http.StatusConflict, "stale_draft_revision", "Draft revision conflict")
		return
	}
	if errors.Is(err, capability.ErrInvalidDraft) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_draft", "Invalid draft")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, adminapi.DraftPreviewResponse{
		DraftRevision: preview.DraftRevision,
		SnapshotHash:  adminapi.Sha256Hash(preview.SnapshotHash),
		Providers:     preview.Providers,
		Models:        preview.Models,
		Tts:           preview.TTS,
		Asr:           preview.ASR,
		Mcp:           preview.MCP,
		Policy:        preview.Policy,
	})
}

func (h *fullAdminHandler) PublishDraft(w http.ResponseWriter, r *http.Request, params adminapi.PublishDraftParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
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
		AdminUserID: admin.UserID, IdempotencyKey: params.IdempotencyKey,
		ExpectedDraftRevision: request.ExpectedDraftRevision, AcknowledgedWarnings: request.AcknowledgedWarningCodes,
	})
	if err != nil {
		writeRuntimeControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, activationWire(result))
}

func (h *fullAdminHandler) ListReleases(w http.ResponseWriter, r *http.Request, params adminapi.ListReleasesParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}
	rows, err := h.services.Capability.ListReleases(r.Context(), limit)
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
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	row, err := h.services.Capability.GetRelease(r.Context(), releaseID)
	if errors.Is(err, capability.ErrReleaseNotFound) {
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
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.GetActivation(r.Context(), activationID)
	if entNotFoundActivation(err) {
		writeProblem(w, http.StatusNotFound, "activation_not_found", "Activation not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, activationWire(result))
}

func (h *fullAdminHandler) RepublishRelease(w http.ResponseWriter, r *http.Request, releaseID adminapi.ReleaseId, params adminapi.RepublishReleaseParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	result, err := h.services.RuntimeControl.Republish(r.Context(), admin.UserID, params.IdempotencyKey, releaseID)
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

func releaseWire(row capability.ReleaseView) adminapi.Release {
	return adminapi.Release{
		ReleaseId:           row.ReleaseID,
		ManagedGeneration:   row.ManagedGeneration,
		SnapshotHash:        row.SnapshotHash,
		Status:              adminapi.ReleaseStatus(row.Status),
		CreatedAt:           row.CreatedAt,
		SourceDraftRevision: row.SourceDraftRevision,
		PublishedAt:         row.CreatedAt,
		PublishedBy:         row.PublishedBy,
		DiffSummary:         row.DiffSummary,
		ActivationHistory:   row.ActivationHistory,
	}
}

func entNotFoundActivation(err error) bool { return runtimecontrol.IsActivationNotFound(err) }

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
