package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
)

func TestUpstreamProbeReportsHTTPObservationNotDeclaredCapabilities(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx := context.Background()
	id := testutil.NewIdentityService(t, st, time.Now().UTC())
	boot, err := id.Bootstrap(ctx, "Test", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	box, err := security.NewSecretBox(make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	svc := upstream.NewService(st.Client, box)
	h := httpapi.NewFull(httpapi.Services{Identity: id, Upstream: svc})
	cookie, csrf := loginAdmin(t, h)
	for _, code := range []int{200, 401, 405, 503} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodHead || r.Header.Get("Authorization") != "" {
					t.Error("unexpected probe request")
				}
				w.WriteHeader(code)
			}))
			defer server.Close()
			view, err := svc.CreateUpstream(ctx, boot.AdminUserID, adminapi.UpstreamConfig{
				Name: "Probe", BaseUrl: server.URL, Auth: adminapi.UpstreamAuth{Type: adminapi.UpstreamAuthTypeNONE},
				TransportCapabilities: []adminapi.UpstreamConfigTransportCapabilities{adminapi.UpstreamConfigTransportCapabilitiesHTTPSTREAMINGSSE},
				CorrelationMode:       adminapi.UpstreamConfigCorrelationModeHEADERECHO, UsageCapabilityLevel: adminapi.LEVEL0,
				TimeoutDefaults: adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000},
			})
			if err != nil {
				t.Fatal(err)
			}
			result := doJSON(t, h, http.MethodPost, "/api/admin/v1/upstreams/"+view.UpstreamID+":test", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, nil)
			var payload map[string]any
			decodeJSON(t, result, &payload)
			if result.Code != http.StatusOK || payload["reachable"] != true || payload["httpStatus"] != float64(code) {
				t.Fatalf("result=%d %s", result.Code, result.Body.String())
			}
			if _, exists := payload["verifiedCapabilities"]; exists {
				t.Fatal("HEAD probe must not claim capability verification")
			}
		})
	}
}
