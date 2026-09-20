package relay_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/testutil"
	relaybudget "measix/platform/internal/relay/budget"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

type hubImageBudgetClient struct {
	service *budget.Service
	lastErr error
}

func (c *hubImageBudgetClient) Admit(ctx context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	known := make([]budget.MeterQuantity, 0)
	if input.KnownUsage != nil {
		for _, value := range *input.KnownUsage {
			if value.Numerator != nil && value.Denominator != nil && *value.Denominator == 1 {
				known = append(known, budget.MeterQuantity{Meter: budget.Meter(value.Meter), Quantity: *value.Numerator})
			}
		}
	}
	supported := make([]budget.Meter, len(input.SupportedMeters))
	for index, meter := range input.SupportedMeters {
		supported[index] = budget.Meter(meter)
	}
	decision, err := c.service.Admit(ctx, budget.AdmitInput{
		RequestID: input.RequestId, RequestHash: input.RequestHash, DeploymentID: input.DeploymentId,
		UserID: input.UserId, InteractionID: input.InteractionId, DeviceID: input.DeviceId,
		Capability: budget.CapabilityImageGeneration, ResourceID: input.ResourceId,
		ClientProtocol: budget.ClientProtocol(input.ClientProtocol), UpstreamID: input.UpstreamId,
		ManagedGeneration: int64(input.ManagedGeneration), ControlRevision: int64(input.ControlRevision),
		AdmittedAt: input.AdmittedAt, KnownQuantities: known, SupportedMeters: supported,
	})
	if err != nil {
		c.lastErr = err
		return usageingestapi.BudgetAdmissionDecision{}, nil, err
	}
	blocking := make([]usageingestapi.BudgetLimitState, len(decision.BlockingLimits))
	for index, limit := range decision.BlockingLimits {
		blocking[index] = usageingestapi.BudgetLimitState{
			Meter: usageingestapi.UsageMeter(limit.Meter), Period: usageingestapi.BudgetPeriod(limit.Period),
			Limit: limit.Limit, Used: limit.Used, Reserved: limit.Reserved, ResetAt: limit.ResetAt,
		}
	}
	return usageingestapi.BudgetAdmissionDecision{
		RequestId: decision.RequestID, Allowed: decision.Allowed, Code: usageingestapi.BudgetAdmissionDecisionCode(decision.Code),
		Mode: usageingestapi.BudgetMode(decision.Mode), Source: usageingestapi.BudgetSource(decision.Source), Revision: int(decision.Revision),
		InFlightRequests: int(decision.InFlightRequests), BlockingLimits: blocking, ResetAt: decision.ResetAt, AsOf: decision.AsOf,
	}, nil, nil
}

func (_ *hubImageBudgetClient) Start(_ context.Context, _ string, _ usageingestapi.BudgetLifecycleEvent) error {
	return nil
}

func (_ *hubImageBudgetClient) Release(_ context.Context, _ string, _ usageingestapi.BudgetReleaseRequest) error {
	return nil
}

var _ relaybudget.Client = (*hubImageBudgetClient)(nil)

func TestImageGenerationIsValidatedMeteredAndForwardedTransparently(t *testing.T) {
	wantBody := []byte(`{"model":"gpt-image-1","prompt":"opaque text","n":2,"size":"1024x1024"}`)
	responseBody := []byte(`{"data":[{"b64_json":"AA=="},{"url":"https://images.example/2"}]}`)
	var forwarded int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		got, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" || !bytes.Equal(got, wantBody) {
			t.Errorf("request changed: method=%s path=%s body=%s", r.Method, r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	}))
	defer upstream.Close()
	fixture, imageID := imageRuntimeFixture(t, upstream.URL)
	defer fixture.close()

	request := fixture.request(t, nil, http.MethodPost, imageID, "/v1/images/generations", bytes.NewReader(wantBody), "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	gotResponse, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Equal(gotResponse, responseBody) || forwarded != 1 {
		t.Fatalf("response status=%d body=%s forwarded=%d", response.StatusCode, gotResponse, forwarded)
	}
	settlements := fixture.recorder.waitForSettlements(t, 1)
	assertSettlementMeter(t, settlements[0], usageingestapi.REQUESTS, 1)
	assertSettlementMeter(t, settlements[0], usageingestapi.REQUESTEDIMAGES, 2)
}

func TestImageGenerationRejectsUnsupportedShapesBeforeForwarding(t *testing.T) {
	var forwarded int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	fixture, imageID := imageRuntimeFixture(t, upstream.URL)
	defer fixture.close()

	tests := []struct {
		name, path, contentType, body string
		method                        string
	}{
		{name: "edit endpoint", method: http.MethodPost, path: "/v1/images/edits", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":1,"size":"1024x1024"}`},
		{name: "multipart", method: http.MethodPost, path: "/v1/images/generations", contentType: "multipart/form-data; boundary=x", body: "--x--"},
		{name: "reference", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":1,"size":"1024x1024","reference_images":["x"]}`},
		{name: "mask", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":1,"size":"1024x1024","mask":"x"}`},
		{name: "stream", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":1,"size":"1024x1024","stream":true}`},
		{name: "async", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":1,"size":"1024x1024","async":true}`},
		{name: "too many", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":4,"size":"1024x1024"}`},
		{name: "unsupported size", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":1,"size":"256x256"}`},
		{name: "duplicate count", method: http.MethodPost, path: "/v1/images/generations", contentType: "application/json", body: `{"model":"gpt-image-1","prompt":"text","n":4,"n":1,"size":"1024x1024"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := fixture.request(t, nil, test.method, imageID, test.path, strings.NewReader(test.body), test.contentType)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusForbidden {
				t.Fatalf("status=%d, want pre-forward rejection", response.StatusCode)
			}
		})
	}
	if forwarded != 0 {
		t.Fatalf("unsupported image requests reached upstream %d times", forwarded)
	}
}

func TestImageGenerationExhaustedRequestAndImageMetersNeverReachUpstream(t *testing.T) {
	for _, meter := range []budget.Meter{budget.MeterRequests, budget.MeterRequestedImages} {
		t.Run(string(meter), func(t *testing.T) {
			var forwarded int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded++
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()
			store := testutil.OpenStoreHandle(t)
			now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
			identityService := testutil.NewIdentityService(t, store, now)
			boot, err := identityService.Bootstrap(context.Background(), "Image Budget", "admin", "Admin", "correct horse battery staple")
			if err != nil {
				t.Fatal(err)
			}
			fixture, imageID := imageRuntimeFixtureForDeployment(t, upstream.URL, identityService.Signer.DeploymentID)
			defer fixture.close()
			member, err := identityService.CreateUser(context.Background(), "image-member", "Image Member", "MEMBER")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Client.Device.Create().SetID(fixture.deviceID).SetUserID(member.ID).SetStatus("ACTIVE").SetCreatedAt(now).Save(context.Background()); err != nil {
				t.Fatal(err)
			}
			state := fixture.store.Current()
			resource := state.Resources[imageID]
			upstreamID := state.Routes[resource.RouteID].UpstreamID
			if _, err := store.Client.Upstream.Create().SetID(upstreamID).SetName("Image upstream").SetConfigRevision(1).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(context.Background()); err != nil {
				t.Fatal(err)
			}
			service, err := budget.NewService(store.Client, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			service.Now = func() time.Time { return now }
			if _, err := service.Put(context.Background(), budget.PutBudgetInput{
				UserID: member.ID, Capability: budget.CapabilityImageGeneration, Mode: budget.ModeLimited,
				Scopes:      []budget.ScopeInput{{Period: budget.PeriodLifetime, Limits: []budget.LimitSpec{{Meter: meter, Limit: 0}}}},
				ActorUserID: boot.AdminUserID, Reason: "deny image requests",
			}); err != nil {
				t.Fatal(err)
			}
			fixture.userID = member.ID
			fixture.server.Close()
			budgetClient := &hubImageBudgetClient{service: service}
			fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, fixture.recorder, budgetClient))

			body := `{"model":"image-test","prompt":"opaque","n":2,"size":"1024x1024"}`
			response, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, imageID, "/v1/images/generations", strings.NewReader(body), "application/json"))
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusTooManyRequests || forwarded != 0 {
				t.Fatalf("status=%d forwarded=%d budgetErr=%v", response.StatusCode, forwarded, budgetClient.lastErr)
			}
		})
	}
}

func imageRuntimeFixture(t *testing.T, upstreamURL string) (*runtimeFixture, string) {
	return imageRuntimeFixtureForDeployment(t, upstreamURL, platformid.New(platformid.Deployment))
}

func imageRuntimeFixtureForDeployment(t *testing.T, upstreamURL, deploymentID string) (*runtimeFixture, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	imageID := platformid.New(platformid.ImageGeneration)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: 1, ActiveManagedGeneration: 1, DeploymentId: deploymentID,
		PrincipalState: relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, DeletedUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes: []relaycontrolapi.ResourceRoute{{
			ResourceId: imageID, RuntimeRouteId: routeID, ResourceKind: relaycontrolapi.ResourceRouteResourceKindIMAGEGENERATION,
			ClientProtocol: relaycontrolapi.OPENAIIMAGESGENERATIONS,
			ImageProfile:   &relaycontrolapi.RuntimeImageProfile{MaxImagesPerRequest: 3, AllowedSizes: []string{"1024x1024", "1536x1024"}},
		}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/images/generations"},
			TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstreamURL, Enabled: true, TransportCapabilities: []string{"HTTP_REQUEST_RESPONSE"},
			Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), imageID
}

func assertSettlementMeter(t *testing.T, settlement usageingestapi.UsageSettlement, meter usageingestapi.UsageMeter, want int64) {
	t.Helper()
	for _, value := range settlement.Meters {
		if value.Meter == meter && value.Numerator != nil && value.Denominator != nil && *value.Numerator == want && *value.Denominator == 1 && value.Completeness == usageingestapi.EXACT {
			return
		}
	}
	t.Fatalf("missing exact %s=%d in %+v", meter, want, settlement.Meters)
}
