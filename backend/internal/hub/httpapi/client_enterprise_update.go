package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/wire/clientapi"
)

func (h *fullClientHandler) ListEnterpriseUpdates(w http.ResponseWriter, r *http.Request, params clientapi.ListEnterpriseUpdatesParams) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	if _, err := h.identity.AuthenticateAccess(r.Context(), token); err != nil {
		writeIdentityError(w, err)
		return
	}

	// Validate limit
	limit := 10
	if params.Limit != nil {
		limit = *params.Limit
		if limit < 1 || limit > 20 {
			writeProblem(w, http.StatusBadRequest, "invalid_argument", "limit must be between 1 and 20")
			return
		}
	}

	// Parse dates from openapi_types.Date (which embeds time.Time)
	var startDate, endDate *time.Time
	if params.StartDate != nil {
		startDate = &params.StartDate.Time
	}
	if params.EndDate != nil {
		endDate = &params.EndDate.Time
	}

	items, truncated, err := h.enterpriseUpdate.ListPublished(r.Context(), startDate, endDate, limit)
	if err != nil {
		if errors.Is(err, enterpriseupdate.ErrInvalidLimit) {
			writeProblem(w, http.StatusBadRequest, "invalid_argument", "limit must be between 1 and 20")
			return
		}
		if errors.Is(err, enterpriseupdate.ErrInvalidDateRange) {
			writeProblem(w, http.StatusBadRequest, "invalid_argument", "start_date must not be after end_date")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}

	// Compute ETag from the authoritative latest feed revision (across all
	// statuses), not from the filtered result set. This ensures the ETag
	// changes whenever any update is published/withdrawn, even if the
	// filtered list is empty or doesn't include the latest revision.
	latestRev, err := h.enterpriseUpdate.LatestFeedRevision(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	etag := fmt.Sprintf(`"rev-%d"`, latestRev)

	// Check If-None-Match for conditional 304
	if params.IfNoneMatch != nil && *params.IfNoneMatch == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	feedItems := make([]clientapi.EnterpriseUpdateItem, 0, len(items))
	for _, item := range items {
		entry := clientapi.EnterpriseUpdateItem{
			UpdateId: clientapi.EnterpriseUpdateId(item.ID),
			Title:    item.Title,
			Content:  item.Content,
		}
		if item.PublishedAt != nil {
			entry.PublishedAt = *item.PublishedAt
		}
		feedItems = append(feedItems, entry)
	}

	// Use deployment timezone (UTC for S0.2)
	tz := "UTC"
	feed := clientapi.EnterpriseUpdateFeed{
		EnterpriseTimezone: tz,
		Items:              feedItems,
		Truncated:          truncated,
	}

	// Write response with ETag header
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(feed)
}

func (h *fullClientHandler) GetEnterpriseUpdate(w http.ResponseWriter, r *http.Request, enterpriseUpdateID clientapi.EnterpriseUpdateId) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	if _, err := h.identity.AuthenticateAccess(r.Context(), token); err != nil {
		writeIdentityError(w, err)
		return
	}
	item, err := h.enterpriseUpdate.Get(r.Context(), string(enterpriseUpdateID))
	if errors.Is(err, enterpriseupdate.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "Enterprise update not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	if item.Status != "PUBLISHED" {
		writeProblem(w, http.StatusNotFound, "not_found", "Enterprise update not found")
		return
	}
	entry := clientapi.EnterpriseUpdateItem{
		UpdateId: clientapi.EnterpriseUpdateId(item.ID),
		Title:    item.Title,
		Content:  item.Content,
	}
	if item.PublishedAt != nil {
		entry.PublishedAt = *item.PublishedAt
	}
	writeJSON(w, http.StatusOK, entry)
}
