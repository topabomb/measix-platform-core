package httpapi_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func TestSYSI1001IdentityHTTPClosedLoop(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStore(t)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := platformid.New(platformid.Deployment)
	signer, err := security.NewAccessSigner(privateKey, deploymentID, "test-key", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := identity.New(st.Client, signer, []byte("01234567890123456789012345678901"))
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	svc.PublicOrigin = "https://platform.example"
	svc.Now = func() time.Time { return now }
	signer.Now = svc.Now
	boot, err := svc.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	h := httpapi.New(svc)

	discovery := doJSON(t, h, http.MethodGet, "/.well-known/measix", nil, nil)
	var bases struct {
		Client  string `json:"clientApiBase"`
		Runtime string `json:"runtimeApiBase"`
	}
	decodeJSON(t, discovery, &bases)
	if bases.Client != "/api/client/v1" || bases.Runtime != "/runtime/v1" {
		t.Fatalf("discovery must expose same-origin paths: %+v", bases)
	}

	login := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin",
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	decodeJSON(t, login, &session)
	adminCookie := login.Result().Cookies()[0]
	if !adminCookie.HttpOnly || !adminCookie.Secure || session.CSRFToken == "" {
		t.Fatalf("invalid admin session cookie/csrf: cookie=%+v csrf=%q", adminCookie, session.CSRFToken)
	}

	createUser := doJSON(t, h, http.MethodPost, "/api/admin/v1/users", map[string]string{
		"Cookie":       adminCookie.Name + "=" + adminCookie.Value,
		"X-CSRF-Token": session.CSRFToken,
	}, map[string]any{
		"username":    "alice",
		"displayName": "Alice",
		"role":        "MEMBER",
	})
	if createUser.Code != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", createUser.Code, createUser.Body.String())
	}
	var user struct {
		UserID string `json:"userId"`
	}
	decodeJSON(t, createUser, &user)
	if err := platformid.Validate(platformid.User, user.UserID); err != nil {
		t.Fatalf("user id: %v", err)
	}

	page := doJSON(t, h, http.MethodGet, "/api/admin/v1/users?limit=1", map[string]string{"Cookie": adminCookie.Name + "=" + adminCookie.Value}, nil)
	var firstPage struct {
		Items []struct {
			ID string `json:"userId"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	decodeJSON(t, page, &firstPage)
	if len(firstPage.Items) != 1 || firstPage.NextCursor == nil {
		t.Fatalf("missing next cursor: %s", page.Body.String())
	}
	page = doJSON(t, h, http.MethodGet, "/api/admin/v1/users?limit=1&cursor="+*firstPage.NextCursor, map[string]string{"Cookie": adminCookie.Name + "=" + adminCookie.Value}, nil)
	var secondPage struct {
		Items []struct {
			ID string `json:"userId"`
		} `json:"items"`
	}
	decodeJSON(t, page, &secondPage)
	if len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID {
		t.Fatal("cursor repeated first page")
	}

	enrollment := doJSON(t, h, http.MethodPost, "/api/admin/v1/users/"+user.UserID+"/enrollments", map[string]string{
		"Cookie":       adminCookie.Name + "=" + adminCookie.Value,
		"X-CSRF-Token": session.CSRFToken,
	}, map[string]any{"expiresInSeconds": 600})
	if enrollment.Code != http.StatusCreated {
		t.Fatalf("create enrollment status=%d body=%s", enrollment.Code, enrollment.Body.String())
	}
	var grant struct {
		Code string `json:"code"`
	}
	decodeJSON(t, enrollment, &grant)

	installationID := platformid.New(platformid.Installation)
	exchange := doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]any{
		"code":           grant.Code,
		"installationId": installationID,
		"platform":       "ANDROID",
		"deviceName":     "test-device",
		"appVersion":     "1.0.0",
	})
	if exchange.Code != http.StatusCreated {
		t.Fatalf("exchange status=%d body=%s", exchange.Code, exchange.Body.String())
	}
	var tokens struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		DeviceID     string `json:"deviceId"`
	}
	decodeJSON(t, exchange, &tokens)
	assertEnrollmentProblem := func(response *httptest.ResponseRecorder, code string) {
		t.Helper()
		if response.Code != http.StatusConflict {
			t.Fatalf("enrollment conflict status=%d body=%s", response.Code, response.Body.String())
		}
		var problem struct {
			Code string `json:"code"`
		}
		decodeJSON(t, response, &problem)
		if problem.Code != code {
			t.Fatalf("enrollment conflict code=%q, want %q", problem.Code, code)
		}
	}
	used := doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]any{
		"code": grant.Code, "installationId": platformid.New(platformid.Installation),
		"platform": "ANDROID", "deviceName": "test-device", "appVersion": "1.0.0",
	})
	assertEnrollmentProblem(used, "enrollment_already_used")

	other, err := svc.CreateUser(ctx, "bob", "Bob", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	otherGrant, err := svc.CreateEnrollment(ctx, other.ID, boot.AdminUserID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	conflict := doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]any{
		"code": otherGrant.Code, "installationId": installationID,
		"platform": "ANDROID", "deviceName": "other-device", "appVersion": "1.0.0",
	})
	assertEnrollmentProblem(conflict, "installation_user_conflict")
	notConsumed := doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]any{
		"code": otherGrant.Code, "installationId": platformid.New(platformid.Installation),
		"platform": "ANDROID", "deviceName": "other-device", "appVersion": "1.0.0",
	})
	if notConsumed.Code != http.StatusCreated {
		t.Fatalf("installation conflict consumed enrollment: status=%d body=%s", notConsumed.Code, notConsumed.Body.String())
	}

	bootstrap := doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", map[string]string{
		"Authorization": "Bearer " + tokens.AccessToken,
	}, nil)
	if bootstrap.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}

	refresh := doJSON(t, h, http.MethodPost, "/api/client/v1/sessions/refresh", map[string]string{"Idempotency-Key": platformid.New(platformid.Idempotency)}, map[string]any{
		"refreshToken": tokens.RefreshToken,
	})
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
	}

	// I1 fixture seeds deny facts directly; runtimecontrol tests own the full
	// revocation transaction. Client errors must retain the denied owner so the
	// native lifecycle can present and retire the right state.
	if _, err := svc.Client.User.UpdateOneID(user.UserID).SetStatus("DISABLED").Save(ctx); err != nil {
		t.Fatal(err)
	}
	userDenied := doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", map[string]string{
		"Authorization": "Bearer " + tokens.AccessToken,
	}, nil)
	assertProblem(t, userDenied, http.StatusForbidden, "user_disabled")
	if _, err := svc.Client.User.UpdateOneID(user.UserID).SetStatus("ACTIVE").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Client.Device.UpdateOneID(tokens.DeviceID).SetStatus("REVOKED").Save(ctx); err != nil {
		t.Fatal(err)
	}
	denied := doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", map[string]string{
		"Authorization": "Bearer " + tokens.AccessToken,
	}, nil)
	assertProblem(t, denied, http.StatusForbidden, "device_revoked")
}

func TestPublicAuthenticationProblemsUseStableCode(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	for _, test := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/client/v1/bootstrap", nil},
		{http.MethodGet, "/api/client/v1/managed/state", nil},
		{http.MethodGet, "/api/client/v1/managed/snapshots/1", nil},
		{http.MethodPut, "/api/client/v1/managed/applied", map[string]any{"managedGeneration": 1, "snapshotHash": "hash"}},
		{http.MethodPost, "/api/client/v1/portal/grants", nil},
		{http.MethodGet, "/api/client/v1/budgets", nil},
		{http.MethodGet, "/api/portal/v1/session", nil},
		{http.MethodGet, "/api/admin/v1/session", nil},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			assertProblem(t, doJSON(t, h, test.method, test.path, nil, test.body), http.StatusUnauthorized, "unauthenticated")
		})
	}
}

func TestEnrollmentExpiryUsesEnrollmentProblemCode(t *testing.T) {
	h, svc, _, ctx, adminID := setupFullHandler(t)
	grant, err := svc.CreateEnrollment(ctx, adminID, adminID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return grant.ExpiresAt }
	response := doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]any{
		"code": grant.Code, "installationId": platformid.New(platformid.Installation),
		"platform": "ANDROID", "deviceName": "expired-device", "appVersion": "1.0.0",
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired enrollment status=%d body=%s", response.Code, response.Body.String())
	}
	var problem struct {
		Code string `json:"code"`
	}
	decodeJSON(t, response, &problem)
	if problem.Code != "enrollment_expired" {
		t.Fatalf("expired enrollment code=%q, want enrollment_expired", problem.Code)
	}
}

func doJSON(t *testing.T, h http.Handler, method, path string, headers map[string]string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("problem status=%d, want %d; body=%s", response.Code, status, response.Body.String())
	}
	var problem struct {
		Code string `json:"code"`
	}
	decodeJSON(t, response, &problem)
	if problem.Code != code {
		t.Fatalf("problem code=%q, want %q; body=%s", problem.Code, code, response.Body.String())
	}
}
