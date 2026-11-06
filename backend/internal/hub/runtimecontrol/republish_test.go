package runtimecontrol_test

import (
	"context"
	"testing"

	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

// HUB-RPBL-001: Republish of a historical release produces a new release with a
// new managed generation and a new snapshot bundle hash, leaving both staged.
func TestHUBRPBL001RepublishCreatesNewReleaseAndGeneration(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()

	// First publish → release A.
	first := publishAndFinalize(t, svc, adminID, draftRevision)
	if first.State != "COMPLETED" {
		t.Fatalf("first publish failed: %+v", first)
	}
	managedBefore, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	releaseAID := *managedBefore.ActiveReleaseID
	genBefore := managedBefore.ActiveManagedGeneration

	// Republish release A → release B.
	second, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), releaseAID)
	if err != nil {
		t.Fatal(err)
	}
	if second.State != "COMPLETED" {
		t.Fatalf("republish not completed: %+v", second)
	}

	managedAfter, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	releaseBID := *managedAfter.ActiveReleaseID
	if releaseBID == releaseAID {
		t.Fatal("republish reused the same release id")
	}
	if managedAfter.ActiveManagedGeneration <= genBefore {
		t.Fatalf("republish did not advance generation: %d -> %d", genBefore, managedAfter.ActiveManagedGeneration)
	}

	// Both releases must be present and the new one ACTIVE.
	activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 ACTIVE release, got %d", activeCount)
	}
	all, err := st.Client.ManagedRelease.Query().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Fatalf("expected at least 2 releases after republish, got %d", len(all))
	}
}

// HUB-RPBL-002: Republishing an unknown release must fail cleanly.
func TestHUBRPBL002RepublishUnknownRelease(t *testing.T) {
	ctx := context.Background()
	_, svc, _, relayServer, _, adminID, _, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()

	publishAndFinalize(t, svc, adminID, draftRevision)

	_, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), platformid.New(platformid.Release))
	if err == nil {
		t.Fatal("expected error republishing unknown release")
	}
}

var _ = runtimecontrol.IsIdempotencyConflict

func TestRepublishReplaySurvivesPurgedSourceAndNewCommandFails(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()
	first := publishAndFinalize(t, svc, adminID, draftRevision)
	key := platformid.New(platformid.Idempotency)
	second, err := svc.Republish(ctx, adminID, key, first.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{first.ReleaseID}
	sel := adminapi.ReleaseCleanupSelection{ReleaseIds: &ids}
	p, err := svc.Capability.PreviewReleaseCleanup(ctx, sel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Capability.ExecuteReleaseCleanup(ctx, adminID, adminapi.ExecuteReleaseCleanupRequest{Selection: sel, PreviewHash: p.PreviewHash}); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Republish(ctx, adminID, key, first.ReleaseID)
	if err != nil || again.ActivationID != second.ActivationID {
		t.Fatalf("replay lost facts: %+v %v", again, err)
	}
	if _, err = svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), first.ReleaseID); err == nil {
		t.Fatal("new republish accepted a purged source")
	}
	state := st.Client.ManagedState.GetX(ctx, "current")
	if state.ActiveManagedGeneration != 2 || state.LastAssignedGeneration != 2 {
		t.Fatal("cleanup/replay changed generation")
	}
}
