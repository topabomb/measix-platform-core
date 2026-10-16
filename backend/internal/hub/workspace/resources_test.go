package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestResourcesReadOnlyRevalidatesBindingAndAdministrator(t *testing.T) {
	s, f, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	before := s.Client.AgentSpace.GetX(ctx, user)
	// Resource-less fake must fail explicitly, without changing any lifecycle state.
	if _, err := s.Resources(ctx, user, f.space, nil); err == nil {
		t.Fatal("accepted missing resources")
	}
	after := s.Client.AgentSpace.GetX(ctx, user)
	if before.BindingRevision != after.BindingRevision || before.State != after.State || !after.DavConfirmed {
		t.Fatal("observation mutated binding")
	}
	_, err := s.Resources(ctx, user, "spc_550e8400-e29b-41d4-a716-446655440001", nil)
	if err == nil || err.Error() != "agent space: workspace_space_mismatch" {
		t.Fatalf("wrong space: %v", err)
	}
	denied := errors.New("administrator revoked")
	_, err = s.Resources(ctx, user, f.space, func(context.Context) error { return denied })
	if !errors.Is(err, denied) {
		t.Fatalf("authorization ignored: %v", err)
	}
}

func TestResourcesObserveDisabledAndRejectChangesDuringRead(t *testing.T) {
	for _, change := range []string{"disabled", "binding", "configuration", "space", "authorization"} {
		t.Run(change, func(t *testing.T) {
			s, f, actor, user := lifecycleFixture(t)
			runCommand(t, s, actor, user, "CREATE")
			ctx := context.Background()
			row := s.Client.AgentSpace.GetX(ctx, user)
			raw, err := os.ReadFile("../../../../api/fixtures/workspace/resources.json")
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			json.Unmarshal(raw, &data)
			data["agentSpaceId"] = f.space
			f.resources = data
			if change == "disabled" {
				runCommand(t, s, actor, user, "DISCONNECT")
				s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetEnabled(false).SaveX(ctx)
				s.Client.User.UpdateOneID(user).SetStatus("DISABLED").SaveX(ctx)
			}
			revoked := false
			denied := errors.New("revoked")
			f.onGet = func() {
				switch change {
				case "binding":
					s.Client.AgentSpace.UpdateOneID(user).AddBindingRevision(1).SaveX(ctx)
				case "configuration":
					s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).AddConfigRevision(1).SaveX(ctx)
				case "space":
					s.Client.AgentSpace.UpdateOneID(user).SetAgentSpaceID("spc_550e8400-e29b-41d4-a716-446655440001").SaveX(ctx)
				case "authorization":
					revoked = true
				}
			}
			out, err := s.Resources(ctx, user, f.space, func(context.Context) error {
				if revoked {
					return denied
				}
				return nil
			})
			if change == "disabled" {
				if err != nil || out.Disk.Value == nil {
					t.Fatalf("disabled observation: %+v %v", out, err)
				}
			} else if err == nil || out.AgentSpaceId != "" {
				t.Fatalf("accepted stale response: %+v %v", out, err)
			}
		})
	}
}
