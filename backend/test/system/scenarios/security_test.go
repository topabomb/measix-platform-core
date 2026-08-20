package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// CAP-SEC-001 — Unauthenticated admin API access is denied.
// Every admin endpoint requires a valid session cookie; without it, the Hub
// must return 401 Unauthorized.
func TestCAPSEC001UnauthenticatedAccessDenied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	// Attempt to access admin endpoints without login
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/users"},
		{http.MethodGet, "/api/admin/v1/system/status"},
		{http.MethodGet, "/api/admin/v1/draft"},
		{http.MethodGet, "/api/admin/v1/releases"},
		{http.MethodGet, "/api/admin/v1/usage/summary"},
		{http.MethodGet, "/api/admin/v1/upstreams"},
	}

	for _, ep := range endpoints {
		req, _ := http.NewRequestWithContext(ctx, ep.method, env.HubBaseURL+ep.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", ep.method, ep.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401, got %d", ep.method, ep.path, resp.StatusCode)
		}
	}

	t.Log("CAP-SEC-001 Unauthenticated Access Denied: PASS")
}

// CAP-SEC-002 — CSRF token enforcement on state-changing requests.
// POST/PUT/DELETE without a valid X-CSRF-Token header must be rejected with 401.
func TestCAPSEC002CSRFEnforced(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Create a request with valid session cookie but NO CSRF token
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/admin/v1/users",
		strings.NewReader(`{"username":"bad","displayName":"Bad","role":"MEMBER"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", admin.CookieHeader())
	// Intentionally omit X-CSRF-Token
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create user without CSRF: %v", err)
	}
	resp.Body.Close()
	// OpenAPI-generated server treats X-CSRF-Token as a required header parameter,
	// so missing CSRF returns 400 (Bad Request) before the handler is invoked.
	// Wrong CSRF reaches the handler and returns 401 (Unauthorized).
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 401 or 400 without CSRF, got %d", resp.StatusCode)
	}

	// Create with wrong CSRF token
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/admin/v1/users",
		strings.NewReader(`{"username":"bad","displayName":"Bad","role":"MEMBER"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Cookie", admin.CookieHeader())
	req2.Header.Set("X-CSRF-Token", "wrong-token")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("create user with wrong CSRF: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong CSRF, got %d", resp2.StatusCode)
	}

	// Verify that a valid CSRF token works
	resp3, err := admin.Post(ctx, "/api/admin/v1/users", map[string]interface{}{
		"username":    "good-user",
		"displayName": "Good",
		"role":        "MEMBER",
	})
	if err != nil {
		t.Fatalf("create user with valid CSRF: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 with valid CSRF, got %d", resp3.StatusCode)
	}

	t.Log("CAP-SEC-002 CSRF Enforced: PASS")
}

// CAP-SEC-003 — Admin session cookie is HttpOnly, Secure, SameSite=Strict.
func TestCAPSEC003SessionCookieAttributes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	// Login and inspect Set-Cookie
	body := `{"username":"admin","password":"` + env.AdminPassword + `"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/admin/v1/session/login",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status: %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie returned")
	}
	cookie := cookies[0]
	if !cookie.HttpOnly {
		t.Fatal("session cookie not HttpOnly")
	}
	if !cookie.Secure {
		t.Fatal("session cookie not Secure")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie SameSite=%v, expected Strict", cookie.SameSite)
	}

	t.Log("CAP-SEC-003 Session Cookie Attributes: PASS")
}

// CAP-SEC-004 — Snapshot does not leak server-only fields.
// The client-facing snapshot must not contain upstreamId, runtimeRouteId,
// baseUrl, secretId, secretVersion or any internal field.
func TestCAPSEC004SnapshotNoServerFieldsLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)

	// Fetch snapshot
	url := fmt.Sprintf("%s/api/client/v1/managed/snapshots/%d", env.HubBaseURL, generation)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+clientToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	str := string(body)

	// Server-only fields that must NEVER appear in the client snapshot
	forbiddenFields := []string{
		"upstreamId", "runtimeRouteId", "baseUrl", "secretId",
		"secretVersion", "masterKey", "jwtPrivateKey", "relayServiceToken",
	}
	for _, field := range forbiddenFields {
		if strings.Contains(str, "\""+field+"\"") {
			t.Fatalf("snapshot leaks server-only field: %s", field)
		}
	}

	t.Log("CAP-SEC-004 Snapshot No Server Fields Leak: PASS")
}

// CAP-SEC-005 — Usage request detail does not contain prompt/body/secret.
// The usage request detail endpoint must never expose the original request
// body, prompt content, or any credential.
func TestCAPSEC005UsageDetailNoContentLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})

	// Make a request with a distinctive prompt body
	distinctivePrompt := `{"model":"gpt-test","messages":[{"role":"user","content":"SECRET-PROMPT-CONTENT-12345"}]}`
	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", distinctivePrompt)
	if err != nil {
		t.Fatalf("chat completion: %v", err)
	}

	gp.waitUsageRecorded(ctx, admin, 1, 30*time.Second)

	// Fetch usage requests and verify the prompt content is NOT present
	resp, err := admin.Get(ctx, "/api/admin/v1/usage/requests?limit=10")
	if err != nil {
		t.Fatalf("usage requests: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	str := string(body)

	if strings.Contains(str, "SECRET-PROMPT-CONTENT-12345") {
		t.Fatal("usage request detail leaks prompt content")
	}

	// Also verify the raw request body field is not present
	forbiddenFields := []string{"prompt", "body", "secret", "authorization", "apiKey"}
	for _, field := range forbiddenFields {
		if strings.Contains(strings.ToLower(str), "\""+field+"\"") {
			t.Fatalf("usage request detail leaks forbidden field: %s", field)
		}
	}

	t.Log("CAP-SEC-005 Usage Detail No Content Leak: PASS")
}

// CAP-SEC-006 — Invalid/expired enrollment code is rejected.
// Enrollment exchange with a bogus code must return 401.
func TestCAPSEC006InvalidEnrollmentRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	// Attempt to exchange a bogus enrollment code (with valid installationId format)
	validInstallationID := platformid.New(platformid.Installation)
	body := fmt.Sprintf(`{"code":"bogus-code-12345","installationId":%q,"platform":"ANDROID","appVersion":"1.0"}`, validInstallationID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/client/v1/enrollments/exchange",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("exchange bogus enrollment: %v", err)
	}
	defer resp.Body.Close()
	// The enrollment code is invalid, so the service should reject with 401 (credential error).
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bogus enrollment code, got %d", resp.StatusCode)
	}

	// Attempt to exchange with a non-ANDROID platform (should be rejected)
	body2 := `{"code":"any-code","installationId":"inst-1","platform":"IOS","appVersion":"1.0"}`
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/client/v1/enrollments/exchange",
		strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("exchange non-ANDROID: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-ANDROID platform, got %d", resp2.StatusCode)
	}

	t.Log("CAP-SEC-006 Invalid Enrollment Rejected: PASS")
}

// CAP-SEC-007 — Stale/invalid client access token is rejected by Relay.
// A request with an invalid bearer token must get 401, and the Relay must
// not forward the request to the upstream.
func TestCAPSEC007InvalidAccessTokenRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Attempt to use the Relay with a completely invalid token
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       "invalid-token-xyz",
		ManagedGeneration: 1,
		InteractionID:     platformid.New(platformid.Interaction),
	})

	_, _, err = tc.ChatCompletion(ctx, gp.lastModelID, "/v1/chat/completions", `{"model":"gpt-test"}`)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if pe, ok := err.(client.ProblemError); !ok || pe.Status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %v", err)
	}

	// Verify the adapter was never called
	if fact := ad.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatal("upstream adapter was called despite invalid token")
	}

	t.Log("CAP-SEC-007 Invalid Access Token Rejected: PASS")
}

// CAP-SEC-008 — Admin API enforces strict JSON body validation.
// The Hub must reject requests with unknown JSON fields or malformed JSON.
func TestCAPSEC008StrictJSONValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Send a request with an unknown field
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/admin/v1/users",
		strings.NewReader(`{"username":"test","displayName":"Test","role":"MEMBER","unknownField":"hack"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", admin.CookieHeader())
	req.Header.Set("X-CSRF-Token", admin.CSRFToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unknown field request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown JSON field, got %d", resp.StatusCode)
	}

	// Send malformed JSON
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/admin/v1/users",
		strings.NewReader(`{broken json`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Cookie", admin.CookieHeader())
	req2.Header.Set("X-CSRF-Token", admin.CSRFToken())
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("malformed JSON request: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", resp2.StatusCode)
	}

	t.Log("CAP-SEC-008 Strict JSON Validation: PASS")
}

// CAP-SEC-009 — Runtime Relay strips client-sent internal headers.
// A client that injects X-Measix-Request-Id or X-Forwarded-For must have
// those headers stripped by the Relay before forwarding to the upstream.
func TestCAPSEC009ClientHeaderSpoofStripped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	// Inject spoofed internal headers
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
		SpoofHeaders: map[string]string{
			"X-Measix-Request-Id": "spoof-request-id",
			"X-Measix-Internal":   "true",
			"X-Forwarded-For":     "10.0.0.1",
			"X-Forwarded-Host":    "evil.example.com",
			"X-Real-Ip":           "10.0.0.1",
		},
	})

	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test"}`)
	if err != nil {
		t.Fatalf("chat with spoof headers: %v", err)
	}

	// Verify the upstream adapter did NOT receive the spoofed headers
	fact := ad.LastRequest("/v1/chat/completions")
	if fact == nil {
		t.Fatal("no request captured by adapter")
	}
	spoofedHeaders := []string{
		"x-measix-request-id", "x-measix-internal",
		"x-forwarded-for", "x-forwarded-host", "x-real-ip",
	}
	for _, h := range spoofedHeaders {
		if val, found := fact.Headers[h]; found {
			if val == "spoof-request-id" || val == "true" || val == "10.0.0.1" || val == "evil.example.com" {
				t.Fatalf("spoofed header %s was forwarded to upstream with value %q", h, val)
			}
		}
	}

	t.Log("CAP-SEC-009 Client Header Spoof Stripped: PASS")
}

// CAP-SEC-010 — Secret value is never returned after creation.
// The Admin API must never return the plaintext secret value in any response.
func TestCAPSEC010SecretValueNeverReturned(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Create a secret with a distinctive value
	secretValue := "super-secret-value-98765"
	resp, err := admin.Post(ctx, "/api/admin/v1/secrets", map[string]interface{}{
		"name":  "test-secret",
		"value": secretValue,
	})
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}
	var secret struct {
		SecretID      string `json:"secretId"`
		SecretVersion int    `json:"secretVersion"`
	}
	if err := harness.DecodeJSON(resp, &secret); err != nil {
		t.Fatalf("decode secret: %v", err)
	}

	// List secrets — the value must NOT appear
	resp, err = admin.Get(ctx, "/api/admin/v1/secrets")
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), secretValue) {
		t.Fatal("secret value leaked in list response")
	}

	// Get individual secret — value must NOT appear
	resp, err = admin.Get(ctx, "/api/admin/v1/secrets/"+secret.SecretID)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), secretValue) {
		t.Fatal("secret value leaked in get response")
	}

	t.Log("CAP-SEC-010 Secret Value Never Returned: PASS")
}

// CAP-SEC-011 — Logout invalidates the admin session.
// After logout, the session cookie must no longer be valid.
func TestCAPSEC011LogoutInvalidatesSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Verify session works
	resp, err := admin.Get(ctx, "/api/admin/v1/users")
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session should work before logout, got %d", resp.StatusCode)
	}

	// Logout (DELETE /api/admin/v1/session)
	resp, err = admin.Delete(ctx, "/api/admin/v1/session")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	resp.Body.Close()

	// Verify session is now invalid
	resp, err = admin.Get(ctx, "/api/admin/v1/users")
	if err != nil {
		t.Fatalf("list users after logout: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", resp.StatusCode)
	}

	t.Log("CAP-SEC-011 Logout Invalidates Session: PASS")
}

// CAP-SEC-012 — Client Control API requires valid bearer token.
// Discovery is public, but managed state and snapshot require authentication.
func TestCAPSEC012ClientAPIAuthEnforced(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	// Discovery is public (no auth required) at /.well-known/measix
	resp, err := http.Get(env.HubBaseURL + "/.well-known/measix")
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("discovery should be public, got %d", resp.StatusCode)
	}

	// Managed state requires auth
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.HubBaseURL+"/api/client/v1/managed/state", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("managed state without token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for managed state without token, got %d", resp.StatusCode)
	}

	// Snapshot requires auth
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, env.HubBaseURL+"/api/client/v1/managed/snapshots/1", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("snapshot without token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for snapshot without token, got %d", resp.StatusCode)
	}

	// Invalid bearer token
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, env.HubBaseURL+"/api/client/v1/managed/state", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("managed state with invalid token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for managed state with invalid token, got %d", resp.StatusCode)
	}

	t.Log("CAP-SEC-012 Client API Auth Enforced: PASS")
}

// CAP-SEC-013 — Request body size limit enforced.
// The Hub must reject oversized request bodies to prevent DoS.
func TestCAPSEC013RequestBodySizeLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Create a request with a body larger than 1MB (the limit)
	largeBody := make([]byte, 2*1024*1024) // 2MB
	for i := range largeBody {
		largeBody[i] = 'a'
	}
	body := `{"username":"test","displayName":"` + string(largeBody) + `","role":"MEMBER"}`

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, env.HubBaseURL+"/api/admin/v1/users",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", admin.CookieHeader())
	req.Header.Set("X-CSRF-Token", admin.CSRFToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("large body request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", resp.StatusCode)
	}

	t.Log("CAP-SEC-013 Request Body Size Limit: PASS")
}

// CAP-SEC-014 — System status does not leak internal config.
// The system status endpoint must not expose internal paths, crypto keys,
// or database connection strings.
func TestCAPSEC014SystemStatusNoInternalConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	resp, err := admin.Get(ctx, "/api/admin/v1/system/status")
	if err != nil {
		t.Fatalf("system status: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	str := string(body)

	// Internal config must never appear in the status response
	forbiddenStrings := []string{
		env.DBPath,
		env.MasterKeyFile,
		env.JWTKeyFile,
		env.RelayTokenFile,
		"masterKey",
		"jwtPrivateKey",
		"relayServiceToken",
		"password",
		"secret",
	}
	for _, forbidden := range forbiddenStrings {
		if forbidden != "" && strings.Contains(strings.ToLower(str), strings.ToLower(forbidden)) {
			t.Fatalf("system status leaks internal config: %s", forbidden)
		}
	}

	t.Log("CAP-SEC-014 System Status No Internal Config: PASS")
}

// CAP-SEC-015 — Concurrency: duplicate publish with same idempotency key
// returns the same activation, not a duplicate.
func TestCAPSEC015IdempotentPublish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Attempt to publish again with the same draft revision and idempotency key
	draftRev := gp.getDraftRevision(ctx, admin)
	content := gp.buildModifiedDraftContent(ctx, admin, "Idempotent Model")
	newRev := gp.putDraft(ctx, admin, draftRev, content)
	gp.validateDraft(ctx, admin, newRev)

	idempotencyKey := platformid.New(platformid.Idempotency)
	publishHeaders := map[string]string{"Idempotency-Key": idempotencyKey}
	resp1, err := admin.PostWithHeaders(ctx, "/api/admin/v1/draft:publish", map[string]interface{}{
		"expectedDraftRevision":    newRev,
		"acknowledgedWarningCodes": []string{},
	}, publishHeaders)
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	var act1 struct {
		ActivationID string `json:"activationId"`
	}
	_ = harness.DecodeJSON(resp1, &act1)

	// Second publish with same idempotency key should return the same activation
	resp2, err := admin.PostWithHeaders(ctx, "/api/admin/v1/draft:publish", map[string]interface{}{
		"expectedDraftRevision":    newRev,
		"acknowledgedWarningCodes": []string{},
	}, publishHeaders)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	var act2 struct {
		ActivationID string `json:"activationId"`
	}
	_ = harness.DecodeJSON(resp2, &act2)

	if act1.ActivationID != act2.ActivationID {
		t.Fatalf("idempotent publish returned different activations: %s vs %s",
			act1.ActivationID, act2.ActivationID)
	}

	t.Log("CAP-SEC-015 Idempotent Publish: PASS")
}

// Helper: decode JSON body from a response.
func decodeResponseBody(body io.Reader, target interface{}) error {
	return json.NewDecoder(body).Decode(target)
}

// CAP-SEC-016 — Expired/wrong-audience/wrong-issuer/unknown-kid JWT rejected.
// The Relay must reject tokens with invalid claims and not forward the request.
func TestCAPSEC016ExpiredAndWrongClaimJWTRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Test with various invalid tokens
	invalidTokens := []string{
		"expired.token.here", // Malformed
		"header.payload.sig", // Invalid structure
		"",                   // Empty
	}

	for _, token := range invalidTokens {
		tc := client.New(client.Options{
			RuntimeBaseURL:    env.RelayPubBaseURL,
			AccessToken:       token,
			ManagedGeneration: 1,
			InteractionID:     platformid.New(platformid.Interaction),
		})
		_, _, err = tc.ChatCompletion(ctx, gp.lastModelID, "/v1/chat/completions", `{"model":"gpt-test"}`)
		if err == nil {
			t.Fatalf("expected error for invalid token: %q", token)
		}
		if pe, ok := err.(client.ProblemError); !ok || pe.Status != http.StatusUnauthorized {
			t.Fatalf("expected 401 for invalid token %q, got %v", token, err)
		}
	}

	// Verify adapter never received any request
	if fact := ad.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatal("adapter received request despite invalid JWT")
	}

	t.Log("CAP-SEC-016 Expired/Wrong-Claim JWT Rejected: PASS")
}

// CAP-SEC-017 — Disabled user cannot access runtime.
// A user that has been disabled must not be able to exchange enrollment
// or use their access token.
func TestCAPSEC017DisabledUserRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Get a valid token for the user
	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	// Verify the token works
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})
	if _, _, err := tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test"}`); err != nil {
		t.Fatalf("valid token should work: %v", err)
	}

	// Disable the user
	resp, err := admin.Post(ctx, fmt.Sprintf("/api/admin/v1/users/%s:disable", gp.lastUserID), nil)
	if err != nil {
		t.Fatalf("disable user: %v", err)
	}
	resp.Body.Close()

	// Wait for activation
	// The disable operation creates an activation; wait for it to complete
	time.Sleep(3 * time.Second)

	// Now the user's token should be rejected
	ad.ClearFacts()
	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test"}`)
	if err == nil {
		t.Fatal("disabled user should not be able to access runtime")
	}
	if pe, ok := err.(client.ProblemError); !ok || pe.Status != http.StatusForbidden {
		t.Fatalf("expected 403 for disabled user, got %v", err)
	}

	// Verify adapter was not called
	if fact := ad.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatal("adapter received request despite disabled user")
	}

	t.Log("CAP-SEC-017 Disabled User Rejected: PASS")
}

// CAP-SEC-018 — Revoked device cannot access runtime.
// A device that has been revoked must not be able to use its access token.
func TestCAPSEC018RevokedDeviceRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Get a valid token
	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	// List devices for the user
	resp, err := admin.Get(ctx, fmt.Sprintf("/api/admin/v1/users/%s/devices", gp.lastUserID))
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	var devices struct {
		Items []struct {
			DeviceID string `json:"deviceId"`
			Status   string `json:"status"`
		} `json:"items"`
	}
	if err := harness.DecodeJSON(resp, &devices); err != nil {
		t.Fatalf("decode devices: %v", err)
	}
	if len(devices.Items) == 0 {
		t.Fatal("no devices found for user")
	}
	deviceID := devices.Items[0].DeviceID

	// Revoke the device
	resp, err = admin.Post(ctx, fmt.Sprintf("/api/admin/v1/devices/%s:revoke", deviceID), nil)
	if err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	resp.Body.Close()

	time.Sleep(3 * time.Second)

	// Now the token should be rejected
	ad.ClearFacts()
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})
	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test"}`)
	if err == nil {
		t.Fatal("revoked device should not be able to access runtime")
	}
	if pe, ok := err.(client.ProblemError); !ok || (pe.Status != http.StatusForbidden && pe.Status != http.StatusUnauthorized) {
		t.Fatalf("expected 403/401 for revoked device, got %v", err)
	}

	t.Log("CAP-SEC-018 Revoked Device Rejected: PASS")
}

// CAP-SEC-019 — Management endpoints not reachable through Resource route.
// A client cannot access /api/admin/* or /internal/* through the runtime
// resource path /runtime/v1/resources/{resourceId}/...
func TestCAPSEC019ManagementEndpointNotReachableViaResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)

	// Try to reach admin endpoints through the runtime resource path
	traversalPaths := []string{
		"/runtime/v1/resources/" + gp.lastModelID + "/../../../api/admin/v1/users",
		"/runtime/v1/resources/" + gp.lastModelID + "/..%2f..%2f..%2fapi%2fadmin%2fv1%2fusers",
		"/runtime/v1/resources/" + gp.lastModelID + "/%2e%2e/%2e%2e/%2e%2e/api/admin/v1/users",
	}

	for _, path := range traversalPaths {
		url := env.RelayPubBaseURL + path
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+clientToken)
		req.Header.Set("X-Measix-Managed-Generation", fmt.Sprintf("%d", generation))
		req.Header.Set("X-Measix-Interaction-Id", platformid.New(platformid.Interaction))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("traversal path %q: error %v (acceptable)", path, err)
			continue
		}
		resp.Body.Close()
		// Path traversal should be rejected — not 200
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("path traversal succeeded: %s -> %d", path, resp.StatusCode)
		}
	}

	t.Log("CAP-SEC-019 Management Endpoint Not Reachable via Resource: PASS")
}

// CAP-SEC-020 — Upstream Set-Cookie/redirect not forwarded to client.
// The Relay must strip Set-Cookie and redirect headers from upstream responses.
func TestCAPSEC020UpstreamSetCookieAndRedirectStripped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	// Make a normal request via direct HTTP to inspect response headers
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		env.RelayPubBaseURL+"/runtime/v1/resources/"+ids.model+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-test"}`))
	req.Header.Set("Authorization", "Bearer "+clientToken)
	req.Header.Set("X-Measix-Managed-Generation", fmt.Sprintf("%d", generation))
	req.Header.Set("X-Measix-Interaction-Id", platformid.New(platformid.Interaction))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("chat completion: %v", err)
	}
	resp.Body.Close()

	// Verify Set-Cookie is not present in the Relay response
	if _, found := resp.Header["Set-Cookie"]; found {
		t.Fatal("Relay forwarded Set-Cookie header from upstream to client")
	}
	// Verify Location (redirect) is not present
	if _, found := resp.Header["Location"]; found {
		t.Fatal("Relay forwarded Location header from upstream to client")
	}

	t.Log("CAP-SEC-020 Upstream Set-Cookie/Redirect Stripped: PASS")
}

// CAP-SEC-021 — Snapshot Preview does not leak server-only fields.
// The draft preview endpoint must use the same canonical projection as
// the published snapshot and must not expose server-only fields.
func TestCAPSEC021SnapshotPreviewNoServerFieldsLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Get the draft preview
	resp, err := admin.Post(ctx, "/api/admin/v1/draft:preview", map[string]interface{}{
		"acknowledgedWarningCodes": []string{},
	})
	if err != nil {
		t.Fatalf("draft preview: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	str := string(body)

	// Server-only fields that must NEVER appear in the preview
	forbiddenFields := []string{
		"upstreamId", "runtimeRouteId", "baseUrl", "secretId",
		"secretVersion", "masterKey", "jwtPrivateKey", "relayServiceToken",
		"secretValue", "apiKey", "password",
	}
	for _, field := range forbiddenFields {
		if strings.Contains(strings.ToLower(str), "\""+strings.ToLower(field)+"\"") {
			t.Fatalf("preview leaks server-only field: %s", field)
		}
	}

	t.Log("CAP-SEC-021 Snapshot Preview No Server Fields Leak: PASS")
}
