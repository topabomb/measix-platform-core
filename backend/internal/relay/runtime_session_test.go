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

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

type sessionStream struct {
	fixture          *runtimeFixture
	resourceID, path string
	response         *http.Response
	upstreamContext  context.Context
	finish           chan struct{}
	calls            *atomic.Int64
}

func startSessionStream(t *testing.T, kind platformid.Kind, sessionLifetime time.Duration) *sessionStream {
	t.Helper()
	started := make(chan context.Context, 2)
	finish := make(chan struct{}, 1)
	calls := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		started <- r.Context()
		select {
		case <-finish:
			_, _ = io.WriteString(w, "data: done\n\n")
		case <-r.Context().Done():
		}
	}))
	f, id := singleRouteFixtureForKind(t, upstream.URL, "session-test-secret", 5000, kind)
	t.Cleanup(func() { close(finish); f.close(); upstream.Close() })
	deadline := f.signer.now.Add(sessionLifetime)
	f.signer.sessionValidUntil = &deadline
	// The access token has only one second left; the signed session deadline is independent.
	f.signer.now = f.signer.now.Add(-10*time.Minute + time.Second)
	path := "/v1/chat/completions"
	if kind == platformid.MCP {
		path = "/mcp"
	}
	req := f.request(t, nil, http.MethodPost, id, path, strings.NewReader(`{"model":"test","stream":true}`), "application/json")
	response, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	select {
	case remote := <-started:
		return &sessionStream{f, id, path, response, remote, finish, calls}
	case <-time.After(time.Second):
		t.Fatal("request did not reach upstream")
		return nil
	}
}

func applySessionControl(t *testing.T, f *runtimeFixture, mutate func(*relaycontrolapi.RuntimeControlState)) {
	t.Helper()
	state := f.controlState
	state.ControlRevision++
	mutate(&state)
	var err error
	state.BundleHash, err = relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Apply(state); err != nil {
		t.Fatal(err)
	}
	f.controlState = state
}

// RLY-AUTH-004 / RLY-CTL-008 / RLY-TRN-003.
func TestSessionStreamsSurviveAccessExpiryAndUnrelatedControlApply(t *testing.T) {
	for _, kind := range []platformid.Kind{platformid.Model, platformid.MCP} {
		for _, update := range []bool{false, true} {
			name := string(kind) + "/expiry"
			if update {
				name += "-and-control"
			}
			t.Run(name, func(t *testing.T) {
				s := startSessionStream(t, kind, time.Minute)
				// Exercise both the real exp timer and a control sweep after JWT time validation would fail.
				time.Sleep(1100 * time.Millisecond)
				s.fixture.advanceClock(2 * time.Second)
				if update {
					applySessionControl(t, s.fixture, func(*relaycontrolapi.RuntimeControlState) {})
				}
				req := s.fixture.request(t, nil, http.MethodPost, s.resourceID, s.path, http.NoBody, "application/json")
				response, err := s.fixture.server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
				if response.StatusCode != 401 || s.calls.Load() != 1 {
					t.Fatalf("expired new request: status=%d upstreamCalls=%d", response.StatusCode, s.calls.Load())
				}
				// Release only this response; leave the helper's finish channel open for cleanup.
				s.finish <- struct{}{}
				body, err := io.ReadAll(s.response.Body)
				if err != nil || !strings.Contains(string(body), "data: done") {
					t.Fatalf("admitted stream interrupted: error=%v body=%q", err, body)
				}
			})
		}
	}
}

func TestSessionStreamsStillCancelOnSecurityChanges(t *testing.T) {
	for _, kind := range []platformid.Kind{platformid.Model, platformid.MCP} {
		for _, change := range []string{"disabled-user", "deleted-user", "revoked-device", "revoked-session", "removed-key", "replaced-key", "removed-resource", "disabled-upstream"} {
			t.Run(string(kind)+"/"+change, func(t *testing.T) {
				s := startSessionStream(t, kind, time.Minute)
				s.fixture.advanceClock(2 * time.Second)
				applySessionControl(t, s.fixture, func(state *relaycontrolapi.RuntimeControlState) {
					switch change {
					case "disabled-user":
						state.PrincipalState.DisabledUserIds = []string{s.fixture.userID}
					case "deleted-user":
						state.PrincipalState.DeletedUserIds = []string{s.fixture.userID}
					case "revoked-device":
						state.PrincipalState.RevokedDeviceIds = []string{s.fixture.deviceID}
					case "revoked-session":
						state.PrincipalState.RevokedSessionIds = []string{s.fixture.sessionID}
					case "removed-key":
						state.AuthKeys = append([]relaycontrolapi.PublicJwk(nil), state.AuthKeys...)
						state.AuthKeys[0].Kid = "replacement-kid"
					case "replaced-key":
						public, _, err := ed25519.GenerateKey(rand.Reader)
						if err != nil {
							t.Fatal(err)
						}
						state.AuthKeys = append([]relaycontrolapi.PublicJwk(nil), state.AuthKeys...)
						state.AuthKeys[0].X = base64.RawURLEncoding.EncodeToString(public)
					case "removed-resource":
						state.ResourceRoutes = nil
					case "disabled-upstream":
						state.Upstreams = append([]relaycontrolapi.RuntimeUpstreamSpec(nil), state.Upstreams...)
						state.Upstreams[0].Enabled = false
					}
				})
				select {
				case <-s.upstreamContext.Done():
				case <-time.After(time.Second):
					t.Fatal("security change did not cancel upstream")
				}
			})
		}
	}
}

func TestSessionDeadlineIsFixedAndExpiredSessionCannotStart(t *testing.T) {
	for _, kind := range []platformid.Kind{platformid.Model, platformid.MCP} {
		t.Run(string(kind), func(t *testing.T) {
			s := startSessionStream(t, kind, 2*time.Second)
			// A later refresh can issue another token; it must not extend the captured request lease.
			later := s.fixture.store.Now().Add(time.Hour)
			s.fixture.signer.sessionValidUntil = &later
			started := time.Now()
			_, err := io.ReadAll(s.response.Body)
			elapsed := time.Since(started)
			if err == nil || elapsed < 1700*time.Millisecond || elapsed > 3*time.Second {
				t.Fatalf("fixed session deadline: elapsed=%v error=%v", elapsed, err)
			}
			select {
			case <-s.upstreamContext.Done():
			case <-time.After(time.Second):
				t.Fatal("session expiry did not cancel upstream")
			}
			expired := s.fixture.store.Now().Add(-time.Second)
			s.fixture.signer.sessionValidUntil = &expired
			s.fixture.signer.now = s.fixture.store.Now()
			req := s.fixture.request(t, nil, http.MethodPost, s.resourceID, s.path, http.NoBody, "application/json")
			response, err := s.fixture.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != 401 || s.calls.Load() != 1 {
				t.Fatalf("expired signed session: status=%d upstreamCalls=%d", response.StatusCode, s.calls.Load())
			}
		})
	}
}
