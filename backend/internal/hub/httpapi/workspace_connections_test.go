package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestRevealMCPHTTPRequiresAdminCSRFAndCurrentTarget(t *testing.T) {
	_, identity, _, ctx, actor := setupFullHandler(t)
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	secrets := upstream.NewService(identity.Client, box)
	svc := workspace.NewService(identity.Client, secrets)
	management, err := secrets.CreateSecret(ctx, actor, "management", "management-fixture")
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.CreateSecret(ctx, actor, "MCP", "key_fixture.secret-fixture")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://127.0.0.1:1"
	service, err := svc.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "Fixture", Config: adminapi.AgentSpaceConfig{AdminOrigin: origin, McpOrigin: origin, ManagementSecret: adminapi.SecretRef{SecretId: management.SecretID, SecretVersion: management.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	identity.Client.WorkspaceService.UpdateOneID(service.WorkspaceServiceId).SetState("ACTIVE").SetEnabled(true).SetActiveConfigRevision(1).SaveX(ctx)
	space := platformid.New(platformid.AgentSpace)
	now := time.Now().UTC()
	identity.Client.AgentSpace.Create().SetID(actor).SetWorkspaceServiceID(service.WorkspaceServiceId).SetRemoteUsername("fixture_user").SetAgentSpaceID(space).SetBindingRevision(3).SetIntent("CONNECTED").SetState("CONNECTED").SetRemoteActive(true).SetStopPending(false).SetDavConfirmed(false).SetMcpSecretID(key.SecretID).SetMcpSecretVersion(int64(key.SecretVersion)).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	h := httpapi.NewFull(httpapi.Services{Identity: identity, Workspace: svc})
	cookie, csrf := loginAdmin(t, h)
	path := "/api/admin/v1/users/" + actor + "/workspace/mcp-connection"
	query := "?agentSpaceId=" + space + "&expectedRevision=3"
	for _, tc := range []struct {
		method, query, cookie, csrf string
		status                      int
	}{
		{"POST", query, "", csrf, 401},
		{"POST", query, cookie, "wrong", 401},
		{"POST", "", cookie, csrf, 400},
		{"POST", "?agentSpaceId=" + space + "&expectedRevision=0", cookie, csrf, 400},
		{"POST", "?agentSpaceId=" + space + "&expectedRevision=2", cookie, csrf, 409},
		{"POST", "?agentSpaceId=" + platformid.New(platformid.AgentSpace) + "&expectedRevision=3", cookie, csrf, 409},
		{"GET", query, cookie, csrf, http.StatusMethodNotAllowed},
		{"POST", query, cookie, csrf, 200},
	} {
		response := doJSON(t, h, tc.method, path+tc.query, map[string]string{"Cookie": tc.cookie, "X-CSRF-Token": tc.csrf}, nil)
		if response.Code != tc.status {
			t.Fatalf("%s: status=%d want=%d", tc.method+tc.query, response.Code, tc.status)
		}
		if response.Code != 200 {
			continue
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("credential response can be cached")
		}
		var info adminapi.WorkspaceMCPConnection
		decodeJSON(t, response, &info)
		if info.Token != "key_fixture.secret-fixture" || info.Username != "fixture_user" || info.McpUrl != origin+"/u/fixture_user/mcp" {
			t.Fatal("wrong credential or target returned")
		}
	}
	// Session role revocation must close the explicit reveal route too.
	identity.Client.User.UpdateOneID(actor).SetRole("MEMBER").SaveX(ctx)
	response := doJSON(t, h, "POST", path+query, map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, nil)
	if response.Code != 401 {
		t.Fatalf("revoked administrator: %d", response.Code)
	}
}
