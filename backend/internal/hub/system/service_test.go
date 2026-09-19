package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

type statusRelay struct {
	runtimecontrol.RelayClient
	err error
}

func (r *statusRelay) Status(context.Context) (relaycontrolapi.ControlStatus, error) {
	return relaycontrolapi.ControlStatus{BuildVersion: "relay-distinct-build", Ready: true}, r.err
}

func TestStatusDoesNotSubstituteHubOrCachedRelayBuild(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx := context.Background()
	_, err := st.Client.ManagedState.Create().SetID("current").SetActiveManagedGeneration(0).
		SetDesiredControlRevision(0).SetManagedStateRevision(0).SetRuntimeStatus("DEGRADED").SetUpdatedAt(time.Now()).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	adminID := platformid.New(platformid.User)
	if _, err := st.Client.User.Create().SetID(adminID).SetUsername("admin").SetDisplayName("Admin").SetRole("ADMIN").SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	lastID, currentID := platformid.New(platformid.Activation), platformid.New(platformid.Activation)
	for i, state := range []string{"FAILED", "UNKNOWN"} {
		id := lastID
		if i == 1 {
			id = currentID
		}
		row := st.Client.Activation.Create().SetID(id).SetKind("PUBLISH").SetState(state).
			SetIdempotencyKey(platformid.New(platformid.Idempotency)).SetRequestHash("test").
			SetControlRevision(int64(i + 1)).SetBundleHash("test").SetTargetDescriptorJSON([]byte(`{}`)).
			SetCreatedByUserID(adminID).SetCreatedAt(now.Add(time.Duration(i) * time.Second))
		if state == "FAILED" {
			row.SetCompletedAt(now)
		}
		if _, err := row.Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	relay := &statusRelay{}
	s := New(st, &runtimecontrol.Service{Client: st.Client, Relay: relay}, "hub-distinct-build")
	status, err := s.Status(ctx)
	if err != nil || status.RelayBuildVersion == nil || *status.RelayBuildVersion != "relay-distinct-build" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if status.CurrentActivation == nil || status.CurrentActivation.ActivationID != currentID || status.LastActivation == nil || status.LastActivation.ActivationID != lastID {
		t.Fatalf("current/last operation not distinguished: %+v", status)
	}
	relay.err = errors.New("Relay unavailable")
	status, err = s.Status(ctx)
	if err != nil || status.RelayBuildVersion != nil || status.LastRelaySeenAt != nil || status.RelayReady {
		t.Fatalf("stale Relay observation retained: %+v err=%v", status, err)
	}
	if _, err := st.Client.Activation.UpdateOneID(currentID).SetState("COMPLETED").SetCompletedAt(now.Add(2 * time.Second)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = s.Status(ctx)
	if err != nil || status.CurrentActivation != nil || status.LastActivation == nil || status.LastActivation.ActivationID != currentID {
		t.Fatalf("completed operation not promoted to last: %+v err=%v", status, err)
	}
}
