package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"

	"measix/platform/ent/workspaceaudit"
)

func TestRevealMCPUsesConfirmedVersionWithoutPublicationDAVOrRotation(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	row := s.Client.AgentSpace.GetX(ctx, user)
	s.Client.ManagedState.UpdateOneID("current").ClearActiveReleaseID().SaveX(ctx)
	s.Client.AgentSpace.UpdateOneID(user).SetDavConfirmed(false).SetFileAccessIntent("REVOKED").SaveX(ctx)
	// A newer stored Secret is not the confirmed binding version.
	if _, err := s.Secrets.ReplaceSecret(ctx, actor, row.McpSecretID, int(row.McpSecretVersion), "unconfirmed-token"); err != nil {
		t.Fatal(err)
	}
	beforeRotates, beforeDAV := remote.rotates, remote.davWrites
	info, err := s.RevealMCP(ctx, actor, user, row.AgentSpaceID, int(row.BindingRevision), func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if info.Token != remote.token || info.Username != row.RemoteUsername || info.McpUrl != remote.url+"/u/"+row.RemoteUsername+"/mcp" {
		t.Fatalf("wrong confirmed connection: username=%s url=%s", info.Username, info.McpUrl)
	}
	if remote.rotates != beforeRotates || remote.davWrites != beforeDAV {
		t.Fatal("viewing credentials mutated remote keys")
	}
	audit := s.Client.WorkspaceAudit.Query().Where(workspaceaudit.ActionEQ("REVEAL_MCP")).OnlyX(ctx)
	if audit.ActorID != actor || audit.AgentSpaceID != row.AgentSpaceID || audit.Outcome != "SUCCEEDED" || audit.Path != "" || strings.Contains(audit.String(), info.Token) {
		t.Fatal("incorrect or secret-bearing audit")
	}
}

func TestRevealMCPRejectsUnavailableOrChangedAuthority(t *testing.T) {
	for _, during := range []bool{false, true} {
		for _, change := range []string{"admin", "user", "service", "service-state", "config", "space", "revision", "secret", "state", "intent", "remote", "stopping", "missing-key"} {
			t.Run(change+map[bool]string{false: "/before", true: "/during"}[during], func(t *testing.T) {
				s, _, actor, user := lifecycleFixture(t)
				runCommand(t, s, actor, user, "CREATE")
				ctx := context.Background()
				row := s.Client.AgentSpace.GetX(ctx, user)
				denied := errors.New("administrator revoked")
				mutate := func() {
					switch change {
					case "user":
						s.Client.User.UpdateOneID(user).SetStatus("DISABLED").SaveX(ctx)
					case "service":
						s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetEnabled(false).SaveX(ctx)
					case "service-state":
						s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetState("APPLYING").SaveX(ctx)
					case "config":
						s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetActiveConfigRevision(2).SaveX(ctx)
					case "space":
						s.Client.AgentSpace.UpdateOneID(user).SetAgentSpaceID("spc_550e8400-e29b-41d4-a716-446655440001").SaveX(ctx)
					case "revision":
						s.Client.AgentSpace.UpdateOneID(user).AddBindingRevision(1).SaveX(ctx)
					case "secret":
						s.Client.AgentSpace.UpdateOneID(user).SetMcpSecretVersion(2).SaveX(ctx)
					case "state":
						s.Client.AgentSpace.UpdateOneID(user).SetState("RESTORING").SaveX(ctx)
					case "intent":
						s.Client.AgentSpace.UpdateOneID(user).SetIntent("DISCONNECTED").SaveX(ctx)
					case "remote":
						s.Client.AgentSpace.UpdateOneID(user).SetRemoteActive(false).SaveX(ctx)
					case "stopping":
						s.Client.AgentSpace.UpdateOneID(user).SetStopPending(true).SaveX(ctx)
					case "missing-key":
						s.Client.AgentSpace.UpdateOneID(user).ClearMcpSecretVersion().SaveX(ctx)
					}
				}
				if !during {
					mutate()
				}
				calls := 0
				info, err := s.RevealMCP(ctx, actor, user, row.AgentSpaceID, int(row.BindingRevision), func(context.Context) error {
					calls++
					if !during || calls == 2 {
						if change == "admin" {
							return denied
						}
						if during {
							mutate()
						}
					}
					return nil
				})
				if err == nil || info.Token != "" {
					t.Fatal("exposed a credential after authority changed")
				}
				if change == "admin" && !errors.Is(err, denied) {
					t.Fatalf("admin failure lost: %v", err)
				}
			})
		}
	}
}
