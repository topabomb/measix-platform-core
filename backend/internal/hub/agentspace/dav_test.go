package agentspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"measix/platform/internal/wire/adminapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFilesListHidesOnlyRootAgentSpace(t *testing.T) {
	for _, tc := range []struct {
		name, path  string
		paths, want []string
	}{
		{"first", "", []string{".agent-space", "", ".agent-space-backup", "xu-tang"}, []string{".agent-space-backup", "xu-tang"}},
		{"middle", "", []string{"", ".agent-space-backup", ".agent-space", "xu-tang"}, []string{".agent-space-backup", "xu-tang"}},
		{"last", "", []string{"", ".agent-space-backup", "xu-tang", ".agent-space"}, []string{".agent-space-backup", "xu-tang"}},
		{"nested", "xu-tang", []string{"xu-tang", "xu-tang/.agent-space", "xu-tang/.agent-space-backup"}, []string{"xu-tang/.agent-space", "xu-tang/.agent-space-backup"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(207)
				io.WriteString(w, `<multistatus xmlns="DAV:">`)
				for _, path := range tc.paths {
					fmt.Fprintf(w, `<response><href>/u/alice/dav/%s</href><propstat><prop><resourcetype><collection/></resourcetype><quota-used-bytes>100</quota-used-bytes><quota-available-bytes>200</quota-available-bytes></prop><status>HTTP/1.1 200 OK</status></propstat></response>`, path)
				}
				io.WriteString(w, `</multistatus>`)
			}))
			defer server.Close()
			c, _ := New(server.URL, server.URL, server.URL, "admin")
			out, err := c.List(context.Background(), "alice", "dav", tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Entries) != len(tc.want) || out.UsedBytes == nil || *out.UsedBytes != 100 || out.AvailableBytes == nil || *out.AvailableBytes != 200 {
				t.Fatalf("unexpected list or root quota: %+v", out)
			}
			for i, want := range tc.want {
				if out.Entries[i].Path != want {
					t.Fatalf("entry %d = %q, want %q", i, out.Entries[i].Path, want)
				}
			}
		})
	}
}

func TestFilesProtectedPathRequestsNeverReachUpstream(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(201) }))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	check := func(err error) {
		t.Helper()
		var remote *Error
		if !errors.As(err, &remote) || remote.Code != "invalid_file_path" || calls != 0 {
			t.Fatalf("protected request err=%v upstreamCalls=%d", err, calls)
		}
	}
	for _, p := range []string{".agent-space", ".agent-space/private"} {
		_, err := c.List(context.Background(), "alice", "dav", p)
		check(err)
		_, err = c.Stat(context.Background(), "alice", "dav", p)
		check(err)
		for _, method := range []string{"GET", "HEAD", "PUT"} {
			_, err = c.Content(context.Background(), "alice", "dav", p, method, strings.NewReader("body"), http.Header{"If-None-Match": {"*"}})
			check(err)
		}
		_, err = c.Mutate(context.Background(), "alice", "dav", adminapi.WorkspaceFileMutation{Action: "MKCOL", Path: p})
		check(err)
		for _, method := range []adminapi.WorkspaceFileMutationAction{"COPY", "MOVE"} {
			_, err = c.Mutate(context.Background(), "alice", "dav", adminapi.WorkspaceFileMutation{Action: method, Path: "ordinary", Destination: &p})
			check(err)
		}
	}
	for _, p := range []string{".agent-space-backup", "ordinary/.agent-space"} {
		out, err := c.Mutate(context.Background(), "alice", "dav", adminapi.WorkspaceFileMutation{Action: "MKCOL", Path: p})
		if err != nil || out.Outcome != "SUCCEEDED" {
			t.Fatalf("ordinary path %q rejected: %v", p, err)
		}
	}
	if calls != 2 {
		t.Fatalf("ordinary paths did not reach upstream: %d", calls)
	}
}

func TestFilesDoesNotHideInvalidProtectedHref(t *testing.T) {
	for _, href := range []string{"https://evil.invalid/u/alice/dav/.agent-space", "/u/bob/dav/.agent-space", "/u/alice/dav/%2e%2e/.agent-space", "/u/alice/dav/.agent-space/private"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(207)
			fmt.Fprintf(w, `<multistatus xmlns="DAV:"><response><href>%s</href></response></multistatus>`, href)
		}))
		c, _ := New(server.URL, server.URL, server.URL, "admin")
		_, err := c.List(context.Background(), "alice", "dav", "")
		var remote *Error
		if !errors.As(err, &remote) || remote.Code != "invalid_dav_href" {
			t.Errorf("href %q must fail as upstream data: %v", href, err)
		}
		server.Close()
	}
}

func TestDAVRejectsNonFinalOrWrongMethodSuccess(t *testing.T) {
	for _, method := range []string{"PUT", "MKCOL", "DELETE", "COPY", "MOVE"} {
		for _, status := range []int{202, 206, 304} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			c, _ := New(server.URL, server.URL, server.URL, "admin")
			response, err := c.davRequest(context.Background(), "alice", "dav", "a", method, nil, http.Header{})
			if response != nil {
				response.Body.Close()
			}
			var remote *Error
			if !errors.As(err, &remote) || !remote.Unknown || remote.Code != "invalid_dav_response" {
				t.Errorf("%s %d must not claim write completed: %v", method, status, err)
			}
			server.Close()
		}
	}
}

func TestDAVDeleteAcceptsCompletedRepresentation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	response, err := c.davRequest(context.Background(), "alice", "dav", "a", "DELETE", nil, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestFilesPreserveIfRangeForNativeMedia(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Range") != `"video-v1"` || r.Header.Get("Range") != "bytes=4294967296-" {
			t.Error("lost range condition")
		}
		w.Header().Set("Content-Range", "bytes 4294967296-4294967296/4294967297")
		w.WriteHeader(206)
		_, _ = w.Write([]byte{0})
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	response, err := c.Content(context.Background(), "alice", "dav", "video.mp4", "GET", nil, http.Header{"Range": {"bytes=4294967296-"}, "If-Range": {`"video-v1"`}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 206 {
		t.Fatal(response.StatusCode)
	}
}

func TestDAVRangeFailurePreservesValidatedLength(t *testing.T) {
	for raw, want := range map[string]string{"bytes */4294967296": "bytes */4294967296", "bytes */0": "bytes */0", "": "", "bytes */-1": "", "bytes */9223372036854775808": "", "bytes 1-2/3": "", "bytes */+1": ""} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Range", raw)
			w.WriteHeader(416)
		}))
		c, _ := New(server.URL, server.URL, server.URL, "admin")
		_, err := c.Content(context.Background(), "alice", "dav", "a", "GET", nil, http.Header{"Range": {"bytes=4294967296-"}})
		var remote *Error
		if !errors.As(err, &remote) || remote.Code != "file_range_invalid" || remote.ContentRange != want {
			t.Errorf("%q: got %+v, want range %q", raw, remote, want)
		}
		server.Close()
	}
}

func TestDAVMetadataBodyIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(207)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	c.DAVIdleTimeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.List(ctx, "alice", "dav", "")
	if err == nil || ctx.Err() != nil {
		t.Fatalf("stalled metadata must release its transfer slot before the request deadline: %v, %v", err, ctx.Err())
	}
}

func TestDAVMetadataETagMatchesHTTPConditionalSyntax(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(207)
		io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>/u/alice/dav/a.txt</href><propstat><prop><resourcetype/><getetag>2005-126-6abb0bd9-377a4ff</getetag></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`)
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	out, err := c.Stat(context.Background(), "alice", "dav", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if out.Etag == nil || *out.Etag != `"2005-126-6abb0bd9-377a4ff"` {
		t.Fatalf("PROPFIND must expose a usable HTTP ETag: %+v", out.Etag)
	}
}

func TestDAVRejectsForeignHrefAndFailedPropstat(t *testing.T) {
	for _, href := range []string{"https://evil.invalid/u/alice/dav/a", "/u/bob/dav/a", "/u/alice/dav/%2e%2e/secret"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(207)
			io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>`+href+`</href><propstat><prop><resourcetype/><getcontentlength>1</getcontentlength></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`)
		}))
		c, _ := New(server.URL, server.URL, server.URL, "admin")
		if _, err := c.List(context.Background(), "alice", "dav-token", ""); err == nil {
			t.Errorf("accepted href %s", href)
		}
		server.Close()
	}
}
func TestDAVConditionalDestinationAndPartialResults(t *testing.T) {
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.WriteHeader(207)
			io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>`+r.URL.Path+`</href><propstat><prop><resourcetype/></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer dav-token" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
			t.Error("wrong DAV authority")
		}
		if r.Header.Get("If-Match") != `"source"` || r.Header.Get("If") != "<"+origin+`/u/alice/dav/new.txt> (["target"])` || r.Header.Get("Overwrite") != "T" {
			t.Errorf("conditions: %v", r.Header)
		}
		w.WriteHeader(207)
		io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>/u/alice/dav/a.txt</href><status>HTTP/1.1 507 Insufficient Storage</status></response></multistatus>`)
	}))
	defer server.Close()
	origin = server.URL
	c, _ := New(origin, origin, origin, "admin")
	source, target, dest := `"source"`, `"target"`, "new.txt"
	yes := true
	result, err := c.Mutate(context.Background(), "alice", "dav-token", adminapi.WorkspaceFileMutation{Action: "MOVE", Path: "a.txt", Destination: &dest, SourceEtag: &source, TargetEtag: &target, Overwrite: &yes})
	if err != nil || result.Outcome != "PARTIAL" || len(result.Failures) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestDAVUploadStreamsAndRequiresCondition(t *testing.T) {
	read := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		read = int(n)
		w.WriteHeader(201)
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	if _, err := c.Content(context.Background(), "alice", "token", "a", "PUT", strings.NewReader("test"), http.Header{}); err == nil {
		t.Fatal("unconditional upload accepted")
	}
	response, err := c.Content(context.Background(), "alice", "token", "a", "PUT", strings.NewReader("test"), http.Header{"If-None-Match": []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if read != 4 {
		t.Fatal(read)
	}
}

func TestDAVUploadRejectsAmbiguousConditionsBeforeForwarding(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes++; w.WriteHeader(201) }))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	for _, headers := range []http.Header{
		{"If-Match": {`"v1"`}, "If-None-Match": {`"other"`}},
		{"If-Match": {`"v1"`, `"v2"`}},
		{"If-None-Match": {"*", `"other"`}},
	} {
		response, err := c.Content(context.Background(), "alice", "token", "a", "PUT", strings.NewReader("test"), headers)
		if response != nil {
			response.Body.Close()
		}
		if err == nil {
			t.Error("accepted ambiguous conditions")
		}
	}
	if writes != 0 {
		t.Fatalf("forwarded %d ambiguous writes", writes)
	}
}

func TestDAVCopyRequiresObservedSourceVersion(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			writes++
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(207)
		io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>/u/alice/dav/a.txt</href><propstat><prop><resourcetype/></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`)
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, server.URL, "admin")
	dest := "copy.txt"
	_, err := c.Mutate(context.Background(), "alice", "token", adminapi.WorkspaceFileMutation{Action: "COPY", Path: "a.txt", Destination: &dest})
	if err == nil || writes != 0 {
		t.Fatalf("unconditional source copied writes=%d err=%v", writes, err)
	}
}

func TestFileConditionRequiresExactlyOneStrongETag(t *testing.T) {
	for _, value := range []string{`"one", "two"`, `"one" "two"`, `"bad quote"`, `W/"weak"`, `*`, "\"tab\tinside\""} {
		if validETag(value) {
			t.Errorf("accepted non-single condition %q", value)
		}
	}
	for _, value := range []string{`"one"`, `"one,two"`, `""`} {
		if !validETag(value) {
			t.Errorf("rejected opaque ETag %q", value)
		}
	}
}
