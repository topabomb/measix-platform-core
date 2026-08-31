package runtimecontrol_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/relay/control"
	"measix/platform/pkg/platformid"
)

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
	relayServer := httptest.NewServer(control.NewHandler(relayStore, "relay-service-token"))
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
	relayServer := httptest.NewServer(control.NewHandler(relayStore, "relay-service-token"))
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
