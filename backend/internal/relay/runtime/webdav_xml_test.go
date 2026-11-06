package runtime

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/encoding/unicode"
)

func davXMLListing(entries int) string {
	var listing strings.Builder
	listing.WriteString(`<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:" xmlns:x="urn:test">`)
	for i := range entries {
		fmt.Fprintf(&listing, `<D:response><D:href>/u/alice/dav/file-%04d%%20%%25.txt</D:href><D:propstat><D:prop><D:displayname>目录 &amp; file</D:displayname><D:getcontentlength>123</D:getcontentlength><D:getetag>"v1"</D:getetag><x:opaque><D:href>/u/bob/dav/private</D:href></x:opaque></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`, i)
	}
	listing.WriteString("</D:multistatus>")
	return listing.String()
}

type davXMLWriteRecorder struct {
	*httptest.ResponseRecorder
	writes  atomic.Int64
	flushes atomic.Int64
}

func (w *davXMLWriteRecorder) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.writes.Add(1)
	}
	return w.ResponseRecorder.Write(p)
}

func (w *davXMLWriteRecorder) Flush() {
	w.flushes.Add(1)
	w.ResponseRecorder.Flush()
}

func TestDAVXMLCoalescesProxyWrites(t *testing.T) {
	for _, entries := range []int{10, 200, 2000} {
		t.Run(fmt.Sprint(entries), func(t *testing.T) {
			input := davXMLListing(entries)
			handler, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				w.WriteHeader(http.StatusMultiStatus)
				_, _ = io.WriteString(w, input)
			}), "FULL")
			request := httptest.NewRequest("PROPFIND", root, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Depth", "1")
			output := &davXMLWriteRecorder{ResponseRecorder: httptest.NewRecorder()}
			handler.ServeHTTP(output, request)
			want := strings.ReplaceAll(input, "/u/alice/dav/", root)
			if output.Code != http.StatusMultiStatus || output.Body.String() != want {
				t.Fatalf("listing changed: status=%d bytes=%d want=%d", output.Code, output.Body.Len(), len(want))
			}
			writes, flushes := output.writes.Load(), output.flushes.Load()
			// Allow a final short block; a larger copy buffer alone cannot
			// aggregate the converter's token-sized Pipe writes.
			maxWrites := int64((len(want)+(32<<10)-1)/(32<<10) + 1)
			t.Logf("entries=%d bytes=%d writes=%d flushes=%d", entries, len(want), writes, flushes)
			if writes > maxWrites || flushes > writes+2 {
				t.Fatalf("token-sized proxy output: writes=%d flushes=%d maxWrites=%d", writes, flushes, maxWrites)
			}
		})
	}
}

func TestDAVXMLBufferedHTTPVersions(t *testing.T) {
	for _, protocol := range []int{1, 2} {
		t.Run(fmt.Sprintf("http%d", protocol), func(t *testing.T) {
			input := davXMLListing(200)
			handler, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				w.Header().Set("Content-Length", fmt.Sprint(len(input)))
				if r.Method == "PROPFIND" {
					w.WriteHeader(http.StatusMultiStatus)
				}
				_, _ = io.WriteString(w, input)
			}), "FULL")
			server := httptest.NewUnstartedServer(handler)
			server.EnableHTTP2 = protocol == 2
			server.StartTLS()
			defer server.Close()
			for _, method := range []string{"PROPFIND", "GET", "HEAD"} {
				request, err := http.NewRequest(method, server.URL+root, nil)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Depth", "1")
				request.Header.Set("Accept-Encoding", "identity")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				want, status, length := input, http.StatusOK, int64(len(input))
				if method == "PROPFIND" {
					want = strings.ReplaceAll(input, "/u/alice/dav/", root)
					status, length = http.StatusMultiStatus, -1
				} else if method == "HEAD" {
					want = ""
				}
				if response.ProtoMajor != protocol || response.StatusCode != status ||
					response.ContentLength != length || string(body) != want {
					t.Fatalf("%s: protocol=%s status=%d length=%d bodyBytes=%d want=%d",
						method, response.Proto, response.StatusCode, response.ContentLength, len(body), len(want))
				}
			}
		})
	}
}

func TestDAVXMLBufferedRequestCancellation(t *testing.T) {
	finished := make(chan struct{})
	handler, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<D:multistatus xmlns:D="DAV:">`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}), "FULL")
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "PROPFIND", server.URL+root, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus {
		t.Fatal(response.StatusCode)
	}
	cancel()
	if _, err := io.ReadAll(response.Body); err == nil {
		t.Fatal("cancelled incomplete XML became a successful EOF")
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("client cancellation did not release the upstream HTTP request")
	}
}

type davXMLObservedBody struct {
	io.ReadCloser
	bytesRead atomic.Int64
	closed    chan struct{}
	eof       chan struct{}
}

func observeDAVXMLBody(body io.ReadCloser) *davXMLObservedBody {
	return &davXMLObservedBody{ReadCloser: body, closed: make(chan struct{}, 4), eof: make(chan struct{}, 1)}
}

func (b *davXMLObservedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytesRead.Add(int64(n))
	if err == io.EOF {
		select {
		case b.eof <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (b *davXMLObservedBody) Close() error {
	err := b.ReadCloser.Close()
	b.closed <- struct{}{}
	return err
}

func awaitDAVXMLClose(t *testing.T, body *davXMLObservedBody) {
	t.Helper()
	select {
	case <-body.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("XML conversion did not release its upstream body")
	}
}

func davXMLResponse(body io.ReadCloser) (*http.Response, davMapper, *url.URL) {
	base, _ := url.Parse("http://upstream/u/alice/dav/")
	mapper := davMapper{upstream: base, publicRoot: "/public/", publicHost: "example.com", publicScheme: "http"}
	response := &http.Response{
		Request: &http.Request{Method: "PROPFIND"},
		Header:  http.Header{"Content-Type": {"application/xml; charset=utf-8"}},
		Body:    body,
	}
	return response, mapper, base
}

func TestDAVXMLBufferedEncodingsAndFinalBlock(t *testing.T) {
	for _, encoding := range []string{"utf8", "gzip", "utf16le", "utf16be-gzip"} {
		t.Run(encoding, func(t *testing.T) {
			input := davXMLListing(10)
			raw := input
			charset := "utf-8"
			if strings.HasPrefix(encoding, "utf16") {
				order := unicode.LittleEndian
				if strings.HasPrefix(encoding, "utf16be") {
					order = unicode.BigEndian
				}
				var err error
				raw, err = unicode.UTF16(order, unicode.UseBOM).NewEncoder().String(strings.Replace(input, "UTF-8", "UTF-16", 1))
				if err != nil {
					t.Fatal(err)
				}
				charset = "utf-16"
			}
			if strings.Contains(encoding, "gzip") {
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				if _, err := io.WriteString(writer, raw); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				raw = compressed.String()
			}
			upstream := observeDAVXMLBody(io.NopCloser(strings.NewReader(raw)))
			response, mapper, base := davXMLResponse(upstream)
			response.ContentLength = int64(len(raw))
			response.Header.Set("Content-Length", fmt.Sprint(len(raw)))
			response.Header.Set("Content-Type", "application/xml; charset="+charset)
			for _, header := range []string{"Content-MD5", "Digest", "Content-Digest"} {
				response.Header.Set(header, "stale")
			}
			if strings.Contains(encoding, "gzip") {
				response.Header.Set("Content-Encoding", "gzip")
			}
			if err := rewriteDAVXML(response, mapper, base); err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			got, err := io.ReadAll(response.Body)
			want := strings.ReplaceAll(input, "/u/alice/dav/", "/public/")
			if err != nil || string(got) != want {
				t.Fatalf("converted body: err=%v bytes=%d want=%d", err, len(got), len(want))
			}
			if response.ContentLength != -1 {
				t.Fatalf("stale response length: %d", response.ContentLength)
			}
			for _, header := range []string{"Content-Length", "Content-Encoding", "Content-MD5", "Digest", "Content-Digest"} {
				if response.Header.Get(header) != "" {
					t.Errorf("%s survived XML conversion", header)
				}
			}
			awaitDAVXMLClose(t, upstream)
		})
	}
}

func TestDAVXMLBufferedErrorsReachReader(t *testing.T) {
	for name, input := range map[string]string{
		"malformed":   `<D:multistatus xmlns:D="DAV:"><D:response></D:multistatus>`,
		"unsafe-href": `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/u/bob/dav/private</D:href></D:response></D:multistatus>`,
		"after-block": strings.TrimSuffix(davXMLListing(200), "</D:multistatus>") + "</broken>",
	} {
		t.Run(name, func(t *testing.T) {
			upstream := observeDAVXMLBody(io.NopCloser(strings.NewReader(input)))
			response, mapper, base := davXMLResponse(upstream)
			if err := rewriteDAVXML(response, mapper, base); err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			got, err := io.ReadAll(response.Body)
			if err == nil {
				t.Fatal("XML conversion error became a successful EOF")
			}
			if name == "after-block" && len(got) < 32<<10 {
				t.Fatalf("error fixture did not exercise partially emitted XML: %d bytes", len(got))
			}
			awaitDAVXMLClose(t, upstream)
		})
	}
}

func TestDAVXMLBufferedCloseReleasesUpstream(t *testing.T) {
	t.Run("final-flush", func(t *testing.T) {
		upstream := observeDAVXMLBody(io.NopCloser(strings.NewReader(davXMLListing(1))))
		response, mapper, base := davXMLResponse(upstream)
		if err := rewriteDAVXML(response, mapper, base); err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		select {
		case <-upstream.eof:
		case <-time.After(3 * time.Second):
			t.Fatal("small XML conversion did not reach its final flush")
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		awaitDAVXMLClose(t, upstream)
		awaitDAVXMLClose(t, upstream)
	})
	t.Run("blocked-output", func(t *testing.T) {
		input := davXMLListing(2000)
		upstream := observeDAVXMLBody(io.NopCloser(strings.NewReader(input)))
		response, mapper, base := davXMLResponse(upstream)
		if err := rewriteDAVXML(response, mapper, base); err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if _, err := io.ReadFull(response.Body, make([]byte, 32<<10)); err != nil {
			t.Fatal(err)
		}
		if upstream.bytesRead.Load() >= int64(len(input)) {
			t.Fatal("streaming conversion consumed the entire listing before its first block")
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		// Close is called by the consumer and by the converter's defer.
		// Observing both proves the blocked converter reached its cleanup.
		awaitDAVXMLClose(t, upstream)
		awaitDAVXMLClose(t, upstream)
	})
	t.Run("blocked-input", func(t *testing.T) {
		reader, writer := io.Pipe()
		defer writer.Close()
		upstream := observeDAVXMLBody(reader)
		written := make(chan error, 1)
		go func() {
			_, err := io.WriteString(writer, `<D:multistatus xmlns:D="DAV:">`)
			written <- err
		}()
		response, mapper, base := davXMLResponse(upstream)
		if err := rewriteDAVXML(response, mapper, base); err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		select {
		case err := <-written:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("XML input was not consumed")
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		awaitDAVXMLClose(t, upstream)
		awaitDAVXMLClose(t, upstream)
	})
}

func TestDAVResourceHrefPathsWithoutAllocations(t *testing.T) {
	for _, tc := range []struct {
		path    string
		foreign bool
		want    bool
	}{
		{"multistatus/response/href", false, true},
		{"multistatus/response/location/href", false, true},
		{"prop/lockdiscovery/activelock/lockroot/href", false, true},
		{"multistatus/response/propstat/prop/lockdiscovery/activelock/lockroot/href", false, true},
		{"error/lock-token-submitted/href", false, true},
		{"error/no-conflicting-lock/href", false, true},
		{"multistatus/response/error/lock-token-submitted/href", false, true},
		{"multistatus/response/error/no-conflicting-lock/href", false, true},
		{"multistatus/response/href", true, false},
		{"multistatus/response/propstat/prop/custom/multistatus/response/href", false, false},
		{"prop/lockdiscovery/activelock/locktoken/href", false, false},
		{"multistatus/response/href/extra", false, false},
		{"href", false, false},
		{"", false, false},
	} {
		name := tc.path
		if tc.foreign {
			name += "/foreign-namespace"
		}
		t.Run(name, func(t *testing.T) {
			var stack []xml.Name
			if tc.path != "" {
				for _, part := range strings.Split(tc.path, "/") {
					stack = append(stack, xml.Name{Space: "DAV:", Local: part})
				}
			}
			if tc.foreign {
				stack[0].Space = "urn:custom"
			}
			var got bool
			allocations := testing.AllocsPerRun(100, func() { got = davResourceHref(stack) })
			if got != tc.want {
				t.Errorf("matched=%t want=%t", got, tc.want)
			}
			if allocations != 0 {
				t.Errorf("fixed-path matching allocated %.0f times", allocations)
			}
		})
	}
}

func BenchmarkDAVXMLRewrite(b *testing.B) {
	for _, entries := range []int{10, 200, 2000} {
		b.Run(fmt.Sprint(entries), func(b *testing.B) {
			input := davXMLListing(entries)
			buffer := make([]byte, 32<<10)
			var reads int
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				response, mapper, base := davXMLResponse(io.NopCloser(strings.NewReader(input)))
				if err := rewriteDAVXML(response, mapper, base); err != nil {
					b.Fatal(err)
				}
				for {
					n, err := response.Body.Read(buffer)
					if n > 0 {
						reads++
					}
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				response.Body.Close()
			}
			b.ReportMetric(float64(reads)/float64(b.N), "reads/op")
		})
	}
}
