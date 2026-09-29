// Package workspace owns optional remote workspace service configuration and durable intent.
// Remote accounts, credentials' validity, disks and files remain Agent Space's.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/workspaceoperation"
	"measix/platform/ent/workspaceservice"
	"measix/platform/ent/workspaceserviceconfig"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

var (
	ErrConflict    = errors.New("workspace_revision_conflict")
	ErrUnavailable = errors.New("workspace_unavailable")
	ErrInvalid     = errors.New("invalid_workspace_request")
	ErrPending     = errors.New("workspace_operation_pending")
)

type Service struct {
	Client  *ent.Client
	Secrets *upstream.Service
	Now     func() time.Time
	// ApplyControl uses runtimecontrol's single Activation writer. It never
	// executes remote management IO while holding the global activation.
	ApplyControl func(context.Context, string, string) (string, bool, error)
	mu           sync.Mutex
	fileMu       sync.Mutex
	files        map[*FileAccess]struct{}
}

func NewService(client *ent.Client, secrets *upstream.Service) *Service {
	return &Service{Client: client, Secrets: secrets, Now: time.Now, files: make(map[*FileAccess]struct{})}
}
func ptr[T any](v T) *T { return &v }
func str(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Service) config(ctx context.Context, id string, revision int64) (adminapi.AgentSpaceConfig, error) {
	row, err := s.Client.WorkspaceServiceConfig.Query().Where(workspaceserviceconfig.WorkspaceServiceIDEQ(id), workspaceserviceconfig.RevisionEQ(revision)).Only(ctx)
	if err != nil {
		return adminapi.AgentSpaceConfig{}, err
	}
	var cfg adminapi.AgentSpaceConfig
	err = json.Unmarshal(row.ConfigJSON, &cfg)
	return cfg, err
}
func (s *Service) adapter(ctx context.Context, cfg adminapi.AgentSpaceConfig) (*remoteapi.Client, error) {
	if s.Secrets == nil {
		return nil, ErrUnavailable
	}
	token, err := s.Secrets.ResolveSecret(ctx, cfg.ManagementSecret.SecretId, cfg.ManagementSecret.SecretVersion)
	if err != nil {
		return nil, err
	}
	client, err := remoteapi.New(cfg.AdminOrigin, cfg.McpOrigin, str(cfg.DavOrigin), string(token))
	if err != nil {
		return nil, err
	}
	transport := client.HTTP.Transport.(*http.Transport)
	transport.DialContext = (&net.Dialer{Timeout: time.Duration(cfg.ConnectTimeoutMs) * time.Millisecond, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = time.Duration(cfg.ConnectTimeoutMs) * time.Millisecond
	client.DAVIdleTimeout = time.Duration(cfg.IdleTimeoutMs) * time.Millisecond
	return client, nil
}
func (s *Service) validateConfig(ctx context.Context, cfg adminapi.AgentSpaceConfig) error {
	// status does not advertise this capability. The operator records the
	// independently verified release identity instead of probing a real user.
	if cfg.ReleaseIdentity != "661d20d8bfe1fb7630a879383257e61602fb6df6" || cfg.ConnectTimeoutMs < 1000 || cfg.ConnectTimeoutMs > 120000 || cfg.IdleTimeoutMs < 1000 || cfg.IdleTimeoutMs > 600000 {
		return ErrInvalid
	}
	_, err := s.adapter(ctx, cfg)
	return err
}
func (s *Service) published(ctx context.Context, workspaceService, mcp string) bool {
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil || managed.ActiveReleaseID == nil {
		return false
	}
	release, err := s.Client.ManagedRelease.Get(ctx, *managed.ActiveReleaseID)
	if err != nil {
		return false
	}
	var content adminapi.ManagedDraftContent
	if json.Unmarshal(release.ReleaseContentJSON, &content) != nil {
		return false
	}
	enabled := false
	for _, m := range content.Mcp {
		if m.McpServerId == mcp && m.Enabled {
			enabled = true
		}
	}
	for _, b := range content.Bindings {
		if enabled && b.ResourceId == mcp && b.TargetKind != nil && *b.TargetKind == "REMOTE_WORKSPACE" && str(b.WorkspaceServiceId) == workspaceService {
			return true
		}
	}
	return false
}
func (s *Service) view(ctx context.Context, row *ent.WorkspaceService) (adminapi.WorkspaceService, error) {
	cfg, err := s.config(ctx, row.ID, row.ConfigRevision)
	if err != nil {
		return adminapi.WorkspaceService{}, err
	}
	out := adminapi.WorkspaceService{WorkspaceServiceId: row.ID, Type: "AGENT_SPACE", Name: row.Name, ConfigRevision: int(row.ConfigRevision), Enabled: row.Enabled, State: adminapi.WorkspaceServiceState(row.State), Config: cfg, McpServerId: row.McpServerID, McpPublished: s.published(ctx, row.ID, row.McpServerID)}
	if row.ActiveConfigRevision != nil {
		out.ActiveConfigRevision = ptr(int(*row.ActiveConfigRevision))
	}
	if row.DiagnosticCode != "" {
		out.DiagnosticCode = ptr(row.DiagnosticCode)
	}
	pending, e := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.WorkspaceServiceIDEQ(row.ID), workspaceoperation.UserIDIsNil(), workspaceoperation.StateNEQ("COMPLETED")).Order(ent.Desc(workspaceoperation.FieldCreatedAt)).First(ctx)
	if e == nil {
		out.OperationId = ptr(pending.ID)
	} else if !ent.IsNotFound(e) {
		return out, e
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, id string) (adminapi.WorkspaceService, error) {
	r, e := s.Client.WorkspaceService.Get(ctx, id)
	if e != nil {
		return adminapi.WorkspaceService{}, e
	}
	return s.view(ctx, r)
}
func (s *Service) List(ctx context.Context) (adminapi.WorkspaceServiceList, error) {
	rows, err := s.Client.WorkspaceService.Query().Order(ent.Asc(workspaceservice.FieldID)).All(ctx)
	out := adminapi.WorkspaceServiceList{Items: []adminapi.WorkspaceService{}}
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		v, e := s.view(ctx, r)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
func (s *Service) Save(ctx context.Context, actor, key, id string, input adminapi.SaveWorkspaceServiceRequest) (adminapi.WorkspaceService, error) {
	if platformid.Validate(platformid.User, actor) != nil || platformid.Validate(platformid.Idempotency, key) != nil || strings.TrimSpace(input.Name) == "" || len(input.Name) > 120 {
		return adminapi.WorkspaceService{}, ErrInvalid
	}
	requestHash := hash(struct {
		ID    string
		Input adminapi.SaveWorkspaceServiceRequest
	}{id, input})
	existing, e := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.CreatedByUserIDEQ(actor), workspaceoperation.IdempotencyKeyEQ(key)).Only(ctx)
	if e == nil {
		if existing.Action != "SAVE" || existing.RequestHash != requestHash {
			return adminapi.WorkspaceService{}, ErrConflict
		}
		var out adminapi.WorkspaceService
		e = json.Unmarshal(existing.ResultJSON, &out)
		return out, e
	}
	if !ent.IsNotFound(e) {
		return adminapi.WorkspaceService{}, e
	}
	if e = s.validateConfig(ctx, input.Config); e != nil {
		return adminapi.WorkspaceService{}, e
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.WorkspaceService{}, err
	}
	defer tx.Rollback()
	now := s.Now().UTC()
	revision := int64(1)
	var row *ent.WorkspaceService
	if id == "" {
		if input.ExpectedRevision != 0 {
			return adminapi.WorkspaceService{}, ErrConflict
		}
		id = platformid.New(platformid.WorkspaceService)
		row, err = tx.WorkspaceService.Create().SetID(id).SetName(strings.TrimSpace(input.Name)).SetConfigRevision(1).SetEnabled(false).SetState("SAVED").SetMcpServerID(platformid.New(platformid.MCP)).SetRuntimeRouteID(platformid.New(platformid.Route)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	} else {
		row, err = tx.WorkspaceService.Get(ctx, id)
		if err != nil {
			return adminapi.WorkspaceService{}, err
		}
		if row.ConfigRevision != int64(input.ExpectedRevision) {
			return adminapi.WorkspaceService{}, ErrConflict
		}
		pending, e := tx.WorkspaceOperation.Query().Where(workspaceoperation.WorkspaceServiceIDEQ(id), workspaceoperation.StateNEQ("COMPLETED")).Exist(ctx)
		if e != nil {
			return adminapi.WorkspaceService{}, e
		}
		if pending {
			return adminapi.WorkspaceService{}, ErrPending
		}
		old, e := tx.WorkspaceServiceConfig.Query().Where(workspaceserviceconfig.WorkspaceServiceIDEQ(id), workspaceserviceconfig.RevisionEQ(row.ConfigRevision)).Only(ctx)
		if e != nil {
			return adminapi.WorkspaceService{}, e
		}
		var oldCfg adminapi.AgentSpaceConfig
		if json.Unmarshal(old.ConfigJSON, &oldCfg) != nil {
			return adminapi.WorkspaceService{}, ErrInvalid
		}
		bound, e := tx.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id)).Exist(ctx)
		if e != nil {
			return adminapi.WorkspaceService{}, e
		}
		if bound && (oldCfg.AdminOrigin != input.Config.AdminOrigin || oldCfg.McpOrigin != input.Config.McpOrigin || str(oldCfg.DavOrigin) != str(input.Config.DavOrigin)) && (input.ConfirmSameDeployment == nil || !*input.ConfirmSameDeployment) {
			return adminapi.WorkspaceService{}, ErrInvalid
		}
		revision = row.ConfigRevision + 1
		row, err = tx.WorkspaceService.UpdateOneID(id).SetName(strings.TrimSpace(input.Name)).SetConfigRevision(revision).SetUpdatedAt(now).Save(ctx)
	}
	if err != nil {
		return adminapi.WorkspaceService{}, err
	}
	data, _ := json.Marshal(input.Config)
	if _, err = tx.WorkspaceServiceConfig.Create().SetWorkspaceServiceID(id).SetRevision(revision).SetConfigJSON(data).SetCreatedByUserID(actor).SetCreatedAt(now).Save(ctx); err != nil {
		return adminapi.WorkspaceService{}, err
	}
	out := adminapi.WorkspaceService{WorkspaceServiceId: id, Type: "AGENT_SPACE", Name: row.Name, ConfigRevision: int(revision), Enabled: row.Enabled, State: adminapi.WorkspaceServiceState(row.State), Config: input.Config, McpServerId: row.McpServerID}
	if row.ActiveConfigRevision != nil {
		out.ActiveConfigRevision = ptr(int(*row.ActiveConfigRevision))
	}
	result, _ := json.Marshal(out)
	_, err = tx.WorkspaceOperation.Create().SetID(platformid.New(platformid.WorkspaceOperation)).SetWorkspaceServiceID(id).SetAction("SAVE").SetIdempotencyKey(key).SetRequestHash(requestHash).SetConfigRevision(revision).SetBindingRevision(0).SetTargetJSON(data).SetResultJSON(result).SetState("COMPLETED").SetStep("SAVED").SetCreatedByUserID(actor).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) Check(ctx context.Context, id string) (adminapi.WorkspaceServiceCheck, error) {
	row, err := s.Client.WorkspaceService.Get(ctx, id)
	if err != nil {
		return adminapi.WorkspaceServiceCheck{}, err
	}
	cfg, err := s.config(ctx, id, row.ConfigRevision)
	if err != nil {
		return adminapi.WorkspaceServiceCheck{}, err
	}
	client, err := s.adapter(ctx, cfg)
	if err != nil {
		return adminapi.WorkspaceServiceCheck{}, err
	}
	out := adminapi.WorkspaceServiceCheck{FilesStatus: "NOT_CONFIGURED"}
	if str(cfg.DavOrigin) != "" {
		out.FilesStatus = "UNVERIFIED"
	}
	if err = client.Check(ctx); err != nil {
		out.DiagnosticCode = ptr(diagnostic(err))
		return out, nil
	}
	rows, err := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id)).All(ctx)
	if err != nil {
		return out, err
	}
	for _, space := range rows {
		if space.AgentSpaceID == "" {
			out.DiagnosticCode = ptr("workspace_target_unconfirmed")
			return out, nil
		}
		remote, e := client.Get(ctx, space.RemoteUsername)
		if e != nil || remote.AgentSpaceID != space.AgentSpaceID {
			out.DiagnosticCode = ptr("workspace_target_mismatch")
			return out, nil
		}
	}
	out.ManagementReady = true
	return out, nil
}
func diagnostic(err error) string {
	var remote *remoteapi.Error
	if errors.As(err, &remote) {
		return remote.Code
	}
	switch {
	case errors.Is(err, ErrConflict):
		return "workspace_revision_conflict"
	case errors.Is(err, ErrInvalid):
		return "invalid_workspace_request"
	case errors.Is(err, ErrPending):
		return "workspace_operation_pending"
	}
	return "workspace_unavailable"
}
func (s *Service) Projection(ctx context.Context, userID string) (adminapi.WorkspaceProjection, error) {
	out := adminapi.WorkspaceProjection{SchemaVersion: 1, ServiceState: "NOT_CONFIGURED", State: "UNPROVISIONED", McpReason: "not_provisioned", FilesReason: "not_provisioned"}
	row, err := s.Client.AgentSpace.Get(ctx, userID)
	if ent.IsNotFound(err) {
		services, e := s.Client.WorkspaceService.Query().All(ctx)
		if e != nil {
			return out, e
		}
		if len(services) > 0 {
			out.ServiceState = "DISABLED"
		}
		for _, service := range services {
			if service.Enabled {
				out.ServiceState = "ENABLED"
				break
			}
		}
		user, e := s.Client.User.Get(ctx, userID)
		if e != nil || user.Status != "ACTIVE" {
			out.McpReason = "user_unavailable"
			out.FilesReason = "user_unavailable"
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.State = adminapi.WorkspaceProjectionState(row.State)
	out.BindingRevision = int(row.BindingRevision)
	out.ObservedAt = row.ObservedAt
	if row.AgentSpaceID != "" {
		out.AgentSpaceId = ptr(row.AgentSpaceID)
	}
	workspaceService, err := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return out, err
	}
	out.McpServerId = ptr(workspaceService.McpServerID)
	out.ServiceState = "DISABLED"
	if workspaceService.Enabled {
		out.ServiceState = "ENABLED"
	}
	reason := "workspace_not_connected"
	user, e := s.Client.User.Get(ctx, userID)
	eligible := e == nil && user.Status == "ACTIVE" && workspaceService.Enabled && workspaceService.State == "ACTIVE" && workspaceService.ActiveConfigRevision != nil && row.Intent == "CONNECTED" && row.State == "CONNECTED" && row.RemoteActive && !row.StopPending
	if !workspaceService.Enabled || workspaceService.State != "ACTIVE" {
		reason = "workspace_service_unavailable"
	} else if e != nil || user.Status != "ACTIVE" {
		reason = "user_unavailable"
	}
	out.McpReason = reason
	out.FilesReason = reason
	if eligible {
		out.McpAvailable = s.published(ctx, workspaceService.ID, workspaceService.McpServerID) && row.McpSecretID != ""
		out.McpReason = "mcp_not_published"
		if out.McpAvailable {
			out.McpReason = "available"
		}
		cfg, e := s.config(ctx, workspaceService.ID, *workspaceService.ActiveConfigRevision)
		if e != nil {
			return out, e
		}
		out.FilesAvailable = str(cfg.DavOrigin) != "" && row.DavConfirmed && row.DavSecretID != ""
		out.FilesReason = "dav_credential_unavailable"
		if str(cfg.DavOrigin) == "" {
			out.FilesReason = "dav_not_configured"
		}
		if out.FilesAvailable {
			out.FilesReason = "available"
		}
	}
	op, e := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.UserIDEQ(userID), workspaceoperation.StateNEQ("COMPLETED")).Order(ent.Desc(workspaceoperation.FieldCreatedAt)).First(ctx)
	if e == nil {
		out.OperationId = ptr(op.ID)
	} else if !ent.IsNotFound(e) {
		return out, e
	}
	return out, nil
}
func operationView(row *ent.WorkspaceOperation) adminapi.WorkspaceOperation {
	out := adminapi.WorkspaceOperation{OperationId: row.ID, WorkspaceServiceId: row.WorkspaceServiceID, Action: row.Action, State: adminapi.WorkspaceOperationState(row.State), Step: row.Step, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.UserID != "" {
		out.UserId = ptr(row.UserID)
	}
	if row.DiagnosticCode != "" {
		out.DiagnosticCode = ptr(row.DiagnosticCode)
	}
	return out
}
func (s *Service) Operation(ctx context.Context, id string) (adminapi.WorkspaceOperation, error) {
	row, err := s.Client.WorkspaceOperation.Get(ctx, id)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	return operationView(row), nil
}
