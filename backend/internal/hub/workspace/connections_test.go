package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"

	"measix/platform/ent/workspaceaudit"
	"measix/platform/pkg/platformid"
)

func TestPublicConnectionsKeepPrivateRuntimeTargets(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	remote.publicMCPOrigin, remote.publicDAVOrigin = "https://space.example.com", "https://files.example.com"
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	row := s.Client.AgentSpace.GetX(ctx, user)
	service := s.Client.WorkspaceService.GetX(ctx, row.WorkspaceServiceID)
	davID := service.DavResourceID
	beforeRotates, beforeDAV := remote.rotates, remote.davWrites
	authorize := func(context.Context) error { return nil }
	links, err := s.Links(ctx, user, row.AgentSpaceID, int(row.BindingRevision), authorize)
	if err != nil || links.McpUrl != remote.publicMCPOrigin+"/u/"+row.RemoteUsername+"/mcp" || str(links.DavUrl) != remote.publicDAVOrigin+"/u/"+row.RemoteUsername+"/dav/" {
		t.Fatalf("wrong public links: %+v %v", links, err)
	}
	mcp, err := s.RevealMCP(ctx, actor, user, row.AgentSpaceID, int(row.BindingRevision), authorize)
	if err != nil || mcp.McpUrl != links.McpUrl || mcp.Token != remote.token {
		t.Fatalf("wrong MCP delivery: %v", err)
	}
	dav, err := s.RevealDAV(ctx, actor, user, authorize)
	if err != nil || dav.DavUrl != str(links.DavUrl) || dav.Token != remote.dav {
		t.Fatalf("wrong DAV delivery: %v", err)
	}
	_, bindings, err := s.RuntimeBindings(ctx, service.ID, service.McpServerID)
	if err != nil || len(bindings) != 1 || bindings[0].Endpoint != remote.url+"/u/"+row.RemoteUsername+"/mcp" {
		t.Fatalf("public origin replaced MCP target: %+v %v", bindings, err)
	}
	_, davBindings, err := s.DAVRuntimeBindings(ctx, service.ID, davID)
	if err != nil || len(davBindings) != 1 || davBindings[0].Endpoint != remote.url+"/u/"+row.RemoteUsername+"/dav/" {
		t.Fatalf("public origin replaced DAV target: %+v %v", davBindings, err)
	}
	if remote.rotates != beforeRotates || remote.davWrites != beforeDAV {
		t.Fatal("connection reads changed credentials")
	}
}

func TestLinksRejectChangedAuthorityAndRemoteIdentity(t *testing.T) {
	for _, change := range []string{"binding", "config", "admin", "remote-space", "remote-disabled"} {
		t.Run(change, func(t *testing.T) {
			s, remote, actor, user := lifecycleFixture(t)
			runCommand(t, s, actor, user, "CREATE")
			ctx := context.Background()
			row := s.Client.AgentSpace.GetX(ctx, user)
			if change == "remote-space" {
				remote.space = platformid.New(platformid.AgentSpace)
			}
			if change == "remote-disabled" {
				remote.enabled = false
			}
			remote.onGet = func() {
				if change == "binding" {
					s.Client.AgentSpace.UpdateOneID(user).AddBindingRevision(1).SaveX(ctx)
				}
				if change == "config" {
					s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetActiveConfigRevision(2).SaveX(ctx)
				}
			}
			calls := 0
			links, err := s.Links(ctx, user, row.AgentSpaceID, int(row.BindingRevision), func(context.Context) error {
				calls++
				if calls > 1 && change == "admin" {
					return errors.New("admin revoked")
				}
				return nil
			})
			if err == nil || links.McpUrl != "" || links.DavUrl != nil {
				t.Fatal("returned links after authority changed")
			}
		})
	}
}

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
