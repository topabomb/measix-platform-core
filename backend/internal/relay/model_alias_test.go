package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

func TestManagedModelAliasMapsAllFourProtocols(t *testing.T) {
	tests := []struct {
		name, protocol, clientPath, upstreamPath string
		body                                     string
		wantModel                                string
	}{
		{"chat completions", "OPENAI_CHAT_COMPLETIONS", "/v1/chat/completions", "/v1/chat/completions", `{"model":"enterprise-chat","messages":[],"extension":{"keep":true}}`, "provider-chat"},
		{"responses", "OPENAI_RESPONSES", "/v1/responses", "/v1/responses", `{"model":"enterprise-responses","input":"hello","extension":{"keep":true}}`, "provider-responses"},
		{"anthropic", "ANTHROPIC_MESSAGES", "/v1/messages", "/v1/messages", `{"model":"enterprise-claude","messages":[],"extension":{"keep":true}}`, "provider-claude"},
		{"google", "GOOGLE_GENERATE_CONTENT", "/v1beta/models/enterprise-gemini:streamGenerateContent", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", `{"contents":[],"extension":{"keep":true}}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotQuery string
			var gotBody map[string]any
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			published := strings.TrimPrefix(strings.Split(strings.TrimPrefix(tc.clientPath, "/v1beta/models/"), ":")[0], "/")
			upstreamKey := strings.TrimPrefix(strings.Split(strings.TrimPrefix(tc.upstreamPath, "/v1beta/models/"), ":")[0], "/")
			if tc.protocol != "GOOGLE_GENERATE_CONTENT" {
				var decoded map[string]any
				_ = json.Unmarshal([]byte(tc.body), &decoded)
				published = decoded["model"].(string)
				upstreamKey = tc.wantModel
			}
			fixture, resourceID := modelAliasFixture(t, upstream.URL, tc.protocol, published, upstreamKey, tc.clientPath, tc.upstreamPath)
			defer fixture.close()
			request := fixture.request(t, nil, http.MethodPost, resourceID, tc.clientPath+"?alt=sse", strings.NewReader(tc.body), "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			responseBody, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
			}
			if gotPath != tc.upstreamPath || gotQuery != "alt=sse" {
				t.Fatalf("upstream target=%s?%s want %s?alt=sse", gotPath, gotQuery, tc.upstreamPath)
			}
			if tc.wantModel != "" && gotBody["model"] != tc.wantModel {
				t.Fatalf("upstream model=%v want %q", gotBody["model"], tc.wantModel)
			}
			if extension, ok := gotBody["extension"].(map[string]any); !ok || extension["keep"] != true {
				t.Fatalf("unknown request fields were not preserved: %#v", gotBody)
			}
		})
	}
}

func TestManagedModelAliasRejectsSelectorBypassBeforeForwarding(t *testing.T) {
	var forwarded atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded.Add(1) }))
	defer upstream.Close()
	fixture, resourceID := modelAliasFixture(t, upstream.URL, "OPENAI_CHAT_COMPLETIONS", "enterprise-chat", "provider-chat", "/v1/chat/completions", "/v1/chat/completions")
	defer fixture.close()

	for _, test := range []struct{ body, code string }{
		{`{}`, "invalid_model_selector"},
		{`{"model":42}`, "invalid_model_selector"},
		{`{"model":"provider-chat"}`, "invalid_model_selector"},
		{`{"model":"another-alias"}`, "invalid_model_selector"},
		{`{invalid`, "invalid_model_request"},
	} {
		body := test.body
		request := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(body), "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var problem relaycontrolapi.Problem
		_ = json.NewDecoder(response.Body).Decode(&problem)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest || problem.Code != test.code || problem.Forwarded == nil || *problem.Forwarded {
			t.Fatalf("body=%q status=%d problem=%+v", body, response.StatusCode, problem)
		}
	}
	if forwarded.Load() != 0 {
		t.Fatalf("invalid selectors reached upstream %d times", forwarded.Load())
	}
}

func modelAliasFixture(t *testing.T, upstreamURL, protocol, publishedKey, upstreamKey, clientPath, upstreamPath string) (*runtimeFixture, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: 1, ActiveManagedGeneration: 1, DeploymentId: platformid.New(platformid.Deployment),
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, DeletedUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{
			ResourceId: resourceID, RuntimeRouteId: routeID, ResourceKind: "MODEL", ClientProtocol: relaycontrolapi.ResourceRouteClientProtocol(protocol),
			LlmProfile:   &relaycontrolapi.RuntimeLlmProfile{},
			ModelMapping: &relaycontrolapi.RuntimeModelMapping{PublishedModelKey: publishedKey, UpstreamModelKey: upstreamKey, ClientRuntimePath: clientPath, UpstreamRuntimePath: upstreamPath},
		}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{upstreamPath},
			TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 5000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"},
			Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), resourceID
}
