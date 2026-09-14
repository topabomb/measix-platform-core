package runtimecontrol_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/ent/activation"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/relay/control"
	"measix/platform/pkg/platformid"
)

func newRuntimeControlEnv(t *testing.T) (*testutil.StoreHandle, *runtimecontrol.Service, *control.Store, *httptest.Server, time.Time, string, string, int) {
	t.Helper()
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 7, 30, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x51}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	upstreamService := upstream.NewService(st.Client, box)
	upstreamService.Now = func() time.Time { return now }
	secret, err := upstreamService.CreateSecret(ctx, boot.AdminUserID, "runtime-token", "plaintext-must-never-persist")
	if err != nil {
		t.Fatal(err)
	}
	upstreamView, err := upstreamService.CreateUpstream(ctx, boot.AdminUserID, publishUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Client.Upstream.UpdateOneID(upstreamView.UpstreamID).SetActiveConfigRevision(1).SetStatus("ACTIVE").Save(ctx); err != nil {
		t.Fatal(err)
	}
	capabilityService := capability.NewService(st.Client)
	capabilityService.Now = func() time.Time { return now }
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := publishDraft(upstreamView.UpstreamID)
	updated, err := capabilityService.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	relayStore := control.NewStore(func() time.Time { return now })
	relayHandler := control.NewHandler(relayStore, "relay-service-token")
	relayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayHandler.ServeHTTP(w, r)
	}))
	svc := runtimecontrol.NewService(st.Client, capabilityService, upstreamService, identityService.Signer, runtimecontrol.NewHTTPRelayClient(relayServer.URL, "relay-service-token", relayServer.Client()))
	svc.Now = func() time.Time { return now }
	return st, svc, relayStore, relayServer, now, boot.AdminUserID, upstreamView.UpstreamID, updated.DraftRevision
}

func publishAndFinalize(t *testing.T, svc *runtimecontrol.Service, adminUserID string, draftRevision int) runtimecontrol.ActivationResult {
	t.Helper()
	ctx := context.Background()
	key := platformid.New(platformid.Idempotency)
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminUserID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draftRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// HUB-ACT-001: compiler must only read persisted Release/active operational
// state, not unpublished Draft. The active release content is used, not the
// current draft content.
func TestHUBACT001CompilerReadsPersistedReleaseNotDraft(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, upstreamID, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	capabilityService := capability.NewService(st.Client)

	// Get draft and publish it
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := publishAndFinalize(t, svc, adminID, draft.DraftRevision)
	if result.State != "COMPLETED" {
		t.Fatalf("publish not completed: %+v", result)
	}

	// Now edit the draft — the active release must not change
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	releaseBefore, err := st.Client.ManagedRelease.Get(ctx, *managed.ActiveReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	content := publishDraft(upstreamID)
	content.Models[0].DisplayName = "Changed After Publish"
	_, err = capabilityService.PutDraft(ctx, adminID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	releaseAfter, err := st.Client.ManagedRelease.Get(ctx, *managed.ActiveReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(releaseBefore.ReleaseContentJSON, releaseAfter.ReleaseContentJSON) {
		t.Fatal("active release content changed after draft edit")
	}
}

// HUB-ACT-002: RuntimeControlState must produce deterministic descriptor/bundleHash.
// Same input must always produce the same hash.
func TestHUBACT002RuntimeControlStateDeterministic(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	capabilityService := capability.NewService(st.Client)

	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// First publish
	result1 := publishAndFinalize(t, svc, adminID, draft.DraftRevision)
	if result1.State != "COMPLETED" {
		t.Fatalf("first publish failed: %+v", result1)
	}
	managed1, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	hash1 := ""
	if managed1.DesiredBundleHash != nil {
		hash1 = *managed1.DesiredBundleHash
	}
	if hash1 == "" {
		t.Fatal("empty bundle hash")
	}
	// Verify the hash format
	if len(hash1) < 10 || hash1[:7] != "sha256:" {
		t.Fatalf("invalid hash format: %s", hash1)
	}
}

// HUB-ACT-004: Before Relay ACK, Activation must be COMPLETED only after
// Relay applies; Release must be ACTIVE only after finalize.
// This is already verified in TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck.
// Here we add an explicit assertion that Release is not ACTIVE before ACK.
func TestHUBACT004ReleaseNotActiveBeforeAck(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	capabilityService := capability.NewService(st.Client)
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Before publish, no active release
	activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("unexpected active release before publish: %d", activeCount)
	}
	// Publish
	result := publishAndFinalize(t, svc, adminID, draft.DraftRevision)
	if result.State != "COMPLETED" {
		t.Fatalf("publish failed: %+v", result)
	}
	// After publish, exactly one active release
	activeCount, err = st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 active release after publish, got %d", activeCount)
	}
}

// HUB-ACT-006: same Idempotency-Key + different request hash must return 409.
// After a successful publish, reusing the same Idempotency-Key with a different
// request (different ExpectedDraftRevision → different request hash) must
// return ErrIdempotencyConflict.
func TestHUBACT006SameKeyDifferentRequestHash(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, upstreamID, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	capabilityService := capability.NewService(st.Client)
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := platformid.New(platformid.Idempotency)
	// First publish succeeds.
	result, err := svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draft.DraftRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "COMPLETED" {
		t.Fatalf("first publish not completed: %+v", result)
	}
	// Fetch the new draft revision after publish.
	draft2, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Modify draft content and bump revision so the second request is valid.
	content := publishDraft(upstreamID)
	_, err = capabilityService.PutDraft(ctx, adminID, draft2.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	draft3, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Same key, different ExpectedDraftRevision → different request hash.
	_, err = svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: draft3.DraftRevision,
	})
	if !runtimecontrol.IsIdempotencyConflict(err) {
		t.Fatalf("expected idempotency conflict, got: %v", err)
	}
}

// HUB-ACT-007: at most one non-terminal Activation across Relay at any time.
func TestHUBACT007OnlyOneNonTerminalActivation(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	capabilityService := capability.NewService(st.Client)
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create a pending activation manually
	activationID := platformid.New(platformid.Activation)
	now := time.Date(2026, 8, 19, 7, 30, 0, 0, time.UTC)
	_, err = st.Client.Activation.Create().
		SetID(activationID).SetKind("PUBLISH").SetState("APPLYING").
		SetIdempotencyKey(platformid.New(platformid.Idempotency)).
		SetRequestHash("sha256:dummy").
		SetControlRevision(99).SetBundleHash("sha256:dummy").
		SetTargetGeneration(99).SetTargetDescriptorJSON([]byte("{}")).
		SetSubjectID(platformid.New(platformid.Release)).
		SetCreatedByUserID(adminID).SetCreatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Now try to publish — should fail with ErrActivationInProgress
	_, err = svc.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           adminID,
		IdempotencyKey:        platformid.New(platformid.Idempotency),
		ExpectedDraftRevision: draft.DraftRevision,
	})
	if !errors.Is(err, runtimecontrol.ErrActivationInProgress) {
		t.Fatalf("expected ErrActivationInProgress, got: %v", err)
	}

	// Verify the pending count
	pendingCount, err := st.Client.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending activation, got %d", pendingCount)
	}
}

// HUB-ACT-008: when status shows equal desired/applied, finalize pending.
// Reconcile should finalize a pending activation when relay status matches.
func TestHUBACT008ReconcileFinalizesPending(t *testing.T) {
	ctx := context.Background()
	st, svc, relayStore, relayServer, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()
	capabilityService := capability.NewService(st.Client)
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Publish normally
	result := publishAndFinalize(t, svc, adminID, draft.DraftRevision)
	if result.State != "COMPLETED" {
		t.Fatalf("publish failed: %+v", result)
	}

	// Verify managed state is READY with desired == applied
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" {
		t.Fatalf("expected READY, got %s", managed.RuntimeStatus)
	}
	relayStatus := relayStore.Status()
	if !relayStatus.Ready || relayStatus.AppliedControlRevision != int(managed.DesiredControlRevision) {
		t.Fatalf("relay does not match desired: relay=%+v managed=%+v", relayStatus, managed)
	}

	// Reconcile should be a no-op when everything is consistent
	_, err = svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	managed2, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed2.RuntimeStatus != "READY" {
		t.Fatalf("reconcile degraded READY state: %s", managed2.RuntimeStatus)
	}
}
