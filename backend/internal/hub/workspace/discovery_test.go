package workspace

import (
	"context"
	"testing"
)

func TestDiscoveryConnectionsFilterBeforePaginationAndIgnorePublication(t *testing.T) {
	s, _, actor, userID := lifecycleFixture(t)
	ctx := context.Background()
	if op := runCommand(t, s, actor, userID, "CREATE"); op.State != "COMPLETED" {
		t.Fatal(op)
	}
	row := s.Client.AgentSpace.GetX(ctx, userID)
	// A new workspace MCP may not be published yet; its connection is usable for discovery.
	s.Client.ManagedRelease.Update().SetReleaseContentJSON([]byte(`{"mcp":[]}`)).SaveX(ctx)
	page, err := s.ListWorkspaces(ctx, row.WorkspaceServiceID, "", "", true)
	if err != nil || len(page.Items) != 1 || page.Items[0].Workspace.McpAvailable {
		t.Fatalf("unpublished connected source not discoverable: %+v %v", page, err)
	}
	page, err = s.ListWorkspaces(ctx, row.WorkspaceServiceID, "Workspace user", "", true)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("display-name search lost connection: %+v %v", page, err)
	}
	cases := []struct {
		name   string
		change func()
	}{
		{"inactive_user", func() { s.Client.User.UpdateOneID(userID).SetStatus("DISABLED").SaveX(ctx) }},
		{"disconnected", func() { s.Client.AgentSpace.UpdateOneID(userID).SetState("DISCONNECTED").SaveX(ctx) }},
		{"disconnect_intent", func() { s.Client.AgentSpace.UpdateOneID(userID).SetIntent("DISCONNECTED").SaveX(ctx) }},
		{"remote_inactive", func() { s.Client.AgentSpace.UpdateOneID(userID).SetRemoteActive(false).SaveX(ctx) }},
		{"stop_pending", func() { s.Client.AgentSpace.UpdateOneID(userID).SetStopPending(true).SaveX(ctx) }},
		{"missing_secret", func() { s.Client.AgentSpace.UpdateOneID(userID).SetMcpSecretID("").SaveX(ctx) }},
		{"missing_secret_version", func() { s.Client.AgentSpace.UpdateOneID(userID).SetMcpSecretVersion(0).SaveX(ctx) }},
		{"disabled_service", func() { s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetEnabled(false).SaveX(ctx) }},
		{"inactive_service", func() { s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetState("INACTIVE").SaveX(ctx) }},
		{"unapplied_service", func() {
			s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).ClearActiveConfigRevision().SaveX(ctx)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			defer func() {
				s.Client.User.UpdateOneID(userID).SetStatus("ACTIVE").SaveX(ctx)
				s.Client.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetEnabled(true).SetState("ACTIVE").SetActiveConfigRevision(1).SaveX(ctx)
				s.Client.AgentSpace.UpdateOneID(userID).SetState(row.State).SetIntent(row.Intent).SetRemoteActive(row.RemoteActive).SetStopPending(row.StopPending).SetMcpSecretID(row.McpSecretID).SetMcpSecretVersion(row.McpSecretVersion).SaveX(ctx)
			}()
			page, err := s.ListWorkspaces(ctx, row.WorkspaceServiceID, "", "", true)
			if err != nil || len(page.Items) != 0 || page.NextCursor != nil {
				t.Fatalf("unusable discovery identity leaked: %+v %v", page, err)
			}
			ordinary, err := s.ListWorkspaces(ctx, row.WorkspaceServiceID, "", "", false)
			if err != nil || len(ordinary.Items) != 1 {
				t.Fatalf("management list lost workspace: %+v %v", ordinary, err)
			}
		})
	}
	// More than one management page of ineligible rows must not hide the eligible row.
	for i := 0; i < 55; i++ {
		id := "aaa-ineligible-" + string(rune('A'+i))
		s.Client.AgentSpace.Create().SetID(id).SetWorkspaceServiceID(row.WorkspaceServiceID).SetRemoteUsername(id).SetBindingRevision(1).SetState("DISCONNECTED").SetIntent("DISCONNECTED").SetDavConfirmed(false).SetRemoteActive(false).SetStopPending(false).SetCreatedAt(row.CreatedAt).SetUpdatedAt(row.UpdatedAt).SaveX(ctx)
	}
	page, err = s.ListWorkspaces(ctx, row.WorkspaceServiceID, "", "", true)
	if err != nil || len(page.Items) != 1 || page.Items[0].UserId != userID || page.NextCursor != nil {
		t.Fatalf("eligibility was filtered after pagination: %+v %v", page, err)
	}
}
