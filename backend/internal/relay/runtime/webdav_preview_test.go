package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/internal/relay/control"
)

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

func TestDAVPreviewRevocationLeavesConnectionReadSideAvailable(t *testing.T) {
	started := make(chan struct{})
	h, token, root, store, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}), "FULL")
	r := httptest.NewRequest("GET", strings.TrimSuffix(root, "dav/")+"previews", nil)
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
		t.Fatal("canceling a bodyless preview must not interrupt the HTTP connection's read side")
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
