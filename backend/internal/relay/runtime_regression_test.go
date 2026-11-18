package relay_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	relaybudget "measix/platform/internal/relay/budget"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

type workspaceRegressionBudget struct {
	allowBudgetClient
	mode string
}

type failingStartRecorder struct{ relayruntime.UsageRecorder }

func (*failingStartRecorder) MarkStarted(string, time.Time) error { return relaybudget.ErrUnavailable }

func (b *workspaceRegressionBudget) Admit(ctx context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	if b.mode == "admit-unavailable" {
		return usageingestapi.BudgetAdmissionDecision{}, nil, relaybudget.ErrUnavailable
	}
	if b.mode == "exhausted" {
		return usageingestapi.BudgetAdmissionDecision{RequestId: input.RequestId, Allowed: false, Code: usageingestapi.BUDGETEXHAUSTED, Mode: usageingestapi.LIMITED, Source: usageingestapi.EXPLICIT, AsOf: input.AdmittedAt}, nil, nil
	}
	return b.allowBudgetClient.Admit(ctx, input)
}

func (b *workspaceRegressionBudget) Start(context.Context, string, usageingestapi.BudgetLifecycleEvent) error {
	if b.mode == "start-unavailable" {
		return relaybudget.ErrUnavailable
	}
	return nil
}

func TestWorkspaceMeteringDegradedPreservesAuthenticatedTarget(t *testing.T) {
	for _, mode := range []string{"allowed", "not-configured", "admit-unavailable", "journal-unavailable", "start-journal-unavailable", "start-unavailable", "exhausted", "missing-binding", "invalid-auth"} {
		t.Run(mode, func(t *testing.T) {
			var forwarded atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/u/synthetic/mcp" || r.Header.Get("Authorization") != "Bearer synthetic-workspace-key" {
					http.Error(w, "wrong workspace target or credential", 401)
					return
				}
				forwarded.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "workspace-ok")
			}))
			defer upstream.Close()
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			state := minimalControlState(t, 1, 1)
			version := relaycontrolapi.RuntimeControlStateProtocolVersion(2)
			state.ProtocolVersion = &version
			workspaceID, mcpID, routeID := platformid.New(platformid.WorkspaceService), platformid.New(platformid.MCP), platformid.New(platformid.Route)
			userID := platformid.New(platformid.User)
			state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: mcpID, RuntimeRouteId: routeID, ResourceKind: "MCP", ClientProtocol: "MCP_STREAMABLE_HTTP"}}
			state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, WorkspaceServiceId: &workspaceID, AllowedMethods: []string{"GET", "POST", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE", TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000}}}
			bindings := []relaycontrolapi.UserRuntimeBinding{{UserId: userID, McpServerId: mcpID, Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: workspaceID, AgentSpaceId: platformid.New(platformid.AgentSpace), RemoteUsername: "synthetic", BindingRevision: 1}, Endpoint: upstream.URL + "/u/synthetic/mcp", SecretRef: relaycontrolapi.SecretRef{SecretId: platformid.New(platformid.Secret), SecretVersion: 1}, Token: "synthetic-workspace-key"}}
			if mode == "missing-binding" {
				bindings = nil
			}
			state.UserBindings = &bindings
			fixture := newRuntimeFixture(t, state, key)
			fixture.userID = userID
			defer fixture.close()
			fixture.server.Close()
			var recorder relayruntime.UsageRecorder = fixture.recorder
			if mode == "journal-unavailable" {
				recorder = &failingAdmissionRecorder{}
			}
			if mode == "start-journal-unavailable" {
				recorder = &failingStartRecorder{recorder}
			}
			var budgetClient relaybudget.Client = &workspaceRegressionBudget{mode: mode}
			if mode == "not-configured" {
				budgetClient = nil
			}
			fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, recorder, budgetClient))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			request := fixture.request(t, ctx, "POST", mcpID, "/mcp", strings.NewReader(`{"userId":"forged"}`), "application/json")
			if mode == "invalid-auth" {
				request.Header.Set("Authorization", "Bearer synthetic-invalid-token")
			}
			response, err := fixture.server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantForwarded := 200, int64(1)
			switch mode {
			case "exhausted":
				wantStatus, wantForwarded = 429, 0
			case "missing-binding":
				wantStatus, wantForwarded = 403, 0
			case "invalid-auth":
				wantStatus, wantForwarded = 401, 0
			}
			if response.StatusCode != wantStatus || forwarded.Load() != wantForwarded {
				t.Fatalf("mode=%s status=%d forwarded=%d body=%s; want status=%d forwarded=%d", mode, response.StatusCode, forwarded.Load(), body, wantStatus, wantForwarded)
			}
		})
	}
}

func TestNegotiatedWebSocketCompressionPreservesAudioMetering(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		name := "plain"
		if compressed {
			name = "permessage-deflate"
		}
		t.Run(name, func(t *testing.T) {
			upgrader := websocket.Upgrader{EnableCompression: compressed}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				conn.EnableWriteCompression(compressed)
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.updated","session":{"input_audio_format":"pcm","sample_rate":16000}}`)); err != nil {
					return
				}
				kind, message, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if err := conn.WriteMessage(kind, message); err != nil {
					return
				}
				_, _, _ = conn.ReadMessage()
			}))
			defer upstream.Close()
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			state := minimalControlState(t, 1, 1)
			resourceID, routeID, upstreamID := platformid.New(platformid.ASR), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
			state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID, ResourceKind: "ASR", ClientProtocol: "DASHSCOPE_REALTIME_ASR", AudioProfile: &relaycontrolapi.RuntimeAudioProfile{Encoding: "PCM16_LE", Channels: 1, SampleRates: []relaycontrolapi.RuntimeAudioProfileSampleRates{16000}}}}
			state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"GET"}, AllowedPathPrefixes: []string{"/api-ws/v1/realtime"}, TransportPolicy: relaycontrolapi.WEBSOCKET, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 3000}}}
			state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"WEBSOCKET"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE}}}
			fixture := newRuntimeFixture(t, state, key)
			defer fixture.close()
			request := fixture.request(t, nil, "GET", resourceID, "/api-ws/v1/realtime", nil, "")
			dialer := websocket.Dialer{EnableCompression: compressed, HandshakeTimeout: 2 * time.Second}
			conn, response, err := dialer.Dial(strings.Replace(request.URL.String(), "http", "ws", 1), request.Header)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			extensions := response.Header.Get("Sec-WebSocket-Extensions")
			if compressed && !strings.Contains(extensions, "permessage-deflate") {
				t.Fatalf("compression was not negotiated: %s", extensions)
			}
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, config, err := conn.ReadMessage()
			if err != nil || !strings.Contains(string(config), "session.updated") {
				t.Fatalf("config=%q err=%v", config, err)
			}
			audio := `{"type":"input_audio_buffer.append","audio":"` + base64.StdEncoding.EncodeToString(make([]byte, 32000)) + `"}`
			if err := conn.WriteMessage(websocket.TextMessage, []byte(audio)); err != nil {
				t.Fatal(err)
			}
			_, echoed, err := conn.ReadMessage()
			if err != nil || string(echoed) != audio {
				t.Fatalf("transport did not preserve audio message: err=%v", err)
			}
			_ = conn.Close()
			settlements := fixture.recorder.waitForSettlements(t, 1)
			for _, meter := range settlements[0].Meters {
				if meter.Meter != "AUDIO_SECONDS" {
					continue
				}
				if meter.Numerator != nil && meter.Denominator != nil && *meter.Numerator == *meter.Denominator && meter.Completeness != "UNKNOWN" {
					t.Logf("extensions=%q audio=1 second completeness=%s", extensions, meter.Completeness)
					return
				}
				t.Fatalf("negotiated compression=%v preserved transport but lost known one-second audio: meter=%+v", compressed, meter)
			}
			t.Fatalf("missing audio meter: %+v", settlements[0])
		})
	}
}

func TestSSEObservationOverflowPreservesTransportAndUnknownUsage(t *testing.T) {
	payload := "data: " + strings.Repeat("x", 3<<20) + "\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\ndata: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	state := minimalControlState(t, 1, 1)
	resourceID, routeID, upstreamID := platformid.New(platformid.Model), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID, ResourceKind: "MODEL", ClientProtocol: "OPENAI_CHAT_COMPLETIONS"}}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: "HTTP_STREAMING_SSE", TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 3000}}}
	state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE}}}
	fixture := newRuntimeFixture(t, state, key)
	defer fixture.close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := fixture.request(t, ctx, "POST", resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"synthetic","stream":true}`), "application/json")
	resp, err := fixture.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || string(body) != payload {
		t.Fatalf("observation changed business transport: status=%d bytes=%d want=%d err=%v", resp.StatusCode, len(body), len(payload), err)
	}
	settlements := fixture.recorder.waitForSettlements(t, 1)
	for _, meter := range settlements[0].Meters {
		if meter.Meter == "TOTAL_TOKENS" {
			if meter.Completeness != "UNKNOWN" || meter.Numerator != nil || meter.Denominator != nil {
				t.Fatalf("overflow became exact/zero usage: %+v", meter)
			}
			return
		}
	}
	t.Fatal("missing unknown token meter")
}
