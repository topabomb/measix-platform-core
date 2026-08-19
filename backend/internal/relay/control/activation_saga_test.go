package control

import (
	"testing"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

func TestFinalS0ActivationSagaPrepareBarrierCommitIsAtomic(t *testing.T) {
	now := time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC)
	store := NewStore(func() time.Time { return now })
	current := minimalStateForActivationTest(t, 10, 4)
	if _, err := store.ApplyOperational(current); err != nil {
		t.Fatalf("hydrate current control: %v", err)
	}

	next := minimalStateForActivationTest(t, 11, 5)
	activationID := platformid.New(platformid.Activation)
	if _, err := store.Prepare(activationID, next); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := store.Current(); got == nil || got.ControlRevision != 10 || got.ActiveManagedGeneration != 4 {
		t.Fatalf("prepare changed current traffic: %+v", got)
	}
	if err := store.EnterBarrier(activationID); err != nil {
		t.Fatalf("enter barrier: %v", err)
	}
	if active, gotID := store.Barrier(); !active || gotID != activationID {
		t.Fatalf("barrier state = %v %q", active, gotID)
	}
	if _, err := store.Commit(activationID); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if active, _ := store.Barrier(); active {
		t.Fatal("commit did not release barrier")
	}
	if got := store.Current(); got == nil || got.ControlRevision != 11 || got.ActiveManagedGeneration != 5 {
		t.Fatalf("commit did not atomically swap current control: %+v", got)
	}
}

func minimalStateForActivationTest(t *testing.T, revision, generation int) relaycontrolapi.RuntimeControlState {
	t.Helper()
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         revision,
		ActiveManagedGeneration: generation,
		DeploymentId:            platformid.New(platformid.Deployment),
		AuthKeys:                []relaycontrolapi.PublicJwk{},
		PrincipalState: relaycontrolapi.PrincipalState{
			DisabledUserIds:   []string{},
			RevokedDeviceIds:  []string{},
			RevokedSessionIds: []string{},
		},
		ResourceRoutes:    []relaycontrolapi.ResourceRoute{},
		Routes:             []relaycontrolapi.RuntimeRouteSpec{},
		Upstreams:          []relaycontrolapi.RuntimeUpstreamSpec{},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	hash, err := HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	state.BundleHash = hash
	return state
}
