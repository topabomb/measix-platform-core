package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestAdminChangesOwnPasswordOverHTTP(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}

	response := doJSON(t, h, http.MethodPost, "/api/admin/v1/session:change-password", headers, map[string]string{
		"currentPassword": "wrong current password",
		"newPassword":     "a different battery staple",
		"confirmPassword": "a different battery staple",
	})
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"invalid_current_password"`) {
		t.Fatalf("wrong-current status=%d body=%s", response.Code, response.Body.String())
	}

	response = doJSON(t, h, http.MethodPost, "/api/admin/v1/session:change-password", headers, map[string]string{
		"currentPassword": "correct horse battery staple",
		"newPassword":     "a different battery staple",
		"confirmPassword": "does not match password",
	})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"password_confirmation_mismatch"`) {
		t.Fatalf("mismatch status=%d body=%s", response.Code, response.Body.String())
	}

	response = doJSON(t, h, http.MethodPost, "/api/admin/v1/session:change-password", headers, map[string]string{
		"currentPassword": "correct horse battery staple",
		"newPassword":     "a different battery staple",
		"confirmPassword": "a different battery staple",
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("change status=%d body=%s", response.Code, response.Body.String())
	}
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("password change must clear admin cookie: %+v", cookies)
	}

	if response = doJSON(t, h, http.MethodGet, "/api/admin/v1/session", map[string]string{"Cookie": cookie}, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("old session remains active: status=%d body=%s", response.Code, response.Body.String())
	}
	if response = doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]string{"username": "admin", "password": "correct horse battery staple"}); response.Code != http.StatusUnauthorized {
		t.Fatalf("old password remains active: status=%d body=%s", response.Code, response.Body.String())
	}
	if response = doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]string{"username": "admin", "password": "a different battery staple"}); response.Code != http.StatusOK {
		t.Fatalf("new password rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}
