package relay_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	mathrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

const modelHTTPBodyLimit = 100 << 20

// These tests use real HTTP peers and the production handler. The control-state
// limit stays at 10 MiB, as on Hub, so changing that shared value cannot satisfy
// the model-only policy or silently raise MCP/WebSocket limits.
func TestModelHTTPBodyLimitBoundaries(t *testing.T) {
	for _, protocol := range []string{"OPENAI_CHAT_COMPLETIONS", "GOOGLE_GENERATE_CONTENT"} {
		t.Run(protocol, func(t *testing.T) {
			var forwarded atomic.Int64
			received := make(chan int64, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded.Add(1)
				n, err := io.Copy(io.Discard, r.Body)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				received <- n
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()
			fixture, resourceID, path := modelBodyLimitFixture(t, upstream.URL, protocol, true)
			defer fixture.close()
			for _, chunked := range []bool{false, true} {
				for _, delta := range []int{-1, 0, 1} {
					t.Run(fmt.Sprintf("chunked=%t/limit%+d", chunked, delta), func(t *testing.T) {
						size := int64(modelHTTPBodyLimit + delta)
						request := fixture.request(t, nil, "POST", resourceID, path, sizedModelBody(size), "application/json")
						if !chunked {
							request.ContentLength = size
							request.Header.Set("Expect", "100-continue")
						}
						before := forwarded.Load()
						response, err := fixture.server.Client().Do(request)
						if err != nil {
							t.Fatal(err)
						}
						defer response.Body.Close()
						if delta > 0 {
							assertBodyLimitProblem(t, response)
							if forwarded.Load() != before {
								t.Fatal("oversized request reached upstream")
							}
							return
						}
						if response.StatusCode != 200 {
							body, _ := io.ReadAll(response.Body)
							t.Fatalf("size=%d status=%d body=%s", size, response.StatusCode, body)
						}
						_, _ = io.Copy(io.Discard, response.Body)
						want := size
						if protocol != "GOOGLE_GENERATE_CONTENT" {
							// The public selector is shorter than the provider selector.
							// The limit applies before this server-owned replacement.
							want += int64(len("provider-model") - len("client"))
						}
						if got := <-received; got != want {
							t.Fatalf("upstream received %d bytes, want %d", got, want)
						}
					})
				}
			}
		})
	}
}

func TestModelHTTPForwardsImageHistoryOver10MiB(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	_, _ = mathrand.New(mathrand.NewSource(1)).Read(pixels.Pix)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	imageData := base64.StdEncoding.EncodeToString(encoded.Bytes())
	openAIImage := `{"type":"image_url","image_url":{"url":"data:image/png;base64,` + imageData + `"}}`
	responseImage := `{"type":"input_image","image_url":"data:image/png;base64,` + imageData + `"}`
	anthropicImage := `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imageData + `"}}`
	googleImage := `{"inlineData":{"mimeType":"image/png","data":"` + imageData + `"}}`
	for _, tc := range []struct{ protocol, body string }{
		{"OPENAI_CHAT_COMPLETIONS", `{"model":"client","messages":[{"role":"user","content":[` + openAIImage + `]},{"role":"assistant","content":"First image received"},{"role":"user","content":[` + openAIImage + `]}]}`},
		{"OPENAI_RESPONSES", `{"model":"client","input":[{"role":"user","content":[` + responseImage + `]},{"role":"assistant","content":"First image received"},{"role":"user","content":[` + responseImage + `]}]}`},
		{"ANTHROPIC_MESSAGES", `{"model":"client","max_tokens":16,"messages":[{"role":"user","content":[` + anthropicImage + `]},{"role":"assistant","content":"First image received"},{"role":"user","content":[` + anthropicImage + `]}]}`},
		{"GOOGLE_GENERATE_CONTENT", `{"contents":[{"role":"user","parts":[` + googleImage + `]},{"role":"model","parts":[{"text":"First image received"}]},{"role":"user","parts":[` + googleImage + `]}]}`},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			if len(tc.body) <= 10<<20 {
				t.Fatalf("image history only contains %d bytes", len(tc.body))
			}
			var expected map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.body), &expected); err != nil {
				t.Fatal(err)
			}
			wantBody := []byte(tc.body)
			if tc.protocol != "GOOGLE_GENERATE_CONTENT" {
				expected["model"] = json.RawMessage(`"provider-model"`)
				wantBody, _ = json.Marshal(expected)
			}
			wantHash := sha256.Sum256(wantBody)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				hash := sha256.New()
				n, err := io.Copy(hash, r.Body)
				if err != nil || n != int64(len(wantBody)) || !bytes.Equal(hash.Sum(nil), wantHash[:]) {
					http.Error(w, "image history changed or truncated", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()
			fixture, resourceID, path := modelBodyLimitFixture(t, upstream.URL, tc.protocol, true)
			defer fixture.close()
			for _, chunked := range []bool{false, true} {
				request := fixture.request(t, nil, "POST", resourceID, path, strings.NewReader(tc.body), "application/json")
				if chunked {
					request.ContentLength = -1
				}
				response, err := fixture.server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if response.StatusCode != 200 {
					t.Fatalf("chunked=%t bytes=%d status=%d body=%s", chunked, len(tc.body), response.StatusCode, body)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("upstream calls=%d", calls.Load())
			}
			t.Logf("forwarded %d-byte PNG history with declared and chunked lengths; upstream SHA-256 matched", len(tc.body))
		})
	}
}

func TestLegacyModelHTTPBodyLimitBeforeForwarding(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	fixture, resourceID, path := modelBodyLimitFixture(t, upstream.URL, "OPENAI_CHAT_COMPLETIONS", false)
	defer fixture.close()
	request := fixture.request(t, nil, "POST", resourceID, path, sizedModelBody(11<<20), "application/json")
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 204 || calls.Load() != 1 {
		t.Fatalf("legacy model status=%d upstream calls=%d", response.StatusCode, calls.Load())
	}
	request = fixture.request(t, nil, "POST", resourceID, path, sizedModelBody(modelHTTPBodyLimit+1), "application/json")
	response, err = fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	assertBodyLimitProblem(t, response)
	if calls.Load() != 1 {
		t.Fatal("oversized legacy request reached upstream")
	}
}

func TestModelHTTPBodyLimitKeepsAdmissionBeforeForwarding(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	fixture, resourceID, path := modelBodyLimitFixture(t, upstream.URL, "OPENAI_CHAT_COMPLETIONS", true)
	fixture.server.Close()
	budget := &bodyLimitBudgetClient{}
	fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, fixture.recorder, budget))
	fixture.server.Client().Transport.(*http.Transport).ExpectContinueTimeout = time.Second
	defer fixture.close()
	for _, tc := range []struct {
		name, code string
		status     int
		change     func(*http.Request)
	}{
		{"auth", "invalid_session", 401, func(r *http.Request) { r.Header.Del("Authorization") }},
		{"generation", "managed_snapshot_required", 428, func(r *http.Request) { r.Header.Set("X-Measix-Managed-Generation", "0") }},
		{"path", "resource_not_allowed", 403, func(r *http.Request) { r.URL.Path += "/denied" }},
		{"selector", "invalid_model_selector", 400, func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"model":"bypass"}`))
			r.ContentLength = -1
		}},
		{"budget", "budget_exhausted", 429, func(*http.Request) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := fixture.request(t, nil, "POST", resourceID, path, sizedModelBody(11<<20), "application/json")
			request.Header.Set("Expect", "100-continue")
			tc.change(request)
			response, err := fixture.server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var problem relaycontrolapi.Problem
			if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status || problem.Code != tc.code || problem.Forwarded == nil || *problem.Forwarded {
				t.Fatalf("status=%d problem=%+v", response.StatusCode, problem)
			}
		})
	}
	if calls.Load() != 0 || budget.calls.Load() != 1 {
		t.Fatalf("upstream calls=%d budget calls=%d", calls.Load(), budget.calls.Load())
	}
}

func TestMCPHTTPBodyLimitUnchanged(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	state := minimalControlState(t, 1, 1)
	resourceID, routeID, upstreamID := platformid.New(platformid.MCP), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
	state.OperationalLimits.MaxRequestBytes = 10 << 20
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID, ResourceKind: "MCP", ClientProtocol: "MCP_STREAMABLE_HTTP"}}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000}}}
	state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"MCP_STREAMABLE_HTTP"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE}}}
	fixture := newRuntimeFixture(t, state, key)
	fixture.server.Client().Transport.(*http.Transport).ExpectContinueTimeout = time.Second
	defer fixture.close()
	request := fixture.request(t, nil, "POST", resourceID, "/mcp", sizedModelBody(10<<20+1), "application/json")
	request.ContentLength = 10<<20 + 1
	request.Header.Set("Expect", "100-continue")
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	assertBodyLimitProblem(t, response)
	if calls.Load() != 0 {
		t.Fatal("oversized MCP request reached upstream")
	}
}

func modelBodyLimitFixture(t *testing.T, upstreamURL, protocol string, mapped bool) (*runtimeFixture, string, string) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	state := minimalControlState(t, 1, 1)
	resourceID, routeID, upstreamID := platformid.New(platformid.Model), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
	path, upstreamPath := "/v1/chat/completions", "/v1/chat/completions"
	switch protocol {
	case "OPENAI_RESPONSES":
		path, upstreamPath = "/v1/responses", "/v1/responses"
	case "ANTHROPIC_MESSAGES":
		path, upstreamPath = "/v1/messages", "/v1/messages"
	case "GOOGLE_GENERATE_CONTENT":
		path, upstreamPath = "/v1beta/models/client:generateContent", "/v1beta/models/provider-model:generateContent"
	}
	resource := relaycontrolapi.ResourceRoute{ResourceId: resourceID, RuntimeRouteId: routeID, ResourceKind: "MODEL", ClientProtocol: relaycontrolapi.ResourceRouteClientProtocol(protocol), LlmProfile: testLlmProfile(relaycontrolapi.ResourceRouteClientProtocol(protocol))}
	if mapped {
		resource.ModelMapping = &relaycontrolapi.RuntimeModelMapping{PublishedModelKey: "client", UpstreamModelKey: "provider-model", ClientRuntimePath: path, UpstreamRuntimePath: upstreamPath}
	}
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{resource}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{upstreamPath}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 10000, IdleMs: 10000}}}
	state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE}}}
	state.OperationalLimits.MaxRequestBytes = 10 << 20
	fixture := newRuntimeFixture(t, state, key)
	// httptest's client has a zero ExpectContinueTimeout by default. Wait for
	// early 413 responses instead of racing them with a 100 MiB upload.
	fixture.server.Client().Transport.(*http.Transport).ExpectContinueTimeout = time.Second
	return fixture, resourceID, path
}

type modelPaddingReader struct{}

func (modelPaddingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

func sizedModelBody(size int64) io.Reader {
	const prefix, suffix = `{"model":"client","messages":[],"padding":"`, `"}`
	return io.MultiReader(strings.NewReader(prefix), io.LimitReader(modelPaddingReader{}, size-int64(len(prefix)+len(suffix))), strings.NewReader(suffix))
}

func assertBodyLimitProblem(t *testing.T, response *http.Response) {
	t.Helper()
	var problem relaycontrolapi.Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 413 || problem.Status != 413 || problem.Code != "request_too_large" || problem.Forwarded == nil || *problem.Forwarded || problem.RequestId == nil || *problem.RequestId != response.Header.Get("X-Measix-Request-Id") || response.Header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status=%d problem=%+v", response.StatusCode, problem)
	}
}

type bodyLimitBudgetClient struct {
	allowBudgetClient
	calls atomic.Int64
}

func (b *bodyLimitBudgetClient) Admit(_ context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	b.calls.Add(1)
	return usageingestapi.BudgetAdmissionDecision{RequestId: input.RequestId, Allowed: false, Code: usageingestapi.BUDGETEXHAUSTED, Mode: usageingestapi.LIMITED, Source: usageingestapi.DEFAULT, AsOf: input.AdmittedAt}, nil, nil
}
