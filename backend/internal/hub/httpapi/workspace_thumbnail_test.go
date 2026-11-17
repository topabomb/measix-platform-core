package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

type thumbnailHTTPCase struct {
	handler              http.Handler
	service              *workspace.Service
	actor, space, cookie string
	csrf                 string
	calls                *atomic.Int64
}

func thumbnailHTTPFixture(t *testing.T, upstreamHandler func(http.ResponseWriter, *http.Request, string)) thumbnailHTTPCase {
	t.Helper()
	_, identity, _, ctx, actor := setupFullHandler(t)
	box, err := security.NewSecretBox(make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	secrets := upstream.NewService(identity.Client, box)
	service := workspace.NewService(identity.Client, secrets)
	key, err := secrets.CreateSecret(ctx, actor, "Preview fixture", "preview-fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	space := platformid.New(platformid.AgentSpace)
	calls := &atomic.Int64{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/u/preview_user/previews" || r.Header.Get("Authorization") != "Bearer preview-fixture-key" {
			t.Error("thumbnail escaped its stored DAV target or key")
			w.WriteHeader(403)
			return
		}
		upstreamHandler(w, r, space)
	}))
	t.Cleanup(remote.Close)
	origin := remote.URL
	configured, err := service.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "Preview fixture", Config: adminapi.AgentSpaceConfig{AdminOrigin: origin, McpOrigin: origin, DavOrigin: &origin, ManagementSecret: adminapi.SecretRef{SecretId: key.SecretID, SecretVersion: key.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	identity.Client.WorkspaceService.UpdateOneID(configured.WorkspaceServiceId).SetState("ACTIVE").SetEnabled(true).SetActiveConfigRevision(1).ExecX(ctx)
	now := time.Now().UTC()
	identity.Client.AgentSpace.Create().SetID(actor).SetWorkspaceServiceID(configured.WorkspaceServiceId).SetRemoteUsername("preview_user").SetAgentSpaceID(space).SetBindingRevision(1).SetIntent("CONNECTED").SetState("CONNECTED").SetDavSecretID(key.SecretID).SetDavSecretVersion(int64(key.SecretVersion)).SetDavConfirmed(true).SetRemoteActive(true).SetStopPending(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	handler := httpapi.NewFull(httpapi.Services{Identity: identity, Workspace: service})
	cookie, csrf := loginAdmin(t, handler)
	return thumbnailHTTPCase{handler: handler, service: service, actor: actor, space: space, cookie: cookie, csrf: csrf, calls: calls}
}

func (fixture thumbnailHTTPCase) endpoint(query string) string {
	return "/api/admin/v1/users/" + fixture.actor + "/workspace/thumbnail?agentSpaceId=" + fixture.space + query
}

func thumbnailBody(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../../api/fixtures/workspace/thumbnail.webp")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestThumbnailGeneratedHTTPRoutesPreserveVersionsAndAuditDeliveredBytes(t *testing.T) {
	data := thumbnailBody(t)
	derived := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	fixture := thumbnailHTTPFixture(t, func(w http.ResponseWriter, r *http.Request, space string) {
		w.Header().Set("Agent-Space-Id", space)
		if r.Method == "OPTIONS" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"schemaVersion":1,"supported":true,"sizes":[128,256,512],"contentType":"image/webp","mediaKinds":["IMAGE","VIDEO"]}`))
			return
		}
		if r.URL.Query().Get("expectedEtag") != `"source"` || r.URL.Query().Get("path") != "picture.png" || r.Header.Get("If-Match") != derived {
			t.Error("source query and derived HTTP condition were conflated")
		}
		w.Header().Set("Preview-Source-ETag", `"source"`)
		w.Header().Set("ETag", derived)
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Set-Cookie", "must-not-forward=1")
		w.Header().Set("Location", "https://must-not-forward.invalid")
		_, _ = w.Write(data)
	})
	capability := doJSON(t, fixture.handler, "OPTIONS", fixture.endpoint(""), map[string]string{"Cookie": fixture.cookie}, nil)
	if capability.Code != 200 || capability.Header().Get("Agent-Space-Id") != fixture.space {
		t.Fatalf("capability: %d %s", capability.Code, capability.Body.String())
	}
	query := "&path=picture.png&size=128&expectedEtag=" + url.QueryEscape(`"source"`)
	for _, method := range []string{"GET", "HEAD"} {
		response := doJSON(t, fixture.handler, method, fixture.endpoint(query), map[string]string{"Cookie": fixture.cookie, "If-Match": derived, "Range": "bytes=0-0"}, nil)
		if response.Code != 200 || response.Header().Get("Content-Type") != "image/webp" || response.Header().Get("ETag") != derived || response.Header().Get("Preview-Source-ETag") != `"source"` || response.Header().Get("Content-Length") != strconv.Itoa(len(data)) || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s thumbnail headers: %d %v %s", method, response.Code, response.Header(), response.Body.String())
		}
		if response.Header().Get("Set-Cookie") != "" || response.Header().Get("Location") != "" || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "inline;") {
			t.Fatal("unsafe upstream headers escaped thumbnail adapter")
		}
		if method == "GET" && response.Body.String() != string(data) || method == "HEAD" && response.Body.Len() != 0 {
			t.Fatal("thumbnail GET/HEAD body mismatch")
		}
	}
	audits, err := fixture.service.Client.WorkspaceAudit.Query().All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	previewBytes, optionBytes := []int64{}, int64(0)
	for _, audit := range audits {
		if audit.Action == "THUMBNAIL" {
			previewBytes = append(previewBytes, audit.Bytes)
		} else if audit.Action == "THUMBNAIL_OPTIONS" {
			optionBytes = audit.Bytes
		}
	}
	if len(previewBytes) != 2 || previewBytes[0]+previewBytes[1] != int64(len(data)) || optionBytes != int64(capability.Body.Len()) {
		t.Fatalf("audit must count delivered bytes, HEAD zero: %v options=%d", previewBytes, optionBytes)
	}
}

func TestThumbnailRejectsBadQueriesBodiesAndCrossAuthWithoutRemoteRequests(t *testing.T) {
	fixture := thumbnailHTTPFixture(t, func(http.ResponseWriter, *http.Request, string) { t.Error("invalid request reached remote") })
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		for _, query := range []string{"&unknown=1", "&agentSpaceId=" + fixture.space, "&path=picture.png&expectedEtag=W%2F%22source%22", "&path=picture.png&size=129&expectedEtag=%22source%22"} {
			response := doJSON(t, fixture.handler, method, fixture.endpoint(query), map[string]string{"Cookie": fixture.cookie}, nil)
			if response.Code != 400 || method == "HEAD" && response.Body.Len() != 0 {
				t.Fatalf("%s invalid query: %d %s", method, response.Code, response.Body.String())
			}
		}
	}
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		response := doJSON(t, fixture.handler, method, fixture.endpoint(""), map[string]string{"Cookie": fixture.cookie}, map[string]string{"unexpected": "body"})
		if response.Code != 400 || method == "HEAD" && response.Body.Len() != 0 {
			t.Fatal("request body or HEAD error body accepted")
		}
	}
	for _, endpoint := range []string{fixture.endpoint(""), "/api/client/v1/workspace/thumbnail"} {
		response := doJSON(t, fixture.handler, "HEAD", endpoint, nil, nil)
		if response.Code != 401 || response.Body.Len() != 0 {
			t.Fatalf("authentication must precede invalid binding and stay HEAD bodyless: %d %s", response.Code, response.Body.String())
		}
	}
	response := doJSON(t, fixture.handler, "OPTIONS", "/api/client/v1/workspace/thumbnail?agentSpaceId="+fixture.space, map[string]string{"Cookie": fixture.cookie}, nil)
	if response.Code != 401 || fixture.calls.Load() != 0 {
		t.Fatal("Client thumbnail accepted Admin/Portal-style cookie authentication")
	}
}

func TestThumbnailForbiddenDoesNotWithdrawFiles(t *testing.T) {
	fixture := thumbnailHTTPFixture(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(403)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 403, "code": "file_forbidden"})
	})
	response := doJSON(t, fixture.handler, "GET", fixture.endpoint("&path=picture.png&expectedEtag=%22source%22"), map[string]string{"Cookie": fixture.cookie}, nil)
	if response.Code != 403 {
		t.Fatalf("preview permission: %d %s", response.Code, response.Body.String())
	}
	view, err := fixture.service.Projection(context.Background(), fixture.actor)
	if err != nil || !view.FilesAvailable {
		t.Fatal("preview403 invalidated ordinary file access")
	}
}

func TestThumbnailBufferedResponseCannotSurviveBindingChange(t *testing.T) {
	data := thumbnailBody(t)
	var fixture thumbnailHTTPCase
	fixture = thumbnailHTTPFixture(t, func(w http.ResponseWriter, _ *http.Request, space string) {
		fixture.service.Client.AgentSpace.UpdateOneID(fixture.actor).AddBindingRevision(1).ExecX(context.Background())
		w.Header().Set("Agent-Space-Id", space)
		w.Header().Set("Preview-Source-ETag", `"source"`)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
	response := doJSON(t, fixture.handler, "GET", fixture.endpoint("&path=picture.png&expectedEtag=%22source%22"), map[string]string{"Cookie": fixture.cookie}, nil)
	if response.Code != 503 || response.Body.String() == string(data) || response.Header().Get("Content-Type") == "image/webp" {
		t.Fatalf("late buffered image escaped a changed binding: %d %s", response.Code, response.Body.String())
	}
}

func TestThumbnailRemote401WithdrawsOnlyCurrentFilesCredential(t *testing.T) {
	fixture := thumbnailHTTPFixture(t, func(w http.ResponseWriter, _ *http.Request, _ string) { w.WriteHeader(401) })
	response := doJSON(t, fixture.handler, "GET", fixture.endpoint("&path=picture.png&expectedEtag=%22source%22"), map[string]string{"Cookie": fixture.cookie}, nil)
	if response.Code != 503 || !strings.Contains(response.Body.String(), `"code":"dav_credential_unavailable"`) {
		t.Fatalf("remote401 became platform authentication failure: %d %s", response.Code, response.Body.String())
	}
	view, err := fixture.service.Projection(context.Background(), fixture.actor)
	if err != nil || view.FilesAvailable {
		t.Fatal("exact failed DAV credential remained available")
	}
}

func TestThumbnailNotModifiedHasNoBodyAndNoAuditBytes(t *testing.T) {
	data := thumbnailBody(t)
	derived := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	fixture := thumbnailHTTPFixture(t, func(w http.ResponseWriter, _ *http.Request, space string) {
		w.Header().Set("Agent-Space-Id", space)
		w.Header().Set("Preview-Source-ETag", `"source"`)
		w.Header().Set("ETag", derived)
		w.WriteHeader(304) // Real Go HTTP intentionally omits Content-Type here.
	})
	for _, method := range []string{"GET", "HEAD"} {
		response := doJSON(t, fixture.handler, method, fixture.endpoint("&path=picture.png&expectedEtag=%22source%22"), map[string]string{"Cookie": fixture.cookie, "If-None-Match": "W/" + derived}, nil)
		if response.Code != 304 || response.Body.Len() != 0 || response.Header().Get("ETag") != derived {
			t.Fatalf("%s304: %d %s", method, response.Code, response.Body.String())
		}
	}
	for _, audit := range fixture.service.Client.WorkspaceAudit.Query().AllX(context.Background()) {
		if audit.Action == "THUMBNAIL" && (audit.Bytes != 0 || audit.Outcome != "SUCCEEDED") {
			t.Fatalf("304 audit: %+v", audit)
		}
	}
}

func TestClientThumbnailUsesBearerOwnerAndDoesNotSelectUserFromQuery(t *testing.T) {
	data := thumbnailBody(t)
	fixture := thumbnailHTTPFixture(t, func(w http.ResponseWriter, _ *http.Request, space string) {
		w.Header().Set("Agent-Space-Id", space)
		w.Header().Set("Preview-Source-ETag", `"source"`)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
	enrollment := doJSON(t, fixture.handler, "POST", "/api/admin/v1/users/"+fixture.actor+"/enrollments", map[string]string{"Cookie": fixture.cookie, "X-CSRF-Token": fixture.csrf}, map[string]any{"expiresInSeconds": 3600})
	if enrollment.Code != 201 {
		t.Fatalf("enrollment status=%d", enrollment.Code)
	}
	var material struct{ Code string }
	if err := json.Unmarshal(enrollment.Body.Bytes(), &material); err != nil {
		t.Fatal(err)
	}
	exchanged := doJSON(t, fixture.handler, "POST", "/api/client/v1/enrollments/exchange", nil, map[string]any{"code": material.Code, "installationId": platformid.New(platformid.Installation), "deviceName": "Thumbnail fixture", "platform": "ANDROID", "appVersion": "test"})
	if exchanged.Code != 201 {
		t.Fatalf("exchange status=%d", exchanged.Code)
	}
	var credential struct{ AccessToken string }
	if err := json.Unmarshal(exchanged.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/client/v1/workspace/thumbnail?agentSpaceId=" + fixture.space + "&path=picture.png&expectedEtag=%22source%22"
	headers := map[string]string{"Authorization": "Bearer " + credential.AccessToken}
	response := doJSON(t, fixture.handler, "GET", endpoint, headers, nil)
	if response.Code != 200 || response.Body.String() != string(data) {
		t.Fatalf("authenticated Client thumbnail: %d %s", response.Code, response.Body.String())
	}
	before := fixture.calls.Load()
	for _, query := range []string{"&userId=" + fixture.actor, "&agentSpaceId=" + platformid.New(platformid.AgentSpace)} {
		response = doJSON(t, fixture.handler, "GET", endpoint+query, headers, nil)
		if response.Code != 400 || fixture.calls.Load() != before {
			t.Fatal("Client query selected a different owner or accepted duplicate target")
		}
	}
}

func TestThumbnailPoolBusyHasRetryAfterAndDoesNotCallRemote(t *testing.T) {
	fixture := thumbnailHTTPFixture(t, func(http.ResponseWriter, *http.Request, string) { t.Error("full preview pool called remote") })
	var leases []*workspace.FileAccess
	for range 4 {
		access, err := fixture.service.OpenFiles(context.Background(), fixture.actor, fixture.actor, fixture.space, "THUMBNAIL", "picture.png", func(context.Context) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		leases = append(leases, access)
	}
	defer func() {
		for _, access := range leases {
			fixture.service.CloseFiles(access, "CANCELLED", 0)
		}
	}()
	for _, method := range []string{"OPTIONS", "GET", "HEAD"} {
		query := ""
		if method != "OPTIONS" {
			query = "&path=picture.png&expectedEtag=%22source%22"
		}
		response := doJSON(t, fixture.handler, method, fixture.endpoint(query), map[string]string{"Cookie": fixture.cookie}, nil)
		if response.Code != 429 || response.Header().Get("Retry-After") != "1" || method == "HEAD" && response.Body.Len() != 0 {
			t.Fatalf("%s busy response: %d %v", method, response.Code, response.Header())
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("pool rejection started a remote decoder")
	}
}

func TestThumbnailRejectsExplicitEmptyDerivedConditions(t *testing.T) {
	data := thumbnailBody(t)
	fixture := thumbnailHTTPFixture(t, func(w http.ResponseWriter, _ *http.Request, space string) {
		w.Header().Set("Agent-Space-Id", space)
		w.Header().Set("Preview-Source-ETag", `"source"`)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
	for _, method := range []string{"GET", "HEAD"} {
		for _, header := range []string{"If-Match", "If-None-Match"} {
			for _, value := range []string{"", " \t "} {
				before := fixture.calls.Load()
				response := doJSON(t, fixture.handler, method, fixture.endpoint("&path=picture.png&expectedEtag=%22source%22"), map[string]string{"Cookie": fixture.cookie, header: value}, nil)
				if response.Code != 400 || fixture.calls.Load() != before || method == "HEAD" && response.Body.Len() != 0 {
					t.Fatalf("%s empty %s was ignored: status=%d", method, header, response.Code)
				}
			}
		}
	}
}
