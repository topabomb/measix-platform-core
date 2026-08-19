package httpapi

import (
	"net/http"

	"measix/platform/ent"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/wire/clientapi"
)

type fullClientHandler struct {
	*clientHandler
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
	row, err := h.identity.Client.ManagedRelease.Query().Where(
		managedrelease.ManagedGenerationEQ(int64(generation)),
		managedrelease.StatusIn("ACTIVE", "SUPERSEDED"),
	).Only(r.Context())
	if ent.IsNotFound(err) {
		writeProblem(w, http.StatusNotFound, "snapshot_not_found", "Managed snapshot not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	etag := `"` + row.SnapshotHash + `"`
	if params.IfNoneMatch != nil && *params.IfNoneMatch == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(row.SnapshotJSON)
}
