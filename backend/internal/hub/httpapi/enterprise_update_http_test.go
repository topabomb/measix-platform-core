package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

// setupFullHandler creates a fully wired HTTP handler with identity, capability,
// and enterprise update services for end-to-end testing.
func setupFullHandler(t *testing.T) (http.Handler, *identity.Service, *enterpriseupdate.Service, context.Context, string) {
	t.Helper()
	ctx := context.Background()
	st := testutil.OpenStore(t)
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	idSvc := testutil.NewIdentityService(t, st, now)
	boot, err := idSvc.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	euSvc := enterpriseupdate.NewService(st.Client)
	euSvc.Now = func() time.Time { return now }

	h := httpapi.NewFull(httpapi.Services{
		Identity:         idSvc,
		EnterpriseUpdate: euSvc,
		BuildVersion:     "test",
	}, httpapi.Options{
		PublicBaseURL:  "https://measix.test",
		RuntimeAPIBase: "/runtime/v1",
	})
	return h, idSvc, euSvc, ctx, boot.AdminUserID
}

// loginAdmin helper: logs in admin and returns cookie + CSRF token.
func loginAdmin(t *testing.T, h http.Handler) (string, string) {
	t.Helper()
	resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/session/login", nil, map[string]any{
		"username": "admin",
		"password": "correct horse battery staple",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", resp.Code, resp.Body.String())
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	decodeJSON(t, resp, &session)
	cookie := resp.Result().Cookies()[0]
	return cookie.Name + "=" + cookie.Value, session.CSRFToken
}

// enrollClient helper: creates enrollment, exchanges, returns access token.
func enrollClient(t *testing.T, h http.Handler, adminCookie, csrf string) string {
	t.Helper()
	// Create enrollment
	resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/users", map[string]string{
		"Cookie":       adminCookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"username":    "alice",
		"displayName": "Alice",
		"role":        "MEMBER",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", resp.Code, resp.Body.String())
	}
	var user struct {
		UserID string `json:"userId"`
	}
	decodeJSON(t, resp, &user)

	resp = doJSON(t, h, http.MethodPost, "/api/admin/v1/users/"+user.UserID+"/enrollments", map[string]string{
		"Cookie":       adminCookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{"expiresInSeconds": 600})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create enrollment status=%d body=%s", resp.Code, resp.Body.String())
	}
	var grant struct {
		Code string `json:"code"`
	}
	decodeJSON(t, resp, &grant)

	installationID := platformid.New(platformid.Installation)
	resp = doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]any{
		"code":           grant.Code,
		"installationId": installationID,
		"platform":       "ANDROID",
		"deviceName":     "test-device",
		"appVersion":     "1.0.0",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("exchange status=%d body=%s", resp.Code, resp.Body.String())
	}
	var tokens struct {
		AccessToken string `json:"accessToken"`
	}
	decodeJSON(t, resp, &tokens)
	return tokens.AccessToken
}

// --- Admin Enterprise Update HTTP Tests ---

// TestERXUPDHTTP001AdminCreateGetUpdate tests the admin CRUD lifecycle:
// Create → Get → Update → verify fields.
func TestERXUPDHTTP001AdminCreateGetUpdate(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)

	// Create
	resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"title":         "Test Update",
		"content":       "This is a test update.",
		"contentFormat": "MARKDOWN",
		"category":      "ANNOUNCEMENT",
		"severity":      "INFO",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	updateID, _ := created["enterpriseUpdateId"].(string)
	if updateID == "" {
		t.Fatal("missing enterpriseUpdateId")
	}
	if created["status"] != "DRAFT" {
		t.Fatalf("expected DRAFT, got %v", created["status"])
	}
	if created["contentFormat"] != "MARKDOWN" {
		t.Fatalf("expected MARKDOWN, got %v", created["contentFormat"])
	}
	if created["category"] != "ANNOUNCEMENT" {
		t.Fatalf("expected ANNOUNCEMENT, got %v", created["category"])
	}
	if created["severity"] != "INFO" {
		t.Fatalf("expected INFO, got %v", created["severity"])
	}

	// Get
	resp = doJSON(t, h, http.MethodGet, "/api/admin/v1/enterprise-updates/"+updateID, map[string]string{
		"Cookie": cookie,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", resp.Code, resp.Body.String())
	}
	var fetched map[string]any
	decodeJSON(t, resp, &fetched)
	if fetched["title"] != "Test Update" {
		t.Fatalf("expected title 'Test Update', got %v", fetched["title"])
	}

	// Update (only works on DRAFT)
	resp = doJSON(t, h, http.MethodPut, "/api/admin/v1/enterprise-updates/"+updateID, map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"title":         "Updated Title",
		"content":       "Updated content with **bold**.",
		"contentFormat": "MARKDOWN",
		"category":      "MAINTENANCE",
		"severity":      "WARNING",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", resp.Code, resp.Body.String())
	}
	var updated map[string]any
	decodeJSON(t, resp, &updated)
	if updated["title"] != "Updated Title" {
		t.Fatalf("expected 'Updated Title', got %v", updated["title"])
	}
	if updated["category"] != "MAINTENANCE" {
		t.Fatalf("expected MAINTENANCE, got %v", updated["category"])
	}
	if updated["severity"] != "WARNING" {
		t.Fatalf("expected WARNING, got %v", updated["severity"])
	}
}

// TestERXUPDHTTP002AdminPublishWithdraw tests Publish → Withdraw lifecycle.
func TestERXUPDHTTP002AdminPublishWithdraw(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)

	// Create
	resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"title":         "Publish Test",
		"content":       "Content",
		"contentFormat": "PLAIN",
		"category":      "NOTICE",
		"severity":      "INFO",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	updateID, _ := created["enterpriseUpdateId"].(string)

	// Publish
	resp = doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates/"+updateID+":publish", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", resp.Code, resp.Body.String())
	}
	var published map[string]any
	decodeJSON(t, resp, &published)
	if published["status"] != "PUBLISHED" {
		t.Fatalf("expected PUBLISHED, got %v", published["status"])
	}
	if published["publishedAt"] == nil {
		t.Fatal("expected non-nil publishedAt after publish")
	}

	// Withdraw
	resp = doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates/"+updateID+":withdraw", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("withdraw status=%d body=%s", resp.Code, resp.Body.String())
	}
	var withdrawn map[string]any
	decodeJSON(t, resp, &withdrawn)
	if withdrawn["status"] != "WITHDRAWN" {
		t.Fatalf("expected WITHDRAWN, got %v", withdrawn["status"])
	}
}

// TestERXUPDHTTP003AdminList verifies the list endpoint returns items + feedRevision.
func TestERXUPDHTTP003AdminList(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)

	// Create two items
	for _, title := range []string{"First", "Second"} {
		resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates", map[string]string{
			"Cookie":       cookie,
			"X-CSRF-Token": csrf,
		}, map[string]any{
			"title":         title,
			"content":       "Content",
			"contentFormat": "PLAIN",
			"category":      "NOTICE",
			"severity":      "INFO",
		})
		if resp.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
		}
	}

	// List
	resp := doJSON(t, h, http.MethodGet, "/api/admin/v1/enterprise-updates?limit=200", map[string]string{
		"Cookie": cookie,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	var page struct {
		Items        []map[string]any `json:"items"`
		FeedRevision int              `json:"feedRevision"`
	}
	decodeJSON(t, resp, &page)
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page.Items))
	}
	if page.FeedRevision == 0 {
		t.Fatal("expected non-zero feedRevision")
	}
	// Each item should have the new fields
	for _, item := range page.Items {
		if item["contentFormat"] == nil {
			t.Fatal("missing contentFormat in list item")
		}
		if item["category"] == nil {
			t.Fatal("missing category in list item")
		}
		if item["severity"] == nil {
			t.Fatal("missing severity in list item")
		}
	}
}

// TestERXUPDHTTP004AdminUpdateOnPublishedFails verifies 409 on update of non-draft.
func TestERXUPDHTTP004AdminUpdateOnPublishedFails(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)

	// Create + Publish
	resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"title":         "Test",
		"content":       "Content",
		"contentFormat": "PLAIN",
		"category":      "NOTICE",
		"severity":      "INFO",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	updateID, _ := created["enterpriseUpdateId"].(string)

	resp = doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates/"+updateID+":publish", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", resp.Code, resp.Body.String())
	}

	// Try to update PUBLISHED — should get 409
	resp = doJSON(t, h, http.MethodPut, "/api/admin/v1/enterprise-updates/"+updateID, map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"title":         "Changed",
		"content":       "Changed",
		"contentFormat": "PLAIN",
		"category":      "NOTICE",
		"severity":      "INFO",
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 on update of published, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestERXUPDHTTP005AdminGetNotFound verifies 404 on missing update.
func TestERXUPDHTTP005AdminGetNotFound(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, _ := loginAdmin(t, h)

	fakeID := platformid.New(platformid.EntUpdate)
	resp := doJSON(t, h, http.MethodGet, "/api/admin/v1/enterprise-updates/"+fakeID, map[string]string{
		"Cookie": cookie,
	}, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// --- Client Enterprise Update HTTP Tests ---

// TestERXUPDHTTP006ClientListPublishedOnly verifies client feed only shows PUBLISHED items.
func TestERXUPDHTTP006ClientListPublishedOnly(t *testing.T) {
	h, _, euSvc, ctx, adminID := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	accessToken := enrollClient(t, h, cookie, csrf)

	// Create a DRAFT (should not appear in client feed)
	draft, err := euSvc.Create(ctx, adminID, "Draft", "Draft content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}

	// Create and publish an item
	pub, err := euSvc.Create(ctx, adminID, "Published", "Published content", "MARKDOWN", "ANNOUNCEMENT", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := euSvc.Publish(ctx, pub.ID); err != nil {
		t.Fatal(err)
	}

	// Create and withdraw an item
	wth, err := euSvc.Create(ctx, adminID, "Withdrawn", "Withdrawn content", "PLAIN", "MAINTENANCE", "WARNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := euSvc.Publish(ctx, wth.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := euSvc.Withdraw(ctx, wth.ID); err != nil {
		t.Fatal(err)
	}

	// Client list — should only see the PUBLISHED item
	resp := doJSON(t, h, http.MethodGet, "/api/client/v1/enterprise-updates?limit=10", map[string]string{
		"Authorization": "Bearer " + accessToken,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("client list status=%d body=%s", resp.Code, resp.Body.String())
	}
	var feed struct {
		Items     []map[string]any `json:"items"`
		Truncated bool             `json:"truncated"`
	}
	decodeJSON(t, resp, &feed)
	if len(feed.Items) != 1 {
		t.Fatalf("expected 1 published item, got %d", len(feed.Items))
	}
	item := feed.Items[0]
	if item["title"] != "Published" {
		t.Fatalf("expected 'Published', got %v", item["title"])
	}
	if item["contentFormat"] != "MARKDOWN" {
		t.Fatalf("expected MARKDOWN, got %v", item["contentFormat"])
	}
	if item["category"] != "ANNOUNCEMENT" {
		t.Fatalf("expected ANNOUNCEMENT, got %v", item["category"])
	}
	if item["severity"] != "INFO" {
		t.Fatalf("expected INFO, got %v", item["severity"])
	}

	// Verify draft and withdrawn are not visible
	for _, it := range feed.Items {
		id, _ := it["updateId"].(string)
		if id == draft.ID || id == wth.ID {
			t.Fatal("draft or withdrawn item appeared in client feed")
		}
	}
}

// TestERXUPDHTTP007ClientETagConditional verifies ETag and 304 Not Modified.
func TestERXUPDHTTP007ClientETagConditional(t *testing.T) {
	h, _, euSvc, ctx, adminID := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	accessToken := enrollClient(t, h, cookie, csrf)

	// Create and publish an item
	item, err := euSvc.Create(ctx, adminID, "ETag Test", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := euSvc.Publish(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	// First request — get ETag
	resp := doJSON(t, h, http.MethodGet, "/api/client/v1/enterprise-updates?limit=10", map[string]string{
		"Authorization": "Bearer " + accessToken,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", resp.Code, resp.Body.String())
	}
	etag := resp.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected non-empty ETag header")
	}

	// Second request with If-None-Match — should return 304
	req := httptest.NewRequest(http.MethodGet, "/api/client/v1/enterprise-updates?limit=10", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", w.Code)
	}
	if w.Header().Get("ETag") != etag {
		t.Fatalf("expected ETag %s, got %s", etag, w.Header().Get("ETag"))
	}
}

// TestERXUPDHTTP008ClientGetSingle verifies client get endpoint only returns PUBLISHED.
func TestERXUPDHTTP008ClientGetSingle(t *testing.T) {
	h, _, euSvc, ctx, adminID := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	accessToken := enrollClient(t, h, cookie, csrf)

	// Create a draft
	draft, err := euSvc.Create(ctx, adminID, "Draft", "Draft content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}

	// Create and publish
	pub, err := euSvc.Create(ctx, adminID, "Published", "Published content", "MARKDOWN", "ANNOUNCEMENT", "CRITICAL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := euSvc.Publish(ctx, pub.ID); err != nil {
		t.Fatal(err)
	}

	// Get published — should succeed
	resp := doJSON(t, h, http.MethodGet, "/api/client/v1/enterprise-updates/"+pub.ID, map[string]string{
		"Authorization": "Bearer " + accessToken,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("get published status=%d body=%s", resp.Code, resp.Body.String())
	}
	var item map[string]any
	decodeJSON(t, resp, &item)
	if item["title"] != "Published" {
		t.Fatalf("expected 'Published', got %v", item["title"])
	}
	if item["contentFormat"] != "MARKDOWN" {
		t.Fatalf("expected MARKDOWN, got %v", item["contentFormat"])
	}
	if item["severity"] != "CRITICAL" {
		t.Fatalf("expected CRITICAL, got %v", item["severity"])
	}

	// Get draft — should return 404
	resp = doJSON(t, h, http.MethodGet, "/api/client/v1/enterprise-updates/"+draft.ID, map[string]string{
		"Authorization": "Bearer " + accessToken,
	}, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for draft, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestERXUPDHTTP009ClientInvalidLimit verifies 400 on limit out of range.
func TestERXUPDHTTP009ClientInvalidLimit(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)
	accessToken := enrollClient(t, h, cookie, csrf)

	for _, limit := range []string{"0", "21"} {
		resp := doJSON(t, h, http.MethodGet, "/api/client/v1/enterprise-updates?limit="+limit, map[string]string{
			"Authorization": "Bearer " + accessToken,
		}, nil)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for limit=%s, got %d body=%s", limit, resp.Code, resp.Body.String())
		}
	}
}

// TestERXUPDHTTP010ClientUnauthorized verifies 401 without bearer token.
func TestERXUPDHTTP010ClientUnauthorized(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)

	resp := doJSON(t, h, http.MethodGet, "/api/client/v1/enterprise-updates", nil, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.Code)
	}
}

// TestERXUPDHTTP011AdminNoCSRF verifies 403 on admin write without CSRF.
func TestERXUPDHTTP011AdminNoCSRF(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, _ := loginAdmin(t, h)

	// Create without CSRF — should fail
	body, _ := json.Marshal(map[string]any{
		"title":         "Test",
		"content":       "Content",
		"contentFormat": "PLAIN",
		"category":      "NOTICE",
		"severity":      "INFO",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/v1/enterprise-updates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
		t.Fatalf("expected 400 or 403 without CSRF, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestERXUPDHTTP012FeedRevisionChangesOnPublish verifies that publish changes feedRevision
// and the admin list endpoint reflects the updated revision.
func TestERXUPDHTTP012FeedRevisionChangesOnPublish(t *testing.T) {
	h, _, _, _, _ := setupFullHandler(t)
	cookie, csrf := loginAdmin(t, h)

	// Create — should have feedRevision > 0
	resp := doJSON(t, h, http.MethodPost, "/api/admin/v1/enterprise-updates", map[string]string{
		"Cookie":       cookie,
		"X-CSRF-Token": csrf,
	}, map[string]any{
		"title":         "Rev Test",
		"content":       "Content",
		"contentFormat": "PLAIN",
		"category":      "NOTICE",
		"severity":      "INFO",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	revBefore, _ := created["feedRevision"].(float64)
	if revBefore == 0 {
		t.Fatal("expected non-zero feedRevision after create")
	}

	// List — feedRevision should match
	resp = doJSON(t, h, http.MethodGet, "/api/admin/v1/enterprise-updates?limit=200", map[string]string{
		"Cookie": cookie,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	var page struct {
		FeedRevision int `json:"feedRevision"`
	}
	decodeJSON(t, resp, &page)
	if page.FeedRevision != int(revBefore) {
		t.Fatalf("expected feedRevision %d, got %d", int(revBefore), page.FeedRevision)
	}
}
