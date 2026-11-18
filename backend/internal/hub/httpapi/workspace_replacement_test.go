package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/ent/workspaceoperation"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestApplyHTTPRequiresConfirmedRevision(t *testing.T) {
	_, identity, _, ctx, actor := setupFullHandler(t)
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	secrets := upstream.NewService(identity.Client, box)
	svc := workspace.NewService(identity.Client, secrets)
	secret, err := secrets.CreateSecret(ctx, actor, "fixture management", "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://127.0.0.1:1"
	service, err := svc.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "Fixture", Config: adminapi.AgentSpaceConfig{AdminOrigin: origin, McpOrigin: origin, ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewFull(httpapi.Services{Identity: identity, Workspace: svc})
	cookie, csrf := loginAdmin(t, h)
	path := "/api/admin/v1/remote-workspace/services/" + service.WorkspaceServiceId + "/apply"
	for _, tc := range []struct {
		body   any
		status int
	}{
		{nil, 400}, {map[string]any{}, 400},
		{map[string]any{"expectedConfigRevision": 0}, 400},
		{map[string]any{"expectedConfigRevision": 2}, 409},
	} {
		headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Idempotency-Key": platformid.New(platformid.Idempotency)}
		if r := doJSON(t, h, "POST", path, headers, tc.body); r.Code != tc.status {
			t.Fatalf("unconfirmed apply status=%d, want %d", r.Code, tc.status)
		}
	}
	if identity.Client.WorkspaceOperation.Query().Where(workspaceoperation.ActionEQ("APPLY")).CountX(ctx) != 0 {
		t.Fatal("unconfirmed revision created an apply operation")
	}
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Idempotency-Key": platformid.New(platformid.Idempotency)}
	input := adminapi.ApplyWorkspaceServiceRequest{ExpectedConfigRevision: 1}
	if r := doJSON(t, h, "POST", path, headers, input); r.Code != 202 {
		t.Fatal("confirmed apply rejected", r.Code, r.Body)
	}
	if r := doJSON(t, h, "POST", path, headers, input); r.Code != 202 {
		t.Fatal("original confirmed apply retry rejected", r.Code)
	}
	if identity.Client.WorkspaceOperation.Query().Where(workspaceoperation.ActionEQ("APPLY")).CountX(ctx) != 1 {
		t.Fatal("confirmed retry duplicated apply")
	}
}

func TestReplacementHTTPRequiresAdminCSRFAndFrozenIntent(t *testing.T) {
	_, identity, _, ctx, actor := setupFullHandler(t)
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	secrets := upstream.NewService(identity.Client, box)
	svc := workspace.NewService(identity.Client, secrets)
	remoteCalls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteCalls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/admin/v1/status" {
			w.Write([]byte(`{"ready":true}`))
			return
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not_found"}`))
	}))
	defer remote.Close()
	secret, err := secrets.CreateSecret(ctx, actor, "fixture management", "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	service, err := svc.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "Fixture", Config: adminapi.AgentSpaceConfig{AdminOrigin: remote.URL, McpOrigin: remote.URL, ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	space := platformid.New(platformid.AgentSpace)
	identity.Client.AgentSpace.Create().SetID(actor).SetWorkspaceServiceID(service.WorkspaceServiceId).SetRemoteUsername("fixture_user").SetAgentSpaceID(space).SetBindingRevision(3).SetDavBindingRevision(4).SetIntent("CONNECTED").SetState("CONNECTED").SetRemoteActive(true).SetStopPending(false).SetDavConfirmed(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	h := httpapi.NewFull(httpapi.Services{Identity: identity, Workspace: svc})
	cookie, csrf := loginAdmin(t, h)
	path := "/api/admin/v1/remote-workspace/services/" + service.WorkspaceServiceId + "/replace"
	key := platformid.New(platformid.Idempotency)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Idempotency-Key": key}
	input := adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 1, ConfirmedDestroyedDeployment: true, Bindings: []adminapi.WorkspaceReplacementBinding{{UserId: actor, AgentSpaceId: space, BindingRevision: 3, DavBindingRevision: 4}}}
	if r := doJSON(t, h, "POST", path, map[string]string{"X-CSRF-Token": csrf, "Idempotency-Key": key}, input); r.Code != 401 {
		t.Fatal("missing session", r.Code)
	}
	if r := doJSON(t, h, "POST", path, map[string]string{"Cookie": cookie, "X-CSRF-Token": "wrong", "Idempotency-Key": key}, input); r.Code != 401 {
		t.Fatal("missing CSRF", r.Code)
	}
	if remoteCalls != 0 {
		t.Fatal("unauthorized request reached remote")
	}
	invalid := input
	invalid.ConfirmedDestroyedDeployment = false
	if r := doJSON(t, h, "POST", path, headers, invalid); r.Code != 400 {
		t.Fatal("missing destruction consent", r.Code)
	}
	r := doJSON(t, h, "POST", path, headers, input)
	if r.Code != 202 || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("accepted intent", r.Code, r.Body)
	}
	var op adminapi.WorkspaceOperation
	decodeJSON(t, r, &op)
	retry := doJSON(t, h, "POST", path, headers, input)
	var same adminapi.WorkspaceOperation
	decodeJSON(t, retry, &same)
	if retry.Code != 202 || same.OperationId != op.OperationId {
		t.Fatal("duplicate operation")
	}
	input.ExpectedConfigRevision = 2
	if r := doJSON(t, h, "POST", path, headers, input); r.Code != 409 {
		t.Fatal("changed idempotent request", r.Code)
	}
	identity.Client.User.UpdateOneID(actor).SetRole("USER").SaveX(ctx)
	if r := doJSON(t, h, "POST", path, headers, input); r.Code != 401 {
		t.Fatal("non-admin replacement", r.Code)
	}
}
