package agentspace

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const previewTestSpace = "spc_550e8400-e29b-41d4-a716-446655440000"
const previewTestCapabilities = `{"schemaVersion":1,"supported":true,"sizes":[128,256,512],"contentType":"image/webp","mediaKinds":["IMAGE","VIDEO"]}`

// Real Pillow-produced 3x2 images: lossy RGB, lossy RGBA and lossless RGBA.
var previewTestImages = []string{
	"UklGRkAAAABXRUJQVlA4IDQAAADwAQCdASoDAAIAAQAcJaACdLoB+AAETAAA/vW4f/6aR40jxpHxcP/ugT90CfugT/3NoAAA",
	"UklGRmIAAABXRUJQVlA4WAoAAAAQAAAAAgAAAQAAQUxQSAcAAAAAUFBQUFBQAFZQOCA0AAAA8AEAnQEqAwACAAEAHCWgAnS6AfgABEwAAP71uH/+mkeNI8aR8XD/7oE/dAn7oE/9zaAAAA==",
	"UklGRhwAAABXRUJQVlA4TA8AAAAvAkAAEAcQ/Y9CBSKi/wEA",
}

func previewTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := New(server.URL, server.URL, server.URL, "management-secret")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func previewTestRequest() PreviewRequest {
	return PreviewRequest{Method: "GET", Path: "媒体/a + %.png", Size: 256, ExpectedETag: `"source,v1"`}
}

func previewTestImage(t *testing.T, index int) []byte {
	t.Helper()
	if index == 0 {
		data, err := os.ReadFile("../../../../api/fixtures/workspace/thumbnail.webp")
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	data, err := base64.StdEncoding.DecodeString(previewTestImages[index])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func previewTestHeaders(w http.ResponseWriter, data []byte) {
	w.Header().Set("Agent-Space-Id", previewTestSpace)
	w.Header().Set("Preview-Source-ETag", `"source,v1"`)
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
}

func previewTestError(t *testing.T, err error, code string, status int) *Error {
	t.Helper()
	var remote *Error
	if !errors.As(err, &remote) || remote.Code != code || remote.Status != status {
		t.Fatalf("error = %#v, want %s/%d", err, code, status)
	}
	return remote
}

func TestPreviewCapabilitiesRequireCompleteV1AndIgnoreFutureFields(t *testing.T) {
	c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "OPTIONS" || r.URL.Path != "/u/alice/previews" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer dav-secret" {
			t.Errorf("unexpected capability request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Agent-Space-Id", previewTestSpace)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, strings.TrimSuffix(previewTestCapabilities, "}")+`,"future":{"url":"https://ignored.invalid"}}`)
	})
	out, err := c.PreviewCapabilities(context.Background(), "alice", "dav-secret", previewTestSpace)
	if err != nil || !out.Supported || out.SchemaVersion != 1 || len(out.Sizes) != 3 || len(out.MediaKinds) != 2 || len(out.AdditionalProperties) != 0 {
		t.Fatalf("capabilities = %+v, %v", out, err)
	}
}

func TestPreviewCapabilitiesRejectMalformedKnownFields(t *testing.T) {
	for _, tc := range []struct{ name, data, mime, space string }{
		{"missing", `{"schemaVersion":1,"supported":true}`, "application/json", previewTestSpace},
		{"null", strings.Replace(previewTestCapabilities, `"sizes":[128,256,512]`, `"sizes":null`, 1), "application/json", previewTestSpace},
		{"future-version", strings.Replace(previewTestCapabilities, `"schemaVersion":1`, `"schemaVersion":2`, 1), "application/json", previewTestSpace},
		{"partial", strings.Replace(previewTestCapabilities, "128,256,512", "128,256", 1), "application/json", previewTestSpace},
		{"duplicate-size", strings.Replace(previewTestCapabilities, "128,256,512", "128,128,512", 1), "application/json", previewTestSpace},
		{"duplicate-key", strings.TrimSuffix(previewTestCapabilities, "}") + `,"supported":true}`, "application/json", previewTestSpace},
		{"false-without-root-proof", strings.Replace(previewTestCapabilities, `"supported":true`, `"supported":false`, 1), "application/json", previewTestSpace},
		{"wrong-mime", previewTestCapabilities, "text/html", previewTestSpace},
		{"wrong-space", previewTestCapabilities, "application/json", "spc_other"},
		{"trailing-json", previewTestCapabilities + `{}`, "application/json", previewTestSpace},
		{"invalid-utf8", strings.TrimSuffix(previewTestCapabilities, "}") + ",\"future\":\"\xff\"}", "application/json", previewTestSpace},
		{"oversized", strings.Repeat(" ", 16<<10) + previewTestCapabilities, "application/json", previewTestSpace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.mime)
				w.Header().Set("Agent-Space-Id", tc.space)
				io.WriteString(w, tc.data)
			})
			_, err := c.PreviewCapabilities(context.Background(), "alice", "dav-secret", previewTestSpace)
			previewTestError(t, err, "preview_protocol_error", 503)
		})
	}
}

func TestPreviewCapabilitiesPreserveSafeProblemsAndAuthenticationFailures(t *testing.T) {
	for _, tc := range []struct {
		status     int
		code       string
		wantStatus int
		wantCode   string
	}{
		{401, "ignored", 503, "dav_credential_unavailable"},
		{403, "ignored", 403, "file_forbidden"},
		{429, "preview_busy", 429, "preview_busy"},
		{503, "preview_dependency_unavailable", 503, "preview_dependency_unavailable"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			var calls atomic.Int32
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"type":"about:blank","title":"secret path","status":%d,"code":%q}`, tc.status, tc.code)
			})
			_, err := c.PreviewCapabilities(context.Background(), "alice", "dav-secret", previewTestSpace)
			remote := previewTestError(t, err, tc.wantCode, tc.wantStatus)
			if calls.Load() != 1 || tc.status == 429 && remote.RetryAfter != "1" {
				t.Fatalf("capability error/probe changed: %+v calls=%d", remote, calls.Load())
			}
		})
	}
}

func TestPreviewLegacyRequiresAuthenticatedRootDirectory(t *testing.T) {
	for _, status := range []int{404, 405} {
		for _, root := range []struct {
			name, xml string
			status    int
			code      string
		}{
			{"directory", `<multistatus xmlns="DAV:"><response><href>/u/alice/dav/</href><propstat><prop><resourcetype><collection/></resourcetype></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`, 207, ""},
			{"file", `<multistatus xmlns="DAV:"><response><href>/u/alice/dav/</href><propstat><prop><resourcetype/></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`, 207, "preview_unavailable"},
			{"wrong-root", `<multistatus xmlns="DAV:"><response><href>/u/bob/dav/</href><propstat><prop><resourcetype><collection/></resourcetype></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`, 207, "preview_unavailable"},
			{"unauthorized", "", 401, "dav_credential_unavailable"},
			{"forbidden", "", 403, "file_forbidden"},
			{"missing-root", "", 404, "preview_unavailable"},
		} {
			t.Run(fmt.Sprintf("%d/%s", status, root.name), func(t *testing.T) {
				calls := 0
				c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Header.Get("Authorization") != "Bearer dav-secret" {
						t.Error("probe lost exact DAV credential")
					}
					if calls == 1 {
						w.WriteHeader(status)
						return
					}
					if r.Method != "PROPFIND" || r.URL.Path != "/u/alice/dav/" || r.Header.Get("Depth") != "0" {
						t.Errorf("wrong legacy probe %s %s", r.Method, r.URL)
					}
					w.WriteHeader(root.status)
					io.WriteString(w, root.xml)
				})
				out, err := c.PreviewCapabilities(context.Background(), "alice", "dav-secret", previewTestSpace)
				if calls != 2 {
					t.Fatalf("legacy capability calls=%d", calls)
				}
				if root.code != "" {
					wantStatus := 503
					if root.status == 403 {
						wantStatus = 403
					}
					previewTestError(t, err, root.code, wantStatus)
				} else if err != nil || out.Supported || out.Sizes == nil || out.MediaKinds == nil || len(out.Sizes) != 0 || len(out.MediaKinds) != 0 {
					t.Fatalf("legacy capability=%+v %v", out, err)
				}
			})
		}
	}
}

func TestPreviewValidatesRealWebPAndSeparatesSourceAndDerivedConditions(t *testing.T) {
	for index := range previewTestImages {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			data := previewTestImage(t, index)
			match := fmt.Sprintf(`"derived,old", "%x"`, sha256.Sum256(data))
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.Method != "GET" || r.URL.Path != "/u/alice/previews" || q.Get("path") != "媒体/a + %.png" || q.Get("expectedEtag") != `"source,v1"` || q.Get("size") != "256" || len(q) != 3 {
					t.Errorf("wrong preview target: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("If-Match") != match || r.Header.Get("If-None-Match") != `W/"derived,weak", "other"` || r.Header.Get("Range") != "" || r.Header.Get("Authorization") != "Bearer dav-secret" {
					t.Errorf("wrong preview conditions: %v", r.Header)
				}
				previewTestHeaders(w, data)
				w.Header().Set("Set-Cookie", "secret=ignored")
				w.Header().Set("Location", "https://attacker.invalid")
				w.Write(data)
			})
			request := previewTestRequest()
			request.IfMatch = match
			request.IfNoneMatch = `W/"derived,weak", "other"`
			out, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, request)
			if err != nil || out.Status != 200 || string(out.Data) != string(data) || out.Header.Get("Cache-Control") != "no-store" || out.Header.Get("X-Content-Type-Options") != "nosniff" || out.Header.Get("Content-Disposition") != `inline; filename="thumbnail.webp"` || out.Header.Get("Set-Cookie") != "" || out.Header.Get("Location") != "" {
				t.Fatalf("preview=%+v %v", out, err)
			}
		})
	}
}

func TestPreviewRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	calls := 0
	c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) })
	for _, mutate := range []func(*PreviewRequest){
		func(r *PreviewRequest) { r.Method = "POST" },
		func(r *PreviewRequest) { r.Path = "" },
		func(r *PreviewRequest) { r.Path = ".agent-space/private.png" },
		func(r *PreviewRequest) { r.Path = "../secret" },
		func(r *PreviewRequest) { r.Size = 1024 },
		func(r *PreviewRequest) { r.ExpectedETag = `W/"weak"` },
		func(r *PreviewRequest) { r.ExpectedETag = `"one", "two"` },
		func(r *PreviewRequest) { r.ExpectedETag = "\"source\"\r\nInjected: secret" },
		func(r *PreviewRequest) { r.IfMatch = `"ok", broken` },
		func(r *PreviewRequest) { r.IfNoneMatch = `*, "other"` },
		func(r *PreviewRequest) { r.IfNoneMatch = `"unterminated` },
		func(r *PreviewRequest) { r.IfMatch = "\"ok\"\nsecret" },
	} {
		request := previewTestRequest()
		mutate(&request)
		_, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, request)
		previewTestError(t, err, "invalid_preview_request", 400)
	}
	if calls != 0 {
		t.Fatalf("invalid requests reached upstream: %d", calls)
	}
}

func TestPreviewRejectsUntrustedRepresentation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(http.ResponseWriter, *[]byte)
	}{
		{"source-mismatch", func(w http.ResponseWriter, _ *[]byte) { w.Header().Set("Preview-Source-ETag", `"other"`) }},
		{"space-mismatch", func(w http.ResponseWriter, _ *[]byte) { w.Header().Set("Agent-Space-Id", "spc_other") }},
		{"wrong-hash", func(w http.ResponseWriter, _ *[]byte) { w.Header().Set("ETag", `"`+strings.Repeat("0", 64)+`"`) }},
		{"wrong-mime", func(w http.ResponseWriter, _ *[]byte) { w.Header().Set("Content-Type", "image/png") }},
		{"no-length", func(w http.ResponseWriter, _ *[]byte) {
			w.Header().Del("Content-Length")
			w.Header().Set("Transfer-Encoding", "chunked")
		}},
		{"oversized-length", func(w http.ResponseWriter, _ *[]byte) { w.Header().Set("Content-Length", "1048577") }},
		{"truncated", func(_ http.ResponseWriter, data *[]byte) { *data = (*data)[:len(*data)-2] }},
		{"not-webp", func(w http.ResponseWriter, data *[]byte) {
			*data = []byte("<html>raw source</html>")
			previewTestHeaders(w, *data)
		}},
		{"duplicate-source", func(w http.ResponseWriter, _ *[]byte) { w.Header().Add("Preview-Source-ETag", `"other"`) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				data := previewTestImage(t, 0)
				previewTestHeaders(w, data)
				tc.mutate(w, &data)
				w.Write(data)
			})
			out, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, previewTestRequest())
			if out != nil {
				t.Fatalf("unvalidated bytes escaped: %+v", out)
			}
			previewTestError(t, err, "preview_protocol_error", 503)
		})
	}
}

func TestPreviewHeadAnd304ValidateHeadersWithoutPretendingToValidateBytes(t *testing.T) {
	for _, tc := range []struct {
		method string
		status int
	}{{"HEAD", 200}, {"GET", 304}, {"HEAD", 304}} {
		t.Run(fmt.Sprintf("%s/%d", tc.method, tc.status), func(t *testing.T) {
			data := previewTestImage(t, 0)
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				previewTestHeaders(w, data)
				if tc.status == 304 {
					w.Header().Del("Content-Length")
				}
				w.WriteHeader(tc.status)
			})
			req := previewTestRequest()
			req.Method = tc.method
			if tc.status == 304 {
				req.IfNoneMatch = `W/"` + fmt.Sprintf("%x", sha256.Sum256(data)) + `"`
			}
			out, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, req)
			if err != nil || out.Status != tc.status || len(out.Data) != 0 || out.Header.Get("ETag") == "" {
				t.Fatalf("header response=%+v %v", out, err)
			}
		})
	}
}

func TestPreviewSanitizesProblemsAndSeparatesCredentialAndForbidden(t *testing.T) {
	for _, tc := range []struct {
		status     int
		code       string
		wantStatus int
		wantCode   string
	}{
		{400, "invalid_preview_request", 400, "invalid_preview_request"},
		{401, "anything", 503, "dav_credential_unavailable"},
		{403, "anything", 403, "file_forbidden"},
		{404, "file_not_found", 404, "file_not_found"},
		{412, "file_version_conflict", 412, "file_version_conflict"},
		{413, "preview_source_limit", 413, "preview_source_limit"},
		{415, "preview_unsupported_media", 415, "preview_unsupported_media"},
		{422, "preview_decode_failed", 422, "preview_decode_failed"},
		{429, "preview_busy", 429, "preview_busy"},
		{503, "preview_dependency_unavailable", 503, "preview_dependency_unavailable"},
		{503, "preview_timeout", 503, "preview_timeout"},
		{500, "internal_secret_path", 503, "preview_unavailable"},
		{503, "file_not_found", 503, "preview_unavailable"},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.code), func(t *testing.T) {
			calls := 0
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/problem+json")
				w.Header().Set("Retry-After", "10000")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"type":"about:blank","title":"/private/path secret","status":%d,"code":%q}`, tc.status, tc.code)
			})
			_, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, previewTestRequest())
			remote := previewTestError(t, err, tc.wantCode, tc.wantStatus)
			if strings.Contains(err.Error(), "secret") || calls != 1 {
				t.Fatalf("unsafe/repeated error: %v calls=%d", err, calls)
			}
			if tc.status == 429 && remote.RetryAfter != "1" {
				t.Fatalf("unsafe RetryAfter=%q", remote.RetryAfter)
			}
		})
	}
}

func TestPreviewMissingLegacyRouteNeedsIndependentCapabilityProof(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(fmt.Sprint(supported), func(t *testing.T) {
			var methods []string
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.Method == "GET" {
					w.WriteHeader(404)
					return
				}
				if r.Method == "OPTIONS" {
					if !supported {
						w.WriteHeader(405)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Agent-Space-Id", previewTestSpace)
					io.WriteString(w, previewTestCapabilities)
					return
				}
				w.WriteHeader(207)
				io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>/u/alice/dav/</href><propstat><prop><resourcetype><collection/></resourcetype></prop><status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`)
			})
			_, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, previewTestRequest())
			if supported {
				previewTestError(t, err, "file_not_found", 404)
			} else {
				previewTestError(t, err, "preview_not_supported", 501)
			}
			if len(methods) < 2 || methods[0] != "GET" || methods[1] != "OPTIONS" {
				t.Fatalf("missing independent proof: %v", methods)
			}
		})
	}
}

func TestPreviewNeverFollowsRedirectEvenWithCustomHTTPClient(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	c.HTTP = &http.Client{}
	_, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, previewTestRequest())
	previewTestError(t, err, "preview_protocol_error", 503)
	if leaked.Load() != 0 {
		t.Fatalf("redirect exposed request to another origin")
	}
}

func TestPreviewBodiesHaveIdleAndCallerCancellation(t *testing.T) {
	for _, capability := range []bool{false, true} {
		t.Run(fmt.Sprint(capability), func(t *testing.T) {
			closed := make(chan struct{})
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				w.Header().Set("Agent-Space-Id", previewTestSpace)
				w.Header().Set("Content-Type", "application/json")
				if !capability {
					previewTestHeaders(w, previewTestImage(t, 0))
				}
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			c.DAVIdleTimeout = 25 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := time.Now()
			var err error
			if capability {
				_, err = c.PreviewCapabilities(ctx, "alice", "dav-secret", previewTestSpace)
			} else {
				_, err = c.Preview(ctx, "alice", "dav-secret", previewTestSpace, previewTestRequest())
			}
			if err == nil || time.Since(started) > time.Second {
				t.Fatalf("stalled preview was not bounded: %v", err)
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("upstream body not closed")
			}
		})
	}
}

func TestPreviewCallerCancellationClosesUpstreamBody(t *testing.T) {
	for _, capability := range []bool{false, true} {
		t.Run(fmt.Sprint(capability), func(t *testing.T) {
			started, closed := make(chan struct{}), make(chan struct{})
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				w.Header().Set("Agent-Space-Id", previewTestSpace)
				w.Header().Set("Content-Type", "application/json")
				if !capability {
					previewTestHeaders(w, previewTestImage(t, 0))
				}
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				if capability {
					_, err = c.PreviewCapabilities(ctx, "alice", "dav-secret", previewTestSpace)
				} else {
					_, err = c.Preview(ctx, "alice", "dav-secret", previewTestSpace, previewTestRequest())
				}
				done <- err
			}()
			<-started
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled response succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("caller cancellation did not release adapter")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("caller cancellation did not close upstream")
			}
		})
	}
}

func TestPreviewRejectsInconsistentDerivedConditions(t *testing.T) {
	data := previewTestImage(t, 0)
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	for _, tc := range []struct {
		name, match, none string
		status            int
	}{
		{"wrong-304", "", `"unrelated"`, 304},
		{"ignored-none", "", etag, 200},
		{"ignored-match", `"unrelated"`, "", 200},
		{"weak-match", "W/" + etag, "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				previewTestHeaders(w, data)
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					w.Write(data)
				}
			})
			request := previewTestRequest()
			request.IfMatch = tc.match
			request.IfNoneMatch = tc.none
			_, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, request)
			previewTestError(t, err, "preview_protocol_error", 503)
		})
	}
}

func TestPreviewWebPRejectsInvalidAlphaHeaderAndUncompressedLength(t *testing.T) {
	data := previewTestImage(t, 0)
	canvas := previewTestChunk("VP8X", []byte{0x10, 0, 0, 0, 2, 0, 0, 1, 0, 0})
	for name, alpha := range map[string][]byte{
		"truncated-uncompressed": {0, 80},
		"invalid-compression":    {2, 80, 80, 80, 80, 80, 80},
		"invalid-preprocessing":  {0x20, 80, 80, 80, 80, 80, 80},
	} {
		t.Run(name, func(t *testing.T) {
			bad := previewTestRIFF(canvas, previewTestChunk("ALPH", alpha), data[12:])
			if validPreviewWebP(bad, 256) {
				t.Fatal("invalid alpha representation passed structural check")
			}
		})
	}
}

func previewTestRIFF(chunks ...[]byte) []byte {
	data := []byte("RIFF\x00\x00\x00\x00WEBP")
	for _, chunk := range chunks {
		data = append(data, chunk...)
	}
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	return data
}
func previewTestChunk(kind string, payload []byte) []byte {
	data := append([]byte(kind), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(payload)))
	data = append(data, payload...)
	if len(payload)%2 != 0 {
		data = append(data, 0)
	}
	return data
}

func TestPreviewWebPStructureRejectsAnimationBoundsAndContainerAmbiguity(t *testing.T) {
	valid := previewTestImage(t, 0)
	bitstream := valid[12:]
	canvas := []byte{0, 0, 0, 0, 2, 0, 0, 1, 0, 0}
	for name, data := range map[string][]byte{
		"trailing":         append(append([]byte{}, valid...), 0),
		"truncated":        valid[:len(valid)-1],
		"only-header":      previewTestRIFF(),
		"animated":         previewTestRIFF(previewTestChunk("ANIM", make([]byte, 6)), bitstream),
		"frame":            previewTestRIFF(previewTestChunk("ANMF", make([]byte, 16)), bitstream),
		"double-image":     previewTestRIFF(bitstream, bitstream),
		"oversized-canvas": previewTestRIFF(previewTestChunk("VP8X", []byte{0, 0, 0, 0, 0, 2, 0, 1, 0, 0}), bitstream),
		"canvas-mismatch":  previewTestRIFF(previewTestChunk("VP8X", []byte{0, 0, 0, 0, 3, 0, 0, 1, 0, 0}), bitstream),
		"late-canvas":      previewTestRIFF(bitstream, previewTestChunk("VP8X", canvas)),
		"empty-bitstream":  previewTestRIFF(previewTestChunk("VP8 ", nil)),
		"oversized-chunk":  previewTestRIFF([]byte{'V', 'P', '8', ' ', 255, 255, 255, 255}),
	} {
		t.Run(name, func(t *testing.T) {
			c := previewTestClient(t, func(w http.ResponseWriter, r *http.Request) { previewTestHeaders(w, data); w.Write(data) })
			_, err := c.Preview(context.Background(), "alice", "dav-secret", previewTestSpace, previewTestRequest())
			previewTestError(t, err, "preview_protocol_error", 503)
		})
	}
}
