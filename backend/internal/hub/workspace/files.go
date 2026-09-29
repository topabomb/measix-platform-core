package workspace

import (
	"context"
	"errors"
	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"time"
)

type FileAccess struct {
	Context                           context.Context
	Remote                            *remoteapi.Client
	Username, Token                   string
	Idle                              time.Duration
	valid                             func(context.Context) bool
	cancel                            context.CancelFunc
	auditID                           int
	userID, davSecretID               string
	bindingRevision, davSecretVersion int64
}

func (s *Service) OpenFiles(ctx context.Context, actor, userID, action, path string, authorize func(context.Context) error) (*FileAccess, error) {
	if _, err := remoteapi.RelativePath(path, false); err != nil {
		return nil, err
	}
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	if len(s.files) >= 8 {
		return nil, &remoteapi.Error{Code: "file_transfer_limit", Status: 429}
	}
	if err := authorize(ctx); err != nil {
		return nil, err
	}
	projection, err := s.Projection(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !projection.FilesAvailable {
		return nil, ErrUnavailable
	}
	row, err := s.Client.AgentSpace.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	workspaceService, err := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return nil, err
	}
	if workspaceService.ActiveConfigRevision == nil {
		return nil, ErrUnavailable
	}
	cfg, err := s.config(ctx, workspaceService.ID, *workspaceService.ActiveConfigRevision)
	if err != nil {
		return nil, err
	}
	remote, err := s.adapter(ctx, cfg)
	if err != nil {
		return nil, err
	}
	token, err := s.Secrets.ResolveSecret(ctx, row.DavSecretID, int(row.DavSecretVersion))
	if err != nil {
		return nil, err
	}
	leaseCtx, cancel := context.WithCancel(ctx)
	access := &FileAccess{Context: leaseCtx, Remote: remote, Username: row.RemoteUsername, Token: string(token), Idle: time.Duration(cfg.IdleTimeoutMs) * time.Millisecond, cancel: cancel, userID: userID, davSecretID: row.DavSecretID, bindingRevision: row.BindingRevision, davSecretVersion: row.DavSecretVersion}
	access.valid = func(checkCtx context.Context) bool {
		if authorize(checkCtx) != nil {
			return false
		}
		current, e := s.Client.AgentSpace.Get(checkCtx, userID)
		if e != nil || current.BindingRevision != row.BindingRevision || current.AgentSpaceID != row.AgentSpaceID || current.DavSecretID != row.DavSecretID || current.DavSecretVersion != row.DavSecretVersion {
			return false
		}
		config, e := s.Client.WorkspaceService.Get(checkCtx, workspaceService.ID)
		if e != nil || config.ActiveConfigRevision == nil || *config.ActiveConfigRevision != *workspaceService.ActiveConfigRevision {
			return false
		}
		view, e := s.Projection(checkCtx, userID)
		return e == nil && view.FilesAvailable
	}
	s.files[access] = struct{}{}
	if !access.valid(ctx) {
		delete(s.files, access)
		cancel()
		return nil, ErrUnavailable
	}
	audit, err := s.Client.WorkspaceAudit.Create().SetRequestID(platformid.New(platformid.Request)).SetActorID(actor).SetUserID(userID).SetAgentSpaceID(row.AgentSpaceID).SetAction(action).SetPath(path).SetOutcome("STARTED").SetBytes(0).SetCreatedAt(s.Now().UTC()).Save(ctx)
	if err != nil {
		delete(s.files, access)
		cancel()
		return nil, err
	}
	access.auditID = audit.ID
	return access, nil
}

// ObserveFileError invalidates only the exact DAV credential which failed. A
// delayed response cannot revoke a newer token or affect MCP availability.
func (s *Service) ObserveFileError(ctx context.Context, access *FileAccess, err error) {
	var remote *remoteapi.Error
	if !errors.As(err, &remote) || remote.Code != "dav_credential_unavailable" {
		return
	}
	_, _ = s.Client.AgentSpace.Update().Where(agentspace.IDEQ(access.userID), agentspace.BindingRevisionEQ(access.bindingRevision), agentspace.DavSecretIDEQ(access.davSecretID), agentspace.DavSecretVersionEQ(access.davSecretVersion)).SetDavConfirmed(false).SetDiagnosticCode(remote.Code).Save(ctx)
}

func (s *Service) CloseFiles(access *FileAccess, outcome string, bytes int64) {
	s.fileMu.Lock()
	delete(s.files, access)
	s.fileMu.Unlock()
	access.cancel()
	access.Token = ""
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.Client.WorkspaceAudit.UpdateOneID(access.auditID).SetOutcome(outcome).SetBytes(bytes).SetCompletedAt(s.Now().UTC()).Save(ctx)
}
func (s *Service) CancelInvalidFiles(ctx context.Context) {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	for access := range s.files {
		if !access.valid(ctx) {
			access.cancel()
		}
	}
}
func (s *Service) RevealDAV(ctx context.Context, actor, userID string, authorize func(context.Context) error) (adminapi.WorkspaceDAVConnection, error) {
	access, err := s.OpenFiles(ctx, actor, userID, "REVEAL_DAV", "", authorize)
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
func (s *Service) ListWorkspaces(ctx context.Context, id, search, cursor string) (adminapi.WorkspaceList, error) {
	query := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id), agentspace.IDGT(cursor))
	if search != "" {
		query = query.Where(agentspace.RemoteUsernameContains(search))
	}
	rows, err := query.Order(ent.Asc(agentspace.FieldID)).Limit(51).All(ctx)
	out := adminapi.WorkspaceList{Items: []adminapi.WorkspaceListItem{}}
	if err != nil {
		return out, err
	}
	if len(rows) > 50 {
		out.NextCursor = ptr(rows[49].ID)
		rows = rows[:50]
	}
	for _, row := range rows {
		view, e := s.Projection(ctx, row.ID)
		if e != nil {
			return out, e
		}
		name := "已删除用户"
		user, e := s.Client.User.Get(ctx, row.ID)
		if e == nil {
			name = user.DisplayName
		} else if !ent.IsNotFound(e) {
			return out, e
		}
		out.Items = append(out.Items, adminapi.WorkspaceListItem{UserId: row.ID, DisplayName: name, RemoteUsername: row.RemoteUsername, Workspace: view})
	}
	return out, nil
}
