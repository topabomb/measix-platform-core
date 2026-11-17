package workspace

import (
	"context"
	"errors"
	"testing"

	remoteapi "measix/platform/internal/hub/agentspace"
)

func TestPreviewLeasesHaveIndependentFourSlotBudget(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID
	authorize := func(context.Context) error { return nil }
	var leases []*FileAccess
	t.Cleanup(func() {
		for _, access := range leases {
			if access != nil {
				s.CloseFiles(access, "CANCELLED", 0)
			}
		}
	})
	open := func(action string) *FileAccess {
		t.Helper()
		access, err := s.OpenFiles(ctx, actor, user, space, action, "picture.png", authorize)
		if err != nil {
			t.Fatalf("open %s: %v", action, err)
		}
		leases = append(leases, access)
		return access
	}
	for range 8 {
		open("GET")
	}
	for range 4 {
		open("THUMBNAIL")
	}
	for action, code := range map[string]string{"GET": "file_transfer_limit", "THUMBNAIL": "preview_busy", "THUMBNAIL_OPTIONS": "preview_busy"} {
		access, err := s.OpenFiles(ctx, actor, user, space, action, "picture.png", authorize)
		var remote *remoteapi.Error
		if access != nil || !errors.As(err, &remote) || remote.Code != code || remote.Status != 429 {
			t.Fatalf("%s budget: access=%v err=%v", action, access, err)
		}
	}
	s.CloseFiles(leases[0], "SUCCEEDED", 17)
	leases[0] = nil
	open("GET") // A full preview pool does not consume the released file slot.
}

func TestPreviewCapabilityLeaseSharesCancellationAndAuditOwner(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID
	revoked := false
	authorize := func(context.Context) error {
		if revoked {
			return errors.New("session revoked")
		}
		return nil
	}
	for _, action := range []string{"THUMBNAIL_OPTIONS", "THUMBNAIL"} {
		access, err := s.OpenFiles(ctx, actor, user, space, action, "", authorize)
		if err != nil {
			t.Fatal(err)
		}
		revoked = true
		s.CancelInvalidFiles(ctx)
		select {
		case <-access.Context.Done():
		default:
			t.Fatal("preview lease survived identity revocation")
		}
		s.CloseFiles(access, "CANCELLED", 0)
		audit, err := s.Client.WorkspaceAudit.Get(ctx, access.auditID)
		if err != nil || audit.Action != action || audit.Bytes != 0 || audit.Outcome != "CANCELLED" || audit.CompletedAt == nil {
			t.Fatalf("preview audit not owned by the existing lease: %+v %v", audit, err)
		}
		revoked = false
	}
}

func TestBufferedPreviewMustRevalidateBeforeDelivery(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID
	access, err := s.OpenFiles(ctx, actor, user, space, "THUMBNAIL", "picture.png", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.CloseFiles(access, "CANCELLED", 0)
	if err := access.Revalidate(ctx); err != nil {
		t.Fatalf("fresh preview rejected: %v", err)
	}
	runCommand(t, s, actor, user, "SET_DAV")
	// No periodic cancellation sweep has run. A buffered old-key response must
	// still be rejected at the final delivery boundary.
	if err := access.Revalidate(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("buffered preview survived credential rotation: %v", err)
	}
	select {
	case <-access.Context.Done():
	default:
		t.Fatal("invalid delivery lease was not cancelled")
	}
}
