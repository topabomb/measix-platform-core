package runtimecontrol_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

type lostAckRelay struct {
	store    *control.Store
	loseNext bool
}

func (r *lostAckRelay) Apply(_ context.Context, state relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error) {
	ack, err := r.store.Apply(state)
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	if r.loseNext {
		r.loseNext = false
		return relaycontrolapi.ControlAck{}, context.DeadlineExceeded
	}
	return ack, nil
}

func (r *lostAckRelay) Status(context.Context) (relaycontrolapi.ControlStatus, error) {
	return r.store.Status(), nil
}

func TestI3ReconcileFinalizesPublishWhenRelayAppliedButAckWasLost(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x52}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	upstreamService := upstream.NewService(st.Client, box)
	upstreamService.Now = func() time.Time { return now }
	secret, err := upstreamService.CreateSecret(ctx, boot.AdminUserID, "runtime-token", "reconcile-secret")
	if err != nil {
		t.Fatal(err)
	}
	upstreamView, err := upstreamService.CreateUpstream(ctx, boot.AdminUserID, publishUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Client.Upstream.UpdateOneID(upstreamView.UpstreamID).SetActiveConfigRevision(1).SetStatus("ACTIVE").SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	capabilityService := capability.NewService(st.Client)
	capabilityService.Now = func() time.Time { return now }
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := capabilityService.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, publishDraft(upstreamView.UpstreamID))
	if err != nil {
		t.Fatal(err)
	}

	relayStore := control.NewStore(func() time.Time { return now })
	relay := &lostAckRelay{store: relayStore, loseNext: true}
	service := runtimecontrol.NewService(st.Client, capabilityService, upstreamService, identityService.Signer, relay)
	service.Now = func() time.Time { return now }
	published, err := service.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID: boot.AdminUserID, IdempotencyKey: platformid.New(platformid.Idempotency), ExpectedDraftRevision: updated.DraftRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "UNKNOWN" {
		t.Fatalf("lost ACK state=%s want UNKNOWN", published.State)
	}
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "DEGRADED" || managed.ActiveManagedGeneration != 0 || !relayStore.Status().Ready {
		t.Fatalf("unexpected ambiguous state hub=%+v relay=%+v", managed, relayStore.Status())
	}

	reconciled, err := service.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled == nil || reconciled.ActivationID != published.ActivationID || reconciled.State != "COMPLETED" {
		t.Fatalf("reconcile result=%+v", reconciled)
	}
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 || managed.ActiveReleaseID == nil {
		t.Fatalf("reconcile did not finalize Hub state: %+v", managed)
	}
}

type divergentRelay struct {
	status relaycontrolapi.ControlStatus
}

func (r divergentRelay) Apply(context.Context, relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error) {
	return relaycontrolapi.ControlAck{}, errors.New("must not overwrite unexpected newer relay state")
}

func (r divergentRelay) Status(context.Context) (relaycontrolapi.ControlStatus, error) {
	return r.status, nil
}

func TestI3ReconcileDoesNotBlindlyOverwriteUnexpectedNewerRelay(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 8, 30, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	if _, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	capabilityService := capability.NewService(st.Client)
	upstreamService := upstream.NewService(st.Client, nil)
	relay := divergentRelay{status: relaycontrolapi.ControlStatus{
		Ready: true, AppliedControlRevision: 9, ActiveManagedGeneration: 4,
		BundleHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", StartedAt: now,
	}}
	service := runtimecontrol.NewService(st.Client, capabilityService, upstreamService, identityService.Signer, relay)
	service.Now = func() time.Time { return now }

	_, err := service.Reconcile(ctx)
	if !runtimecontrol.IsRelayDiverged(err) {
		t.Fatalf("unexpected newer Relay reconcile err=%v", err)
	}
	managed, readErr := st.Client.ManagedState.Get(ctx, "current")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if managed.RuntimeStatus != "DEGRADED" || managed.DesiredControlRevision != 0 || managed.ActiveManagedGeneration != 0 {
		t.Fatalf("Hub authority was overwritten by Relay: %+v", managed)
	}
}
func TestLogoutConvergesAndRelayRestartUsesPersistedDescriptor(t *testing.T) {
	ctx := context.Background()
	st, svc, _, server, _, adminID, _, draftRevision := newRuntimeControlEnv(t)
	defer server.Close()
	published := publishAndFinalize(t, svc, adminID, draftRevision)
	// The durable session deny is also used after lost logout responses/re-enrollment.
	sessionID := platformid.New(platformid.Session)
	now := svc.Now().UTC()
	_, err := st.Client.Session.Create().SetID(sessionID).SetUserID(adminID).SetChannel("ANDROID").SetStatus("REVOKED").SetExpiresAt(now.Add(time.Hour)).SetCreatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := svc.Reconcile(ctx)
	if err != nil || reconciled == nil || reconciled.Kind != "SECURITY_CHANGE" || reconciled.DesiredControlRevision <= published.DesiredControlRevision {
		t.Fatalf("logout not converged: %+v %v", reconciled, err)
	}
	// Restart must rehydrate the exact committed descriptor, not today's mutable facts.
	_, err = st.Client.Session.Create().SetID(platformid.New(platformid.Session)).SetUserID(adminID).SetChannel("ADMIN_WEB").SetStatus("REVOKED").SetExpiresAt(now.Add(time.Hour)).SetCreatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	relay := &lostAckRelay{store: control.NewStore(time.Now)}
	svc.Relay = relay
	if _, err = svc.Reconcile(ctx); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if _, denied := relay.store.Current().RevokedSessions[sessionID]; !denied {
		t.Fatal("recovered bundle lost session deny")
	}
}
