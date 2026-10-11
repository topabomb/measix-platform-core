package capability_test

import (
	"context"
	"encoding/json"
	"errors"
	"measix/platform/ent"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"reflect"
	"testing"
	"time"
)

func seedHistory(t *testing.T, c *ent.Client, svc *capability.Service, actor string, g int, status string, at time.Time) *ent.ManagedRelease {
	t.Helper()
	ctx := context.Background()
	dep, err := c.Deployment.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(draft.Content)
	id := platformid.New(platformid.Release)
	snap, hash, err := svc.CompileSnapshot(capability.SnapshotInput{DeploymentID: dep.ID, ReleaseID: id, ManagedGeneration: g, Content: draft.Content, PublishedAt: at, PublishedByUserID: actor})
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := json.Marshal(snap)
	row, err := c.ManagedRelease.Create().SetID(id).SetManagedGeneration(int64(g)).SetStatus(status).SetReleaseContentJSON(raw).SetSnapshotJSON(bytes).SetSnapshotHash(hash).SetSourceDraftRevision(1).SetCreatedByUserID(actor).SetCreatedAt(at).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return row
}
func cleanup(t *testing.T, svc *capability.Service, actor string, sel adminapi.ReleaseCleanupSelection) adminapi.ReleaseCleanupResult {
	t.Helper()
	p, err := svc.PreviewReleaseCleanup(context.Background(), sel)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.ExecuteReleaseCleanup(context.Background(), actor, adminapi.ExecuteReleaseCleanupRequest{Selection: sel, PreviewHash: p.PreviewHash})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCleanupProtectsActiveStagedAndPreservesOriginalDiff(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	svc := capability.NewService(st.Client)
	svc.Now = func() time.Time { return now }
	old := seedHistory(t, st.Client, svc, boot.AdminUserID, 1, "SUPERSEDED", now.Add(-48*time.Hour))
	active := seedHistory(t, st.Client, svc, boot.AdminUserID, 2, "ACTIVE", now.Add(-24*time.Hour))
	staged := seedHistory(t, st.Client, svc, boot.AdminUserID, 3, "STAGED", now)
	failed := seedHistory(t, st.Client, svc, boot.AdminUserID, 4, "ACTIVATION_FAILED", now)
	if _, err := st.Client.ManagedState.UpdateOneID("current").SetActiveReleaseID(active.ID).SetActiveManagedGeneration(2).SetLastAssignedGeneration(4).SetRuntimeStatus("READY").Save(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := svc.GetRelease(ctx, active.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{old.ID, active.ID, staged.ID, failed.ID}
	sel := adminapi.ReleaseCleanupSelection{ReleaseIds: &ids}
	p, err := svc.PreviewReleaseCleanup(ctx, sel)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 2 || len(p.Protected) != 2 || p.ActivationBlocked {
		t.Fatalf("preview=%+v", p)
	}
	out := cleanup(t, svc, boot.AdminUserID, sel)
	if out.DeletedCount != 2 || out.ReclaimedBytes == 0 {
		t.Fatalf("result=%+v", out)
	}
	retained, err := st.Client.ManagedRelease.Get(ctx, active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(retained.SnapshotJSON) != string(active.SnapshotJSON) || retained.SnapshotHash != active.SnapshotHash || string(retained.ReleaseContentJSON) != string(active.ReleaseContentJSON) {
		t.Fatal("retained immutable bytes changed")
	}
	after, err := svc.GetRelease(ctx, active.ID)
	if err != nil || !reflect.DeepEqual(before.DiffSummary, after.DiffSummary) {
		t.Fatalf("original diff changed: %+v vs %+v err=%v", before.DiffSummary, after.DiffSummary, err)
	}
	next, err := capability.NextGeneration(ctx, st.Client)
	if err != nil || next != 5 {
		t.Fatalf("generation reused: %d %v", next, err)
	}
	if _, err = svc.GetSnapshot(ctx, 1); !errors.Is(err, capability.ErrReleaseNotFound) {
		t.Fatalf("purged snapshot=%v", err)
	}
	audit, err := svc.ListReleaseHistoryAudit(ctx)
	if err != nil || len(audit.Items) != 1 || len(audit.Items[0].ReleaseGenerations) != 2 {
		t.Fatalf("audit=%+v err=%v", audit, err)
	}
}

func TestCleanupRejectsStalePreviewBusyAndInvalidModes(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	svc := capability.NewService(st.Client)
	old := seedHistory(t, st.Client, svc, boot.AdminUserID, 1, "SUPERSEDED", now)
	st.Client.ManagedState.UpdateOneID("current").SetRuntimeStatus("READY").SaveX(ctx)
	ids := []string{old.ID}
	sel := adminapi.ReleaseCleanupSelection{ReleaseIds: &ids}
	p, err := svc.PreviewReleaseCleanup(ctx, sel)
	if err != nil {
		t.Fatal(err)
	}
	st.Client.ManagedState.UpdateOneID("current").AddManagedStateRevision(1).SaveX(ctx)
	_, err = svc.ExecuteReleaseCleanup(ctx, boot.AdminUserID, adminapi.ExecuteReleaseCleanupRequest{Selection: sel, PreviewHash: p.PreviewHash})
	if !errors.Is(err, capability.ErrCleanupStale) {
		t.Fatalf("stale=%v", err)
	}
	st.Client.ManagedState.UpdateOneID("current").SetRuntimeStatus("DEGRADED").SaveX(ctx)
	p, err = svc.PreviewReleaseCleanup(ctx, sel)
	if err != nil || !p.ActivationBlocked {
		t.Fatalf("busy plan=%+v %v", p, err)
	}
	_, err = svc.ExecuteReleaseCleanup(ctx, boot.AdminUserID, adminapi.ExecuteReleaseCleanupRequest{Selection: sel, PreviewHash: p.PreviewHash})
	if !errors.Is(err, capability.ErrCleanupBusy) {
		t.Fatalf("busy execution=%v", err)
	}
	if count, _ := st.Client.ManagedRelease.Query().Count(ctx); count != 1 {
		t.Fatal("rejected cleanup deleted history")
	}
	for _, bad := range []adminapi.ReleaseCleanupSelection{{}, {ReleaseIds: &ids, PublishedBefore: &now}, {PublishedAfter: &now}, {ReleaseIds: &[]string{old.ID, old.ID}}} {
		if _, err = svc.PreviewReleaseCleanup(ctx, bad); !errors.Is(err, capability.ErrCleanupSelection) {
			t.Fatalf("invalid selection accepted: %+v %v", bad, err)
		}
	}
}

func TestRetentionKeepsUnionAndSweepsWithoutChangingRuntime(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	svc := capability.NewService(st.Client)
	svc.Now = func() time.Time { return now }
	for i := 1; i <= 5; i++ {
		age := time.Duration(10-i) * 24 * time.Hour
		if i == 3 {
			age = time.Hour
		}
		seedHistory(t, st.Client, svc, boot.AdminUserID, i, "SUPERSEDED", now.Add(-age))
	}
	st.Client.ManagedState.UpdateOneID("current").SetLastAssignedGeneration(5).SetRuntimeStatus("READY").SaveX(ctx)
	if err := svc.SweepReleaseRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.Client.ManagedRelease.Query().Count(ctx); n != 5 {
		t.Fatal("disabled rule removed records")
	}
	n, d := 2, 2
	_, err := svc.UpdateReleaseRetention(ctx, boot.AdminUserID, adminapi.UpdateReleaseRetentionRequest{ExpectedRevision: 1, Rule: adminapi.ReleaseRetentionRule{Enabled: true, KeepLast: &n, KeepDays: &d}})
	if err != nil {
		t.Fatal(err)
	}
	state := st.Client.ManagedState.GetX(ctx, "current")
	if err = svc.SweepReleaseRetention(ctx); err != nil {
		t.Fatal(err)
	}
	rows := st.Client.ManagedRelease.Query().Order(ent.Asc(managedrelease.FieldManagedGeneration)).AllX(ctx)
	if len(rows) != 3 || rows[0].ManagedGeneration != 3 || rows[2].ManagedGeneration != 5 {
		t.Fatalf("retention union=%+v", rows)
	}
	beforeState, _ := json.Marshal(state)
	afterState, _ := json.Marshal(st.Client.ManagedState.GetX(ctx, "current"))
	if string(beforeState) != string(afterState) {
		t.Fatal("cleanup changed runtime or control state")
	}
	before := st.Client.ReleaseHistoryAudit.Query().CountX(ctx)
	if err = svc.SweepReleaseRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if st.Client.ReleaseHistoryAudit.Query().CountX(ctx) != before {
		t.Fatal("hourly sweep was not throttled")
	}
}

func TestCleanupTimeWindowIsHalfOpenAndBounded(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	ctx := context.Background()
	svc := capability.NewService(st.Client)
	svc.Now = func() time.Time { return now }
	after := now.Add(-24 * time.Hour)
	excluded := seedHistory(t, st.Client, svc, boot.AdminUserID, 1, "SUPERSEDED", after.Add(-time.Second))
	for i := 2; i <= 203; i++ {
		seedHistory(t, st.Client, svc, boot.AdminUserID, i, "SUPERSEDED", after)
	}
	seedHistory(t, st.Client, svc, boot.AdminUserID, 204, "SUPERSEDED", now)
	st.Client.ManagedState.UpdateOneID("current").SetRuntimeStatus("READY").SetLastAssignedGeneration(204).SaveX(ctx)
	sel := adminapi.ReleaseCleanupSelection{PublishedAfter: &after, PublishedBefore: &now}
	p, err := svc.PreviewReleaseCleanup(ctx, sel)
	if err != nil || len(p.Candidates) != 200 || !p.HasMore {
		t.Fatalf("batch=%d more=%v err=%v", len(p.Candidates), p.HasMore, err)
	}
	cleanup(t, svc, boot.AdminUserID, sel)
	if st.Client.ManagedRelease.Query().CountX(ctx) != 4 {
		t.Fatal("batch exceeded 200")
	}
	if _, err = st.Client.ManagedRelease.Get(ctx, excluded.ID); err != nil {
		t.Fatal("outside window deleted")
	}
	p, err = svc.PreviewReleaseCleanup(ctx, sel)
	if err != nil || len(p.Candidates) != 2 || p.HasMore {
		t.Fatalf("next batch=%+v err=%v", p, err)
	}
}
