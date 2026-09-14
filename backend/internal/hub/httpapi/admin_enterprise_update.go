package httpapi

import (
	"errors"
	"net/http"

	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) ListEnterpriseUpdates(w http.ResponseWriter, r *http.Request, params adminapi.ListEnterpriseUpdatesParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, after, valid := pageParams(w, r, params.Limit, params.Cursor)
	if !valid {
		return
	}
	items, feedRevision, err := h.services.EnterpriseUpdate.List(r.Context(), limit+1, after)
	if errors.Is(err, enterpriseupdate.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid enterprise update")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	items, next := pageResult(r, items, limit, func(v enterpriseupdate.UpdateView) string { return v.ID })
	updates := make([]adminapi.EnterpriseUpdate, 0, len(items))
	for _, item := range items {
		updates = append(updates, enterpriseUpdateWire(item))
	}
	writeJSON(w, http.StatusOK, adminapi.EnterpriseUpdatePage{
		Items:        updates,
		NextCursor:   next,
		FeedRevision: int(feedRevision),
	})
}

func (h *fullAdminHandler) CreateEnterpriseUpdate(w http.ResponseWriter, r *http.Request, params adminapi.CreateEnterpriseUpdateParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.CreateEnterpriseUpdateRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	item, err := h.services.EnterpriseUpdate.Create(r.Context(), admin.UserID, request.Title, request.Content, string(request.ContentFormat), string(request.Category), string(request.Severity))
	if errors.Is(err, enterpriseupdate.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid enterprise update")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusCreated, enterpriseUpdateWire(item))
}

func (h *fullAdminHandler) GetEnterpriseUpdate(w http.ResponseWriter, r *http.Request, enterpriseUpdateID adminapi.EnterpriseUpdateId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	item, err := h.services.EnterpriseUpdate.Get(r.Context(), string(enterpriseUpdateID))
	if errors.Is(err, enterpriseupdate.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "Enterprise update not found")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid enterprise update")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, enterpriseUpdateWire(item))
}

func (h *fullAdminHandler) UpdateEnterpriseUpdate(w http.ResponseWriter, r *http.Request, enterpriseUpdateID adminapi.EnterpriseUpdateId, params adminapi.UpdateEnterpriseUpdateParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.UpdateEnterpriseUpdateRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	item, err := h.services.EnterpriseUpdate.Update(r.Context(), string(enterpriseUpdateID), request.Title, request.Content, string(request.ContentFormat), string(request.Category), string(request.Severity))
	if errors.Is(err, enterpriseupdate.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "Enterprise update not found")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidStatus) {
		writeProblem(w, http.StatusConflict, "invalid_status", "Cannot update a non-draft enterprise update")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid enterprise update")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, enterpriseUpdateWire(item))
}

func (h *fullAdminHandler) PublishEnterpriseUpdate(w http.ResponseWriter, r *http.Request, enterpriseUpdateID adminapi.EnterpriseUpdateId, params adminapi.PublishEnterpriseUpdateParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	item, err := h.services.EnterpriseUpdate.Publish(r.Context(), string(enterpriseUpdateID))
	if errors.Is(err, enterpriseupdate.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "Enterprise update not found")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidStatus) {
		writeProblem(w, http.StatusConflict, "invalid_status", "Cannot publish a non-draft enterprise update")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid enterprise update")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, enterpriseUpdateWire(item))
}

func (h *fullAdminHandler) WithdrawEnterpriseUpdate(w http.ResponseWriter, r *http.Request, enterpriseUpdateID adminapi.EnterpriseUpdateId, params adminapi.WithdrawEnterpriseUpdateParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	item, err := h.services.EnterpriseUpdate.Withdraw(r.Context(), string(enterpriseUpdateID))
	if errors.Is(err, enterpriseupdate.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "Enterprise update not found")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidStatus) {
		writeProblem(w, http.StatusConflict, "invalid_status", "Cannot withdraw a non-published enterprise update")
		return
	}
	if errors.Is(err, enterpriseupdate.ErrInvalidInput) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid enterprise update")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	writeJSON(w, http.StatusOK, enterpriseUpdateWire(item))
}

func enterpriseUpdateWire(v enterpriseupdate.UpdateView) adminapi.EnterpriseUpdate {
	return adminapi.EnterpriseUpdate{
		EnterpriseUpdateId: v.ID, Title: v.Title, Content: v.Content,
		ContentFormat: adminapi.EnterpriseUpdateContentFormat(v.ContentFormat),
		Category:      adminapi.EnterpriseUpdateCategory(v.Category), Severity: adminapi.EnterpriseUpdateSeverity(v.Severity),
		Status: adminapi.EnterpriseUpdateStatus(v.Status), PublishedAt: v.PublishedAt,
		FeedRevision: int(v.FeedRevision), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}
