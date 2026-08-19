package runtimecontrol_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/capability"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/runtimecontrol"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/upstream"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
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
