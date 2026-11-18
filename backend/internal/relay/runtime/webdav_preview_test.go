package runtime

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/internal/relay/control"
)

func TestDAVPreviewLimitsUpstreamBodiesAndPreservesOrdinaryDAV(t *testing.T) {
	const metadataLimit = 16 << 10
	const webpLimit = 1 << 20
	var space string
	h, token, root, _, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Agent-Space-Id", space)
		switch r.URL.Query().Get("case") {
		case "metadata":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "16385")
		case "declared":
			w.Header().Set("Content-Type", "image/webp")
			w.Header().Set("Content-Length", "1048577")
		case "wrong-type":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>"))
			return
		default:
			w.Header().Set("Content-Type", "image/webp")
			w.(http.Flusher).Flush()
			size := webpLimit
			if r.URL.Query().Get("case") == "chunked" || !strings.HasSuffix(r.URL.Path, "/previews") {
				size++
			}
			w.Write(bytes.Repeat([]byte{'x'}, size))
			return
		}
	}), "READ_ONLY")
	space = (*state.DavBindings)[0].Target.AgentSpaceId
	server := httptest.NewServer(h)
	defer server.Close()
	preview := server.URL + strings.TrimSuffix(root, "dav/") + "previews?case="
	for _, tc := range []struct {
		name, method, url string
		status, bytes     int
		truncated         bool
	}{
		{"metadata", "OPTIONS", preview + "metadata", 502, -1, false},
		{"declared", "GET", preview + "declared", 502, -1, false},
		{"wrong type", "GET", preview + "wrong-type", 502, -1, false},
		{"exact limit", "GET", preview + "exact", 200, webpLimit, false},
		{"chunked overflow", "GET", preview + "chunked", 200, webpLimit, true},
		{"ordinary download", "GET", server.URL + root + "large.webp", 200, webpLimit + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+token)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			read, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, webpLimit+2))
			if response.StatusCode != tc.status || (tc.bytes >= 0 && read != int64(tc.bytes)) || (readErr != nil) != tc.truncated {
				t.Fatalf("status=%d bytes=%d readErr=%v", response.StatusCode, read, readErr)
			}
			if tc.name == "metadata" && read > metadataLimit {
				t.Fatalf("metadata reached client: %d bytes", read)
			}
		})
	}
}

func TestDAVPreviewPreservesUpstreamProtocol(t *testing.T) {
	var space string
	const query = "path=clip%2F%E5%9B%BE.mp4&size=256&expectedEtag=%22source%22"
	h, token, root, _, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/u/alice/previews" || r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Errorf("wrong upstream request: %s", r.URL.Path)
		}
		for _, name := range []string{"Origin", "Sec-Fetch-Site", "Destination", "If", "Cookie"} {
			if r.Header.Get(name) != "" {
				t.Errorf("unexpected upstream header %s", name)
			}
		}
		w.Header().Set("Agent-Space-Id", space)
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == "OPTIONS" {
			if r.URL.RawQuery != "" {
				t.Errorf("OPTIONS forwarded a query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Allow", "OPTIONS, GET, HEAD")
			w.Write([]byte(`{"schemaVersion":1}`))
			return
		}
		if r.URL.RawQuery != query {
			t.Errorf("query changed: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("ETag", `"derived"`)
		w.Header().Set("Preview-Source-ETag", `"source"`)
		switch r.Header.Get("If-None-Match") {
		case `"derived"`:
			w.WriteHeader(http.StatusNotModified)
		case `"busy"`:
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		case `"wrong-space"`:
			w.Header().Set("Agent-Space-Id", "other-space")
			w.Write([]byte("must not be delivered"))
		case `"upstream-auth"`:
			w.Header().Set("WWW-Authenticate", `Bearer realm="upstream"`)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			if r.Header.Get("If-Match") == `"stale"` {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			w.Header().Set("Content-Length", "4")
			w.Write([]byte("webp"))
		}
	}), "READ_ONLY")
	space = (*state.DavBindings)[0].Target.AgentSpaceId
	endpoint := strings.TrimSuffix(root, "dav/") + "previews"
	for _, test := range []struct {
		name, method, condition, body string
		status                        int
	}{
		{"capability", "OPTIONS", "", `{"schemaVersion":1}`, 200},
		{"body", "GET", "", "webp", 200},
		{"head", "HEAD", "", "", 200},
		{"not modified", "GET", `"derived"`, "", 304},
		{"precondition", "GET", "", "", 412},
		{"busy", "GET", `"busy"`, "", 429},
		{"wrong space", "GET", `"wrong-space"`, "", 502},
		{"upstream auth", "GET", `"upstream-auth"`, "", 502},
		{"head upstream auth", "HEAD", `"upstream-auth"`, "", 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := endpoint
			if test.method != "OPTIONS" {
				path += "?" + query
			}
			r := httptest.NewRequest(test.method, path, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Cookie", "ignore=1")
			r.Header.Set("Origin", "https://example.com")
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("Sec-Fetch-Site", "same-origin")
			r.Header.Set("Destination", "http://other.invalid/")
			r.Header.Set("If", "<http://other.invalid/> (<opaque-lock>)")
			if test.condition != "" {
				r.Header.Set("If-None-Match", test.condition)
			}
			if test.name == "precondition" {
				r.Header.Set("If-Match", `"stale"`)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status || (test.status < 500 || test.method == "HEAD") && w.Body.String() != test.body || strings.Contains(w.Body.String(), "must not be delivered") {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("WWW-Authenticate") != "" {
				t.Fatalf("unexpected response headers: %v", w.Header())
			}
			if test.status == 429 && w.Header().Get("Retry-After") != "1" {
				t.Fatal("lost upstream retry interval")
			}
			if test.method == "HEAD" && test.status == 200 && (w.Header().Get("Content-Length") != "4" || w.Header().Get("ETag") != `"derived"`) {
				t.Fatal("lost HEAD metadata")
			}
			if test.name == "upstream auth" && !strings.Contains(w.Body.String(), "workspace_credential_unavailable") {
				t.Fatal("upstream 401 lost credential classification")
			}
		})
	}
}

type previewDeadlineRecorder struct {
	*httptest.ResponseRecorder
	readInterrupted atomic.Bool
}

func (r *previewDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		r.readInterrupted.Store(true)
	}
	return nil
}

func TestDAVBodylessRevocationLeavesConnectionReadSideAvailable(t *testing.T) {
	for _, endpoint := range []string{"previews", "dav/file"} {
		t.Run(endpoint, func(t *testing.T) { testDAVBodylessRevocation(t, endpoint) })
	}
}

func testDAVBodylessRevocation(t *testing.T, endpoint string) {
	t.Helper()
	started := make(chan struct{})
	h, token, root, store, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}), "FULL")
	r := httptest.NewRequest("GET", strings.TrimSuffix(root, "dav/")+endpoint, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := &previewDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	finished := make(chan struct{})
	go func() {
		h.ServeHTTP(w, r)
		close(finished)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preview did not reach upstream")
	}
	state.ControlRevision++
	(*state.DavBindings)[0].Target.BindingRevision++
	state.BundleHash, _ = control.HashDescriptor(state)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("revocation did not cancel the preview")
	}
	if w.readInterrupted.Load() {
		t.Fatal("canceling a bodyless DAV request interrupted the HTTP connection's read side")
	}
}

func TestDAVPreviewAcceptsBodylessHTTP2(t *testing.T) {
	var space string
	h, token, root, _, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Agent-Space-Id", space)
		if r.Method == "OPTIONS" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{}"))
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Write([]byte("webp"))
	}), "READ_ONLY")
	space = (*state.DavBindings)[0].Target.AgentSpaceId
	server := httptest.NewUnstartedServer(h)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	endpoint := server.URL + strings.TrimSuffix(root, "dav/") + "previews"
	for _, test := range []struct {
		method, body string
		status       int
	}{{"OPTIONS", "", 200}, {"GET", "", 200}, {"HEAD", "", 200}, {"GET", "payload", 400}} {
		r, err := http.NewRequest(test.method, endpoint, strings.NewReader(test.body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.ProtoMajor != 2 || response.StatusCode != test.status {
			t.Fatalf("%s: got HTTP/%d %d, want HTTP/2 %d", test.method, response.ProtoMajor, response.StatusCode, test.status)
		}
	}
}

type davTestTransport func(*http.Request) (*http.Response, error)

func (f davTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDAVPreviewWithholdsHeadersAfterLeaseRevocation(t *testing.T) {
	for _, test := range []struct {
		method string
		status int
	}{{"HEAD", 200}, {"GET", 304}} {
		t.Run(test.method, func(t *testing.T) {
			h, token, root, store, state := davFixture(t, nil, "READ_ONLY")
			policy := state.Routes[0].TimeoutPolicy
			fake := davTestTransport(func(r *http.Request) (*http.Response, error) {
				state.ControlRevision++
				(*state.DavBindings)[0].Target.BindingRevision++
				state.BundleHash, _ = control.HashDescriptor(state)
				if _, err := store.Apply(state); err != nil {
					t.Fatal(err)
				}
				header := make(http.Header)
				header.Set("Agent-Space-Id", (*state.DavBindings)[0].Target.AgentSpaceId)
				header.Set("ETag", `"old"`)
				return &http.Response{StatusCode: test.status, Header: header, Body: http.NoBody}, nil
			})
			h.(*Handler).transports.Store(transportKey{connectMs: policy.ConnectMs, responseHeaderMs: policy.ResponseHeaderMs}, fake)
			r := httptest.NewRequest(test.method, strings.TrimSuffix(root, "dav/")+"previews", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 502 || w.Header().Get("ETag") != "" {
				t.Fatalf("revoked response headers escaped: %d %v", w.Code, w.Header())
			}
		})
	}
}

func TestDAVWriteCannotRearmAfterRevocation(t *testing.T) {
	recorder := httptest.NewRecorder()
	lease, cancel := context.WithCancel(context.Background())
	defer cancel()
	activity := newDAVActivity(time.Hour, lease, cancel, recorder, false)
	defer activity.close()
	activity.interrupt()
	writer := &davActivityWriter{ResponseWriter: recorder, activity: activity}
	if n, err := writer.Write([]byte("stale")); n != 0 || err == nil || recorder.Body.Len() != 0 {
		t.Fatalf("write after revocation sent %d bytes: %v", n, err)
	}
}

func TestDAVActivityTouchReusesTimer(t *testing.T) {
	lease, cancel := context.WithCancel(context.Background())
	defer cancel()
	activity := newDAVActivity(time.Hour, lease, cancel, httptest.NewRecorder(), false)
	defer activity.close()
	if allocations := testing.AllocsPerRun(100, activity.touch); allocations != 0 {
		t.Fatalf("progress tracking allocated %.2f times per chunk", allocations)
	}
}

func TestDAVPreviewDeliveryDeadlineSurvivesProgress(t *testing.T) {
	lease, cancel := context.WithCancel(context.Background())
	defer cancel()
	activity := newDAVActivity(time.Hour, lease, cancel, httptest.NewRecorder(), false)
	defer activity.close()
	activity.startDelivery(30 * time.Millisecond)
	pulse := time.NewTicker(time.Millisecond)
	defer pulse.Stop()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-lease.Done():
			return
		case <-pulse.C:
			activity.touch()
		case <-timeout.C:
			t.Fatal("progress extended the absolute preview delivery deadline")
		}
	}
}

func TestDAVPreviewRejectsRequestsOutsideItsBindingBeforeForwarding(t *testing.T) {
	for _, mode := range []string{"READ_ONLY", "FULL"} {
		t.Run(mode, func(t *testing.T) {
			var forwarded atomic.Int32
			h, token, root, _, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded.Add(1)
				w.WriteHeader(404)
			}), mode)
			base := strings.TrimSuffix(root, "dav/")
			space := (*state.DavBindings)[0].Target.AgentSpaceId
			for _, test := range []struct {
				method, path, body, origin, fetchSite string
				status                                int
			}{
				{"PUT", base + "previews", "data", "", "", 405},
				{"PROPFIND", base + "previews", "", "", "", 405},
				{"GET", base + "previews", "data", "", "", 400},
				{"OPTIONS", base + "previews", "data", "", "", 400},
				{"GET", base + "previews", "", "http://foreign.invalid", "", 403},
				{"GET", base + "previews", "", "", "cross-site", 403},
				{"GET", base + "previews/extra", "", "", "", 403},
				{"GET", base + "previews/", "", "", "", 403},
				{"GET", base + "%70reviews", "", "", "", 403},
				{"GET", strings.Replace(base, space, "other-space", 1) + "previews", "", "", "", 403},
			} {
				r := httptest.NewRequest(test.method, test.path, nil)
				if test.body != "" {
					r = httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
				}
				r.Header.Set("Authorization", "Bearer "+token)
				if test.origin != "" {
					r.Header.Set("Origin", test.origin)
				}
				if test.fetchSite != "" {
					r.Header.Set("Sec-Fetch-Site", test.fetchSite)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != test.status || test.status == 405 && w.Header().Get("Allow") != "OPTIONS, GET, HEAD" {
					t.Errorf("%s %s: got %d, want %d", test.method, test.path, w.Code, test.status)
				}
			}
			if forwarded.Load() != 0 {
				t.Fatal("invalid requests reached upstream")
			}
			r := httptest.NewRequest("OPTIONS", base+"previews", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 404 || forwarded.Load() != 1 {
				t.Fatal("legacy OPTIONS result did not come from upstream", w.Code)
			}
		})
	}
}
