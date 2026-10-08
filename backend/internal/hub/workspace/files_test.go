package workspace

import (
	"context"
	"errors"
	remoteapi "measix/platform/internal/hub/agentspace"
	"testing"
)

func TestFileLeaseRevokesAndLateDAVFailureCannotRevokeNewVersion(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	sessionRevoked := false
	authorize := func(context.Context) error {
		if sessionRevoked {
			return errors.New("session revoked")
		}
		return nil
	}
	access, e := s.OpenFiles(ctx, actor, user, s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID, "GET", "a.txt", authorize)
	if e != nil {
		t.Fatal(e)
	}
	defer s.CloseFiles(access, "CANCELLED", 0)
	runCommand(t, s, actor, user, "SET_DAV")
	s.ObserveFileError(ctx, access, &remoteapi.Error{Code: "dav_credential_unavailable", Status: 401})
	view, _ := s.Projection(ctx, user)
	if !view.FilesAvailable || !view.McpAvailable {
		t.Fatalf("late failure revoked new credential %+v", view)
	}
	s.CancelInvalidFiles(ctx)
	select {
	case <-access.Context.Done():
	default:
		t.Fatal("old credential lease not cancelled")
	}
	fresh, e := s.OpenFiles(ctx, actor, user, s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID, "GET", "a.txt", authorize)
	if e != nil {
		t.Fatal(e)
	}
	defer s.CloseFiles(fresh, "CANCELLED", 0)
	sessionRevoked = true
	s.CancelInvalidFiles(ctx)
	select {
	case <-fresh.Context.Done():
	default:
		t.Fatal("logout did not cancel file lease")
	}
	if _, e = s.OpenFiles(ctx, actor, user, s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID, "GET", "a.txt", authorize); e == nil {
		t.Fatal("revoked session registered a file lease")
	}
}

func TestCurrentDAVFailureOnlyInvalidatesFiles(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	a, e := s.OpenFiles(ctx, actor, user, s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID, "LIST", "", func(context.Context) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	defer s.CloseFiles(a, "REJECTED", 0)
	s.ObserveFileError(ctx, a, &remoteapi.Error{Code: "dav_credential_unavailable", Status: 401})
	view, _ := s.Projection(ctx, user)
	if view.FilesAvailable || !view.McpAvailable {
		t.Fatalf("credential availability not separated: %+v", view)
	}
}

func TestPersistedRuntimeBindingCannotOutliveCurrentIntent(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user)
	workspaceService := s.Client.WorkspaceService.GetX(ctx, space.WorkspaceServiceID)
	_, bindings, e := s.RuntimeBindings(ctx, workspaceService.ID, workspaceService.McpServerID)
	if e != nil || len(bindings) != 1 {
		t.Fatal(e)
	}
	if e = s.ValidateRuntimeBinding(ctx, bindings[0]); e != nil {
		t.Fatal(e)
	}
	s.Client.AgentSpace.UpdateOneID(user).SetIntent("DISCONNECTED").SetState("DISCONNECTING").SaveX(ctx)
	if e = s.ValidateRuntimeBinding(ctx, bindings[0]); e == nil {
		t.Fatal("restart would restore withdrawn intent")
	}
	_ = actor
}
