package httpapi

import (
	remoteapi "measix/platform/internal/hub/agentspace"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceFileErrorsHaveActionableStatus(t *testing.T) {
	for code, want := range map[string]int{"invalid_file_destination": 400, "workspace_space_mismatch": 409, "file_listing_limit": 422, "file_version_conflict": 409, "dav_credential_unavailable": 503, "file_storage_full": 507, "file_transfer_limit": 429} {
		w := httptest.NewRecorder()
		writeWorkspaceError(w, &remoteapi.Error{Code: code})
		if w.Code != want {
			t.Errorf("%s: got %d, want %d", code, w.Code, want)
		}
	}
}
