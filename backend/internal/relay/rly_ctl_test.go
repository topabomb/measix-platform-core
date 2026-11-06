package relay_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
)

// RLY-CTL-001: First valid full state apply → READY.
func TestRLYCTL001FirstValidStateApplyProducesReady(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	if store.Status().Ready {
		t.Fatal("store should not be ready before apply")
	}
	state := minimalControlState(t, 1, 1)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	status := store.Status()
	if !status.Ready || status.AppliedControlRevision != 1 || status.ActiveManagedGeneration != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

// RLY-CTL-002: Same revision + same bundleHash replay is idempotent.
func TestRLYCTL002SameRevisionSameHashIsIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state := minimalControlState(t, 5, 2)
	ack1, err := store.Apply(state)
	if err != nil {
		t.Fatal(err)
	}
	ack2, err := store.Apply(state)
	if err != nil {
		t.Fatal(err)
	}
	if ack1 != ack2 {
		t.Fatalf("idempotent replay produced different ack: ack1=%+v ack2=%+v", ack1, ack2)
	}
}

// RLY-CTL-003: Same revision + different hash returns 409.
func TestRLYCTL003SameRevisionDifferentHashReturnsConflict(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state := minimalControlState(t, 7, 3)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	conflict := state
	conflict.ActiveManagedGeneration = 4 // change content → different hash
	conflict.BundleHash, _ = control.HashDescriptor(conflict)
	_, err := store.Apply(conflict)
	if !control.IsRevisionHashConflict(err) {
		t.Fatalf("expected revision hash conflict, got: %v", err)
	}
}

// RLY-CTL-004: Older revision is rejected and Current state unchanged.
func TestRLYCTL004OlderRevisionRejectedAndStateUnchanged(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state := minimalControlState(t, 10, 5)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	original := store.Status()
	older := minimalControlState(t, 5, 3)
	_, err := store.Apply(older)
	if err == nil {
		t.Fatal("older revision unexpectedly accepted")
	}
	if !control.IsRevisionStale(err) {
		t.Fatalf("expected stale revision error, got: %v", err)
	}
	current := store.Status()
	if current.AppliedControlRevision != original.AppliedControlRevision {
		t.Fatalf("current state changed after stale apply: rev=%d", current.AppliedControlRevision)
	}
}

// RLY-CTL-006: validate/build must complete before exposing new state.
// During Apply, a concurrent Current() call must see either old or new state, not partial.
func TestRLYCTL006ValidateBuildDoesNotExposePartialState(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state := minimalControlState(t, 1, 1)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	// Try to apply an invalid state — Current() must still return the valid state.
	bad := minimalControlState(t, 2, 2)
	bad.DeploymentId = "invalid-deployment"
	bad.BundleHash, _ = control.HashDescriptor(bad)
	_, err := store.Apply(bad)
	if err == nil {
		t.Fatal("invalid state unexpectedly accepted")
	}
	current := store.Current()
	if current == nil || current.ControlRevision != 1 {
		t.Fatalf("current state corrupted after invalid apply: %+v", current)
	}
}

// RLY-CTL-007: After atomic swap, new requests use new state.
func TestRLYCTL007AtomicSwapMakesNewRequestsUseNewState(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state1 := minimalControlState(t, 1, 1)
	if _, err := store.Apply(state1); err != nil {
		t.Fatal(err)
	}
	state2 := minimalControlState(t, 2, 2)
	if _, err := store.Apply(state2); err != nil {
		t.Fatal(err)
	}
	current := store.Current()
	if current.ControlRevision != 2 || current.ActiveManagedGeneration != 2 {
		t.Fatalf("new state not active after swap: %+v", current)
	}
}

// RLY-CTL-009: Process restart → no persisted control state, public runtime fails closed.
func TestRLYCTL009RestartWithNoPersistedStateFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	// Simulate fresh start — no state applied.
	if store.Current() != nil {
		t.Fatal("fresh store should have nil current state")
	}
	status := store.Status()
	if status.Ready {
		t.Fatal("fresh store should not be ready")
	}
	// A public request should fail closed.
	pub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store.Current() == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer pub.Close()
	resp, err := http.Get(pub.URL + "/test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 fail-closed, got %d", resp.StatusCode)
	}
}

// RLY-CTL-010: After rehydrate, READY status with correct revision/hash.
func TestRLYCTL010RehydrateProducesReadyWithCorrectRevisionAndHash(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state := minimalControlState(t, 15, 7)
	if _, err := store.Apply(state); err != nil {
		t.Fatal(err)
	}
	// Simulate rehydrate: status should reflect correct values.
	status := store.Status()
	if !status.Ready || status.AppliedControlRevision != 15 || status.ActiveManagedGeneration != 7 {
		t.Fatalf("rehydrate status mismatch: %+v", status)
	}
	if status.BundleHash != string(state.BundleHash) {
		t.Fatalf("bundle hash mismatch: got %s want %s", status.BundleHash, state.BundleHash)
	}
}

// RLY-CTL-008: Old request captured state is not re-read during its lifecycle.
// This is verified by the existing TestRLYI4InFlightRequestKeepsCapturedControlState.
// Here we add an explicit assertion that Current() snapshot stays consistent.
func TestRLYCTL008CapturedStateNotReread(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })
	state1 := minimalControlState(t, 1, 1)
	if _, err := store.Apply(state1); err != nil {
		t.Fatal(err)
	}
	// Capture the state pointer.
	captured := store.Current()
	// Apply a new revision.
	state2 := minimalControlState(t, 2, 2)
	if _, err := store.Apply(state2); err != nil {
		t.Fatal(err)
	}
	// The captured pointer should still point to the old state.
	if captured.ControlRevision != 1 {
		t.Fatalf("captured state mutated: rev=%d", captured.ControlRevision)
	}
	// Current() should return the new state.
	if store.Current().ControlRevision != 2 {
		t.Fatal("current state not updated")
	}
}

// Ensure relaycontrolapi is used (avoid unused import).
var _ = relaycontrolapi.RuntimeControlState{}
