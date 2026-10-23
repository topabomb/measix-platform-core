package runtimecontrol_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/relay/control"
	"measix/platform/pkg/platformid"
)

func TestUserDeletionDeniesRetiredCredentialsAndPurgesOwnedState(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 9, 20, 13, 0, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	member, err := identityService.CreateUser(ctx, "member-delete", "Member Delete", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := identityService.CreateEnrollment(ctx, member.ID, boot.AdminUserID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	installationID := platformid.New(platformid.Installation)
	credential, err := identityService.ExchangeEnrollment(ctx, grant.Code, installationID, "Delete device", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	budgetService, err := budget.NewService(st.Client, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = func() time.Time { return now }
	templateID := platformid.New(platformid.BudgetTemplate)
	if _, err := budgetService.CreateTemplate(ctx, budget.CreateTemplateInput{
		TemplateID: templateID, Name: "Shared template", Description: "survives actor deletion",
		ActorUserID: member.ID, Reason: "member authored shared template",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := budgetService.Put(ctx, budget.PutBudgetInput{
		UserID: member.ID, Capability: budget.CapabilityModel, Mode: budget.ModeUnlimited,
		ActorUserID: boot.AdminUserID, Reason: "initial allocation",
	}); err != nil {
		t.Fatal(err)
	}
	upstreamID := platformid.New(platformid.Upstream)
	if _, err := st.Client.Upstream.Create().SetID(upstreamID).SetName("Deletion upstream").SetConfigRevision(1).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	inFlightID := platformid.New(platformid.Request)
	if decision, err := budgetService.Admit(ctx, budget.AdmitInput{
		RequestID: inFlightID, RequestHash: testSHA256("deletion-in-flight"), DeploymentID: identityService.Signer.DeploymentID,
		UserID: member.ID, DeviceID: &credential.DeviceID, Capability: budget.CapabilityModel,
		ResourceID: platformid.New(platformid.Model), ClientProtocol: budget.ProtocolOpenAIResponses, UpstreamID: upstreamID,
		AdmittedAt: now, KnownQuantities: []budget.MeterQuantity{{Meter: budget.MeterRequests, Quantity: 1}}, SupportedMeters: []budget.Meter{budget.MeterRequests},
	}); err != nil || !decision.Allowed {
		t.Fatalf("create in-flight request: %+v %v", decision, err)
	}

	box, _ := security.NewSecretBox(bytes.Repeat([]byte{9}, 32), 1)
	upstreams := upstream.NewService(st.Client, box)
	capabilities := capability.NewService(st.Client)
	relayStore := control.NewStore(func() time.Time { return now })
	relayServer := httptest.NewServer(control.NewHandler(relayStore, "relay-service-token", "test-relay", nil))
	defer relayServer.Close()
	service := runtimecontrol.NewService(st.Client, capabilities, upstreams, identityService.Signer, runtimecontrol.NewHTTPRelayClient(relayServer.URL, "relay-service-token", relayServer.Client()))
	service.Now = func() time.Time { return now }

	if _, err := service.DeleteUser(ctx, boot.AdminUserID, platformid.New(platformid.Idempotency), member.ID, runtimecontrol.DeleteUserInput{ConfirmationUsername: "wrong", Reason: "employment ended"}); !errors.Is(err, runtimecontrol.ErrDeleteConfirmation) {
		t.Fatalf("confirmation mismatch err=%v", err)
	}
	if _, err := service.DeleteUser(ctx, boot.AdminUserID, platformid.New(platformid.Idempotency), member.ID, runtimecontrol.DeleteUserInput{ConfirmationUsername: member.Username, Reason: "employment ended"}); !errors.Is(err, runtimecontrol.ErrDeleteInFlight) {
		t.Fatalf("active request did not block deletion: %v", err)
	}
	if exists, _ := st.Client.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(member.ID)).Exist(ctx); exists {
		t.Fatal("preflight failure installed a deletion tombstone")
	}
	if err := budgetService.Release(ctx, budget.ReleaseInput{LifecycleInput: budget.LifecycleInput{
		RequestID: inFlightID, Revision: 1, OccurredAt: now, EventHash: testSHA256("release-deletion-in-flight"),
	}, Reason: "request never forwarded"}); err != nil {
		t.Fatal(err)
	}
	result, err := service.DeleteUser(ctx, boot.AdminUserID, platformid.New(platformid.Idempotency), member.ID, runtimecontrol.DeleteUserInput{ConfirmationUsername: member.Username, Reason: "employment ended"})
	if err != nil || result.State != "COMPLETED" {
		t.Fatalf("delete result=%+v err=%v", result, err)
	}
	if _, err := st.Client.User.Get(ctx, member.ID); !ent.IsNotFound(err) {
		t.Fatalf("deleted user remains: %v", err)
	}
	if count, _ := st.Client.Session.Query().Count(ctx); count != 0 {
		t.Fatalf("sessions remain: %d", count)
	}
	if count, _ := st.Client.Device.Query().Count(ctx); count != 0 {
		t.Fatalf("devices remain: %d", count)
	}
	if count, _ := st.Client.UserBudget.Query().Count(ctx); count != 0 {
		t.Fatalf("budget remains: %d", count)
	}
	templateAudit, err := budgetService.ListTemplateAudit(ctx, templateID, 10, "")
	if err != nil || len(templateAudit.Items) != 1 || templateAudit.Items[0].ActorUserID != "deleted_principal" {
		t.Fatalf("shared template audit attribution=%+v err=%v", templateAudit, err)
	}
	if exists, _ := st.Client.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(member.ID)).Exist(ctx); !exists {
		t.Fatal("security tombstone missing")
	}
	if _, denied := relayStore.Current().DeletedUsers[member.ID]; !denied {
		t.Fatal("Relay did not receive deleted principal")
	}
	if _, err := identityService.AuthenticateAccess(ctx, credential.AccessToken); !errors.Is(err, identity.ErrIdentityDeleted) {
		t.Fatalf("retired credential error=%v", err)
	}
	if _, err := identityService.Refresh(ctx, credential.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrIdentityDeleted) {
		t.Fatalf("retired refresh credential error=%v", err)
	}
	if _, err := budgetService.Admit(ctx, budget.AdmitInput{
		RequestID: platformid.New(platformid.Request), RequestHash: testSHA256("deleted-user-admission"),
		DeploymentID: identityService.Signer.DeploymentID, UserID: member.ID, Capability: budget.CapabilityModel,
		ResourceID: platformid.New(platformid.Model), ClientProtocol: budget.ProtocolOpenAIResponses,
		UpstreamID: platformid.New(platformid.Upstream), AdmittedAt: now,
		KnownQuantities: []budget.MeterQuantity{{Meter: budget.MeterRequests, Quantity: 1}},
		SupportedMeters: []budget.Meter{budget.MeterRequests},
	}); !errors.Is(err, budget.ErrIdentityDeleted) {
		t.Fatalf("deleted identity budget admission err=%v", err)
	}

	recreated, err := identityService.CreateUser(ctx, member.Username, "Recreated Member", "MEMBER")
	if err != nil {
		t.Fatalf("recreate username: %v", err)
	}
	if recreated.ID == member.ID {
		t.Fatal("recreated username reused the deleted principal id")
	}
	if exists, err := st.Client.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(recreated.ID)).Exist(ctx); err != nil || exists {
		t.Fatalf("fresh principal inherited deletion tombstone: exists=%v err=%v", exists, err)
	}
	reenrollment, err := identityService.CreateEnrollment(ctx, recreated.ID, boot.AdminUserID, 10*time.Minute)
	if err != nil {
		t.Fatalf("create enrollment for fresh principal: %v", err)
	}
	freshCredential, err := identityService.ExchangeEnrollment(ctx, reenrollment.Code, installationID, "Delete device", "2.0")
	if err != nil {
		t.Fatalf("re-enroll deleted installation for fresh principal: %v", err)
	}
	freshPrincipal, err := identityService.AuthenticateAccess(ctx, freshCredential.AccessToken)
	if err != nil || freshPrincipal.UserID != recreated.ID {
		t.Fatalf("fresh credential principal=%+v err=%v", freshPrincipal, err)
	}
	if _, err := identityService.Refresh(ctx, freshCredential.RefreshToken, platformid.New(platformid.Idempotency)); err != nil {
		t.Fatalf("fresh refresh credential rejected: %v", err)
	}
	if _, err := identityService.AuthenticateAccess(ctx, credential.AccessToken); !errors.Is(err, identity.ErrIdentityDeleted) {
		t.Fatalf("fresh enrollment revived retired access credential: %v", err)
	}
	if _, err := identityService.Refresh(ctx, credential.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrIdentityDeleted) {
		t.Fatalf("fresh enrollment revived retired refresh credential: %v", err)
	}
}

func testSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func TestI5SecurityDisableIsDenyFirstAndEnableIsAllowLast(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC)
	identity := testutil.NewIdentityService(t, st, now)
	boot, err := identity.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateUser(ctx, "member", "Member", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	box, _ := security.NewSecretBox(bytes.Repeat([]byte{7}, 32), 1)
	upstreams := upstream.NewService(st.Client, box)
	capabilities := capability.NewService(st.Client)
	relayStore := control.NewStore(func() time.Time { return now })
	relayServer := httptest.NewServer(control.NewHandler(relayStore, "relay-service-token", "test-relay", nil))
	defer relayServer.Close()
	relayClient := runtimecontrol.NewHTTPRelayClient(relayServer.URL, "relay-service-token", relayServer.Client())
	service := runtimecontrol.NewService(st.Client, capabilities, upstreams, identity.Signer, relayClient)
	service.Now = func() time.Time { return now }

	grant, err := identity.CreateEnrollment(ctx, member.ID, boot.AdminUserID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := identity.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Device", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	disable, err := service.DisableUser(ctx, boot.AdminUserID, platformid.New(platformid.Idempotency), member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if disable.State != "COMPLETED" || disable.Kind != "SECURITY_CHANGE" {
		t.Fatalf("unexpected disable activation: %+v", disable)
	}
	user, _ := st.Client.User.Get(ctx, member.ID)
	if user.Status != "DISABLED" {
		t.Fatalf("user not disabled after deny-first activation: %s", user.Status)
	}
	if _, denied := relayStore.Current().DisabledUsers[member.ID]; !denied {
		t.Fatal("Relay did not receive disabled user")
	}

	key := platformid.New(platformid.Idempotency)
	enable, err := service.EnableUser(ctx, boot.AdminUserID, key, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if enable.State != "COMPLETED" || enable.Kind != "SECURITY_CHANGE" {
		t.Fatalf("unexpected enable activation: %+v", enable)
	}
	user, _ = st.Client.User.Get(ctx, member.ID)
	if user.Status != "ACTIVE" {
		t.Fatalf("user not enabled after Relay ACK: %s", user.Status)
	}
	if _, err := identity.Refresh(ctx, credential.RefreshToken, platformid.New(platformid.Idempotency)); err == nil {
		t.Fatal("enable resurrected pre-disable refresh credential")
	}
	if _, denied := relayStore.Current().RevokedSessions[credential.SessionID]; !denied {
		t.Fatal("pre-disable session not denied by Relay")
	}
	replay, err := service.EnableUser(ctx, boot.AdminUserID, key, member.ID)
	if err != nil || replay.ActivationID != enable.ActivationID {
		t.Fatalf("enable idempotency replay: %+v %v", replay, err)
	}
	if _, denied := relayStore.Current().DisabledUsers[member.ID]; denied {
		t.Fatal("Relay retained user deny after enable")
	}
}

func TestI5DeviceRevokeIsAppliedToRelay(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC)
	identity := testutil.NewIdentityService(t, st, now)
	boot, err := identity.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	member, _ := identity.CreateUser(ctx, "member2", "Member 2", "MEMBER")
	enrollment, _ := identity.CreateEnrollment(ctx, member.ID, boot.AdminUserID, 10*time.Minute)
	exchange, err := identity.ExchangeEnrollment(ctx, enrollment.Code, platformid.New(platformid.Installation), "Test device", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	box, _ := security.NewSecretBox(bytes.Repeat([]byte{8}, 32), 1)
	upstreams := upstream.NewService(st.Client, box)
	capabilities := capability.NewService(st.Client)
	relayStore := control.NewStore(func() time.Time { return now })
	relayServer := httptest.NewServer(control.NewHandler(relayStore, "relay-service-token", "test-relay", nil))
	defer relayServer.Close()
	service := runtimecontrol.NewService(st.Client, capabilities, upstreams, identity.Signer, runtimecontrol.NewHTTPRelayClient(relayServer.URL, "relay-service-token", relayServer.Client()))
	service.Now = func() time.Time { return now }

	result, err := service.RevokeDevice(ctx, boot.AdminUserID, platformid.New(platformid.Idempotency), exchange.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "COMPLETED" {
		t.Fatalf("unexpected revoke activation: %+v", result)
	}
	device, _ := st.Client.Device.Get(ctx, exchange.DeviceID)
	if device.Status != "REVOKED" {
		t.Fatalf("device not revoked: %s", device.Status)
	}
	sessionRow, err := st.Client.Session.Get(ctx, exchange.SessionID)
	if err != nil || sessionRow.Status != "REVOKED" {
		t.Fatalf("session not revoked: %+v %v", sessionRow, err)
	}
	if _, denied := relayStore.Current().RevokedDevices[exchange.DeviceID]; !denied {
		t.Fatal("Relay did not receive revoked device")
	}
}
