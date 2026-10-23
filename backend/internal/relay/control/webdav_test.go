package control

import (
	"bytes"
	"testing"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

func davControlFixture() relaycontrolapi.RuntimeControlState {
	resource, route, service := platformid.New(platformid.WebDAV), platformid.New(platformid.Route), platformid.New(platformid.WorkspaceService)
	v := relaycontrolapi.RuntimeControlStateProtocolVersion(3)
	bindings := []relaycontrolapi.UserDAVBinding{{UserId: platformid.New(platformid.User), ResourceId: resource, AccessMode: "READ_ONLY", Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: service, AgentSpaceId: platformid.New(platformid.AgentSpace), RemoteUsername: "alice", BindingRevision: 7}, Endpoint: "http://127.0.0.1:8000/u/alice/dav/", SecretRef: relaycontrolapi.SecretRef{SecretId: platformid.New(platformid.Secret), SecretVersion: 2}, Token: "test-dav-private-credential"}}
	return relaycontrolapi.RuntimeControlState{ProtocolVersion: &v, DeploymentId: platformid.New(platformid.Deployment), ControlRevision: 1, ActiveManagedGeneration: 1, OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1024}, DavBindings: &bindings, ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: resource, RuntimeRouteId: route, ResourceKind: "WEBDAV", ClientProtocol: "WEBDAV"}}, Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: route, WorkspaceServiceId: &service, AllowedMethods: []string{"OPTIONS", "GET", "HEAD", "PROPFIND", "PUT", "DELETE", "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK"}, AllowedPathPrefixes: []string{"/dav"}, TransportPolicy: "HTTP_REQUEST_RESPONSE", TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000}}}}
}

func TestDAVControlHashIncludesPermissionButNeverCredential(t *testing.T) {
	input := davControlFixture()
	b, err := relaystate.DescriptorJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("test-dav-private-credential")) || !bytes.Contains(b, []byte("READ_ONLY")) {
		t.Fatal("descriptor omitted permission or exposed credential")
	}
	first, _ := relaystate.HashDescriptor(input)
	(*input.DavBindings)[0].Token = "different-private-value"
	second, _ := relaystate.HashDescriptor(input)
	if first != second {
		t.Fatal("credential entered descriptor hash")
	}
	(*input.DavBindings)[0].AccessMode = "FULL"
	third, _ := relaystate.HashDescriptor(input)
	if third == first {
		t.Fatal("permission not bound by hash")
	}
}

func TestDAVControlSupportsLocalAndLANHTTPAndRejectsAmbiguity(t *testing.T) {
	for _, origin := range []string{"http://localhost:8000", "http://192.168.1.10:8000", "http://[::1]:8000", "https://files.example"} {
		input := davControlFixture()
		(*input.DavBindings)[0].Endpoint = origin + "/u/alice/dav/"
		input.BundleHash, _ = HashDescriptor(input)
		store := NewStore(nil)
		ack, err := store.Apply(input)
		if err != nil || ack.ProtocolVersion == nil || *ack.ProtocolVersion != 3 {
			t.Fatalf("%s: %v", origin, err)
		}
	}
	for _, mutate := range []func(*relaycontrolapi.RuntimeControlState){
		func(s *relaycontrolapi.RuntimeControlState) { *s.ProtocolVersion = 2 },
		func(s *relaycontrolapi.RuntimeControlState) { (*s.DavBindings)[0].AccessMode = "" },
		func(s *relaycontrolapi.RuntimeControlState) {
			(*s.DavBindings)[0].Endpoint = "http://localhost/u/bob/dav/"
		},
		func(s *relaycontrolapi.RuntimeControlState) {
			*s.DavBindings = append(*s.DavBindings, (*s.DavBindings)[0])
		},
	} {
		input := davControlFixture()
		mutate(&input)
		input.BundleHash, _ = HashDescriptor(input)
		if _, err := build(input, time.Now()); err == nil {
			t.Fatal("invalid DAV control accepted")
		}
	}
}
