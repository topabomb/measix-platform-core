package httpapi

import (
	"context"
	"io"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestFilesNativeVideoUsesVersionedRangeAndSafeContentType(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStore(t)
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	secrets := upstream.NewService(st.Client, box)
	svc := workspace.NewService(st.Client, secrets)
	user, space := platformid.New(platformid.User), platformid.New(platformid.AgentSpace)
	now := time.Now().UTC()
	st.Client.User.Create().SetID(user).SetUsername("media").SetDisplayName("Media").SetRole("USER").SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	secret, err := secrets.CreateSecret(ctx, user, "media", "test-only-token")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/u/media/dav/video.mp4" && r.Header.Get("If-Match") != `"v1"` {
			t.Error("video URL did not pin its version")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Range", "bytes 4294967296-4294967296/4294967297")
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(206)
		_, _ = w.Write([]byte{0})
	}))
	defer remote.Close()
	service, err := svc.Save(ctx, user, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "media", Config: adminapi.AgentSpaceConfig{AdminOrigin: remote.URL, McpOrigin: remote.URL, DavOrigin: &remote.URL, ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	st.Client.WorkspaceService.UpdateOneID(service.WorkspaceServiceId).SetState("ACTIVE").SetEnabled(true).SetActiveConfigRevision(1).ExecX(ctx)
	st.Client.AgentSpace.Create().SetID(user).SetWorkspaceServiceID(service.WorkspaceServiceId).SetRemoteUsername("media").SetAgentSpaceID(space).SetBindingRevision(1).SetIntent("CONNECTED").SetState("CONNECTED").SetDavSecretID(secret.SecretID).SetDavSecretVersion(1).SetDavConfirmed(true).SetRemoteActive(true).SetStopPending(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	for _, tc := range []struct {
		file, query, content, disposition string
		status                            int
	}{
		{"video.mp4", "&preview=video&expectedEtag=%22v1%22", "video/mp4", "inline; filename=video.mp4", 206},
		{"unsafe.html", "", "application/octet-stream", "attachment; filename=unsafe.html", 206},
		{"video.mp4", "&preview=video", "", "", 400},
		{"unsafe.svg", "&preview=video&expectedEtag=%22v1%22", "", "", 400},
		{"video.mp4", "&preview=video&expectedEtag=W%2F%22v1%22", "", "", 400},
	} {
		r := httptest.NewRequest("GET", "/content?agentSpaceId="+url.QueryEscape(space)+tc.query, nil)
		r.Header.Set("Range", "bytes=4294967296-")
		w := httptest.NewRecorder()
		before := calls
		serveWorkspaceFiles(w, r, svc, user, user, "GET", tc.file, func(context.Context) error { return nil })
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.file, tc.query, w.Code, w.Body.String())
		}
		if tc.status == 400 {
			if calls != before {
				t.Fatal("invalid preview reached upstream")
			}
			continue
		}
		if w.Header().Get("Content-Type") != tc.content || w.Header().Get("Content-Disposition") != tc.disposition || w.Header().Get("Content-Range") != "bytes 4294967296-4294967296/4294967297" {
			t.Fatalf("incorrect media response: %v", w.Header())
		}
	}
}

func TestWorkspaceFileErrorsHaveActionableStatus(t *testing.T) {
	for code, want := range map[string]int{"invalid_file_path": 400, "invalid_file_destination": 400, "invalid_dav_href": 503, "invalid_dav_xml": 503, "workspace_space_mismatch": 409, "file_listing_limit": 422, "file_version_conflict": 412, "file_locked": 423, "file_range_invalid": 416, "file_conflict": 409, "dav_credential_unavailable": 503, "file_storage_full": 507, "file_transfer_limit": 429} {
		w := httptest.NewRecorder()
		writeWorkspaceError(w, &remoteapi.Error{Code: code})
		if w.Code != want {
			t.Errorf("%s: got %d, want %d", code, w.Code, want)
		}
	}
}

func TestWorkspaceRangeErrorCarriesRepresentationLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeWorkspaceError(w, &remoteapi.Error{Code: "file_range_invalid", Status: 416, ContentRange: "bytes */4294967296"})
	}))
	defer server.Close()
	for _, method := range []string{"GET", "HEAD"} {
		r, _ := http.NewRequest(method, server.URL, nil)
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 416 || response.Header.Get("Content-Range") != "bytes */4294967296" || method == "HEAD" && len(body) != 0 {
			t.Fatalf("%s lost range semantics: %d %v %q %v", method, response.StatusCode, response.Header, body, err)
		}
	}
}
