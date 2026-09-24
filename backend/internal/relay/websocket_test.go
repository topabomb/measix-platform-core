package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

func TestRealtimeASRWebSocketAdmissionAndFrames(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "ws"
		if secure {
			name = "wss"
		}
		t.Run(name, func(t *testing.T) { testRealtimeASRWebSocketAdmissionAndFrames(t, secure) })
	}
}

func testRealtimeASRWebSocketAdmissionAndFrames(t *testing.T, secure bool) {
	upgrader := websocket.Upgrader{}
	closed := make(chan struct{}, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-asr-key" || r.URL.Query().Get("intent") != "transcription" {
			http.Error(w, "wrong credential or query", 401)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		defer func() { closed <- struct{}{} }()
		for {
			kind, body, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err = conn.WriteMessage(kind, body); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	state := minimalControlState(t, 1, 1)
	resourceID, routeID, upstreamID := platformid.New(platformid.ASR), platformid.New(platformid.Route), platformid.New(platformid.Upstream)
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"GET"}, AllowedPathPrefixes: []string{"/v1/realtime"}, TransportPolicy: relaycontrolapi.WEBSOCKET, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 3000}}}
	state.Upstreams = []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"WEBSOCKET"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": "synthetic-asr-key"}}}}
	fixture := newRuntimeFixture(t, state, key)
	fixture.server.Close()
	recorder := &captureUsageRecorder{}
	if secure {
		fixture.server = httptest.NewTLSServer(relayruntime.NewHandler(fixture.store, recorder, &allowBudgetClient{}))
	} else {
		fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, recorder, &allowBudgetClient{}))
	}
	defer fixture.close()
	req := fixture.request(t, nil, "GET", resourceID, "/v1/realtime?intent=transcription", nil, "")
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	if secure {
		// Trust this isolated server's certificate; do not disable TLS verification.
		dialer.TLSClientConfig = fixture.server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	}
	conn, resp, err := dialer.Dial(strings.Replace(req.URL.String(), "http", "ws", 1), req.Header)
	if err != nil {
		if resp != nil {
			t.Fatalf("upgrade status=%d err=%v", resp.StatusCode, err)
		}
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for _, frame := range []struct {
		kind int
		body string
	}{{websocket.TextMessage, `{"type":"session.update","session":{"type":"transcription"}}`}, {websocket.TextMessage, `{"type":"input_audio_buffer.append","audio":"AAABAA=="}`}, {websocket.BinaryMessage, "\x00\x01\xff\x00"}} {
		if err := conn.WriteMessage(frame.kind, []byte(frame.body)); err != nil {
			t.Fatal(err)
		}
		kind, body, err := conn.ReadMessage()
		if err != nil || kind != frame.kind || string(body) != frame.body {
			t.Fatalf("frame changed kind=%d body=%q err=%v", kind, body, err)
		}
	}
	conn.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("client disconnect did not close upstream")
	}
	events := recorder.waitFor(t, 1)
	if events[0].HttpStatus != 101 || events[0].RequestBytes == 0 || events[0].ResponseBytes == 0 {
		t.Fatalf("upgraded connection not metered correctly: %+v", events[0])
	}
	req.Header.Set("X-Measix-Managed-Generation", "0")
	_, denied, err := dialer.Dial(strings.Replace(req.URL.String(), "http", "ws", 1), req.Header)
	if err == nil || denied == nil || denied.StatusCode != 428 {
		t.Fatalf("stale generation handshake: response=%v err=%v", denied, err)
	}
	denied.Body.Close()
	req.Header.Set("X-Measix-Managed-Generation", "1")
	unauthenticated := req.Header.Clone()
	unauthenticated.Del("Authorization")
	_, denied, err = dialer.Dial(strings.Replace(req.URL.String(), "http", "ws", 1), unauthenticated)
	if err == nil || denied == nil || denied.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated handshake: response=%v err=%v", denied, err)
	}
	denied.Body.Close()
	t.Run("http-route-rejects-upgrade", func(t *testing.T) {
		httpState := minimalControlState(t, 1, 1)
		httpState.ResourceRoutes = state.ResourceRoutes
		httpState.Upstreams = state.Upstreams
		httpState.Routes = append([]relaycontrolapi.RuntimeRouteSpec(nil), state.Routes...)
		httpState.Routes[0].TransportPolicy = relaycontrolapi.HTTPREQUESTRESPONSE
		ordinary := newRuntimeFixture(t, httpState, key)
		defer ordinary.close()
		request := ordinary.request(t, nil, "GET", resourceID, "/v1/realtime?intent=transcription", nil, "")
		_, response, err := dialer.Dial(strings.Replace(request.URL.String(), "http://", "ws://", 1), request.Header)
		if err == nil || response == nil || response.StatusCode != http.StatusBadRequest {
			t.Fatalf("HTTP route accepted upgrade: response=%v err=%v", response, err)
		}
		response.Body.Close()
	})
	for _, scenario := range []struct {
		name, expected string
		limit          int
		send           bool
		overall        bool
	}{
		{"idle", "UPSTREAM_TIMEOUT", 1024, false, false},
		{"byte-limit", "REQUEST_TOO_LARGE", 8, true, false},
		{"overall", "UPSTREAM_TIMEOUT", 1024, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			state.Routes[0].TimeoutPolicy.IdleMs = 100
			state.Routes[0].TimeoutPolicy.OverallMs = nil
			if scenario.overall {
				overall := 100
				state.Routes[0].TimeoutPolicy.IdleMs = 3000
				state.Routes[0].TimeoutPolicy.OverallMs = &overall
			}
			state.OperationalLimits.MaxRequestBytes = scenario.limit
			bounded := newRuntimeFixture(t, state, key)
			bounded.server.Close()
			records := &captureUsageRecorder{}
			bounded.server = httptest.NewServer(relayruntime.NewHandler(bounded.store, records, &allowBudgetClient{}))
			defer bounded.close()
			request := bounded.request(t, nil, "GET", resourceID, "/v1/realtime?intent=transcription", nil, "")
			stream, _, err := dialer.Dial(strings.Replace(request.URL.String(), "http://", "ws://", 1), request.Header)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			stream.SetReadDeadline(time.Now().Add(time.Second))
			if scenario.send {
				_ = stream.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 32)))
			}
			if _, _, err := stream.ReadMessage(); err == nil {
				t.Fatal("bounded stream remained open")
			}
			facts := records.waitFor(t, 1)
			if facts[0].ErrorClass == nil || *facts[0].ErrorClass != scenario.expected {
				t.Fatalf("wrong terminal fact: %+v", facts[0])
			}
		})
	}
}
