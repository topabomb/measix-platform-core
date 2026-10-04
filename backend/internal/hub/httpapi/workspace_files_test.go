package httpapi

import (
	"io"
	remoteapi "measix/platform/internal/hub/agentspace"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceFileErrorsHaveActionableStatus(t *testing.T) {
	for code, want := range map[string]int{"invalid_file_destination": 400, "workspace_space_mismatch": 409, "file_listing_limit": 422, "file_version_conflict": 412, "file_locked": 423, "file_range_invalid": 416, "file_conflict": 409, "dav_credential_unavailable": 503, "file_storage_full": 507, "file_transfer_limit": 429} {
		w := httptest.NewRecorder()
		writeWorkspaceError(w, &remoteapi.Error{Code: code})
		if w.Code != want {
			t.Errorf("%s: got %d, want %d", code, w.Code, want)
		}
	}
}

func TestWorkspaceRangeErrorCarriesRepresentationLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeWorkspaceError(w, &remoteapi.Error{Code: "file_range_invalid", Status: 416, ContentRange: "bytes */4294967296"})
	}))
	defer server.Close()
	for _, method := range []string{"GET", "HEAD"} {
		r, _ := http.NewRequest(method, server.URL, nil)
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 416 || response.Header.Get("Content-Range") != "bytes */4294967296" || method == "HEAD" && len(body) != 0 {
			t.Fatalf("%s lost range semantics: %d %v %q %v", method, response.StatusCode, response.Header, body, err)
		}
	}
}
