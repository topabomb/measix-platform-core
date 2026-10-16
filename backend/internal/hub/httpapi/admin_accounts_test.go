package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"measix/platform/ent"
	"measix/platform/ent/session"
	"measix/platform/internal/hub/identity"
	"measix/platform/pkg/platformid"
)

func TestAdminAccountMutationRollback(t *testing.T) {
	for _, changeRole := range []bool{false, true} {
		t.Run(map[bool]string{false: "password", true: "role"}[changeRole], func(t *testing.T) {
			h, svc, _, ctx, actorID := setupFullHandler(t)
			member, err := svc.CreateUser(ctx, "rollbacktarget", "Target", "ADMIN")
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.SetPassword(ctx, member.ID, targetPassword); err != nil {
				t.Fatal(err)
			}
			accountRequest(t, h, "POST", "/api/admin/v1/session/login", nil, map[string]string{"username": "rollbacktarget", "password": targetPassword}, 200, "")
			before, _ := svc.GetUser(ctx, member.ID)
			injected := errors.New("synthetic session revocation failure")
			svc.Client.Session.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					if mutation.Op().Is(ent.OpUpdate) {
						return nil, injected
					}
					return next.Mutate(ctx, mutation)
				})
			})
			auth := identity.AdminAccountAuthorization{UserID: actorID, Password: actorPassword, Source: "test"}
			if changeRole {
				_, err = svc.SetAccountRole(ctx, auth, member.ID, "MEMBER", "ADMIN", "", "")
			} else {
				err = svc.ResetAccountPassword(ctx, auth, member.ID, "synthetic replacement password", "synthetic replacement password")
			}
			if !errors.Is(err, injected) {
				t.Fatalf("expected injected failure, got %v", err)
			}
			after, _ := svc.GetUser(ctx, member.ID)
			if after.Role != before.Role || *after.PasswordHash != *before.PasswordHash {
				t.Fatal("failed session revocation left an account mutation")
			}
			active, err := svc.Client.Session.Query().Where(session.UserIDEQ(member.ID), session.ChannelEQ("ADMIN_WEB"), session.StatusEQ("ACTIVE")).Count(ctx)
			if err != nil || active != 1 {
				t.Fatalf("failed transaction changed sessions: %d %v", active, err)
			}
		})
	}
}

func TestConcurrentAdminDemotionPreservesLogin(t *testing.T) {
	_, svc, _, ctx, actorID := setupFullHandler(t)
	other, err := svc.CreateUser(ctx, "otheradmin", "Other", "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassword(ctx, other.ID, targetPassword); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, pair := range [][3]string{{actorID, actorPassword, other.ID}, {other.ID, targetPassword, actorID}} {
		go func(pair [3]string) {
			<-start
			_, err := svc.SetAccountRole(ctx, identity.AdminAccountAuthorization{UserID: pair[0], Password: pair[1], Source: pair[0]}, pair[2], "MEMBER", "ADMIN", "", "")
			results <- err
		}(pair)
	}
	close(start)
	succeeded := 0
	for range 2 {
		err := <-results
		if err == nil {
			succeeded++
		} else if !errors.Is(err, identity.ErrNotAuthorized) {
			t.Fatalf("unexpected competing result: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("expected exactly one role change, got %d", succeeded)
	}
	first, _ := svc.GetUser(ctx, actorID)
	second, _ := svc.GetUser(ctx, other.ID)
	if (first.Role == "ADMIN") == (second.Role == "ADMIN") {
		t.Fatal("competing mutations did not retain exactly one administrator")
	}
}

const actorPassword = "correct horse battery staple"
const targetPassword = "synthetic target password"

func accountRequest(t *testing.T, h http.Handler, method, path string, headers map[string]string, body any, status int, code string) *httptest.ResponseRecorder {
	t.Helper()
	r := doJSON(t, h, method, path, headers, body)
	if r.Code != status || (code != "" && !strings.Contains(r.Body.String(), `"code":"`+code+`"`)) {
		t.Fatalf("%s %s: status=%d body=%s; want %d %s", method, path, r.Code, r.Body.String(), status, code)
	}
	return r
}

// HUB-ID-020: real HTTP and ordered-migration SQLite, with synthetic credentials.
func TestAdminAccountManagementGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   map[string]string
		status int
		code   string
	}{
		{"missing password", map[string]string{"username": "newadmin", "displayName": "New admin", "role": "ADMIN"}, 400, "admin_password_required"},
		{"wrong actor password", map[string]string{"username": "newadmin", "displayName": "New admin", "role": "ADMIN", "currentPassword": "wrong actor password", "newPassword": targetPassword, "confirmPassword": targetPassword}, 403, "invalid_current_password"},
		{"mismatch", map[string]string{"username": "newadmin", "displayName": "New admin", "role": "ADMIN", "currentPassword": actorPassword, "newPassword": targetPassword, "confirmPassword": "does not match"}, 400, "password_confirmation_mismatch"},
	} {
		t.Run("create/"+tc.name, func(t *testing.T) {
			h, svc, _, ctx, _ := setupFullHandler(t)
			cookie, csrf := loginAdmin(t, h)
			accountRequest(t, h, "POST", "/api/admin/v1/users", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, tc.body, tc.status, tc.code)
			rows, err := svc.ListUserViews(ctx, "newadmin", 50, "")
			if err != nil || len(rows) != 0 {
				t.Fatalf("failed create left user: %v %v", rows, err)
			}
		})
	}
	t.Run("self reset and wrong actor password", func(t *testing.T) {
		h, svc, _, ctx, id := setupFullHandler(t)
		cookie, csrf := loginAdmin(t, h)
		headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
		body := map[string]string{"currentPassword": actorPassword, "newPassword": targetPassword, "confirmPassword": targetPassword}
		accountRequest(t, h, "POST", "/api/admin/v1/users/"+id+":set-password", headers, body, 409, "cannot_reset_own_password")
		member, _ := svc.CreateUser(ctx, "target", "Target", "MEMBER")
		body["currentPassword"] = "wrong actor password"
		accountRequest(t, h, "POST", "/api/admin/v1/users/"+member.ID+":set-password", headers, body, 403, "invalid_current_password")
		fresh, _ := svc.GetUser(ctx, member.ID)
		if fresh.PasswordHash != nil {
			t.Fatal("failed reset changed password")
		}
	})
	t.Run("legacy PUT cannot bypass role protection", func(t *testing.T) {
		h, svc, _, ctx, _ := setupFullHandler(t)
		cookie, csrf := loginAdmin(t, h)
		member, _ := svc.CreateUser(ctx, "target", "Target", "MEMBER")
		accountRequest(t, h, "PUT", "/api/admin/v1/users/"+member.ID, map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]string{"username": "target", "displayName": "Target", "role": "ADMIN", "expectedRole": "MEMBER", "currentPassword": "wrong actor password", "newPassword": targetPassword, "confirmPassword": targetPassword}, 403, "invalid_current_password")
		fresh, _ := svc.GetUser(ctx, member.ID)
		if fresh.Role != "MEMBER" || fresh.PasswordHash != nil {
			t.Fatal("legacy PUT bypassed protection")
		}
	})
}

func TestAdminAccountRoleAndPasswordHTTPClosedLoop(t *testing.T) {
	h, svc, _, ctx, actorID := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	member, err := svc.CreateUser(ctx, "target", "Target original name", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := svc.CreateEnrollment(ctx, member.ID, actorID, 0)
	if err != nil {
		t.Fatal(err)
	}
	client, err := svc.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Account test device", "test")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/v1/users/" + member.ID + ":set-role"
	body := map[string]string{"role": "ADMIN", "expectedRole": "MEMBER", "currentPassword": actorPassword}
	accountRequest(t, h, "POST", path, headers, body, 400, "admin_password_required")
	body["newPassword"], body["confirmPassword"] = targetPassword, targetPassword
	result := accountRequest(t, h, "POST", path, headers, body, 200, "")
	if !strings.Contains(result.Body.String(), `"passwordConfigured":true`) {
		t.Fatal("missing derived password state")
	}
	fresh, _ := svc.GetUser(ctx, member.ID)
	if fresh.DisplayName != "Target original name" || fresh.Status != "ACTIVE" {
		t.Fatal("role command altered profile")
	}
	login := accountRequest(t, h, "POST", "/api/admin/v1/session/login", nil, map[string]string{"username": "target", "password": targetPassword}, 200, "")
	c := login.Result().Cookies()[0]
	targetHeaders := map[string]string{"Cookie": c.Name + "=" + c.Value}
	accountRequest(t, h, "POST", path, headers, body, 409, "user_role_conflict")
	body = map[string]string{"role": "MEMBER", "expectedRole": "ADMIN", "currentPassword": actorPassword}
	accountRequest(t, h, "POST", path, headers, body, 200, "")
	accountRequest(t, h, "GET", "/api/admin/v1/session", targetHeaders, nil, 401, "")
	body["role"], body["expectedRole"] = "ADMIN", "MEMBER"
	accountRequest(t, h, "POST", path, headers, body, 200, "")
	accountRequest(t, h, "GET", "/api/admin/v1/session", targetHeaders, nil, 401, "")
	login = accountRequest(t, h, "POST", "/api/admin/v1/session/login", nil, map[string]string{"username": "target", "password": targetPassword}, 200, "")
	c = login.Result().Cookies()[0]
	targetHeaders["Cookie"] = c.Name + "=" + c.Value
	reset := map[string]string{"currentPassword": actorPassword, "newPassword": "synthetic replacement password", "confirmPassword": "synthetic replacement password"}
	accountRequest(t, h, "POST", "/api/admin/v1/users/"+member.ID+":set-password", headers, reset, 204, "")
	accountRequest(t, h, "GET", "/api/admin/v1/session", targetHeaders, nil, 401, "")
	accountRequest(t, h, "GET", "/api/admin/v1/session", headers, nil, 200, "")
	accountRequest(t, h, "POST", "/api/admin/v1/session/login", nil, map[string]string{"username": "target", "password": targetPassword}, 401, "")
	accountRequest(t, h, "POST", "/api/admin/v1/session/login", nil, map[string]string{"username": "target", "password": "synthetic replacement password"}, 200, "")
	accountRequest(t, h, "POST", "/api/admin/v1/users/"+actorID+":set-role", headers, map[string]string{"role": "MEMBER", "expectedRole": "ADMIN", "currentPassword": actorPassword}, 409, "cannot_modify_current_admin")
	if principal, err := svc.AuthenticateAccess(ctx, client.AccessToken); err != nil || principal.UserID != member.ID {
		t.Fatalf("Admin operations changed Android identity: %+v %v", principal, err)
	}
}
