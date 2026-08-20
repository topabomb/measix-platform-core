package runtimecontrol_test

import (
	"context"
	"errors"
	"testing"

	"measix/platform/ent/activation"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/pkg/platformid"
)

// HUB-APLY-001: ApplyUpstream produces a new active upstream revision, applies
// it through the Relay, and finalizes to COMPLETED with a new managed generation.
func TestHUBAPLY001ApplyUpstreamClosedLoop(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, upstreamID, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()

	// A release must exist for the operational state to carry a managed generation.
	publishResult := publishAndFinalize(t, svc, adminID, draftRevision)
	if publishResult.State != "COMPLETED" {
		t.Fatalf("setup publish failed: %+v", publishResult)
	}

	result, err := svc.ApplyUpstream(ctx, adminID, platformid.New(platformid.Idempotency), upstreamID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "COMPLETED" {
		t.Fatalf("apply upstream not completed: %+v", result)
	}

	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := st.Client.Upstream.Get(ctx, upstreamID)
	if err != nil {
		t.Fatal(err)
	}
	if upstream.ActiveConfigRevision == nil {
		t.Fatalf("upstream active revision not set after apply")
	}
	if *upstream.ActiveConfigRevision != upstream.ConfigRevision {
		t.Fatalf("active revision %d does not match config revision %d", *upstream.ActiveConfigRevision, upstream.ConfigRevision)
	}
	if upstream.Status != "ACTIVE" {
		t.Fatalf("expected upstream ACTIVE, got %s", upstream.Status)
	}
	if managed.RuntimeStatus != "READY" {
		t.Fatalf("expected READY after apply, got %s", managed.RuntimeStatus)
	}
	if managed.ActiveManagedGeneration < 1 {
		t.Fatalf("managed generation not set after apply: %d", managed.ActiveManagedGeneration)
	}
}

// HUB-APLY-002: ApplyUpstream is idempotent — same key + same request hash
// returns the same result without creating a second activation.
func TestHUBAPLY002ApplyUpstreamIdempotent(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, upstreamID, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()

	publishAndFinalize(t, svc, adminID, draftRevision)

	key := platformid.New(platformid.Idempotency)
	first, err := svc.ApplyUpstream(ctx, adminID, key, upstreamID)
	if err != nil {
		t.Fatal(err)
	}
	// Re-issue the identical request with the same key → replay the stored result.
	second, err := svc.ApplyUpstream(ctx, adminID, key, upstreamID)
	if err != nil {
		t.Fatal(err)
	}
	if second.State != first.State {
		t.Fatalf("idempotent replay returned different state: %v vs %v", second.State, first.State)
	}
	// The publish activation and the apply activation are distinct; the
	// idempotent re-issue must not create a second RUNTIME_CONFIG activation.
	count, err := st.Client.Activation.Query().Where(activation.KindEQ("RUNTIME_CONFIG")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 RUNTIME_CONFIG activation after idempotent apply, got %d", count)
	}
}

// HUB-APLY-003: ApplyUpstream with an unknown upstream must fail cleanly.
func TestHUBAPLY003ApplyUpstreamUnknownUpstream(t *testing.T) {
	ctx := context.Background()
	_, svc, _, relayServer, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer relayServer.Close()

	_, err := svc.ApplyUpstream(ctx, adminID, platformid.New(platformid.Idempotency), platformid.New(platformid.Upstream))
	if err == nil {
		t.Fatal("expected error for unknown upstream")
	}
	if errors.Is(err, runtimecontrol.ErrActivationInProgress) {
		t.Fatalf("unexpected activation-in-progress: %v", err)
	}
}
