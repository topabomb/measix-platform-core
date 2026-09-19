package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

func TestAdminCookieUsesConfiguredPublicScheme(t *testing.T) {
	for _, origin := range []string{"http://192.168.1.20:9000", "https://platform.example"} {
		t.Run(origin, func(t *testing.T) {
			h, id, _, _, _ := setupFullHandler(t)
			id.PublicOrigin = origin
			response := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", map[string]string{"X-Forwarded-Proto": "https"}, map[string]any{"username": "admin", "password": "correct horse battery staple"})
			if response.Code != 200 {
				t.Fatalf("login: %d", response.Code)
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing session cookie")
			}
			cookie := cookies[0]
			if cookie.Secure != (origin == "https://platform.example") || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatalf("incorrect cookie flags: secure=%v httpOnly=%v sameSite=%v", cookie.Secure, cookie.HttpOnly, cookie.SameSite)
			}
		})
	}
}

func TestEnrollmentUsesConfiguredPublicOrigin(t *testing.T) {
	h, id, _, _, _ := setupFullHandler(t)
	id.PublicOrigin = "http://192.168.1.20:9000"
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	created := doJSON(t, h, http.MethodPost, "/api/admin/v1/users", headers, map[string]any{"username": "origin-user", "displayName": "Origin User", "role": "MEMBER"})
	var user struct {
		UserID string `json:"userId"`
	}
	decodeJSON(t, created, &user)
	issuedAt := id.Now().UTC()
	grant := doJSON(t, h, http.MethodPost, "/api/admin/v1/users/"+user.UserID+"/enrollments", headers, map[string]any{})
	if grant.Code != 201 {
		t.Fatalf("grant status: %d", grant.Code)
	}
	var material struct {
		PlatformURL string    `json:"platformUrl"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	decodeJSON(t, grant, &material)
	if material.PlatformURL != id.PublicOrigin {
		t.Fatalf("public origin = %q", material.PlatformURL)
	}
	if want := issuedAt.Add(time.Hour); !material.ExpiresAt.Equal(want) {
		t.Fatalf("default expiry = %s, want %s", material.ExpiresAt, want)
	}
}

func TestEnrollmentWithoutPublicOriginDoesNotIssueCredential(t *testing.T) {
	h, id, _, ctx, adminID := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	id.PublicOrigin = ""
	before, err := id.Client.Enrollment.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	response := doJSON(t, h, http.MethodPost, "/api/admin/v1/users/"+adminID+"/enrollments", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]any{"expiresInSeconds": 600})
	if response.Code != 503 {
		t.Fatalf("missing origin: %d", response.Code)
	}
	after, err := id.Client.Enrollment.Query().Count(ctx)
	if err != nil || after != before {
		t.Fatal("issued a credential before platform origin was configured")
	}
}
