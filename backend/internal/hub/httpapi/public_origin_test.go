package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminCookieUsesConfiguredPublicScheme(t *testing.T) {
	for _, origin := range []string{"http://192.168.1.20:9000", "https://platform.example"} {
		t.Run(origin, func(t *testing.T) {
			h, id, _, _, _ := setupFullHandler(t)
			id.SetPublicOrigin(origin)
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

func TestAdminRememberLoginCookiePolicy(t *testing.T) {
	h, id, _, _, _ := setupFullHandler(t)
	id.SetPublicOrigin("https://platform.example")
	now := id.Now().UTC()

	ordinary := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin", "password": "correct horse battery staple",
	})
	if ordinary.Code != http.StatusOK {
		t.Fatalf("ordinary login: %d %s", ordinary.Code, ordinary.Body)
	}
	ordinaryCookie := ordinary.Result().Cookies()[0]
	if !ordinaryCookie.Expires.IsZero() || ordinaryCookie.MaxAge != 0 {
		t.Fatalf("ordinary login persisted cookie: %+v", ordinaryCookie)
	}
	var ordinarySession struct {
		ExpiresAt time.Time `json:"expiresAt"`
	}
	decodeJSON(t, ordinary, &ordinarySession)
	if want := now.Add(12 * time.Hour); !ordinarySession.ExpiresAt.Equal(want) {
		t.Fatalf("ordinary expiry=%s want=%s", ordinarySession.ExpiresAt, want)
	}

	remembered := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin", "password": "correct horse battery staple", "rememberMe": true,
	})
	if remembered.Code != http.StatusOK {
		t.Fatalf("remembered login: %d %s", remembered.Code, remembered.Body)
	}
	rememberedCookie := remembered.Result().Cookies()[0]
	if rememberedCookie.MaxAge != 30*24*60*60 || !rememberedCookie.Expires.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("remembered cookie lifetime: %+v", rememberedCookie)
	}

	id.SetPublicOrigin("http://192.168.1.20:9000")
	httpRemembered := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin", "password": "correct horse battery staple", "rememberMe": true,
	})
	if httpRemembered.Code != http.StatusOK {
		t.Fatalf("HTTP remembered login=%d %s", httpRemembered.Code, httpRemembered.Body)
	}
	httpCookie := httpRemembered.Result().Cookies()[0]
	if httpCookie.Secure || httpCookie.MaxAge != 30*24*60*60 || !httpCookie.Expires.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("HTTP remembered cookie policy: %+v", httpCookie)
	}
}

func TestAdminLoginThrottleReturnsRetryAfter(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	for range 5 {
		response := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
			"username": "admin", "password": "wrong password value",
		})
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("credential failure=%d %s", response.Code, response.Body)
		}
	}
	response := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin", "password": "correct horse battery staple",
	})
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "5" || !strings.Contains(response.Body.String(), "login_throttled") {
		t.Fatalf("throttle response=%d retry=%q body=%s", response.Code, response.Header().Get("Retry-After"), response.Body)
	}
}

func TestAdminLoginFailureDoesNotRevealUsername(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	known := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin", "password": "wrong password value",
	})
	unknown := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "does-not-exist", "password": "wrong password value",
	})
	if known.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized || known.Body.String() != unknown.Body.String() {
		t.Fatalf("login failure revealed username: known=%d %s unknown=%d %s", known.Code, known.Body, unknown.Code, unknown.Body)
	}
}

func TestEnrollmentUsesConfiguredPublicOrigin(t *testing.T) {
	h, id, _, _, _ := setupFullHandler(t)
	id.SetPublicOrigin("http://192.168.1.20:9000")
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
	if material.PlatformURL != id.PublicOrigin() {
		t.Fatalf("public origin = %q", material.PlatformURL)
	}
	if want := issuedAt.Add(time.Hour); !material.ExpiresAt.Equal(want) {
		t.Fatalf("default expiry = %s, want %s", material.ExpiresAt, want)
	}
}

func TestEnrollmentWithoutPublicOriginDoesNotIssueCredential(t *testing.T) {
	h, id, _, ctx, adminID := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	id.SetPublicOrigin("")
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
