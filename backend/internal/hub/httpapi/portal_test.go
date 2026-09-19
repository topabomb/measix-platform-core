package httpapi_test

import (
	"context"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/wire/clientapi"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPortalGrantRequiresAndroidAuthentication(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	got := doJSON(t, h, http.MethodPost, "/api/client/v1/portal/grants", nil, nil)
	if got.Code != http.StatusUnauthorized {
		t.Fatalf("grant without Android authentication status=%d, want 401", got.Code)
	}
}

func TestPortalCloseMissingCSRFIsForbidden(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	got := doJSON(t, h, http.MethodDelete, "/api/portal/v1/session", nil, nil)
	if got.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d, want 403", got.Code)
	}
}

func TestPortalExchangeAcceptsNativeOpaqueOriginButRejectsCrossSite(t *testing.T) {
	h, id, _, ctx, _ := setupFullHandler(t)
	id.PublicOrigin = "http://192.168.1.20:9000"
	admin, csrf := loginAdmin(t, h)
	token := enrollClient(t, h, admin, csrf)
	exchange := func(ticket, fetchSite string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/portal/session/exchange",
			strings.NewReader(url.Values{"ticket": {ticket}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		r.Header.Set("Origin", "null")
		r.Header.Set("Sec-Fetch-Site", fetchSite)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	grant, err := id.CreatePortalGrant(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if got := exchange(grant.Ticket, "none"); got.Code != http.StatusSeeOther {
		t.Fatalf("native opaque-origin exchange status=%d body=%s", got.Code, got.Body)
	}
	grant, err = id.CreatePortalGrant(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if got := exchange(grant.Ticket, "cross-site"); got.Code != http.StatusForbidden {
		t.Fatalf("cross-site opaque-origin exchange status=%d, want 403", got.Code)
	}
}

func TestPortalSessionLifecycleAndIsolation(t *testing.T) {
	for _, origin := range []string{"https://platform.example", "http://192.168.1.20:9000", "http://platform.example:9000"} {
		t.Run(origin, func(t *testing.T) { testPortalSessionLifecycle(t, origin) })
	}
}

func testPortalSessionLifecycle(t *testing.T, origin string) {
	h, id, updates, ctx, _ := setupFullHandler(t)
	id.PublicOrigin = origin
	admin, csrf := loginAdmin(t, h)
	token := enrollClient(t, h, admin, csrf)
	principal, err := id.AuthenticateAccess(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := id.Client.Session.Get(ctx, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	grantResponse := doJSON(t, h, "POST", "/api/client/v1/portal/grants", map[string]string{"Authorization": "Bearer " + token}, nil)
	if grantResponse.Code != 201 {
		t.Fatalf("grant %d %s", grantResponse.Code, grantResponse.Body)
	}
	var grant clientapi.PortalGrant
	decodeJSON(t, grantResponse, &grant)
	if grant.ExchangeUrl != origin+"/portal/session/exchange" || grantResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe grant metadata")
	}
	exchange := func(ticket, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/portal/session/exchange", strings.NewReader(url.Values{"ticket": {ticket}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := exchange(grant.Ticket, "https://evil.example"); got.Code != 403 {
		t.Fatal("wrong origin accepted")
	}
	got := exchange(grant.Ticket, "")
	if got.Code != 303 || got.Header().Get("Location") != "/portal/" {
		t.Fatalf("exchange %d %s", got.Code, got.Body)
	}
	c := got.Result().Cookies()[0]
	if !c.HttpOnly || c.Secure != strings.HasPrefix(origin, "https://") || c.SameSite != http.SameSiteStrictMode || c.Domain != "" || c.Path != "/" {
		t.Fatal("unsafe cookie")
	}
	cookie := c.Name + "=" + c.Value
	if got := exchange(grant.Ticket, ""); got.Code != 401 {
		t.Fatal("ticket replay accepted")
	}
	sessionResponse := doJSON(t, h, "GET", "/api/portal/v1/session", map[string]string{"Cookie": cookie}, nil)
	var session clientapi.PortalSession
	decodeJSON(t, sessionResponse, &session)
	if sessionResponse.Code != 200 || session.SessionId != principal.SessionID || session.ExpiresAt.After(id.Now().Add(10*time.Minute)) {
		t.Fatalf("invalid Portal session %d", sessionResponse.Code)
	}
	for _, path := range []string{"/api/client/v1/bootstrap", "/api/admin/v1/session"} {
		if got := doJSON(t, h, "GET", path, map[string]string{"Cookie": cookie}, nil); got.Code != 401 {
			t.Fatalf("cookie escaped scope: %s status %d", path, got.Code)
		}
	}
	if got := doJSON(t, h, "POST", "/api/client/v1/portal/grants", map[string]string{"Cookie": cookie}, nil); got.Code != 401 {
		t.Fatal("cookie issued grant")
	}
	if got := doJSON(t, h, "GET", "/api/client/v1/enterprise/updates", map[string]string{"Cookie": cookie}, nil); got.Code != 200 {
		t.Fatalf("feed %d %s", got.Code, got.Body)
	}
	if got := doJSON(t, h, "DELETE", "/api/portal/v1/session", map[string]string{"Cookie": cookie, "Origin": id.PublicOrigin, "X-CSRF-Token": "wrong"}, nil); got.Code != 403 {
		t.Fatalf("CSRF accepted: %d", got.Code)
	}
	if got := doJSON(t, h, "DELETE", "/api/portal/v1/session", map[string]string{"Cookie": cookie, "X-CSRF-Token": session.CsrfToken}, nil); got.Code != 403 {
		t.Fatal("missing origin accepted")
	}
	// Recreate the handler/service boundary: consumed tickets and cookies survive.
	h = httpapi.NewFull(httpapi.Services{Identity: id, EnterpriseUpdate: updates})
	if got := exchange(grant.Ticket, ""); got.Code != 401 {
		t.Fatal("recreated handler allowed replay")
	}
	if got := doJSON(t, h, "DELETE", "/api/portal/v1/session", map[string]string{"Cookie": cookie, "Origin": id.PublicOrigin, "X-CSRF-Token": session.CsrfToken}, nil); got.Code != 204 {
		t.Fatalf("close %d %s", got.Code, got.Body)
	}
	if got := doJSON(t, h, "GET", "/api/portal/v1/session", map[string]string{"Cookie": cookie}, nil); got.Code != 401 {
		t.Fatal("closed session accepted")
	}
	after, err := id.Client.Session.Get(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ExpiresAt.Equal(parent.ExpiresAt) || after.Status != parent.Status {
		t.Fatal("Portal mutated parent session")
	}

	t.Run("concurrent_one_use", func(t *testing.T) {
		g, err := id.CreatePortalGrant(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		success := make(chan string, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, _, err := id.ExchangePortalGrant(ctx, g.Ticket)
				if err == nil {
					success <- c
				}
			}()
		}
		wg.Wait()
		close(success)
		cookies := []string{}
		for c := range success {
			cookies = append(cookies, c)
		}
		if len(cookies) != 1 {
			t.Fatalf("successful exchanges=%d", len(cookies))
		}
		if _, err := id.AuthenticatePortal(ctx, cookies[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := id.Client.Device.UpdateOneID(principal.DeviceID).SetStatus("REVOKED").Save(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := id.AuthenticatePortal(ctx, cookies[0]); err == nil {
			t.Fatal("revoked parent device accepted")
		}
	})
}

func TestPortalExpiryAndOriginValidation(t *testing.T) {
	h, id, _, ctx, _ := setupFullHandler(t)
	id.PublicOrigin = "https://platform.example"
	admin, csrf := loginAdmin(t, h)
	token := enrollClient(t, h, admin, csrf)
	now := id.Now()
	id.Now = func() time.Time { return now }
	id.Signer.Now = id.Now
	grant, err := id.CreatePortalGrant(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, _, err := id.ExchangePortalGrant(ctx, grant.Ticket); err == nil {
		t.Fatal("expired ticket accepted")
	}
	grant, err = id.CreatePortalGrant(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	cookie, expiry, err := id.ExchangePortalGrant(ctx, grant.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	now = expiry
	if _, err := id.AuthenticatePortal(context.Background(), cookie); err == nil {
		t.Fatal("expired cookie accepted")
	}
}
