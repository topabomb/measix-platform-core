package runtime

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/text/encoding/unicode"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

type davRecorder struct {
	events []usageingestapi.UsageSettlement
}

func TestDAVProtectsRootAndUnsafePathsBeforeForwarding(t *testing.T) {
	forwarded := 0
	h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		w.WriteHeader(204)
	}), "FULL")
	for _, tc := range []struct{ method, path, destination string }{
		{"DELETE", "", ""}, {"DELETE", "/", ""},
		{"MOVE", "", "child"}, {"PUT", "", ""}, {"MKCOL", "", ""},
		{"COPY", "child", ""}, {"MOVE", "child", ""},
		{"GET", "../private", ""},
		{"PUT", "%2e%2e/private", ""},
		{"COPY", "child", "../private"},
		{"MOVE", "child", "/"},
	} {
		t.Run(tc.method+"/"+tc.path+"/"+tc.destination, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, root+tc.path, strings.NewReader("body"))
			r.Header.Set("Authorization", "Bearer "+token)
			if tc.method == "COPY" || tc.method == "MOVE" {
				r.Header.Set("Destination", "http://example.com"+root+tc.destination)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 || forwarded != 0 {
				t.Fatalf("protected request status=%d forwarded=%d", w.Code, forwarded)
			}
		})
	}
	for _, method := range []string{"OPTIONS", "GET", "HEAD", "PROPFIND", "LOCK", "UNLOCK", "PROPPATCH"} {
		r := httptest.NewRequest(method, root, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatalf("root %s should retain DAV semantics: %d", method, w.Code)
		}
	}
}

func TestDAVForwardsFilesProtectedNames(t *testing.T) {
	for _, tc := range []struct{ method, path, destination string }{
		{"GET", ".agent-space/private", ""},
		{"PUT", "%2eagent-space/private", ""},
		{"MKCOL", ".agent-space-backup", ""},
		{"COPY", "ordinary", ".agent-space/private"},
		{"MOVE", "ordinary/.agent-space", ".agent-space-backup"},
	} {
		t.Run(tc.method+"/"+tc.path, func(t *testing.T) {
			calls := 0
			h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(204)
			}), "FULL")
			r := httptest.NewRequest(tc.method, root+tc.path, strings.NewReader("body"))
			r.Header.Set("Authorization", "Bearer "+token)
			if tc.destination != "" {
				r.Header.Set("Destination", "http://example.com"+root+tc.destination)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 204 || calls != 1 {
				t.Fatalf("root-owned DAV target rejected: status=%d calls=%d", w.Code, calls)
			}
		})
	}
}

func TestDAVRootListingPreservesAgentSpaceAndCompleteXML(t *testing.T) {
	h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		io.WriteString(w, `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/u/alice/dav/ordinary/</D:href></D:response><D:response><D:href>/u/alice/dav/.agent-space/</D:href><D:status>HTTP/1.1 403 Forbidden</D:status></D:response><D:response><D:href>/u/alice/dav/xu-tang/</D:href></D:response></D:multistatus>`)
	}), "FULL")
	server := httptest.NewServer(h)
	defer server.Close()
	for _, method := range []string{"PROPFIND", "DELETE"} {
		path := root
		if method == "DELETE" {
			path += "ordinary"
		}
		r, _ := http.NewRequest(method, server.URL+path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Depth", "1")
		res, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 207 || !strings.HasSuffix(string(body), "</D:multistatus>") || !strings.Contains(string(body), root+".agent-space/") || !strings.Contains(string(body), "403 Forbidden") {
			t.Fatalf("%s lost DAV response: status=%d bodyError=%v body=%s", method, res.StatusCode, err, body)
		}
	}
}

func TestDAVXMLBoundsTokensDepthAndHrefButNotListingSize(t *testing.T) {
	base, _ := url.Parse("http://upstream/u/alice/dav/")
	mapper := davMapper{upstream: base, publicRoot: "/dav/", publicHost: "public", publicScheme: "http"}
	for name, value := range map[string]string{
		"text":      "<prop>" + strings.Repeat("a", 2<<20) + "</prop>",
		"attribute": `<prop value="` + strings.Repeat("a", 2<<20) + `"/>`,
		"depth":     strings.Repeat("<a>", 300) + strings.Repeat("</a>", 300),
		"href":      `<multistatus xmlns="DAV:"><response><href>` + strings.Repeat("a", 20<<10) + `</href></response></multistatus>`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := mapDAVXML(io.Discard, strings.NewReader(value), mapper, base); err == nil {
				t.Fatal("unbounded XML accepted")
			}
		})
	}
	listing := `<multistatus xmlns="DAV:">` + strings.Repeat(`<response><href>/u/alice/dav/a</href><propstat><prop><x xmlns="urn:test">value</x></prop></propstat></response>`, 50000) + `</multistatus>`
	if len(listing) < 4<<20 {
		t.Fatal("fixture too small")
	}
	if err := mapDAVXML(io.Discard, strings.NewReader(listing), mapper, base); err != nil {
		t.Fatal("large streaming listing rejected", err)
	}
}

func (r *davRecorder) PersistAdmission(usageingestapi.BudgetAdmissionRequest, string) error {
	panic("DAV budget admission")
}
func (r *davRecorder) MarkStarted(string, time.Time) error { panic("DAV budget start") }
func (r *davRecorder) AbortAdmission(string) error         { panic("DAV budget abort") }
func (r *davRecorder) RecordDenied(usageingestapi.BudgetAdmissionRequest, string, usageingestapi.UsageSettlement) error {
	panic("DAV budget denial")
}
func (r *davRecorder) Record(e usageingestapi.UsageSettlement) error {
	r.events = append(r.events, e)
	return nil
}

func TestDAVRecordsOnlyBasicStatisticsWithoutAdmission(t *testing.T) {
	h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); w.Write([]byte("data")) }), "FULL")
	recorder := &davRecorder{}
	h.(*Handler).recorder = recorder
	r := httptest.NewRequest("PUT", root+"file", strings.NewReader("upload"))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if len(recorder.events) != 1 {
		t.Fatal("missing DAV statistic")
	}
	e := recorder.events[0]
	if e.StatisticsOnly == nil || !*e.StatisticsOnly || len(e.Meters) != 0 || e.Request.HttpMethod == nil || *e.Request.HttpMethod != "PUT" || e.Request.RequestBytes != 6 || e.Request.ResponseBytes != 4 || !e.Request.Forwarded || e.Request.DavSecretRef == nil {
		t.Fatalf("wrong DAV statistics: %+v", e)
	}
}

func davFixture(t *testing.T, upstream http.Handler, mode string, clocks ...func() time.Time) (http.Handler, string, string, *control.Store, relaycontrolapi.RuntimeControlState) {
	t.Helper()
	remote := httptest.NewServer(upstream)
	t.Cleanup(remote.Close)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	dep, user, device, session := platformid.New(platformid.Deployment), platformid.New(platformid.User), platformid.New(platformid.Device), platformid.New(platformid.Session)
	res, route, service, space := platformid.New(platformid.WebDAV), platformid.New(platformid.Route), platformid.New(platformid.WorkspaceService), platformid.New(platformid.AgentSpace)
	v := relaycontrolapi.RuntimeControlStateProtocolVersion(3)
	bindings := []relaycontrolapi.UserDAVBinding{{UserId: user, ResourceId: res, AccessMode: relaycontrolapi.DAVAccessMode(mode), Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: service, AgentSpaceId: space, RemoteUsername: "alice", BindingRevision: 1}, Endpoint: remote.URL + "/u/alice/dav/", SecretRef: relaycontrolapi.SecretRef{SecretId: platformid.New(platformid.Secret), SecretVersion: 1}, Token: "upstream-secret"}}
	state := relaycontrolapi.RuntimeControlState{ProtocolVersion: &v, DeploymentId: dep, ControlRevision: 1, ActiveManagedGeneration: 1, AuthKeys: []relaycontrolapi.PublicJwk{{Kty: "OKP", Crv: "Ed25519", Alg: "EdDSA", Use: "sig", Kid: "dav-test", X: base64.RawURLEncoding.EncodeToString(pub)}}, OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1024}, DavBindings: &bindings, ResourceRoutes: []relaycontrolapi.ResourceRoute{{ResourceId: res, RuntimeRouteId: route, ResourceKind: "WEBDAV", ClientProtocol: "WEBDAV"}}, Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: route, WorkspaceServiceId: &service, AllowedMethods: []string{"OPTIONS", "GET", "HEAD", "PROPFIND", "PUT", "DELETE", "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK"}, AllowedPathPrefixes: []string{"/dav"}, TransportPolicy: "HTTP_REQUEST_RESPONSE", TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 3000, IdleMs: 3000}}}}
	state.BundleHash, _ = control.HashDescriptor(state)
	clock := time.Now
	if len(clocks) > 0 {
		clock = clocks[0]
	}
	store := control.NewStore(clock)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	now := clock()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"iss": dep, "sub": user, "aud": []string{"runtime", "client"}, "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "sessionValidUntil": now.Add(time.Hour).Unix(), "deploymentId": dep, "deviceId": device, "sessionId": session})
	token.Header["kid"] = "dav-test"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(store, nil, nil), signed, "/runtime/v1/resources/" + res + "/spaces/" + space + "/dav/", store, state
}

func TestDAVXMLPreservesNestedDeadPropertiesAndMapsEmptyResourceHref(t *testing.T) {
	endpoint, _ := url.Parse("http://upstream/u/alice/dav/")
	mapper := davMapper{upstream: endpoint, publicRoot: "/public/", publicHost: "example.com", publicScheme: "http"}
	custom := `<x:custom xmlns:x="urn:test"><D:multistatus><D:response><D:href>/u/bob/dav/private</D:href></D:response></D:multistatus><D:activelock><D:lockroot><D:href>/u/bob/dav/private</D:href></D:lockroot></D:activelock></x:custom>`
	input := `<D:multistatus xmlns:D="DAV:"><D:response><D:href/><D:propstat><D:prop>` + custom + `</D:prop></D:propstat></D:response></D:multistatus>`
	var output bytes.Buffer
	if err := mapDAVXML(&output, strings.NewReader(input), mapper, endpoint); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), custom) || !strings.Contains(output.String(), `<D:href>/public/</D:href>`) {
		t.Fatal(output.String())
	}
}

func TestDAVUTF16ProtocolResponseIsReadableAndMapped(t *testing.T) {
	endpoint, _ := url.Parse("http://upstream/u/alice/dav/")
	mapper := davMapper{upstream: endpoint, publicRoot: "/public/", publicHost: "example.com", publicScheme: "http"}
	input := `<?xml version="1.0" encoding="UTF-16"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/u/alice/dav/%E4%B8%AD.txt</D:href></D:response></D:multistatus>`
	raw, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().String(input)
	if err != nil {
		t.Fatal(err)
	}
	response := &http.Response{Header: http.Header{"Content-Type": {"application/xml; charset=utf-16"}}, Body: io.NopCloser(strings.NewReader(raw))}
	if err = rewriteDAVXML(response, mapper, endpoint); err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `<D:href>/public/%E4%B8%AD.txt</D:href>`) || strings.Contains(strings.ToLower(string(body)), `encoding="utf-16"`) || !strings.Contains(response.Header.Get("Content-Type"), "utf-8") {
		t.Fatalf("invalid converted XML: %q", body)
	}
}

func TestDAVTransferSurvivesShortTokenExpiryButStopsOnKeyRevocation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "unrelated-update", true: "key-revoked"}[revoke], func(t *testing.T) {
			clock := time.Now()
			started, proceed := make(chan context.Context, 1), make(chan struct{})
			h, token, root, store, state := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				started <- r.Context()
				select {
				case <-proceed:
					_, _ = w.Write([]byte("done"))
				case <-r.Context().Done():
				}
			}), "FULL", func() time.Time { return clock })
			server := httptest.NewServer(h)
			defer server.Close()
			r, _ := http.NewRequest("GET", server.URL+root+"file", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			result := make(chan error, 1)
			go func() {
				response, err := server.Client().Do(r)
				if err == nil {
					_, err = io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
				result <- err
			}()
			remoteContext := <-started
			clock = clock.Add(2 * time.Minute)
			state.ControlRevision++
			if revoke {
				state.AuthKeys[0].Kid = "new-signing-key"
			}
			state.BundleHash, _ = control.HashDescriptor(state)
			if _, err := store.Apply(state); err != nil {
				t.Fatal(err)
			}
			if revoke {
				select {
				case <-remoteContext.Done():
				case <-time.After(time.Second):
					close(proceed)
					t.Fatal("removed signing key did not cancel transfer")
				}
			} else if remoteContext.Err() != nil {
				t.Fatal("short JWT expiration cancelled admitted transfer")
			}
			close(proceed)
			select {
			case err := <-result:
				if !revoke && err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("transfer did not finish")
			}
		})
	}
}

func TestDAVStreamsBeyondModelLimitAndPreservesRangeAndEscaping(t *testing.T) {
	payload := bytes.Repeat([]byte("abcdefgh"), 256*1024)
	var stored []byte
	h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("Cookie") != "" {
			t.Error("credential substitution failed")
		}
		if r.URL.EscapedPath() != "/u/alice/dav/%E4%B8%AD%20%25.mp4" {
			t.Error("path changed", r.URL.EscapedPath())
		}
		if r.Method == "PUT" {
			stored, _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"video"`)
			w.WriteHeader(201)
			return
		}
		if r.Header.Get("Range") != "bytes=10-19" || r.Header.Get("If-Range") != `"video"` {
			t.Error("lost range precondition")
		}
		w.Header().Set("ETag", `"video"`)
		w.Header().Set("Content-Range", "bytes 10-19/2097152")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(206)
		w.Write(stored[10:20])
	}), "FULL")
	request := httptest.NewRequest("PUT", root+"%E4%B8%AD%20%25.mp4", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Cookie", "must-not-leak")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 201 || !bytes.Equal(stored, payload) {
		t.Fatalf("large PUT status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest("GET", root+"%E4%B8%AD%20%25.mp4", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Range", "bytes=10-19")
	request.Header.Set("If-Range", `"video"`)
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 206 || response.Header().Get("Content-Range") != "bytes 10-19/2097152" || !bytes.Equal(response.Body.Bytes(), payload[10:20]) {
		t.Fatal("range forwarding failed", response.Code, response.Body.String())
	}
}

func TestDAVReadOnlyBlocksWritesBeforeUpstreamAndMapsOwnChallenge(t *testing.T) {
	calls := 0
	h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
		w.WriteHeader(401)
	}), "READ_ONLY")
	for _, method := range []string{"PUT", "DELETE", "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK"} {
		r := httptest.NewRequest(method, root+"file", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 || calls != 0 {
			t.Fatalf("%s reached upstream or wrong denial %d", method, w.Code)
		}
	}
	r := httptest.NewRequest("GET", root+"file", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 502 || w.Header().Get("WWW-Authenticate") != "" || strings.Contains(w.Body.String(), "private") {
		t.Fatal("upstream auth escaped", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", root+"file", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || !strings.HasPrefix(w.Header().Get("WWW-Authenticate"), "Bearer") {
		t.Fatal("platform challenge missing")
	}
}

func TestDAVMapsStandardURLsAndPreservesOpaquePropertiesAndLocks(t *testing.T) {
	var root string
	h, token, path, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "COPY" {
			if !strings.HasSuffix(r.Header.Get("Destination"), "/u/alice/dav/copy%20%25.txt") || !strings.Contains(r.Header.Get("If"), "/u/alice/dav/source.txt> (<opaquelocktoken:keep-me> [\"v1\"])") {
				t.Error("standard URI headers not mapped", r.Header.Get("Destination"), r.Header.Get("If"))
			}
			w.WriteHeader(201)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(207)
		io.WriteString(w, `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:" xmlns:x="urn:test"><D:response><D:href>/u/alice/dav/a%20%25.txt</D:href><D:propstat><D:prop><D:lockdiscovery><D:activelock><D:locktoken><D:href>opaquelocktoken:unchanged</D:href></D:locktoken><D:lockroot><D:href>/u/alice/dav/</D:href></D:lockroot></D:activelock></D:lockdiscovery><x:custom><D:href>/u/bob/dav/user-data</D:href></x:custom></D:prop><D:status>HTTP/1.1 403 Forbidden</D:status></D:propstat></D:response></D:multistatus>`)
	}), "FULL")
	root = path
	r := httptest.NewRequest("COPY", root+"source.txt", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Destination", "http://example.com"+root+"copy%20%25.txt")
	r.Header.Set("If", "<http://example.com"+root+"source.txt> (<opaquelocktoken:keep-me> [\"v1\"])")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal("COPY", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("PROPFIND", root, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	if w.Code != 207 || !strings.Contains(body, "<D:href>"+root+"a%20%25.txt</D:href>") || !strings.Contains(body, "<D:lockroot><D:href>"+root+"</D:href>") || !strings.Contains(body, `<x:custom><D:href>/u/bob/dav/user-data</D:href></x:custom>`) || !strings.Contains(body, "opaquelocktoken:unchanged") || !strings.Contains(body, "HTTP/1.1 403 Forbidden") {
		t.Fatal("DAV XML mapping/opaque content", body)
	}
	for _, destination := range []string{"http://elsewhere.invalid" + root + "x", "http://example.com" + strings.Replace(root, "/spaces/", "/spaces/wrong", 1) + "x", "http://example.com" + root + "%2e%2e/x"} {
		r = httptest.NewRequest("COPY", root+"source.txt", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Destination", destination)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("outside destination accepted", w.Code)
		}
	}
}
