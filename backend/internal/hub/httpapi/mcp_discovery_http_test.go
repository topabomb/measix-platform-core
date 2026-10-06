package httpapi_test

import (
	"net/http"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/pkg/platformid"
)

func TestMcpDiscoveryHTTPRequiresAdminCSRFAndCurrentDraft(t *testing.T) {
	_, id, _, ctx, _ := setupFullHandler(t)
	svc := capability.NewService(id.Client)
	h := httpapi.NewFull(httpapi.Services{Identity: id, Capability: svc})
	cookie, csrf := loginAdmin(t, h)
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/v1/draft/mcp/" + platformid.New(platformid.MCP) + ":discover"
	for _, tc := range []struct {
		name    string
		headers map[string]string
		body    any
		status  int
		code    string
	}{
		{"unauthenticated", map[string]string{"X-CSRF-Token": "synthetic-csrf"}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 401, ""},
		{"missing-csrf", map[string]string{"Cookie": cookie}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 400, ""},
		{"invalid-csrf", map[string]string{"Cookie": cookie, "X-CSRF-Token": "synthetic-csrf"}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 401, "unauthenticated"},
		{"missing-revision", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{}, 400, "invalid_request"},
		{"unknown-field", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expectedDraftRevision": draft.DraftRevision, "endpoint": "http://private"}, 400, "invalid_request"},
		{"stale-revision", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expectedDraftRevision": draft.DraftRevision + 1}, 409, "stale_draft_revision"},
		{"unavailable-source", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expectedDraftRevision": draft.DraftRevision}, 422, "mcp_source_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := doJSON(t, h, http.MethodPost, path, tc.headers, tc.body)
			if response.Code != tc.status {
				t.Fatalf("expected %d got %d: %s", tc.status, response.Code, response.Body.String())
			}
			if tc.code != "" {
				var problem struct {
					Code string `json:"code"`
				}
				decodeJSON(t, response, &problem)
				if problem.Code != tc.code {
					t.Fatalf("wrong safe problem %s", problem.Code)
				}
			}
			current, err := svc.GetDraft(ctx)
			if err != nil || current.DraftRevision != draft.DraftRevision {
				t.Fatal("rejected discovery changed draft")
			}
		})
	}
}
