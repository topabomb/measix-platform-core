package capability_test

import (
	"context"
	"measix/platform/internal/hub/capability"
	"measix/platform/pkg/platformid"
	"testing"
)

func TestCorruptReleaseCannotBecomeEmptySuccessfulDiff(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	id := platformid.New(platformid.Release)
	_, err := st.Client.ManagedRelease.Create().SetID(id).SetManagedGeneration(1).SetStatus("ACTIVE").
		SetReleaseContentJSON([]byte("{bad")).SetSnapshotSchemaVersion(2).SetSnapshotJSON([]byte("{}")).
		SetSnapshotHash("sha256:test").SetSourceDraftRevision(1).SetCreatedByUserID(boot.AdminUserID).SetCreatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = capability.NewService(st.Client).GetRelease(ctx, id); err == nil {
		t.Fatal("corrupt release reported as successful empty diff")
	}
}
