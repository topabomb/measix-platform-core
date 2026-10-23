package workspace

import (
	"context"

	"measix/platform/ent"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

type connectionRead struct {
	space    *ent.AgentSpace
	remote   remoteapi.User
	validate func() error
}

// readConnection keeps public-link reads and credential delivery under the same
// captured authority. The advertised links never enter RuntimeBindings.
func (s *Service) readConnection(ctx context.Context, userID, spaceID string, revision int, authorize func(context.Context) error) (*connectionRead, error) {
	if platformid.Validate(platformid.AgentSpace, spaceID) != nil || revision < 1 {
		return nil, ErrInvalid
	}
	if err := authorize(ctx); err != nil {
		return nil, err
	}
	row, err := s.Client.AgentSpace.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if row.AgentSpaceID != spaceID {
		return nil, &remoteapi.Error{Code: "workspace_space_mismatch"}
	}
	if row.BindingRevision != int64(revision) {
		return nil, ErrConflict
	}
	service, err := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return nil, err
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
		return nil, ErrUnavailable
	}
	cfg, err := s.config(ctx, service.ID, *service.ActiveConfigRevision)
	if err != nil {
		return nil, err
	}
	client, err := s.adapter(ctx, cfg)
	if err != nil {
		return nil, err
	}
	remote, readErr := client.Get(ctx, row.RemoteUsername)
	validate := func() error {
		if err := authorize(ctx); err != nil {
			return err
		}
		if !valid() {
			return ErrUnavailable
		}
		return nil
	}
	if err = validate(); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	if remote.AgentSpaceID != row.AgentSpaceID {
		return nil, &remoteapi.Error{Code: "workspace_space_mismatch"}
	}
	if remote.Status != "active" || remote.StopPending {
		return nil, ErrUnavailable
	}
	return &connectionRead{space: row, remote: remote, validate: validate}, nil
}

func (s *Service) Links(ctx context.Context, userID, spaceID string, revision int, authorize func(context.Context) error) (adminapi.WorkspaceLinks, error) {
	read, err := s.readConnection(ctx, userID, spaceID, revision, authorize)
	if err != nil {
		return adminapi.WorkspaceLinks{}, err
	}
	out := adminapi.WorkspaceLinks{McpUrl: read.remote.MCPURL}
	if read.remote.DAVURL != "" {
		out.DavUrl = &read.remote.DAVURL
	}
	return out, nil
}

func (s *Service) RevealMCP(ctx context.Context, actor, userID, spaceID string, revision int, authorize func(context.Context) error) (adminapi.WorkspaceMCPConnection, error) {
	read, err := s.readConnection(ctx, userID, spaceID, revision, authorize)
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	row := read.space
	token, err := s.Secrets.ResolveSecret(ctx, row.McpSecretID, int(row.McpSecretVersion))
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	if err = read.validate(); err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	now := s.Now().UTC()
	_, err = s.Client.WorkspaceAudit.Create().SetRequestID(platformid.New(platformid.Request)).SetActorID(actor).SetUserID(userID).SetAgentSpaceID(spaceID).SetAction("REVEAL_MCP").SetPath("").SetOutcome("SUCCEEDED").SetBytes(0).SetCreatedAt(now).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return adminapi.WorkspaceMCPConnection{}, err
	}
	return adminapi.WorkspaceMCPConnection{McpUrl: read.remote.MCPURL, Username: row.RemoteUsername, Token: string(token)}, nil
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
	outcome := "FAILED"
	defer func() { s.CloseFiles(access, outcome, 0) }()
	remote, err := access.Remote.Get(access.Context, access.Username)
	if err != nil {
		return adminapi.WorkspaceDAVConnection{}, err
	}
	if remote.AgentSpaceID != str(view.AgentSpaceId) {
		return adminapi.WorkspaceDAVConnection{}, &remoteapi.Error{Code: "workspace_space_mismatch"}
	}
	if remote.DAVURL == "" || remote.Status != "active" || remote.StopPending {
		return adminapi.WorkspaceDAVConnection{}, ErrUnavailable
	}
	if !access.valid(ctx) {
		return adminapi.WorkspaceDAVConnection{}, ErrUnavailable
	}
	outcome = "SUCCEEDED"
	return adminapi.WorkspaceDAVConnection{DavUrl: remote.DAVURL, Username: access.Username, Token: access.Token}, nil
}
