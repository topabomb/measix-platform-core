package workspace

import (
	"context"
	"encoding/json"
	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/workspaceoperation"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

func (s *Service) RuntimeBindings(ctx context.Context, id, mcp string) (adminapi.AgentSpaceConfig, []relaycontrolapi.UserRuntimeBinding, error) {
	row, err := s.Client.WorkspaceService.Get(ctx, id)
	if err != nil {
		return adminapi.AgentSpaceConfig{}, nil, err
	}
	revision := row.ActiveConfigRevision
	if row.State == "APPLYING" {
		revision = &row.ConfigRevision
	}
	if revision == nil || row.McpServerID != mcp {
		return adminapi.AgentSpaceConfig{}, nil, ErrUnavailable
	}
	cfg, err := s.config(ctx, id, *revision)
	if err != nil {
		return cfg, nil, err
	}
	out := []relaycontrolapi.UserRuntimeBinding{}
	if !row.Enabled || row.State == "DISABLING" || row.State == "DISABLED" {
		return cfg, out, nil
	}
	rows, err := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id), agentspace.IntentEQ("CONNECTED"), agentspace.StateIn("CONNECTING", "RESTORING", "CONNECTED"), agentspace.RemoteActiveEQ(true), agentspace.StopPendingEQ(false)).All(ctx)
	if err != nil {
		return cfg, nil, err
	}
	for _, space := range rows {
		user, e := s.Client.User.Get(ctx, space.ID)
		if ent.IsNotFound(e) {
			continue
		}
		if e != nil {
			return cfg, nil, e
		}
		if user.Status != "ACTIVE" || space.McpSecretID == "" || space.AgentSpaceID == "" {
			continue
		}
		token, e := s.Secrets.ResolveSecret(ctx, space.McpSecretID, int(space.McpSecretVersion))
		if e != nil {
			return cfg, nil, e
		}
		out = append(out, relaycontrolapi.UserRuntimeBinding{UserId: space.ID, McpServerId: mcp, Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: id, AgentSpaceId: space.AgentSpaceID, RemoteUsername: space.RemoteUsername, BindingRevision: int(space.BindingRevision)}, Endpoint: cfg.McpOrigin + "/u/" + space.RemoteUsername + "/mcp", SecretRef: relaycontrolapi.SecretRef{SecretId: space.McpSecretID, SecretVersion: int(space.McpSecretVersion)}, Token: string(token)})
	}
	return cfg, out, nil
}

// stageDefinition mutates only the existing capability draft in the caller transaction.
// It never publishes unrelated changes or creates a per-user MCP definition.
func stageDefinition(ctx context.Context, tx *ent.Tx, row *ent.WorkspaceService, actor string) error {
	draft, err := tx.ManagedDraft.Query().Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var content adminapi.ManagedDraftContent
	if err = json.Unmarshal(draft.ContentJSON, &content); err != nil {
		return err
	}
	for _, m := range content.Mcp {
		if m.McpServerId == row.McpServerID {
			return nil
		}
	}
	content.Mcp = append(content.Mcp, adminapi.McpDefinition{McpServerId: row.McpServerID, DisplayName: "远程工作区", Enabled: true, ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "ENTERPRISE_MANAGED", RuntimePath: "/mcp"})
	kind := adminapi.RuntimeBindingDefinitionTargetKind("REMOTE_WORKSPACE")
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{ResourceId: row.McpServerID, RuntimeRouteId: row.RuntimeRouteID, TargetKind: &kind, WorkspaceServiceId: &row.ID, AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: "HTTP_STREAMING_SSE"})
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	_, err = tx.ManagedDraft.UpdateOneID(draft.ID).SetContentJSON(data).SetDraftRevision(draft.DraftRevision + 1).SetUpdatedByUserID(actor).SetUpdatedAt(row.UpdatedAt).Save(ctx)
	return err
}

// ValidateRuntimeBinding prevents restart recovery from reviving an older local
// intent. The caller retains the immutable descriptor; it never edits its hash.
func (s *Service) ValidateRuntimeBinding(ctx context.Context, binding relaycontrolapi.UserRuntimeBinding) error {
	row, err := s.Client.AgentSpace.Get(ctx, binding.UserId)
	if err != nil {
		return err
	}
	if row.Intent != "CONNECTED" || (row.State != "CONNECTED" && row.State != "CONNECTING" && row.State != "RESTORING") || !row.RemoteActive || row.StopPending || row.WorkspaceServiceID != binding.Target.WorkspaceServiceId || row.AgentSpaceID != binding.Target.AgentSpaceId || row.RemoteUsername != binding.Target.RemoteUsername || row.BindingRevision != int64(binding.Target.BindingRevision) || row.McpSecretID != binding.SecretRef.SecretId || row.McpSecretVersion != int64(binding.SecretRef.SecretVersion) {
		return ErrUnavailable
	}
	user, err := s.Client.User.Get(ctx, row.ID)
	if err != nil {
		return err
	}
	if user.Status != "ACTIVE" {
		return ErrUnavailable
	}
	workspaceService, err := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return err
	}
	if !workspaceService.Enabled || workspaceService.ActiveConfigRevision == nil || workspaceService.McpServerID != binding.McpServerId {
		return ErrUnavailable
	}
	revision := *workspaceService.ActiveConfigRevision
	if workspaceService.State == "APPLYING" {
		revision = workspaceService.ConfigRevision
	}
	cfg, err := s.config(ctx, workspaceService.ID, revision)
	if err != nil {
		return err
	}
	if binding.Endpoint != cfg.McpOrigin+"/u/"+row.RemoteUsername+"/mcp" {
		return ErrUnavailable
	}
	return nil
}

// StageMCP is an explicit draft edit. Service configuration and file-only
// provisioning never call it; the administrator still owns normal publication.
func (s *Service) StageMCP(ctx context.Context, actor, key, id string, expectedRevision int) error {
	if platformid.Validate(platformid.User, actor) != nil || platformid.Validate(platformid.Idempotency, key) != nil {
		return ErrInvalid
	}
	requestHash := hash(struct {
		ID       string
		Revision int
	}{id, expectedRevision})
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	prior, err := tx.WorkspaceOperation.Query().Where(workspaceoperation.CreatedByUserIDEQ(actor), workspaceoperation.IdempotencyKeyEQ(key)).Only(ctx)
	if err == nil {
		if prior.Action != "MCP_DRAFT" || prior.RequestHash != requestHash {
			return ErrConflict
		}
		return nil
	}
	if !ent.IsNotFound(err) {
		return err
	}
	row, err := tx.WorkspaceService.Get(ctx, id)
	if err != nil {
		return err
	}
	if !row.Enabled || row.State != "ACTIVE" {
		return ErrUnavailable
	}
	draft, err := tx.ManagedDraft.Query().Only(ctx)
	if err != nil {
		return err
	}
	if draft.DraftRevision != int64(expectedRevision) {
		return ErrConflict
	}
	if err := stageDefinition(ctx, tx, row, actor); err != nil {
		return err
	}
	now := s.Now().UTC()
	_, err = tx.WorkspaceOperation.Create().SetID(platformid.New(platformid.WorkspaceOperation)).SetWorkspaceServiceID(id).SetAction("MCP_DRAFT").SetIdempotencyKey(key).SetRequestHash(requestHash).SetConfigRevision(row.ConfigRevision).SetBindingRevision(0).SetTargetJSON([]byte(`{}`)).SetState("COMPLETED").SetStep("DONE").SetCreatedByUserID(actor).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return err
	}
	return tx.Commit()
}
