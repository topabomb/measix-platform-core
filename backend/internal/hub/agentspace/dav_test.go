package agentspace

import (
	"context"
	"io"
	"measix/platform/internal/wire/adminapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
