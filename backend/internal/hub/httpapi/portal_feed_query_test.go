package httpapi_test

import (
	"testing"
	"time"

	"measix/platform/internal/wire/clientapi"
)

func TestPortalCookieFeedDatesETagAndRevocation(t *testing.T) {
	h, id, updates, ctx, adminID := setupFullHandler(t)
	id.PortalOrigin = "https://platform.example"
	admin, csrf := loginAdmin(t, h)
	token := enrollClient(t, h, admin, csrf)
	grant, err := id.CreatePortalGrant(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, err := id.ExchangePortalGrant(ctx, grant.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := id.AuthenticateAccess(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := id.Client.Deployment.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = id.Client.Deployment.UpdateOneID(deployment.ID).SetTimezone("Asia/Shanghai").Save(ctx); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"2026-08-27T15:59:59Z", "2026-08-27T16:00:00Z", "2026-08-28T15:59:59Z", "2026-08-28T16:00:00Z"} {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatal(err)
		}
		updates.Now = func() time.Time { return at }
		item, err := updates.Create(ctx, adminID, raw, "Body", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = updates.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	headers := map[string]string{"Cookie": "measix_portal_session=" + cookie}
	path := "/api/client/v1/enterprise/updates?limit=20&startDate=2026-08-28&endDate=2026-08-28"
	got := doJSON(t, h, "GET", path, headers, nil)
	var feed clientapi.EnterpriseUpdateFeed
	decodeJSON(t, got, &feed)
	if got.Code != 200 || len(feed.Items) != 2 || feed.EnterpriseTimezone != "Asia/Shanghai" {
		t.Fatalf("date boundary: %d %s", got.Code, got.Body)
	}
	headers["If-None-Match"] = got.Header().Get("ETag")
	if got := doJSON(t, h, "GET", path, headers, nil); got.Code != 304 {
		t.Fatalf("same query %d", got.Code)
	}
	if got := doJSON(t, h, "GET", "/api/client/v1/enterprise/updates?limit=20", headers, nil); got.Code != 200 {
		t.Fatalf("different query reused ETag %d", got.Code)
	}
	for _, query := range []string{"startDate=2026-08-29&endDate=2026-08-28", "startDate=2026-02-30", "limit=21"} {
		if got := doJSON(t, h, "GET", "/api/client/v1/enterprise/updates?"+query, headers, nil); got.Code != 400 {
			t.Fatalf("invalid query %s: %d", query, got.Code)
		}
	}
	if _, err = id.Client.Device.UpdateOneID(principal.DeviceID).SetStatus("REVOKED").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if got := doJSON(t, h, "GET", path, headers, nil); got.Code != 401 {
		t.Fatalf("ETag bypassed revoked auth %d", got.Code)
	}
}
