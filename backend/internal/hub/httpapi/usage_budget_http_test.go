package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestAdminUsageReconciliationHTTPClosedLoop(t *testing.T) {
	_, identityService, _, ctx, adminID := setupFullHandler(t)
	now := identityService.Now().UTC()
	budgetService, err := budget.NewService(identityService.Client, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = identityService.Now
	usageService := usage.NewService(identityService.Client, budgetService)
	usageService.Now = identityService.Now
	h := httpapi.NewFull(httpapi.Services{Identity: identityService, Budget: budgetService, Usage: usageService})
	adminCookie, csrf := loginAdmin(t, h)
	member, err := identityService.CreateUser(ctx, "reconcile-member", "Reconcile Member", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	requestID := seedUsageRequest(t, ctx, identityService, budgetService, usageService, adminID, member.ID, nil, now, false)

	response := doJSON(t, h, http.MethodGet, "/api/admin/v1/usage/reconciliations?limit=10", map[string]string{"Cookie": adminCookie}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list reconciliation: %d %s", response.Code, response.Body)
	}
	var page adminapi.ReconciliationPage
	decodeJSON(t, response, &page)
	if len(page.Items) != 1 || page.Items[0].RequestId != requestID || page.Items[0].State != adminapi.ReconciliationViewStatePENDING {
		t.Fatalf("unexpected reconciliation page: %+v", page)
	}
	response = doJSON(t, h, http.MethodPost, "/api/admin/v1/usage/reconciliations/"+requestID+":resolve", map[string]string{
		"Cookie": adminCookie, "X-CSRF-Token": csrf,
	}, map[string]any{"expectedState": "PENDING", "action": "ACCEPT_OBSERVED", "reason": "verified against upstream trace"})
	if response.Code != http.StatusOK {
		t.Fatalf("resolve reconciliation: %d %s", response.Code, response.Body)
	}
	var resolved adminapi.ReconciliationView
	decodeJSON(t, response, &resolved)
	if resolved.State != adminapi.ReconciliationViewStateRESOLVED || resolved.ResolvedBy == nil || *resolved.ResolvedBy != adminID || resolved.ResolutionReason == nil {
		t.Fatalf("resolved response omitted audit identity: %+v", resolved)
	}
	response = doJSON(t, h, http.MethodGet, "/api/admin/v1/usage/reconciliations?limit=10", map[string]string{"Cookie": adminCookie}, nil)
	decodeJSON(t, response, &page)
	if len(page.Items) != 0 {
		t.Fatalf("resolved record remained pending: %+v", page.Items)
	}
}

func TestPortalUsageIsSelfScopedAndNeverCacheable(t *testing.T) {
	baseHandler, identityService, _, ctx, adminID := setupFullHandler(t)
	adminCookie, csrf := loginAdmin(t, baseHandler)
	session := enrollClientSession(t, baseHandler, adminCookie, csrf)
	principal, err := identityService.AuthenticateAccess(ctx, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	now := identityService.Now().UTC()
	budgetService, err := budget.NewService(identityService.Client, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = identityService.Now
	usageService := usage.NewService(identityService.Client, budgetService)
	usageService.Now = identityService.Now
	requestID := seedUsageRequest(t, ctx, identityService, budgetService, usageService, adminID, principal.UserID, &principal.DeviceID, now, true)
	if _, err := budgetService.Put(ctx, budget.PutBudgetInput{
		UserID: principal.UserID, Capability: budget.CapabilityModel, ExpectedRevision: 1, Mode: budget.ModeUnlimited,
		ActorUserID: adminID, Reason: "usage history remains visible while unlimited",
	}); err != nil {
		t.Fatal(err)
	}
	other, err := identityService.CreateUser(ctx, "other-usage", "Other Usage", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	otherRequestID := seedUsageRequest(t, ctx, identityService, budgetService, usageService, adminID, other.ID, nil, now, true)
	usageService.Now = func() time.Time { return now.Add(2 * time.Second) }

	h := httpapi.NewFull(httpapi.Services{Identity: identityService, Budget: budgetService, Usage: usageService})
	adminResponse := doJSON(t, h, http.MethodGet, "/api/admin/v1/users/"+principal.UserID+"/budgets", map[string]string{"Cookie": adminCookie}, nil)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin budgets: %d %s", adminResponse.Code, adminResponse.Body)
	}
	var adminBudgets adminapi.UserBudgetView
	decodeJSON(t, adminResponse, &adminBudgets)
	foundAdminUsage := false
	for _, item := range adminBudgets.Items {
		if item.Capability == adminapi.BudgetCapability("MODEL") {
			foundAdminUsage = item.Mode == adminapi.BudgetMode("UNLIMITED") && len(item.UsageMeters) == 1 && item.UsageMeters[0].Quantity == "1"
		}
	}
	if !foundAdminUsage {
		t.Fatalf("admin budget omitted retained usage meters: %+v", adminBudgets.Items)
	}
	grant, err := identityService.CreatePortalGrant(ctx, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	portalSecret, _, err := identityService.ExchangePortalGrant(ctx, grant.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Cookie": "measix_portal_session=" + portalSecret}
	for _, path := range []string{
		"/api/portal/v1/budgets",
		"/api/portal/v1/usage/summary",
		"/api/portal/v1/usage/trend",
		"/api/portal/v1/usage/distribution",
		"/api/portal/v1/usage/requests",
		"/api/portal/v1/usage/requests/" + requestID,
	} {
		response := doJSON(t, h, http.MethodGet, path, headers, nil)
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("portal endpoint %s: status=%d cache=%q body=%s", path, response.Code, response.Header().Get("Cache-Control"), response.Body)
		}
	}
	response := doJSON(t, h, http.MethodGet, "/api/portal/v1/usage/summary", headers, nil)
	var summary clientapi.UsageSummary
	decodeJSON(t, response, &summary)
	if summary.RequestCount != 1 || summary.ForwardedRequestCount != 1 {
		t.Fatalf("portal summary leaked another user: %+v", summary)
	}
	response = doJSON(t, h, http.MethodGet, "/api/portal/v1/budgets", headers, nil)
	var budgets clientapi.UserBudgetView
	decodeJSON(t, response, &budgets)
	foundModelUsage := false
	for _, item := range budgets.Items {
		if item.Capability != clientapi.BudgetCapability("MODEL") {
			continue
		}
		foundModelUsage = item.Mode == clientapi.BudgetMode("UNLIMITED") && len(item.UsageMeters) == 1 &&
			item.UsageMeters[0].Meter == clientapi.REQUESTS && item.UsageMeters[0].Quantity == "1"
	}
	if !foundModelUsage {
		t.Fatalf("unlimited budget omitted retained usage meters: %+v", budgets.Items)
	}
	response = doJSON(t, h, http.MethodGet, "/api/portal/v1/usage/requests/"+otherRequestID, headers, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("other user's request was distinguishable: %d %s", response.Code, response.Body)
	}
}

func seedUsageRequest(t *testing.T, ctx context.Context, identityService *identity.Service, budgetService *budget.Service, usageService *usage.Service, adminID, userID string, deviceID *string, now time.Time, complete bool) string {
	t.Helper()
	if _, err := budgetService.Put(ctx, budget.PutBudgetInput{
		UserID: userID, Capability: budget.CapabilityModel, ExpectedRevision: 0, Mode: budget.ModeLimited,
		Scopes:      []budget.ScopeInput{{Period: budget.PeriodLifetime, Limits: []budget.LimitSpec{{Meter: budget.MeterRequests, Limit: 100}}}},
		ActorUserID: adminID, Reason: "HTTP usage test budget",
	}); err != nil {
		t.Fatal(err)
	}
	requestID := platformid.New(platformid.Request)
	resourceID := platformid.New(platformid.Model)
	upstreamID := platformid.New(platformid.Upstream)
	routeID := platformid.New(platformid.Route)
	if _, err := identityService.Client.Upstream.Create().SetID(upstreamID).SetName("HTTP usage upstream").SetConfigRevision(1).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	decision, err := budgetService.Admit(ctx, budget.AdmitInput{
		RequestID: requestID, RequestHash: httpTestHash("admit:" + requestID), DeploymentID: identityService.Signer.DeploymentID,
		UserID: userID, DeviceID: deviceID, Capability: budget.CapabilityModel, ResourceID: resourceID,
		ClientProtocol: budget.ProtocolOpenAIResponses, UpstreamID: upstreamID, ManagedGeneration: 1, ControlRevision: 1,
		AdmittedAt: now, KnownQuantities: []budget.MeterQuantity{{Meter: budget.MeterRequests, Quantity: 1}},
		SupportedMeters: []budget.Meter{budget.MeterRequests},
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("admit: %+v %v", decision, err)
	}
	if err := budgetService.Start(ctx, budget.LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: now, EventHash: httpTestHash("start:" + requestID)}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	state, completeness := usageingestapi.RECONCILIATIONREQUIRED, usageingestapi.PARTIAL
	if complete {
		state, completeness = usageingestapi.SETTLED, usageingestapi.EXACT
	}
	event := usageingestapi.UsageSettlement{
		RequestId: requestID, Revision: 1, EventHash: httpTestHash("settle:" + requestID), SourceEventId: requestID + ":1",
		OccurredAt: now.Add(time.Second), State: state, Completeness: completeness,
		Meters: []usageingestapi.MeterValue{{Meter: usageingestapi.REQUESTS, Numerator: &one, Denominator: &one, Completeness: usageingestapi.EXACT}},
		Request: usageingestapi.RequestUsageFact{
			DeploymentId: identityService.Signer.DeploymentID, UserId: userID, DeviceId: deviceID,
			ResourceId: resourceID, ResourceKind: usageingestapi.ResourceKindMODEL, ClientProtocol: usageingestapi.OPENAIRESPONSES,
			RuntimeRouteId: routeID, UpstreamId: upstreamID, ManagedGeneration: 1, ControlRevision: 1,
			StartedAt: now, CompletedAt: now.Add(time.Second), Forwarded: true, HttpStatus: 200,
			RequestBytes: 10, ResponseBytes: 20, DurationMs: 1000,
		},
	}
	if _, err := usageService.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{event}}); err != nil {
		t.Fatal(err)
	}
	return requestID
}

func httpTestHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}
