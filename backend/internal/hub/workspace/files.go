package workspace

import (
	"context"
	"errors"
	"time"

	"entgo.io/ent/dialect/sql"

	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/user"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

type FileAccess struct {
	Context                           context.Context
	Remote                            *remoteapi.Client
	Username, Token                   string
	Idle                              time.Duration
	valid                             func(context.Context) bool
	cancel                            context.CancelFunc
	slots                             chan struct{}
	auditID                           int
	userID, davSecretID               string
	bindingRevision, davSecretVersion int64
}

// Revalidate fences buffered responses against identity, binding or credential
// changes which happened after admission and before the next cancellation sweep.
func (access *FileAccess) Revalidate(ctx context.Context) error {
	if ctx.Err() != nil || access.Context.Err() != nil || !access.valid(ctx) || access.Context.Err() != nil {
		access.cancel()
		return ErrUnavailable
	}
	return nil
}

func (s *Service) OpenFiles(ctx context.Context, actor, userID, expectedSpaceID, action, path string, authorize func(context.Context) error) (*FileAccess, error) {
	if platformid.Validate(platformid.AgentSpace, expectedSpaceID) != nil {
		return nil, ErrInvalid
	}
	if _, err := remoteapi.FilePath(path, false); err != nil {
		return nil, err
	}
	slots, code := s.fileSlots, "file_transfer_limit"
	if action == "THUMBNAIL" || action == "THUMBNAIL_OPTIONS" {
		slots, code = s.previewSlots, "preview_busy"
	}
	select {
	case slots <- struct{}{}:
	default:
		return nil, &remoteapi.Error{Code: code, Status: 429}
	}
	leaseCtx, cancel := context.WithCancel(ctx)
	access := &FileAccess{Context: leaseCtx, cancel: cancel, slots: slots}
	admitted := false
	defer func() {
		if !admitted {
			s.releaseFileAccess(access)
		}
	}()
	if err := authorize(leaseCtx); err != nil {
		return nil, err
	}
	row, err := s.Client.AgentSpace.Get(leaseCtx, userID)
	if ent.IsNotFound(err) || err == nil && row.AgentSpaceID != expectedSpaceID {
		return nil, &remoteapi.Error{Code: "workspace_space_mismatch", Status: 409}
	}
	if err != nil {
		return nil, err
	}
	workspaceService, err := s.Client.WorkspaceService.Get(leaseCtx, row.WorkspaceServiceID)
	if err != nil {
		return nil, err
	}
	if !s.fileWorkspaceConnected(leaseCtx, row, workspaceService) {
		return nil, ErrUnavailable
	}
	cfg, err := s.config(leaseCtx, workspaceService.ID, *workspaceService.ActiveConfigRevision)
	if err != nil {
		return nil, err
	}
	if !filesAvailable(row, cfg) {
		return nil, ErrUnavailable
	}
	remote, err := s.adapter(leaseCtx, cfg)
	if err != nil {
		return nil, err
	}
	token, err := s.Secrets.ResolveSecret(leaseCtx, row.DavSecretID, int(row.DavSecretVersion))
	if err != nil {
		return nil, err
	}
	access.Remote, access.Username, access.Token = remote, row.RemoteUsername, string(token)
	access.Idle = time.Duration(cfg.IdleTimeoutMs) * time.Millisecond
	access.userID, access.davSecretID = userID, row.DavSecretID
	access.bindingRevision, access.davSecretVersion = row.BindingRevision, row.DavSecretVersion
	access.valid = func(checkCtx context.Context) bool {
		if authorize(checkCtx) != nil {
			return false
		}
		current, e := s.Client.AgentSpace.Get(checkCtx, userID)
		if e != nil || current.BindingRevision != row.BindingRevision || current.AgentSpaceID != row.AgentSpaceID ||
			current.WorkspaceServiceID != row.WorkspaceServiceID || current.RemoteUsername != row.RemoteUsername ||
			current.DavSecretID != row.DavSecretID || current.DavSecretVersion != row.DavSecretVersion || !filesAvailable(current, cfg) {
			return false
		}
		service, e := s.Client.WorkspaceService.Get(checkCtx, workspaceService.ID)
		return e == nil && service.ActiveConfigRevision != nil && *service.ActiveConfigRevision == *workspaceService.ActiveConfigRevision &&
			s.fileWorkspaceConnected(checkCtx, current, service)
	}
	// Register first, then recheck. A revocation either sees this entry or
	// finishes before this check and is already reflected in the durable state.
	s.fileMu.Lock()
	s.files[access] = struct{}{}
	s.fileMu.Unlock()
	if err := access.Revalidate(leaseCtx); err != nil {
		return nil, err
	}
	audit, err := s.Client.WorkspaceAudit.Create().SetRequestID(platformid.New(platformid.Request)).SetActorID(actor).SetUserID(userID).SetAgentSpaceID(row.AgentSpaceID).SetAction(action).SetPath(path).SetOutcome("STARTED").SetBytes(0).SetCreatedAt(s.Now().UTC()).Save(leaseCtx)
	if err != nil {
		return nil, err
	}
	access.auditID = audit.ID
	admitted = true
	return access, nil
}

func (s *Service) fileWorkspaceConnected(ctx context.Context, row *ent.AgentSpace, service *ent.WorkspaceService) bool {
	active, err := s.Client.User.Query().Where(user.IDEQ(row.ID), user.StatusEQ("ACTIVE")).Exist(ctx)
	return workspaceConnected(row, service, err == nil && active)
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

func (s *Service) releaseFileAccess(access *FileAccess) bool {
	s.fileMu.Lock()
	slots := access.slots
	access.slots = nil
	delete(s.files, access)
	s.fileMu.Unlock()
	if slots == nil {
		return false
	}
	access.cancel()
	<-slots
	access.Token = ""
	return true
}

func (s *Service) CloseFiles(access *FileAccess, outcome string, bytes int64) {
	if !s.releaseFileAccess(access) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.Client.WorkspaceAudit.UpdateOneID(access.auditID).SetOutcome(outcome).SetBytes(bytes).SetCompletedAt(s.Now().UTC()).Save(ctx)
}
func (s *Service) CancelInvalidFiles(ctx context.Context) {
	s.fileMu.Lock()
	active := make([]*FileAccess, 0, len(s.files))
	for access := range s.files {
		active = append(active, access)
	}
	s.fileMu.Unlock()
	for _, access := range active {
		if ctx.Err() != nil {
			return
		}
		if access.Context.Err() == nil && !access.valid(ctx) && ctx.Err() == nil {
			access.cancel()
		}
	}
}
func (s *Service) ListWorkspaces(ctx context.Context, id, search, cursor string, discoveryEligible bool) (adminapi.WorkspaceList, error) {
	out := adminapi.WorkspaceList{Items: []adminapi.WorkspaceListItem{}}
	query := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id), agentspace.IDGT(cursor))
	if discoveryEligible {
		service, err := s.Client.WorkspaceService.Get(ctx, id)
		if err != nil {
			return out, err
		}
		if !service.Enabled || service.State != "ACTIVE" || service.ActiveConfigRevision == nil {
			return out, nil
		}
		query.Where(agentspace.StateEQ("CONNECTED"), agentspace.IntentEQ("CONNECTED"), agentspace.RemoteActiveEQ(true), agentspace.StopPendingEQ(false), agentspace.McpSecretIDNEQ(""), agentspace.McpSecretVersionGT(0))
		// AgentSpace deliberately has no user FK: cleanup survives user removal.
		query.Where(func(s *sql.Selector) {
			users := sql.Table(user.Table)
			s.Where(sql.In(s.C(agentspace.FieldID), sql.Select(users.C(user.FieldID)).From(users).Where(sql.EQ(users.C(user.FieldStatus), "ACTIVE"))))
		})
	}
	if search != "" {
		query.Where(func(s *sql.Selector) {
			users := sql.Table(user.Table)
			s.Where(sql.Or(sql.Contains(s.C(agentspace.FieldRemoteUsername), search), sql.In(s.C(agentspace.FieldID), sql.Select(users.C(user.FieldID)).From(users).Where(sql.Or(sql.Contains(users.C(user.FieldDisplayName), search), sql.Contains(users.C(user.FieldUsername), search))))))
		})
	}
	rows, err := query.Order(ent.Asc(agentspace.FieldID)).Limit(51).All(ctx)
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
