package httpapi_test

import (
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"testing"
)

func TestReleaseRetentionDefaultAndRevisionHTTP(t *testing.T) {
	_, id, _, _, _ := setupFullHandler(t)
	h := httpapi.NewFull(httpapi.Services{Identity: id, Capability: capability.NewService(id.Client)})
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	accountRequest(t, h, "GET", "/api/admin/v1/releases/retention", headers, nil, 200, "")
	body := map[string]any{"expectedRevision": 1, "rule": map[string]any{"enabled": false, "keepLast": 2}}
	accountRequest(t, h, "PUT", "/api/admin/v1/releases/retention", map[string]string{"Cookie": cookie, "X-CSRF-Token": "invalid"}, body, 401, "")
	accountRequest(t, h, "PUT", "/api/admin/v1/releases/retention", headers, body, 200, "")
	accountRequest(t, h, "PUT", "/api/admin/v1/releases/retention", headers, body, 409, "stale_release_retention_revision")
}
