package workspace

import (
	"context"
	"encoding/json"

	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/manageddraft"
	"measix/platform/ent/workspaceservice"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// Upgraded active services acquire their derived definition without replaying
// remote provisioning or publishing unrelated administrator edits.
func (s *Service) stageExistingDAV(ctx context.Context) error {
	rows, err := s.Client.WorkspaceService.Query().Where(workspaceservice.StateEQ("ACTIVE"), workspaceservice.EnabledEQ(true), workspaceservice.DavStagedEQ(false)).All(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ActiveConfigRevision == nil {
			continue
		}
		cfg, err := s.config(ctx, row.ID, *row.ActiveConfigRevision)
		if err != nil {
			return err
		}
		if str(cfg.DavOrigin) == "" {
			continue
		}
		tx, err := s.Client.Tx(ctx)
		if err != nil {
			return err
		}
		draft, err := tx.ManagedDraft.Query().Only(ctx)
		if ent.IsNotFound(err) {
			_ = tx.Rollback()
			continue
		}
		if err == nil {
			row, err = tx.WorkspaceService.Get(ctx, row.ID)
			if err == nil {
				err = stageDAV(ctx, tx, row, draft.UpdatedByUserID)
			}
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DAVRuntimeBindings(ctx context.Context, serviceID, resourceID string) (adminapi.AgentSpaceConfig, []relaycontrolapi.UserDAVBinding, error) {
	return s.davRuntimeBindings(ctx, serviceID, resourceID, false)
}

func (s *Service) davRuntimeBindings(ctx context.Context, serviceID, resourceID string, replayRejected bool) (adminapi.AgentSpaceConfig, []relaycontrolapi.UserDAVBinding, error) {
	service, err := s.Client.WorkspaceService.Get(ctx, serviceID)
	if err != nil {
		return adminapi.AgentSpaceConfig{}, nil, err
	}
	revision := service.ActiveConfigRevision
	if service.State == "APPLYING" {
		revision = &service.ConfigRevision
	}
	if revision == nil || service.DavResourceID != resourceID {
		return adminapi.AgentSpaceConfig{}, nil, ErrUnavailable
	}
	cfg, err := s.config(ctx, serviceID, *revision)
	if err != nil {
		return cfg, nil, err
	}
	out := []relaycontrolapi.UserDAVBinding{}
	if !service.Enabled || service.State == "DISABLING" || service.State == "DISABLED" || str(cfg.DavOrigin) == "" {
		return cfg, out, nil
	}
	health := agentspace.DavConfirmedEQ(true)
	if replayRejected {
		health = agentspace.Or(health, agentspace.And(agentspace.DavConfirmedEQ(false), agentspace.DiagnosticCodeEQ("dav_credential_unavailable")))
	}
	spaces, err := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(serviceID), agentspace.IntentEQ("CONNECTED"), agentspace.StateIn("CONNECTED", "CONNECTING", "RESTORING"), agentspace.RemoteActiveEQ(true), agentspace.StopPendingEQ(false), health, agentspace.FileAccessIntentEQ("AUTHORIZED")).All(ctx)
	if err != nil {
		return cfg, nil, err
	}
	for _, space := range spaces {
		user, e := s.Client.User.Get(ctx, space.ID)
		if ent.IsNotFound(e) {
			continue
		}
		if e != nil {
			return cfg, nil, e
		}
		if user.Status != "ACTIVE" {
			continue
		}
		if space.DavSecretID == "" || space.DavSecretVersion < 1 {
			continue
		}
		mode := relaycontrolapi.DAVAccessMode(space.DavAccessMode)
		if !mode.Valid() {
			return cfg, nil, ErrInvalid
		}
		token, e := s.Secrets.ResolveSecret(ctx, space.DavSecretID, int(space.DavSecretVersion))
		if e != nil {
			return cfg, nil, e
		}
		out = append(out, relaycontrolapi.UserDAVBinding{UserId: space.ID, ResourceId: resourceID, AccessMode: mode, Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: serviceID, AgentSpaceId: space.AgentSpaceID, RemoteUsername: space.RemoteUsername, BindingRevision: int(space.DavBindingRevision)}, Endpoint: str(cfg.DavOrigin) + "/u/" + space.RemoteUsername + "/dav/", SecretRef: relaycontrolapi.SecretRef{SecretId: space.DavSecretID, SecretVersion: int(space.DavSecretVersion)}, Token: string(token)})
	}
	return cfg, out, nil
}

func (s *Service) ValidateDAVRuntimeBinding(ctx context.Context, binding relaycontrolapi.UserDAVBinding) error {
	return s.validateDAVBinding(ctx, binding, false)
}

// ValidateDAVReplayBinding only closes an already-persisted pending control
// operation after a health observation. It never grants a new binding or marks
// a rejected key confirmed. Explicit intent, target, key and permission changes
// still fail exact comparison; the next Activation withdraws the rejected key.
func (s *Service) ValidateDAVReplayBinding(ctx context.Context, binding relaycontrolapi.UserDAVBinding) error {
	return s.validateDAVBinding(ctx, binding, true)
}

func (s *Service) validateDAVBinding(ctx context.Context, binding relaycontrolapi.UserDAVBinding, replayRejected bool) error {
	_, bindings, err := s.davRuntimeBindings(ctx, binding.Target.WorkspaceServiceId, binding.ResourceId, replayRejected)
	if err != nil {
		return err
	}
	for _, current := range bindings {
		if current.UserId == binding.UserId && current.ResourceId == binding.ResourceId && current.Target == binding.Target && current.Endpoint == binding.Endpoint && current.SecretRef == binding.SecretRef && current.AccessMode == binding.AccessMode {
			return nil
		}
	}
	return ErrUnavailable
}

func (s *Service) projectDAV(ctx context.Context, out *adminapi.WorkspaceProjection, service *ent.WorkspaceService, space *ent.AgentSpace) {
	if service.DavResourceID == "" {
		return
	}
	enabled := s.published(ctx, service.ID, service.DavResourceID)
	if service.Enabled && enabled {
		out.FileExchangeWebdavResourceId = ptr(service.DavResourceID)
	}
	dav := &adminapi.WorkspaceDAVProjection{ResourceId: service.DavResourceID, Reason: "not_provisioned"}
	out.Webdav = dav
	if space != nil {
		dav.DesiredAccessMode = ptr(adminapi.DAVAccessMode(space.DavAccessMode))
		dav.DesiredBindingRevision = ptr(space.DavBindingRevision)
		if space.DavAppliedAccessMode != "" {
			dav.LastConfirmedAccessMode = ptr(adminapi.DAVAccessMode(space.DavAppliedAccessMode))
		}
		dav.Reason = out.FilesReason
		if out.FilesAvailable {
			if space.DavAppliedRevision != space.DavBindingRevision || space.DavAppliedAccessMode != space.DavAccessMode {
				dav.Reason = "dav_activation_pending"
			} else {
				dav.Available = true
				dav.Reason = "available"
				dav.AccessMode = ptr(adminapi.DAVAccessMode(space.DavAppliedAccessMode))
				dav.BindingRevision = ptr(space.DavAppliedRevision)
			}
		}
	}
	if !enabled {
		dav.Available = false
		dav.Reason = "dav_not_published"
	}
	if !service.Enabled {
		dav.Available = false
		dav.Reason = "workspace_service_unavailable"
	}
	if dav.Available {
		managed, err := s.Client.ManagedState.Get(ctx, "current")
		if err != nil || managed.RuntimeStatus != "READY" {
			dav.Available = false
			dav.Reason = "dav_activation_pending"
		}
	}
	if !dav.Available {
		dav.AccessMode = nil
		dav.BindingRevision = nil
	}
}

func stageDAV(ctx context.Context, tx *ent.Tx, row *ent.WorkspaceService, actor string) error {
	if row.DavStaged {
		return nil
	}
	if platformid.Validate(platformid.WebDAV, row.DavResourceID) != nil || platformid.Validate(platformid.Route, row.DavRuntimeRouteID) != nil {
		return ErrInvalid
	}
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
	if content.Webdav == nil {
		content.Webdav = &[]adminapi.WebDAVDefinition{}
	}
	found := false
	for _, def := range *content.Webdav {
		if def.ResourceId != row.DavResourceID {
			return ErrConflict
		}
		found = true
	}
	if !found {
		*content.Webdav = append(*content.Webdav, adminapi.WebDAVDefinition{ResourceId: row.DavResourceID, DisplayName: "远程工作区文件", Enabled: true, ClientProtocol: "WEBDAV", AuthOwnership: "ENTERPRISE_MANAGED", Purpose: "WORKSPACE_FILE_EXCHANGE", RootPathTemplate: capability.DAVRootPathTemplate(row.DavResourceID)})
		kind := adminapi.RuntimeBindingDefinitionTargetKind("REMOTE_WORKSPACE")
		content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{ResourceId: row.DavResourceID, RuntimeRouteId: row.DavRuntimeRouteID, TargetKind: &kind, WorkspaceServiceId: &row.ID, AllowedMethods: capability.DAVMethods(), AllowedPathPrefixes: []string{"/dav"}, TransportPolicy: "HTTP_REQUEST_RESPONSE"})
		raw, e := json.Marshal(content)
		if e != nil {
			return e
		}
		n, e := tx.ManagedDraft.Update().Where(manageddraft.IDEQ(draft.ID), manageddraft.DraftRevisionEQ(draft.DraftRevision)).SetContentJSON(raw).AddDraftRevision(1).SetUpdatedByUserID(actor).SetUpdatedAt(row.UpdatedAt).Save(ctx)
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrConflict
		}
	}
	_, err = tx.WorkspaceService.UpdateOneID(row.ID).SetDavStaged(true).Save(ctx)
	return err
}

// Each remote credential change has a withdrawal ACK before remote IO and an
// independent apply ACK afterwards. A completed first phase cannot acknowledge
// the second one, including after a process restart.
func (s *Service) davControl(ctx context.Context, op *ent.WorkspaceOperation, phase string) (bool, error) {
	if s.ApplyControl == nil {
		return false, ErrUnavailable
	}
	id, done, err := s.ApplyControl(ctx, op.CreatedByUserID, op.ID+":"+phase)
	if err != nil || !done {
		return false, err
	}
	if id != "" {
		if _, err = s.Client.WorkspaceOperation.UpdateOneID(op.ID).SetActivationID(id).Save(ctx); err != nil {
			return false, err
		}
	}
	return true, nil
}
