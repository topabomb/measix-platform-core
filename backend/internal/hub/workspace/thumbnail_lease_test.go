package workspace

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	remoteapi "measix/platform/internal/hub/agentspace"
)

func TestFileAdmissionDoesNotHoldTheLeaseRegistryDuringAuthorization(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID
	var revoked atomic.Bool
	access, err := s.OpenFiles(ctx, actor, user, space, "THUMBNAIL", "picture.png", func(context.Context) error {
		if revoked.Load() {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.CloseFiles(access, "CANCELLED", 0)

	entered, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = s.OpenFiles(ctx, actor, user, space, "THUMBNAIL", "another.png", func(ctx context.Context) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	defer func() { cancel(); <-finished }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("second admission did not begin")
	}
	revoked.Store(true)
	swept := make(chan struct{})
	go func() { s.CancelInvalidFiles(ctx); close(swept) }()
	select {
	case <-swept:
		if access.Context.Err() == nil {
			t.Error("revoked lease was not cancelled")
		}
	case <-time.After(time.Second):
		t.Error("slow admission blocked revocation of a registered lease")
		cancel()
		<-swept
	}
}

func TestFilesReuseTransportAcrossDAVCredentialRotation(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID
	open := func() *FileAccess {
		t.Helper()
		access, err := s.OpenFiles(ctx, actor, user, space, "THUMBNAIL", "picture.png", func(context.Context) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.CloseFiles(access, "SUCCEEDED", 0) })
		return access
	}
	first := open()
	runCommand(t, s, actor, user, "SET_DAV")
	second := open()
	if first.Token == second.Token || first.Revalidate(ctx) == nil {
		t.Fatal("rotation must invalidate the old lease and use a fresh credential")
	}
	if first.Remote.HTTP.Transport != second.Remote.HTTP.Transport {
		t.Fatal("requests with the same transport policy cannot reuse DAV connections")
	}
}

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
