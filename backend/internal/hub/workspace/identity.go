package workspace

import (
	"context"
	"encoding/json"
	"measix/platform/ent"
	"measix/platform/ent/workspaceoperation"
	"measix/platform/ent/workspaceserviceconfig"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"time"
)

// PreserveDeletionTx runs before the existing user purge, in that same
// transaction. The AgentSpace row and fixed operation target have no user FK.
func PreserveDeletionTx(ctx context.Context, tx *ent.Tx, userID string, now time.Time) error {
	row, err := tx.AgentSpace.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.AgentSpace.UpdateOneID(userID).SetIntent("DELETED").SetState("DELETING").SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	return enqueueLifecycle(ctx, tx, row, "DELETE", now)
}
func enqueueLifecycle(ctx context.Context, tx *ent.Tx, row *ent.AgentSpace, action string, now time.Time) error {
	if row.AgentSpaceID == "" {
		// A lost create response still needs explicit identity verification.
		// Never guess a space ID or replay the create during cleanup.
		return nil
	}
	pending, err := tx.WorkspaceOperation.Query().Where(workspaceoperation.UserIDEQ(row.ID), workspaceoperation.StateNEQ("COMPLETED")).All(ctx)
	if err != nil {
		return err
	}
	for _, op := range pending {
		// Enabling or creating may still be in flight remotely. Unlike a token
		// write, these can undo a subsequent disable and require explicit proof
		// of completion before cleanup can safely supersede them.
		if op.Step == "ENABLE_SENT" || op.Step == "CREATE_SENT" {
			_, err = tx.AgentSpace.UpdateOneID(row.ID).SetDiagnosticCode("revocation_requires_remote_confirmation").Save(ctx)
			return err
		}
		if op.Action == "DELETE" || op.Action == "SUSPEND" || op.Action == "DISCONNECT" {
			return nil
		}
	}
	// Revocation is a separate operation against the known immutable target.
	// It must not wait for recovery of an unrelated credential write, and does
	// not replay that unknown write. Keep its evidence in the superseded row.
	for _, op := range pending {
		if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetState("COMPLETED").SetEvidence("Superseded by identity revocation; prior state=" + op.State + "; step=" + op.Step).SetStep("SUPERSEDED_BY_IDENTITY_REVOCATION").SetUpdatedAt(now).Save(ctx); err != nil {
			return err
		}
	}
	workspaceService, err := tx.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return err
	}
	if workspaceService.ActiveConfigRevision == nil {
		return ErrUnavailable
	}
	config, err := tx.WorkspaceServiceConfig.Query().Where(workspaceserviceconfig.WorkspaceServiceIDEQ(workspaceService.ID), workspaceserviceconfig.RevisionEQ(*workspaceService.ActiveConfigRevision)).Only(ctx)
	if err != nil {
		return err
	}
	var cfg adminapi.AgentSpaceConfig
	if err = json.Unmarshal(config.ConfigJSON, &cfg); err != nil {
		return err
	}
	target, _ := json.Marshal(operationTarget{Config: cfg, Username: row.RemoteUsername, SpaceID: row.AgentSpaceID})
	id := platformid.New(platformid.WorkspaceOperation)
	state := "DISCONNECTING"
	if action == "DELETE" {
		state = "DELETING"
	}
	if _, err = tx.AgentSpace.UpdateOneID(row.ID).SetBindingRevision(row.BindingRevision + 1).SetState(state).SetAppliedControlRevision(0).SetDavConfirmed(false).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	_, err = tx.WorkspaceOperation.Create().SetID(id).SetWorkspaceServiceID(workspaceService.ID).SetUserID(row.ID).SetAction(action).SetIdempotencyKey(platformid.New(platformid.Idempotency)).SetRequestHash(hash(id)).SetConfigRevision(*workspaceService.ActiveConfigRevision).SetBindingRevision(row.BindingRevision + 1).SetTargetJSON(target).SetState("PENDING").SetStep("START").SetCreatedByUserID("system_workspace_cleanup").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	return err
}
func (s *Service) syncIdentity(ctx context.Context) error {
	rows, err := s.Client.AgentSpace.Query().All(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		workspaceService, e := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
		if e != nil {
			return e
		}
		user, e := s.Client.User.Get(ctx, row.ID)
		missing := ent.IsNotFound(e)
		if e != nil && !missing {
			return e
		}
		if row.Intent == "DELETED" || missing || user.Status != "ACTIVE" || !workspaceService.Enabled {
			action := "SUSPEND"
			if row.Intent == "DELETED" || missing {
				action = "DELETE"
			}
			if action == "SUSPEND" && !row.RemoteActive {
				continue
			}
			tx, e := s.Client.Tx(ctx)
			if e != nil {
				return e
			}
			e = enqueueLifecycle(ctx, tx, row, action, s.Now().UTC())
			if e == nil {
				e = tx.Commit()
			} else {
				tx.Rollback()
			}
			if e != nil {
				return e
			}
		} else if row.Intent == "CONNECTED" && row.State == "DISCONNECTED" && workspaceService.State == "ACTIVE" {
			// A system resume preserves prior connection intent. It never restores DAV.
			_, e = s.Command(ctx, user.ID, platformid.New(platformid.Idempotency), user.ID, adminapi.WorkspaceCommand{Action: "RESTORE", ExpectedRevision: int(row.BindingRevision)})
			if e != nil && e != ErrPending {
				return e
			}
		}
	}
	return nil
}
