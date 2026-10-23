package workspace

import (
	"context"
	"errors"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/pkg/platformid"
	"testing"
)

func TestUnavailableSpaceStillRejectsStaleTargetAsMismatch(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	row := s.Client.AgentSpace.GetX(ctx, user)
	s.Client.AgentSpace.UpdateOneID(user).SetDavConfirmed(false).SaveX(ctx)
	_, err := s.OpenFiles(ctx, actor, user, platformid.New(platformid.AgentSpace), "PUT", "a.txt", func(context.Context) error { return nil })
	var remote *remoteapi.Error
	if !errors.As(err, &remote) || remote.Code != "workspace_space_mismatch" {
		t.Fatalf("stale target should prompt a space refresh: %v", err)
	}
	_, err = s.OpenFiles(ctx, actor, user, row.AgentSpaceID, "PUT", "a.txt", func(context.Context) error { return nil })
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("current target should report unavailable: %v", err)
	}
}

func TestUnprovisionedProjectionReportsServiceIntent(t *testing.T) {
	s, _, _, user := lifecycleFixture(t)
	ctx := context.Background()
	row := s.Client.WorkspaceService.Query().OnlyX(ctx)
	for _, enabled := range []bool{true, false} {
		s.Client.WorkspaceService.UpdateOneID(row.ID).SetEnabled(enabled).SaveX(ctx)
		got, err := s.Projection(ctx, user)
		want := "DISABLED"
		if enabled {
			want = "ENABLED"
		}
		if err != nil || string(got.ServiceState) != want || got.State != "UNPROVISIONED" || got.FilesAvailable {
			t.Fatalf("projection: %+v, %v; want %s and unprovisioned", got, err, want)
		}
	}
	s.Client.WorkspaceOperation.Delete().ExecX(ctx)
	s.Client.WorkspaceService.UpdateOneID(row.ID).ClearActiveConfigRevision().SaveX(ctx)
	s.Client.WorkspaceServiceConfig.Delete().ExecX(ctx)
	s.Client.WorkspaceService.Delete().ExecX(ctx)
	got, err := s.Projection(ctx, user)
	if err != nil || got.ServiceState != "NOT_CONFIGURED" {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestFileLeaseRejectsMissingOrReplacedSpaceBeforeAccess(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	for _, target := range []string{"", "invalid", platformid.New(platformid.AgentSpace)} {
		access, err := s.OpenFiles(ctx, actor, user, target, "PUT", "new.txt", func(context.Context) error { return nil })
		if err == nil {
			s.CloseFiles(access, "REJECTED", 0)
			t.Fatalf("accepted stale target %q", target)
		}
	}
	if n := s.Client.WorkspaceAudit.Query().CountX(ctx); n != 0 {
		t.Fatalf("rejected request created %d audits", n)
	}
}
