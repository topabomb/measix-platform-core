package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceTargetsFollowVerifiedUserAndRevokeInFlight(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/u/"), "/mcp")
		if r.Header.Get("Authorization") != "Bearer key-"+name {
			w.WriteHeader(401)
			return
		}
		if session := r.Header.Get("Mcp-Session-Id"); session != "" && session != "session-"+name {
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(stopped)
			return
		}
		w.Header().Set("Mcp-Session-Id", "session-"+name)
		io.WriteString(w, name)
	}))
	defer upstream.Close()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	state := minimalControlState(t, 1, 1)
	version := relaycontrolapi.RuntimeControlStateProtocolVersion(2)
	state.ProtocolVersion = &version
	workspaceService, mcp, route := platformid.New(platformid.WorkspaceService), platformid.New(platformid.MCP), platformid.New(platformid.Route)
	state.Upstreams = nil
	state.ResourceRoutes = []relaycontrolapi.ResourceRoute{{ResourceId: mcp, RuntimeRouteId: route, ResourceKind: "MCP", ClientProtocol: "MCP_STREAMABLE_HTTP"}}
	state.Routes = []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: route, WorkspaceServiceId: &workspaceService, AllowedMethods: []string{"GET", "POST", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE", TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000}}}
	users := []string{platformid.New(platformid.User), platformid.New(platformid.User)}
	bindings := []relaycontrolapi.UserRuntimeBinding{}
	for _, user := range users {
		name := "measix_" + strings.TrimPrefix(user, "usr_")
		bindings = append(bindings, relaycontrolapi.UserRuntimeBinding{UserId: user, McpServerId: mcp, Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: workspaceService, AgentSpaceId: platformid.New(platformid.AgentSpace), RemoteUsername: name, BindingRevision: 1}, Endpoint: upstream.URL + "/u/" + name + "/mcp", SecretRef: relaycontrolapi.SecretRef{SecretId: platformid.New(platformid.Secret), SecretVersion: 1}, Token: "key-" + name})
	}
	state.UserBindings = &bindings
	fixture := newRuntimeFixture(t, state, key)
	defer fixture.close()
	for i, user := range users {
		fixture.userID = user
		req := fixture.request(t, nil, "POST", mcp, "/mcp", strings.NewReader(`{"userId":"forged"}`), "application/json")
		req.Header.Set("X-User-Id", users[1-i])
		res, e := fixture.server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || string(body) != bindings[i].Target.RemoteUsername {
			t.Fatalf("identity routing %d %s", res.StatusCode, body)
		}
	}
	facts := fixture.recorder.waitFor(t, 2)
	for i, fact := range facts {
		if fact.UpstreamId != "" || fact.WorkspaceTarget == nil || fact.WorkspaceTarget.AgentSpaceId != bindings[i].Target.AgentSpaceId || fact.TargetVersion == nil || *fact.TargetVersion != 2 {
			t.Fatalf("wrong immutable attribution %+v", fact)
		}
	}
	fixture.userID = users[1]
	req := fixture.request(t, nil, "POST", mcp, "/mcp", strings.NewReader(`{}`), "application/json")
	req.Header.Set("Mcp-Session-Id", "session-"+bindings[0].Target.RemoteUsername)
	res, e := fixture.server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("cross-user session accepted: %d", res.StatusCode)
	}
	fixture.userID = platformid.New(platformid.User)
	req = fixture.request(t, nil, "POST", mcp, "/mcp", strings.NewReader(`{}`), "application/json")
	res, e = fixture.server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("missing binding fallback %d", res.StatusCode)
	}
	fixture.userID = users[0]
	req = fixture.request(t, nil, "GET", mcp, "/mcp", nil, "")
	res, e = fixture.server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	<-started
	retained := bindings[1:]
	state.UserBindings = &retained
	state.ControlRevision++
	state.AuthKeys = []relaycontrolapi.PublicJwk{fixture.signer.publicJWK()}
	hash, e := relaystate.HashDescriptor(state)
	if e != nil {
		t.Fatal(e)
	}
	state.BundleHash = hash
	if _, e = fixture.store.Apply(state); e != nil {
		t.Fatal(e)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked user's SSE remained alive")
	}
	if _, e = fixture.store.Apply(state); e != nil {
		t.Fatal("idempotent revoke", e)
	}
	fixture.userID = users[1]
	req = fixture.request(t, nil, "POST", mcp, "/mcp", strings.NewReader(`{}`), "application/json")
	res, e = fixture.server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("other user's binding lost")
	}
}
