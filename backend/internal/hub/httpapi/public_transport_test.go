package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

// Real sockets and a cookie jar cover both cleartext and certificate-verified
// HTTPS. TLS is terminated by this test server, as by the deployment ingress.
func TestPublicHTTPAndHTTPSControlLifecycle(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "http"
		if secure {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			_, identity, updates, ctx, adminID := setupFullHandler(t)
			capabilityService := capability.NewService(identity.Client)
			h := httpapi.NewFull(httpapi.Services{Identity: identity, Capability: capabilityService, EnterpriseUpdate: updates})
			identity.Now = func() time.Time { return time.Now().UTC() }
			identity.Signer.Now = identity.Now
			server := httptest.NewUnstartedServer(h)
			if secure {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			identity.SetPublicOrigin(server.URL)
			client := server.Client()
			client.Jar, _ = cookiejar.New(nil)
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			send := func(method, path string, headers map[string]string, body any, want int, target any) *http.Response {
				t.Helper()
				var encoded []byte
				if form, ok := body.(url.Values); ok {
					encoded = []byte(form.Encode())
				} else if body != nil {
					var err error
					encoded, err = json.Marshal(body)
					if err != nil {
						t.Fatal(err)
					}
				}
				request, err := http.NewRequest(method, server.URL+path, bytes.NewReader(encoded))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				for key, value := range headers {
					request.Header.Set(key, value)
				}
				response, err := client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != want {
					t.Fatalf("%s %s: status %d want %d", method, path, response.StatusCode, want)
				}
				if target != nil {
					if err := json.NewDecoder(response.Body).Decode(target); err != nil {
						t.Fatal(err)
					}
				} else {
					_, _ = io.Copy(io.Discard, response.Body)
				}
				return response
			}
			var login struct {
				CSRFToken string `json:"csrfToken"`
			}
			response := send("POST", "/api/admin/v1/session/login", nil, map[string]string{"username": "admin", "password": "correct horse battery staple"}, 200, &login)
			if cookies := response.Cookies(); len(cookies) != 1 || cookies[0].Secure != secure || !cookies[0].HttpOnly {
				t.Fatal("incorrect Admin cookie flags")
			}
			adminHeaders := map[string]string{"X-CSRF-Token": login.CSRFToken}
			var grant adminapi.CreateEnrollmentResponse
			send("POST", "/api/admin/v1/users/"+adminID+"/enrollments", adminHeaders, map[string]int{"expiresInSeconds": 600}, 201, &grant)
			if grant.PlatformUrl != server.URL {
				t.Fatal("enrollment origin mismatch")
			}
			var session clientapi.EnrollmentExchangeResponse
			send("POST", "/api/client/v1/enrollments/exchange", nil, map[string]string{"code": grant.Code, "installationId": platformid.New(platformid.Installation), "deviceName": "Transport test", "appVersion": "test", "platform": "ANDROID"}, 201, &session)
			auth := map[string]string{"Authorization": "Bearer " + session.AccessToken}
			var devicePage struct {
				Items []struct {
					DeviceName string `json:"deviceName"`
				} `json:"items"`
			}
			send("GET", "/api/admin/v1/users/"+adminID+"/devices", nil, nil, 200, &devicePage)
			if len(devicePage.Items) != 1 || devicePage.Items[0].DeviceName != "Transport test" {
				t.Fatal("enrollment device name was lost in Admin response")
			}

			// Arrange a published release; publication itself is covered by the
			// Admin workflow suite and the real browser acceptance environment.
			raw, err := os.ReadFile("../../../../api/fixtures/draft/s02-client-profile.json")
			if err != nil {
				t.Fatal(err)
			}
			var draft adminapi.ManagedDraftContent
			if err := json.Unmarshal(raw, &draft); err != nil {
				t.Fatal(err)
			}
			releaseID := platformid.New(platformid.Release)
			snapshot, _, err := capability.NewService(identity.Client).CompileSnapshot(capability.SnapshotInput{DeploymentID: identity.Signer.DeploymentID, ReleaseID: releaseID, ManagedGeneration: 1, Content: draft, PublishedAt: identity.Now(), PublishedByUserID: adminID})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := identity.Client.ManagedRelease.Create().SetID(releaseID).SetManagedGeneration(1).SetStatus("ACTIVE").SetReleaseContentJSON(raw).SetSnapshotJSON(encoded).SetSnapshotHash(snapshot.SnapshotHash).SetSourceDraftRevision(1).SetCreatedByUserID(adminID).SetCreatedAt(identity.Now()).Save(ctx); err != nil {
				t.Fatal(err)
			}
			if err := identity.Client.ManagedState.UpdateOneID("current").SetActiveManagedGeneration(1).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			var downloaded clientapi.ManagedSnapshot
			response = send("GET", "/api/client/v1/managed/snapshots/1", auth, nil, 200, &downloaded)
			if response.Header.Get("ETag") != "\""+downloaded.SnapshotHash+"\"" {
				t.Fatal("snapshot ETag mismatch")
			}
			send("PUT", "/api/client/v1/managed/applied", auth, map[string]any{"managedGeneration": 1, "snapshotHash": downloaded.SnapshotHash}, 204, nil)

			var portal clientapi.PortalGrant
			send("POST", "/api/client/v1/portal/grants", auth, nil, 201, &portal)
			if portal.ExchangeUrl != server.URL+"/portal/session/exchange" {
				t.Fatal("Portal origin mismatch")
			}
			formHeaders := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": server.URL}
			response = send("POST", "/portal/session/exchange", formHeaders, url.Values{"ticket": {portal.Ticket}}, 303, nil)
			if response.Header.Get("Location") != "/portal/" {
				t.Fatal("unexpected redirect")
			}
			if cookies := response.Cookies(); len(cookies) != 1 || cookies[0].Secure != secure || !cookies[0].HttpOnly {
				t.Fatal("incorrect Portal cookie flags")
			}
			var portalSession struct {
				CSRFToken string `json:"csrfToken"`
			}
			send("GET", "/api/portal/v1/session", nil, nil, 200, &portalSession)
			portalHeaders := map[string]string{"Origin": server.URL, "X-CSRF-Token": portalSession.CSRFToken}
			send("DELETE", "/api/portal/v1/session", map[string]string{"Origin": "https://wrong.example", "X-CSRF-Token": portalSession.CSRFToken}, nil, 403, nil)
			send("DELETE", "/api/portal/v1/session", portalHeaders, nil, 204, nil)
			send("GET", "/api/portal/v1/session", nil, nil, 401, nil)
			if !strings.HasPrefix(server.URL, name+"://") {
				t.Fatal("wrong transport")
			}
		})
	}
}
