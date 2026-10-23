package runtimecontrol_test

import (
	"testing"

	"measix/platform/pkg/platformid"
)

func TestCurrentAdminCannotBeDisabled(t *testing.T) {
	ctx, st, svc, _, adminID, _ := setupCrashTestEnv(t)
	before, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DisableUser(ctx, adminID, platformid.New(platformid.Idempotency), adminID); err == nil {
		t.Fatal("current administrator was disabled")
	}
	row, _ := st.Client.User.Get(ctx, adminID)
	after, _ := st.Client.ManagedState.Get(ctx, "current")
	if row.Status != "ACTIVE" || after.DesiredControlRevision != before.DesiredControlRevision {
		t.Fatal("rejected disable had side effects")
	}
}
