package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
)

func TestAdminSecretMetadataCanBeSelectedAfterReload(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	identity := testutil.NewIdentityService(t, st, now)
	if _, err := identity.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	box, err := security.NewSecretBox(make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewFull(httpapi.Services{Identity: identity, Upstream: upstream.NewService(st.Client, box)})
	if got := doJSON(t, h, http.MethodGet, "/api/admin/v1/secrets", nil, nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated secret list status=%d", got.Code)
	}
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	for _, name := range []string{"First", "Second", "Third"} {
		response := doJSON(t, h, http.MethodPost, "/api/admin/v1/secrets", headers, map[string]any{
			"name": name, "value": "private-" + name,
		})
		if response.Code != http.StatusCreated {
			t.Fatalf("create secret status=%d body=%s", response.Code, response.Body.String())
		}
	}
	all := doJSON(t, h, http.MethodGet, "/api/admin/v1/secrets", map[string]string{"Cookie": cookie}, nil)
	var initial struct {
		Items []struct {
			SecretID string `json:"secretId"`
		} `json:"items"`
	}
	decodeJSON(t, all, &initial)
	if len(initial.Items) != 3 {
		t.Fatalf("initial secret count=%d", len(initial.Items))
	}
	replaced := doJSON(t, h, http.MethodPost, "/api/admin/v1/secrets/"+initial.Items[0].SecretID+":replace", headers, map[string]any{
		"expectedSecretVersion": 1, "value": "private-updated",
	})
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace status=%d body=%s", replaced.Code, replaced.Body.String())
	}

	first := doJSON(t, h, http.MethodGet, "/api/admin/v1/secrets?limit=2", map[string]string{"Cookie": cookie}, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", first.Code, first.Body.String())
	}
	var page struct {
		Items []struct {
			SecretID      string `json:"secretId"`
			Name          string `json:"name"`
			SecretVersion int    `json:"secretVersion"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	decodeJSON(t, first, &page)
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("first secret page count=%d cursor=%q", len(page.Items), page.NextCursor)
	}
	if strings.Contains(first.Body.String(), "private-") || strings.Contains(first.Body.String(), "encryptedPayload") {
		t.Fatal("secret list leaked credential material")
	}
	for _, item := range page.Items {
		if item.SecretID == "" || item.Name == "" || item.SecretVersion != 1 && item.SecretVersion != 2 {
			t.Fatalf("invalid secret metadata: %+v", item)
		}
	}
	if page.Items[0].SecretID == initial.Items[0].SecretID && page.Items[0].SecretVersion != 2 {
		t.Fatal("secret list did not expose the latest version")
	}
	second := doJSON(t, h, http.MethodGet, "/api/admin/v1/secrets?limit=2&cursor="+page.NextCursor, map[string]string{"Cookie": cookie}, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", second.Code, second.Body.String())
	}
	var next map[string]json.RawMessage
	decodeJSON(t, second, &next)
	var items []map[string]any
	if err := json.Unmarshal(next["items"], &items); err != nil || len(items) != 1 {
		t.Fatalf("second page count=%d error=%v", len(items), err)
	}
	if items[0]["secretId"] == page.Items[0].SecretID || items[0]["secretId"] == page.Items[1].SecretID {
		t.Fatal("secret list repeated an item across pages")
	}
}
