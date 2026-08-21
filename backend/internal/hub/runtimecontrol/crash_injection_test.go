package runtimecontrol_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/ent/activation"
	"measix/platform/ent/idempotencyrecord"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// crashPointRelay is a test relay that can simulate Hub crashes at
// deterministic points A through E in the publish pipeline.
//
// A: intent durable commit before → no state persisted
// B: intent committed, Relay apply before → activation APPLYING persisted
// C: Relay applied, ACK before → Relay has new state, Hub hasn't confirmed
// D: ACK received, finalize commit before → activation still APPLYING
// E: finalize committed, response before → everything complete
type crashPointRelay struct {
	store      *control.Store
	loseNext   bool
	failApply  bool
	applyCalls atomic.Int32
}

func (r *crashPointRelay) Apply(ctx context.Context, state relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error) {
	r.applyCalls.Add(1)
	if r.failApply {
		return relaycontrolapi.ControlAck{}, errors.New("simulated relay apply failure (crash point B)")
	}
	ack, err := r.store.Apply(state)
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	if r.loseNext {
		r.loseNext = false
		// Simulate crash point C: Relay applied state but Hub lost the ACK
		// (network timeout). The state IS applied on the Relay side.
		return relaycontrolapi.ControlAck{}, context.DeadlineExceeded
	}
	return ack, nil
}

func (r *crashPointRelay) Status(context.Context) (relaycontrolapi.ControlStatus, error) {
	return r.store.Status(), nil
}

// setupCrashTestEnv creates a full environment for crash injection tests.
func setupCrashTestEnv(t *testing.T) (ctx context.Context, st *testutil.StoreHandle, svc *runtimecontrol.Service, relay *crashPointRelay, adminID string, draftRevision int) {
	t.Helper()
	ctx = context.Background()
	st = testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x43}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	upstreamService := upstream.NewService(st.Client, box)
	upstreamService.Now = func() time.Time { return now }
	secret, err := upstreamService.CreateSecret(ctx, boot.AdminUserID, "runtime-token", "crash-test-secret")
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
	relay = &crashPointRelay{store: relayStore}
	svc = runtimecontrol.NewService(st.Client, capabilityService, upstreamService, identityService.Signer, relay)
	svc.Now = func() time.Time { return now }
	return ctx, st, svc, relay, boot.AdminUserID, updated.DraftRevision
}

// ErrSimulatedCrashA is returned by the BeforeIntentCommit barrier hook
// to simulate a Hub crash before the intent transaction commits.
var ErrSimulatedCrashA = errors.New("simulated hub crash: before intent commit (point A)")

// ErrSimulatedCrashB is returned by the AfterIntentCommit barrier hook
// to simulate a Hub crash after intent commit but before Relay.Apply.
var ErrSimulatedCrashB = errors.New("simulated hub crash: after intent commit (point B)")

// ErrSimulatedCrashC is returned by the AfterRelayApplied barrier hook
// to simulate a Hub crash after Relay applied state (ACK lost).
var ErrSimulatedCrashC = errors.New("simulated hub crash: after relay applied (point C)")

// ErrSimulatedCrashD is returned by the AfterAck barrier hook
// to simulate a Hub crash after ACK validation but before finalizePublish.
var ErrSimulatedCrashD = errors.New("simulated hub crash: after ack (point D)")

// ErrSimulatedCrashE is returned by the AfterFinalize barrier hook
// to simulate a Hub crash after finalize committed (response lost).
var ErrSimulatedCrashE = errors.New("simulated hub crash: after finalize (point E)")

// ============================================================================
// Crash Point A: Hub crashes before intent durable commit.
// ============================================================================

// TestCrashPointA_BeforeIntentCommit verifies that when the Hub crashes
// before persistPublishIntent commits, no activation, release, or managed
// state change is persisted. The intent transaction is atomic: either all
// of (activation + release + idempotency record + managed state update)
// commit, or none do.
//
// With the barrier hook, BeforeIntentCommit fires before the transaction
// begins. The Publish call returns an error, and the DB must be clean.
func TestCrashPointA_BeforeIntentCommit(t *testing.T) {
	ctx, st, svc, _, adminID, draftRev := setupCrashTestEnv(t)
	defer st.Close()

	// Install barrier hook at point A
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		BeforeIntentCommit: func(context.Context) error {
			return ErrSimulatedCrashA
		},
	})

	key := platformid.New(platformid.Idempotency)
	_, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if !errors.Is(err, ErrSimulatedCrashA) {
		t.Fatalf("expected ErrSimulatedCrashA, got: %v", err)
	}

	// Verify NO activation was persisted (intent transaction never committed)
	activationCount, err := st.Client.Activation.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activationCount != 0 {
		t.Fatalf("crash point A: %d activations persisted, want 0", activationCount)
	}

	// Verify NO release was persisted
	releaseCount, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if releaseCount != 0 {
		t.Fatalf("crash point A: %d releases persisted, want 0", releaseCount)
	}

	// Verify NO idempotency record was persisted
	idempotencyCount, err := st.Client.IdempotencyRecord.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if idempotencyCount != 0 {
		t.Fatalf("crash point A: %d idempotency records, want 0", idempotencyCount)
	}

	// Verify NO STAGED releases (intent never committed)
	stagedCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("STAGED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stagedCount != 0 {
		t.Fatalf("crash point A: %d STAGED releases, want 0", stagedCount)
	}

	// Verify managed state is unchanged (no ACTIVATING, no desired revision bump)
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.DesiredControlRevision != 0 {
		t.Fatalf("crash point A: desired revision=%d, want 0", managed.DesiredControlRevision)
	}
	if managed.RuntimeStatus != "READY" {
		t.Fatalf("crash point A: runtime status=%s, want READY", managed.RuntimeStatus)
	}

	// Now remove the barrier hook and retry with the same idempotency key.
	// It should succeed because no idempotency record was persisted.
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("retry after crash point A: %v", err)
	}
	if result.State != "COMPLETED" {
		t.Fatalf("retry result state=%s, want COMPLETED", result.State)
	}

	// Verify state converged
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 {
		t.Fatalf("post-recovery state: %+v", managed)
	}
}

// ============================================================================
// Crash Point B: Hub crashes after intent commit, before Relay apply.
// ============================================================================

// TestCrashPointB_AfterIntentBeforeRelayApply verifies that when the Hub
// crashes after persistPublishIntent commits but before Relay.Apply is
// called, the activation is persisted as UNKNOWN and the managed state is
// DEGRADED. On reconciliation, the pending activation must be detected and
// re-applied to the Relay.
func TestCrashPointB_AfterIntentBeforeRelayApply(t *testing.T) {
	ctx, st, svc, relay, adminID, draftRev := setupCrashTestEnv(t)
	defer st.Close()

	// Install barrier hook at point B
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterIntentCommit: func(context.Context) error {
			return ErrSimulatedCrashB
		},
	})

	key := platformid.New(platformid.Idempotency)
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("publish should return activation on crash B, got: %v", err)
	}
	if result.State != "UNKNOWN" {
		t.Fatalf("crash point B: state=%s, want UNKNOWN", result.State)
	}

	// Verify intent WAS persisted (activation exists in UNKNOWN state)
	actCount, err := st.Client.Activation.Query().Where(activation.StateEQ("UNKNOWN")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if actCount != 1 {
		t.Fatalf("expected 1 UNKNOWN activation, got %d", actCount)
	}

	// Verify activation has correct target generation and control revision
	actRows, err := st.Client.Activation.Query().Where(activation.StateEQ("UNKNOWN")).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if actRows[0].TargetGeneration == nil || *actRows[0].TargetGeneration != 1 {
		t.Fatalf("crash B: target generation=%v, want 1", actRows[0].TargetGeneration)
	}
	if actRows[0].ControlRevision != 1 {
		t.Fatalf("crash B: control revision=%d, want 1", actRows[0].ControlRevision)
	}

	// Verify release was persisted as STAGED (not ACTIVE)
	stagedCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("STAGED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stagedCount != 1 {
		t.Fatalf("expected 1 STAGED release, got %d", stagedCount)
	}

	// Verify idempotency record was persisted
	idemCount, err := st.Client.IdempotencyRecord.Query().Where(
		idempotencyrecord.IdempotencyKeyEQ(key),
	).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if idemCount != 1 {
		t.Fatalf("crash B: expected 1 idempotency record, got %d", idemCount)
	}

	// Verify Relay.Apply was NOT called (crash happened before apply)
	if relay.applyCalls.Load() != 0 {
		t.Fatalf("crash B: relay apply calls=%d, want 0", relay.applyCalls.Load())
	}

	// Verify managed state is DEGRADED (intent committed but not applied)
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "DEGRADED" {
		t.Fatalf("expected DEGRADED, got %s", managed.RuntimeStatus)
	}
	if managed.DesiredControlRevision != 1 {
		t.Fatalf("expected desired revision 1, got %d", managed.DesiredControlRevision)
	}

	// Verify Relay has NOT received the state yet
	relayStatus := relay.store.Status()
	if relayStatus.Ready {
		t.Fatal("relay should not have state applied at crash point B")
	}

	// Remove barrier hook and reconcile
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	reconciled, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile after crash B: %v", err)
	}
	if reconciled == nil || reconciled.State != "COMPLETED" {
		t.Fatalf("reconcile result: %+v", reconciled)
	}

	// Verify final state converged
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 {
		t.Fatalf("post-reconcile: %+v", managed)
	}

	// Verify Relay.Apply was called exactly once during reconcile (B needs reapply)
	if relay.applyCalls.Load() != 1 {
		t.Fatalf("crash B post-reconcile: relay apply calls=%d, want 1", relay.applyCalls.Load())
	}

	// Verify Relay now has the correct state
	relayStatus = relay.store.Status()
	if !relayStatus.Ready || relayStatus.AppliedControlRevision != 1 {
		t.Fatalf("relay not converged after reconcile: %+v", relayStatus)
	}

	// Verify activation is now COMPLETED
	completedCount, err := st.Client.Activation.Query().Where(activation.StateEQ("COMPLETED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if completedCount != 1 {
		t.Fatalf("crash B post-reconcile: expected 1 COMPLETED activation, got %d", completedCount)
	}

	// Verify release is now ACTIVE
	activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("crash B post-reconcile: expected 1 ACTIVE release, got %d", activeCount)
	}

	// Verify no duplicate activations or releases
	totalAct, err := st.Client.Activation.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totalAct != 1 {
		t.Fatalf("crash B: total activations=%d, want 1", totalAct)
	}
	totalRel, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totalRel != 1 {
		t.Fatalf("crash B: total releases=%d, want 1", totalRel)
	}
}

// ============================================================================
// Crash Point C: Relay applied state but Hub lost the ACK.
// ============================================================================

// TestCrashPointC_RelayAppliedAckLost verifies that when the Relay applies
// the state but the Hub loses the ACK (simulated via AfterRelayApplied hook),
// the activation goes UNKNOWN while the Relay has the correct state. On
// reconciliation, the Hub detects the Relay already has the desired state
// and finalizes without re-applying.
func TestCrashPointC_RelayAppliedAckLost(t *testing.T) {
	ctx, st, svc, relay, adminID, draftRev := setupCrashTestEnv(t)
	defer st.Close()

	// Install barrier hook at point C: after Relay.Apply succeeds,
	// simulate Hub losing the ACK.
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterRelayApplied: func(context.Context) error {
			return ErrSimulatedCrashC
		},
	})

	key := platformid.New(platformid.Idempotency)
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("publish error: %v", err)
	}
	if result.State != "UNKNOWN" {
		t.Fatalf("crash point C: state=%s, want UNKNOWN", result.State)
	}

	// Verify Relay actually has the new state applied
	relayStatus := relay.store.Status()
	if !relayStatus.Ready {
		t.Fatal("relay should have state applied despite lost ACK")
	}
	if relayStatus.AppliedControlRevision != 1 {
		t.Fatalf("relay applied revision: %d, want 1", relayStatus.AppliedControlRevision)
	}

	// Verify Relay.Apply was called exactly once
	if relay.applyCalls.Load() != 1 {
		t.Fatalf("crash C: relay apply calls=%d, want 1", relay.applyCalls.Load())
	}

	// Verify release was persisted as STAGED (finalize not committed)
	stagedCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("STAGED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stagedCount != 1 {
		t.Fatalf("crash C: expected 1 STAGED release, got %d", stagedCount)
	}

	// Verify idempotency record was persisted
	idemCount, err := st.Client.IdempotencyRecord.Query().Where(
		idempotencyrecord.IdempotencyKeyEQ(key),
	).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if idemCount != 1 {
		t.Fatalf("crash C: expected 1 idempotency record, got %d", idemCount)
	}

	// Hub state should be DEGRADED (ack was lost)
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "DEGRADED" {
		t.Fatalf("expected DEGRADED, got %s", managed.RuntimeStatus)
	}

	// Activation should be UNKNOWN
	actCount, err := st.Client.Activation.Query().Where(activation.StateEQ("UNKNOWN")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if actCount != 1 {
		t.Fatalf("expected 1 UNKNOWN activation, got %d", actCount)
	}

	// Reconcile should detect Relay has desired state and finalize
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	reconciled, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled == nil || reconciled.State != "COMPLETED" {
		t.Fatalf("reconcile result: %+v", reconciled)
	}

	// Verify Relay.Apply was NOT called again during reconcile (C: Relay already has state)
	if relay.applyCalls.Load() != 1 {
		t.Fatalf("crash C post-reconcile: relay apply calls=%d, want 1 (no re-apply)", relay.applyCalls.Load())
	}

	// Verify convergence
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 {
		t.Fatalf("post-reconcile: %+v", managed)
	}

	// Verify activation is now COMPLETED
	completedCount, err := st.Client.Activation.Query().Where(activation.StateEQ("COMPLETED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if completedCount != 1 {
		t.Fatalf("crash C post-reconcile: expected 1 COMPLETED activation, got %d", completedCount)
	}

	// Verify release is now ACTIVE
	activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("crash C post-reconcile: expected 1 ACTIVE release, got %d", activeCount)
	}

	// Verify no duplicate activations or releases
	totalAct, err := st.Client.Activation.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totalAct != 1 {
		t.Fatalf("crash C: total activations=%d, want 1", totalAct)
	}
	totalRel, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totalRel != 1 {
		t.Fatalf("crash C: total releases=%d, want 1", totalRel)
	}
}

// ============================================================================
// Crash Point D: ACK received but finalize commit hasn't happened.
// ============================================================================

// TestCrashPointD_AckReceivedBeforeFinalize verifies that when the Hub
// receives and validates the ACK but crashes before finalizePublish commits,
// the activation is left as UNKNOWN. The Relay has the correct state, and
// on reconciliation, the Hub must detect this and finalize the activation.
func TestCrashPointD_AckReceivedBeforeFinalize(t *testing.T) {
	ctx, st, svc, relay, adminID, draftRev := setupCrashTestEnv(t)
	defer st.Close()

	// Install barrier hook at point D: after ACK is validated,
	// before finalizePublish.
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterAck: func(context.Context) error {
			return ErrSimulatedCrashD
		},
	})

	key := platformid.New(platformid.Idempotency)
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("publish error: %v", err)
	}
	if result.State != "UNKNOWN" {
		t.Fatalf("crash point D: state=%s, want UNKNOWN", result.State)
	}

	// Verify Relay has the correct state applied
	relayStatus := relay.store.Status()
	if !relayStatus.Ready {
		t.Fatal("relay should have state applied at crash point D")
	}
	if relayStatus.AppliedControlRevision != 1 {
		t.Fatalf("relay applied revision: %d, want 1", relayStatus.AppliedControlRevision)
	}

	// Verify Relay.Apply was called exactly once
	if relay.applyCalls.Load() != 1 {
		t.Fatalf("crash D: relay apply calls=%d, want 1", relay.applyCalls.Load())
	}

	// Verify release was persisted as STAGED (finalize not committed)
	stagedCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("STAGED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stagedCount != 1 {
		t.Fatalf("crash D: expected 1 STAGED release, got %d", stagedCount)
	}

	// Verify idempotency record was persisted
	idemCount, err := st.Client.IdempotencyRecord.Query().Where(
		idempotencyrecord.IdempotencyKeyEQ(key),
	).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if idemCount != 1 {
		t.Fatalf("crash D: expected 1 idempotency record, got %d", idemCount)
	}

	// Hub state should be DEGRADED (finalize not committed)
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "DEGRADED" {
		t.Fatalf("expected DEGRADED, got %s", managed.RuntimeStatus)
	}

	// Activation should be UNKNOWN (not COMPLETED, because finalize didn't commit)
	actCount, err := st.Client.Activation.Query().Where(activation.StateEQ("UNKNOWN")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if actCount != 1 {
		t.Fatalf("expected 1 UNKNOWN activation, got %d", actCount)
	}

	// No ACTIVE release yet (finalize didn't commit)
	activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("expected 0 ACTIVE releases, got %d", activeCount)
	}

	// Reconcile should finalize
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	reconciled, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled == nil || reconciled.State != "COMPLETED" {
		t.Fatalf("reconcile result: %+v", reconciled)
	}

	// Verify final state converged
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 {
		t.Fatalf("post-reconcile: %+v", managed)
	}

	// Verify release is now ACTIVE
	activeCount, err = st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 ACTIVE release after reconcile, got %d", activeCount)
	}

	// Verify Relay.Apply was NOT called again during reconcile (D: Relay already has state)
	if relay.applyCalls.Load() != 1 {
		t.Fatalf("crash D post-reconcile: relay apply calls=%d, want 1 (no re-apply)", relay.applyCalls.Load())
	}

	// Verify activation is now COMPLETED
	completedCount, err := st.Client.Activation.Query().Where(activation.StateEQ("COMPLETED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if completedCount != 1 {
		t.Fatalf("crash D post-reconcile: expected 1 COMPLETED activation, got %d", completedCount)
	}

	// Verify no duplicate activations or releases
	totalAct, err := st.Client.Activation.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totalAct != 1 {
		t.Fatalf("crash D: total activations=%d, want 1", totalAct)
	}
	totalRel, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totalRel != 1 {
		t.Fatalf("crash D: total releases=%d, want 1", totalRel)
	}
}

// ============================================================================
// Crash Point E: finalize committed but response not sent to Admin.
// ============================================================================

// TestCrashPointE_FinalizedResponseLost verifies that when the Hub
// finalizes the publish (everything committed) but the response is lost
// before reaching the Admin, a retry with the same idempotency key returns
// the same result without creating duplicates.
func TestCrashPointE_FinalizedResponseLost(t *testing.T) {
	ctx, st, svc, _, adminID, draftRev := setupCrashTestEnv(t)
	defer st.Close()

	// Install barrier hook at point E: after finalizePublish commits.
	// The hook returns an error to simulate "response lost", but the
	// Publish method still returns the COMPLETED activation.
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterFinalize: func(context.Context) error {
			return ErrSimulatedCrashE
		},
	})

	key := platformid.New(platformid.Idempotency)
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if result.State != "COMPLETED" {
		t.Fatalf("first publish state: %s, want COMPLETED", result.State)
	}

	// Verify everything is committed
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 {
		t.Fatalf("post-finalize state: %+v", managed)
	}

	// Verify exactly 1 ACTIVE release
	activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 ACTIVE release, got %d", activeCount)
	}

	// Verify activation is COMPLETED
	completedCount, err := st.Client.Activation.Query().Where(activation.StateEQ("COMPLETED")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if completedCount != 1 {
		t.Fatalf("crash E: expected 1 COMPLETED activation, got %d", completedCount)
	}

	// Verify idempotency record was persisted
	idemCount, err := st.Client.IdempotencyRecord.Query().Where(
		idempotencyrecord.IdempotencyKeyEQ(key),
	).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if idemCount != 1 {
		t.Fatalf("crash E: expected 1 idempotency record, got %d", idemCount)
	}

	// Simulate Admin retry with same idempotency key
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	replay, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("idempotent replay error: %v", err)
	}
	if replay.ActivationID != result.ActivationID {
		t.Fatalf("idempotent replay created different activation: first=%s replay=%s",
			result.ActivationID, replay.ActivationID)
	}
	if replay.State != "COMPLETED" {
		t.Fatalf("replay state: %s, want COMPLETED", replay.State)
	}

	// Verify no duplicate releases
	releaseCount, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if releaseCount != 1 {
		t.Fatalf("idempotent replay created duplicate release: count=%d", releaseCount)
	}
}

// ============================================================================
// Comprehensive: all crash points with no duplicate generations.
// ============================================================================

// TestCrashInjectionAllPointsNoDuplicateGenerations verifies that across
// all crash points A through E, no duplicate generation is ever created.
// Each crash point is tested individually, followed by reconciliation and
// a subsequent publish, to ensure generation sequencing is correct.
func TestCrashInjectionAllPointsNoDuplicateGenerations(t *testing.T) {
	ctx, st, svc, relay, adminID, draftRev := setupCrashTestEnv(t)
	defer st.Close()

	// --- Crash Point A: before intent commit ---
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		BeforeIntentCommit: func(context.Context) error { return ErrSimulatedCrashA },
	})
	keyA := platformid.New(platformid.Idempotency)
	_, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyA,
		ExpectedDraftRevision: draftRev,
	})
	if !errors.Is(err, ErrSimulatedCrashA) {
		t.Fatalf("crash A: expected ErrSimulatedCrashA, got: %v", err)
	}
	// No activation persisted, so retry with same key should work
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	resultA, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyA,
		ExpectedDraftRevision: draftRev,
	})
	if err != nil {
		t.Fatalf("retry A: %v", err)
	}
	if resultA.State != "COMPLETED" {
		t.Fatalf("retry A state: %s", resultA.State)
	}
	// Verify generation 1
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.ActiveManagedGeneration != 1 {
		t.Fatalf("after A: expected generation 1, got %d", managed.ActiveManagedGeneration)
	}

	// --- Crash Point B: after intent, before relay ---
	// Need a new draft revision for a second publish
	capService := capability.NewService(st.Client)
	capService.Now = svc.Now
	draft2, err := capService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated2, err := capService.PutDraft(ctx, adminID, draft2.DraftRevision, publishDraft(getFirstUpstreamID(t, st)))
	if err != nil {
		t.Fatal(err)
	}

	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterIntentCommit: func(context.Context) error { return ErrSimulatedCrashB },
	})
	keyB := platformid.New(platformid.Idempotency)
	resultB, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyB,
		ExpectedDraftRevision: updated2.DraftRevision,
	})
	if err != nil {
		t.Fatalf("crash B publish: %v", err)
	}
	if resultB.State != "UNKNOWN" {
		t.Fatalf("crash B state: %s", resultB.State)
	}
	// Reconcile
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	reconciledB, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile B: %v", err)
	}
	if reconciledB == nil || reconciledB.State != "COMPLETED" {
		t.Fatalf("reconcile B result: %+v", reconciledB)
	}
	// Verify generation 2
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.ActiveManagedGeneration != 2 {
		t.Fatalf("after B: expected generation 2, got %d", managed.ActiveManagedGeneration)
	}

	// --- Crash Point C: Relay applied, ACK lost ---
	draftC, err := capService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updatedC, err := capService.PutDraft(ctx, adminID, draftC.DraftRevision, publishDraft(getFirstUpstreamID(t, st)))
	if err != nil {
		t.Fatal(err)
	}
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterRelayApplied: func(context.Context) error { return ErrSimulatedCrashC },
	})
	keyC := platformid.New(platformid.Idempotency)
	resultC, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyC,
		ExpectedDraftRevision: updatedC.DraftRevision,
	})
	if err != nil {
		t.Fatalf("crash C publish: %v", err)
	}
	if resultC.State != "UNKNOWN" {
		t.Fatalf("crash C state: %s, want UNKNOWN", resultC.State)
	}
	// Reconcile C — Relay already has state, no re-apply needed
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	reconciledC, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile C: %v", err)
	}
	if reconciledC == nil || reconciledC.State != "COMPLETED" {
		t.Fatalf("reconcile C result: %+v", reconciledC)
	}
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.ActiveManagedGeneration != 3 {
		t.Fatalf("after C: expected generation 3, got %d", managed.ActiveManagedGeneration)
	}

	// --- Crash Point D: ACK received, before finalize ---
	draftD, err := capService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updatedD, err := capService.PutDraft(ctx, adminID, draftD.DraftRevision, publishDraft(getFirstUpstreamID(t, st)))
	if err != nil {
		t.Fatal(err)
	}
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterAck: func(context.Context) error { return ErrSimulatedCrashD },
	})
	keyD := platformid.New(platformid.Idempotency)
	resultD, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyD,
		ExpectedDraftRevision: updatedD.DraftRevision,
	})
	if err != nil {
		t.Fatalf("crash D publish: %v", err)
	}
	if resultD.State != "UNKNOWN" {
		t.Fatalf("crash D state: %s, want UNKNOWN", resultD.State)
	}
	// Reconcile D — Relay already has state, no re-apply needed
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	reconciledD, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile D: %v", err)
	}
	if reconciledD == nil || reconciledD.State != "COMPLETED" {
		t.Fatalf("reconcile D result: %+v", reconciledD)
	}
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.ActiveManagedGeneration != 4 {
		t.Fatalf("after D: expected generation 4, got %d", managed.ActiveManagedGeneration)
	}

	// --- Crash Point E: finalize committed, response lost ---
	draft3, err := capService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated3, err := capService.PutDraft(ctx, adminID, draft3.DraftRevision, publishDraft(getFirstUpstreamID(t, st)))
	if err != nil {
		t.Fatal(err)
	}

	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{
		AfterFinalize: func(context.Context) error { return ErrSimulatedCrashE },
	})
	keyE := platformid.New(platformid.Idempotency)
	resultE, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyE,
		ExpectedDraftRevision: updated3.DraftRevision,
	})
	if err != nil {
		t.Fatalf("crash E publish: %v", err)
	}
	if resultE.State != "COMPLETED" {
		t.Fatalf("crash E state: %s", resultE.State)
	}
	// Verify generation 5
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.ActiveManagedGeneration != 5 {
		t.Fatalf("after E: expected generation 5, got %d", managed.ActiveManagedGeneration)
	}

	// Idempotent replay of E should not create a sixth generation
	svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
	replayE, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        keyE,
		ExpectedDraftRevision: updated3.DraftRevision,
	})
	if err != nil {
		t.Fatalf("replay E: %v", err)
	}
	if replayE.ActivationID != resultE.ActivationID {
		t.Fatalf("replay E created different activation")
	}
	managed, err = st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.ActiveManagedGeneration != 5 {
		t.Fatalf("after replay E: expected generation 5, got %d", managed.ActiveManagedGeneration)
	}

	// Verify exactly 5 releases total
	releaseCount, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if releaseCount != 5 {
		t.Fatalf("expected 5 releases, got %d", releaseCount)
	}

	// Verify relay is converged
	relayStatus := relay.store.Status()
	if !relayStatus.Ready || relayStatus.AppliedControlRevision != 5 {
		t.Fatalf("relay not converged: %+v", relayStatus)
	}
}

// getFirstUpstreamID retrieves the first upstream ID from the store for
// creating draft content in tests.
func getFirstUpstreamID(t *testing.T, st *testutil.StoreHandle) string {
	t.Helper()
	ctx := context.Background()
	upstreams, err := st.Client.Upstream.Query().All(ctx)
	if err != nil || len(upstreams) == 0 {
		t.Fatalf("no upstream found: %v", err)
	}
	return upstreams[0].ID
}

// Ensures the error from format is used to avoid unused import warnings
var _ = fmt.Sprintf
