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

	"github.com/topabomb/measix-platform-core/backend/internal/hub/httpapi"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/identity"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
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
	svc.Now = func() time.Time { return now }
	signer.Now = svc.Now
	if _, err := svc.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	h := httpapi.New(svc, httpapi.Options{
		PublicBaseURL:  "https://measix.test",
		RuntimeAPIBase: "/runtime/v1",
	})

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
	if exchange.Code != http.StatusOK {
		t.Fatalf("exchange status=%d body=%s", exchange.Code, exchange.Body.String())
	}
	var tokens struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		DeviceID     string `json:"deviceId"`
	}
	decodeJSON(t, exchange, &tokens)

	bootstrap := doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", map[string]string{
		"Authorization": "Bearer " + tokens.AccessToken,
	}, nil)
	if bootstrap.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}

	refresh := doJSON(t, h, http.MethodPost, "/api/client/v1/sessions/refresh", nil, map[string]any{
		"refreshToken": tokens.RefreshToken,
	})
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
	}

	if err := svc.RevokeDevice(ctx, tokens.DeviceID); err != nil {
		t.Fatal(err)
	}
	denied := doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", map[string]string{
		"Authorization": "Bearer " + tokens.AccessToken,
	}, nil)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("revoked access status=%d body=%s", denied.Code, denied.Body.String())
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
