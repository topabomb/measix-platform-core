package relay_test

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

func TestRuntimeProxyPreservesCompressedResponseBytes(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`{"result":"transparent"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), compressed.Bytes()...)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		_, _ = w.Write(want)
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	request := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`), "application/json")
	clientTransport := http.DefaultTransport.(*http.Transport).Clone()
	clientTransport.DisableCompression = true
	response, err := (&http.Client{Transport: clientTransport}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", response.Header.Get("Content-Encoding"))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("compressed response changed: got %d bytes, want %d", len(got), len(want))
	}
}

func TestRuntimeProxyEnforcesHTTPStreamIdleTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("data: too-late\n\n"))
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixtureWithIdle(t, upstream.URL, "runtime-secret", 100)
	defer fixture.close()
	request := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"test","stream":true}`), "application/json")
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr == nil {
		t.Fatal("idle upstream stream completed without an idle-timeout error")
	}
	if elapsed := time.Since(started); elapsed >= 400*time.Millisecond {
		t.Fatalf("idle timeout took %v, want substantially less than upstream's 500ms stall", elapsed)
	}
}

func TestRuntimeProxyIdleTimeoutAllowsActiveLongStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for index := range 6 {
			_, _ = fmt.Fprintf(w, "data: %d\n\n", index)
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	fixture, resourceID := singleRouteFixtureWithIdle(t, upstream.URL, "runtime-secret", 100)
	defer fixture.close()
	request := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"test","stream":true}`), "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatalf("active long stream was interrupted: %v", readErr)
	}
	if !bytes.Contains(body, []byte("data: 5")) {
		t.Fatalf("active long stream was truncated: %q", body)
	}
}

func TestRuntimeRelayReusesUpstreamConnectionsAcrossHundredUserBursts(t *testing.T) {
	const concurrency = 100
	type barrier struct {
		entered atomic.Int64
		ready   chan struct{}
		release chan struct{}
	}
	newBarrier := func() *barrier { return &barrier{ready: make(chan struct{}), release: make(chan struct{})} }
	first, second := newBarrier(), newBarrier()
	var phase atomic.Int64
	phase.Store(1)
	var connections atomic.Int64

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := first
		if phase.Load() == 2 {
			current = second
		}
		if current.entered.Add(1) == concurrency {
			close(current.ready)
		}
		<-current.release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	upstream.Start()
	defer upstream.Close()

	fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
	defer fixture.close()
	tokens := make([]string, concurrency)
	for index := range tokens {
		tokens[index] = fixture.signer.sign(t, platformid.New(platformid.User), platformid.New(platformid.Device), platformid.New(platformid.Session))
	}
	clientTransport := http.DefaultTransport.(*http.Transport).Clone()
	clientTransport.MaxIdleConns = 256
	clientTransport.MaxIdleConnsPerHost = 128
	client := &http.Client{Transport: clientTransport, Timeout: 10 * time.Second}

	runWave := func(current *barrier) time.Duration {
		t.Helper()
		started := time.Now()
		resultErrors := make(chan error, concurrency)
		var group sync.WaitGroup
		group.Add(concurrency)
		for index := range concurrency {
			index := index
			go func() {
				defer group.Done()
				request, err := http.NewRequest(http.MethodPost, fixture.server.URL+"/runtime/v1/resources/"+resourceID+"/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
				if err == nil {
					request.Header.Set("Authorization", "Bearer "+tokens[index])
					request.Header.Set("X-Measix-Managed-Generation", "1")
					request.Header.Set("X-Measix-Interaction-Id", platformid.New(platformid.Interaction))
					request.Header.Set("Content-Type", "application/json")
					var response *http.Response
					response, err = client.Do(request)
					if response != nil {
						_, readErr := io.Copy(io.Discard, response.Body)
						closeErr := response.Body.Close()
						if err == nil {
							err = errors.Join(readErr, closeErr)
						}
						if err == nil && response.StatusCode != http.StatusOK {
							err = fmt.Errorf("runtime status %d", response.StatusCode)
						}
					}
				}
				resultErrors <- err
			}()
		}
		select {
		case <-current.ready:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d/%d requests reached upstream", current.entered.Load(), concurrency)
		}
		close(current.release)
		group.Wait()
		close(resultErrors)
		for err := range resultErrors {
			if err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(started)
	}

	firstDuration := runWave(first)
	phase.Store(2)
	secondDuration := runWave(second)
	if got := connections.Load(); got > concurrency+5 {
		t.Fatalf("two 100-user bursts opened %d upstream connections; want reuse after the first burst", got)
	}
	t.Logf("100-user relay burst: first=%v (%.1f req/s), warm=%v (%.1f req/s), upstream connections=%d",
		firstDuration, concurrency/firstDuration.Seconds(), secondDuration, concurrency/secondDuration.Seconds(), connections.Load())
}

func TestRLYI4TransportsStreamWithoutProtocolTranslation(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte{0x49, 0x44, 0x33, 0x01, 0x02})
		case "/v1/audio/transcriptions":
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
				http.Error(w, "multipart required", http.StatusBadRequest)
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("model") != "whisper-test" {
				http.Error(w, "invalid multipart", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"text":"ok"}`))
		case "/mcp":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	fixture, ids := multiTransportFixture(t, upstream.URL)
	defer fixture.close()

	t.Run("TTS binary", func(t *testing.T) {
		request := fixture.request(t, nil, http.MethodPost, ids.tts, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`), "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "audio/mpeg" || !bytes.Equal(body, []byte{0x49, 0x44, 0x33, 0x01, 0x02}) {
			t.Fatalf("unexpected binary response: status=%d type=%q body=%v", response.StatusCode, response.Header.Get("Content-Type"), body)
		}
	})

	t.Run("ASR multipart", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("model", "whisper-test")
		part, err := writer.CreateFormFile("file", "sample.wav")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("RIFF-test"))
		_ = writer.Close()
		request := fixture.request(t, nil, http.MethodPost, ids.asr, "/v1/audio/transcriptions", &body, writer.FormDataContentType())
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("unexpected ASR status: %d", response.StatusCode)
		}
	})

	t.Run("MCP Streamable HTTP", func(t *testing.T) {
		request := fixture.request(t, nil, http.MethodPost, ids.mcp, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"tools"`)) {
			t.Fatalf("unexpected MCP response: status=%d body=%s", response.StatusCode, body)
		}
	})

	if gotAuth != "Bearer runtime-secret" {
		t.Fatalf("Relay did not inject service-side upstream credential: %q", gotAuth)
	}
}

type transportIDs struct{ tts, asr, mcp string }

func multiTransportFixture(t *testing.T, upstreamURL string) (*runtimeFixture, transportIDs) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ids := transportIDs{tts: platformid.New(platformid.TTS), asr: platformid.New(platformid.ASR), mcp: platformid.New(platformid.MCP)}
	upstreamID := platformid.New(platformid.Upstream)
	ttsRoute := platformid.New(platformid.Route)
	asrRoute := platformid.New(platformid.Route)
	mcpRoute := platformid.New(platformid.Route)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            platformid.New(platformid.Deployment),
		PrincipalState:          relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{
			{ResourceId: ids.tts, RuntimeRouteId: ttsRoute},
			{ResourceId: ids.asr, RuntimeRouteId: asrRoute},
			{ResourceId: ids.mcp, RuntimeRouteId: mcpRoute},
		},
		Routes: []relaycontrolapi.RuntimeRouteSpec{
			{RuntimeRouteId: ttsRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/speech"}, TransportPolicy: relaycontrolapi.HTTPBINARYSTREAM, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: asrRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/transcriptions"}, TransportPolicy: relaycontrolapi.HTTPMULTIPART, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
			{RuntimeRouteId: mcpRoute, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}},
		},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true,
			TransportCapabilities: []string{"HTTP_BINARY_STREAM", "HTTP_MULTIPART", "MCP_STREAMABLE_HTTP"},
			Auth:                  relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "runtime-secret"}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), ids
}
