package httpapi

import (
	"errors"
	"net/http"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/clientapi"
)

type fullClientHandler struct {
	*clientHandler
	capability *capability.Service
}

func (h *fullClientHandler) GetManagedSnapshot(w http.ResponseWriter, r *http.Request, generation int, params clientapi.GetManagedSnapshotParams) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	if _, err := h.identity.AuthenticateAccess(r.Context(), token); err != nil {
		writeIdentityError(w, err)
		return
	}
	snapshot, err := h.capability.GetSnapshot(r.Context(), generation)
	if errors.Is(err, capability.ErrReleaseNotFound) {
		writeProblem(w, http.StatusNotFound, "snapshot_not_found", "Managed snapshot not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	etag := `"` + snapshot.Hash + `"`
	if params.IfNoneMatch != nil && *params.IfNoneMatch == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(snapshot.JSON)
}
