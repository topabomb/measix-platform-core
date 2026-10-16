package workspace

import (
	"context"

	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func (s *Service) RevealMCP(ctx context.Context, actor, userID, spaceID string, revision int, authorize func(context.Context) error) (adminapi.WorkspaceMCPConnection, error) {
	if platformid.Validate(platformid.AgentSpace, spaceID) != nil || revision < 1 {
		return adminapi.WorkspaceMCPConnection{}, ErrInvalid
	}
	if err := authorize(ctx); err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	row, err := s.Client.AgentSpace.Get(ctx, userID)
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	if row.AgentSpaceID != spaceID {
		return adminapi.WorkspaceMCPConnection{}, &remoteapi.Error{Code: "workspace_space_mismatch"}
	}
	if row.BindingRevision != int64(revision) {
		return adminapi.WorkspaceMCPConnection{}, ErrConflict
	}
	service, err := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	// Direct credentials do not depend on enterprise MCP publication or DAV.
	// MCP writes withdraw the connected state before issuing a new key.
	valid := func() bool {
		user, e := s.Client.User.Get(ctx, userID)
		if e != nil || user.Status != "ACTIVE" {
			return false
		}
		current, e := s.Client.AgentSpace.Get(ctx, userID)
		if e != nil || current.State != "CONNECTED" || current.Intent != "CONNECTED" || !current.RemoteActive || current.StopPending || current.AgentSpaceID != spaceID || current.WorkspaceServiceID != row.WorkspaceServiceID || current.RemoteUsername != row.RemoteUsername || current.BindingRevision != row.BindingRevision || current.McpSecretID == "" || current.McpSecretVersion < 1 || current.McpSecretID != row.McpSecretID || current.McpSecretVersion != row.McpSecretVersion {
			return false
		}
		active, e := s.Client.WorkspaceService.Get(ctx, service.ID)
		return e == nil && active.Enabled && active.State == "ACTIVE" && active.ActiveConfigRevision != nil && service.ActiveConfigRevision != nil && *active.ActiveConfigRevision == *service.ActiveConfigRevision
	}
	if !valid() {
		return adminapi.WorkspaceMCPConnection{}, ErrUnavailable
	}
	cfg, err := s.config(ctx, service.ID, *service.ActiveConfigRevision)
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	token, err := s.Secrets.ResolveSecret(ctx, row.McpSecretID, int(row.McpSecretVersion))
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	if err = authorize(ctx); err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	if !valid() {
		return adminapi.WorkspaceMCPConnection{}, ErrUnavailable
	}
	now := s.Now().UTC()
	_, err = s.Client.WorkspaceAudit.Create().SetRequestID(platformid.New(platformid.Request)).SetActorID(actor).SetUserID(userID).SetAgentSpaceID(spaceID).SetAction("REVEAL_MCP").SetPath("").SetOutcome("SUCCEEDED").SetBytes(0).SetCreatedAt(now).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	return adminapi.WorkspaceMCPConnection{McpUrl: cfg.McpOrigin + "/u/" + row.RemoteUsername + "/mcp", Username: row.RemoteUsername, Token: string(token)}, nil
}

func (s *Service) RevealDAV(ctx context.Context, actor, userID string, authorize func(context.Context) error) (adminapi.WorkspaceDAVConnection, error) {
	view, err := s.Projection(ctx, userID)
	if err != nil {
		return adminapi.WorkspaceDAVConnection{}, err
	}
	access, err := s.OpenFiles(ctx, actor, userID, str(view.AgentSpaceId), "REVEAL_DAV", "", authorize)
	if err != nil {
		return adminapi.WorkspaceDAVConnection{}, err
	}
	defer s.CloseFiles(access, "SUCCEEDED", 0)
	raw, err := access.Remote.DAVURL(access.Username, "")
	if err != nil {
		return adminapi.WorkspaceDAVConnection{}, err
	}
	if !access.valid(ctx) {
		return adminapi.WorkspaceDAVConnection{}, ErrUnavailable
	}
	return adminapi.WorkspaceDAVConnection{DavUrl: raw, Username: access.Username, Token: access.Token}, nil
}
