package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/secretversion"
	"measix/platform/ent/workspaceoperation"
	"measix/platform/ent/workspaceservice"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

type operationTarget struct {
	Config   adminapi.AgentSpaceConfig `json:"config"`
	Username string                    `json:"username"`
	SpaceID  string                    `json:"agentSpaceId"`
	// Capture the prior availability in the same transaction as APPLY intent.
	// A failed read-only check must not enable a previously disabled service.
	PreviousEnabled bool   `json:"previousEnabled,omitempty"`
	PreviousState   string `json:"previousState,omitempty"`
}

func (s *Service) WorkspaceServiceCommand(ctx context.Context, actor, key, id, action string) (adminapi.WorkspaceOperation, error) {
	if action != "APPLY" && action != "DISABLE" {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	row, err := s.Client.WorkspaceService.Get(ctx, id)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	cfg, err := s.config(ctx, id, row.ConfigRevision)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	return s.begin(ctx, actor, key, id, "", action, row.ConfigRevision, 0, operationTarget{Config: cfg}, nil)
}
func (s *Service) Command(ctx context.Context, actor, key, userID string, input adminapi.WorkspaceCommand) (adminapi.WorkspaceOperation, error) {
	if platformid.Validate(platformid.User, userID) != nil {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	if input.ManagementSecret != nil && input.Action != "CONTINUE" {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	if input.Action == "REQUERY" || input.Action == "CONTINUE" {
		return s.resume(ctx, actor, userID, input)
	}
	existing, err := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.CreatedByUserIDEQ(actor), workspaceoperation.IdempotencyKeyEQ(key)).Only(ctx)
	if err == nil {
		if existing.UserID != userID || existing.RequestHash != hash(input) {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		return operationView(existing), nil
	}
	if !ent.IsNotFound(err) {
		return adminapi.WorkspaceOperation{}, err
	}
	row, err := s.Client.AgentSpace.Get(ctx, userID)
	fresh := ent.IsNotFound(err)
	if err != nil && !fresh {
		return adminapi.WorkspaceOperation{}, err
	}
	var workspaceService *ent.WorkspaceService
	if fresh {
		if input.Action != "CREATE" && input.Action != "TAKEOVER" {
			return adminapi.WorkspaceOperation{}, ErrUnavailable
		}
		if input.ExpectedRevision != 0 {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		workspaceService, err = s.Client.WorkspaceService.Query().Where(workspaceservice.EnabledEQ(true), workspaceservice.StateEQ("ACTIVE")).Only(ctx)
	} else {
		if row.BindingRevision != int64(input.ExpectedRevision) {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		workspaceService, err = s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	}
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if workspaceService.ActiveConfigRevision == nil {
		return adminapi.WorkspaceOperation{}, ErrUnavailable
	}
	cfg, err := s.config(ctx, workspaceService.ID, *workspaceService.ActiveConfigRevision)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if input.Action == "SET_DAV" && str(cfg.DavOrigin) == "" {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	target := operationTarget{Config: cfg}
	if fresh {
		target.Username, err = remoteapi.Username(userID)
	} else {
		target.Username = row.RemoteUsername
		target.SpaceID = row.AgentSpaceID
	}
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	switch input.Action {
	case "CREATE":
		if !fresh {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
	case "TAKEOVER":
		if strings.TrimSpace(str(input.Evidence)) == "" || input.RemoteUsername == nil || input.AgentSpaceId == nil || platformid.Validate(platformid.AgentSpace, *input.AgentSpaceId) != nil {
			return adminapi.WorkspaceOperation{}, ErrInvalid
		}
		if !fresh && target.Username != *input.RemoteUsername {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		target.Username = *input.RemoteUsername
		target.SpaceID = *input.AgentSpaceId
		client, e := s.adapter(ctx, cfg)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		observed, e := client.Get(ctx, target.Username)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		if observed.AgentSpaceID != target.SpaceID {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
	case "RESTORE":
		if fresh || row.State != "DISCONNECTED" || row.StopPending {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
	case "DELETE":
		if target.SpaceID == "" || str(input.Confirmation) != target.SpaceID {
			return adminapi.WorkspaceOperation{}, ErrInvalid
		}
	case "DISCONNECT", "RESET_MCP", "SET_DAV", "REVOKE_DAV":
	default:
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	if input.Action == "CREATE" || input.Action == "RESTORE" || input.Action == "TAKEOVER" || input.Action == "RESET_MCP" || input.Action == "SET_DAV" {
		user, e := s.Client.User.Get(ctx, userID)
		if e != nil || user.Status != "ACTIVE" || !workspaceService.Enabled || workspaceService.State != "ACTIVE" {
			return adminapi.WorkspaceOperation{}, ErrUnavailable
		}
	}
	return s.begin(ctx, actor, key, workspaceService.ID, userID, string(input.Action), *workspaceService.ActiveConfigRevision, int64(input.ExpectedRevision), target, &input)
}

func (s *Service) begin(ctx context.Context, actor, key, workspaceService, userID, action string, configRevision, bindingRevision int64, target operationTarget, input *adminapi.WorkspaceCommand) (adminapi.WorkspaceOperation, error) {
	if platformid.Validate(platformid.User, actor) != nil || platformid.Validate(platformid.Idempotency, key) != nil {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	requestHash := hash(struct {
		ID, Action string
		Revision   int64
	}{workspaceService, action, configRevision})
	if input != nil {
		requestHash = hash(*input)
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	defer tx.Rollback()
	prior, err := tx.WorkspaceOperation.Query().Where(workspaceoperation.CreatedByUserIDEQ(actor), workspaceoperation.IdempotencyKeyEQ(key)).Only(ctx)
	if err == nil {
		if prior.RequestHash != requestHash {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		return operationView(prior), nil
	}
	if !ent.IsNotFound(err) {
		return adminapi.WorkspaceOperation{}, err
	}
	controlPending, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Exist(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if controlPending {
		return adminapi.WorkspaceOperation{}, ErrPending
	}
	pending := tx.WorkspaceOperation.Query().Where(workspaceoperation.WorkspaceServiceIDEQ(workspaceService), workspaceoperation.StateNEQ("COMPLETED"))
	if userID != "" {
		pending = pending.Where(workspaceoperation.Or(workspaceoperation.UserIDEQ(userID), workspaceoperation.UserIDIsNil()))
	}
	busy, err := pending.Clone().Exist(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if busy {
		// A lost create response cannot be replayed. Explicit takeover of the
		// observed original account is the only permitted replacement operation.
		prior, e := pending.Only(ctx)
		if e != nil || action != "TAKEOVER" || prior.Action != "CREATE" || prior.State != "UNKNOWN" || prior.Step != "CREATE_SENT" {
			return adminapi.WorkspaceOperation{}, ErrPending
		}
		if _, e = tx.WorkspaceOperation.UpdateOneID(prior.ID).SetState("COMPLETED").SetStep("SUPERSEDED_BY_VERIFIED_TAKEOVER").SetEvidence(actor + ": " + str(input.Evidence)).Save(ctx); e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
	}
	now := s.Now().UTC()
	intent, state := "CONNECTED", "CONNECTING"
	switch action {
	case "DISCONNECT":
		intent, state = "DISCONNECTED", "DISCONNECTING"
	case "DELETE":
		intent, state = "DELETED", "DELETING"
	case "RESTORE", "TAKEOVER", "RESET_MCP":
		state = "RESTORING"
	}
	if userID != "" {
		row, e := tx.AgentSpace.Get(ctx, userID)
		if ent.IsNotFound(e) {
			builder := tx.AgentSpace.Create().SetID(userID).SetWorkspaceServiceID(workspaceService).SetRemoteUsername(target.Username).SetBindingRevision(1).SetIntent(intent).SetState(state).SetRemoteActive(false).SetStopPending(false).SetDavConfirmed(false).SetCreatedAt(now).SetUpdatedAt(now)
			if target.SpaceID != "" {
				builder.SetAgentSpaceID(target.SpaceID)
			}
			if _, err = builder.Save(ctx); err != nil {
				return adminapi.WorkspaceOperation{}, err
			}
		} else {
			if e != nil {
				return adminapi.WorkspaceOperation{}, e
			}
			if row.BindingRevision != bindingRevision {
				return adminapi.WorkspaceOperation{}, ErrConflict
			}
			update := tx.AgentSpace.UpdateOneID(userID).SetBindingRevision(bindingRevision + 1).SetUpdatedAt(now)
			if action == "SET_DAV" || action == "REVOKE_DAV" {
				// DAV versions have their own immutable reference. Rotating DAV must
				// not change the MCP runtime target or interrupt an MCP session.
				update.SetDavConfirmed(false).SetBindingRevision(bindingRevision)
			} else {
				update.SetIntent(intent).SetState(state).SetAppliedControlRevision(0)
				if action == "TAKEOVER" {
					update.SetAgentSpaceID(target.SpaceID).SetDavConfirmed(false)
				}
			}
			if _, err = update.Save(ctx); err != nil {
				return adminapi.WorkspaceOperation{}, err
			}
		}
		if action != "SET_DAV" && action != "REVOKE_DAV" {
			bindingRevision++
		}
	} else {
		prior, e := tx.WorkspaceService.Get(ctx, workspaceService)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		if prior.ConfigRevision != configRevision {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		target.PreviousEnabled, target.PreviousState = prior.Enabled, prior.State
		update := tx.WorkspaceService.UpdateOneID(workspaceService).SetUpdatedAt(now)
		if action == "APPLY" {
			update.SetEnabled(true).SetState("APPLYING")
		} else {
			update.SetEnabled(false).SetState("DISABLING")
		}
		if _, err = update.Save(ctx); err != nil {
			return adminapi.WorkspaceOperation{}, err
		}
	}
	data, _ := json.Marshal(target)
	builder := tx.WorkspaceOperation.Create().SetID(platformid.New(platformid.WorkspaceOperation)).SetWorkspaceServiceID(workspaceService).SetAction(action).SetIdempotencyKey(key).SetRequestHash(requestHash).SetConfigRevision(configRevision).SetBindingRevision(bindingRevision).SetTargetJSON(data).SetState("PENDING").SetStep("START").SetCreatedByUserID(actor).SetCreatedAt(now).SetUpdatedAt(now)
	if userID != "" {
		builder.SetUserID(userID)
	}
	if input != nil && input.Evidence != nil {
		builder.SetEvidence(*input.Evidence)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	return operationView(row), nil
}

func (s *Service) resume(ctx context.Context, actor, userID string, input adminapi.WorkspaceCommand) (adminapi.WorkspaceOperation, error) {
	row, err := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.UserIDEQ(userID), workspaceoperation.StateIn("UNKNOWN", "NEEDS_ATTENTION")).Only(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	var target operationTarget
	if json.Unmarshal(row.TargetJSON, &target) != nil {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	if input.Action == "CONTINUE" && strings.TrimSpace(str(input.Evidence)) == "" {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	if input.ManagementSecret != nil {
		// Recovery replaces only the secret reference; addresses and account
		// identity remain pinned to the original operation, never latest config.
		target.Config.ManagementSecret = *input.ManagementSecret
	}
	client, err := s.adapter(ctx, target.Config)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	remote, err := client.Get(ctx, target.Username)
	if input.Action == "REQUERY" {
		code := "remote_observed_requires_verification"
		if err != nil {
			code = diagnostic(err)
		} else if target.SpaceID != "" && remote.AgentSpaceID != target.SpaceID {
			code = "remote_identity_mismatch"
		}
		row, err = s.Client.WorkspaceOperation.UpdateOneID(row.ID).SetDiagnosticCode(code).SetUpdatedAt(s.Now().UTC()).Save(ctx)
		if err != nil {
			return adminapi.WorkspaceOperation{}, err
		}
		return operationView(row), nil
	}
	space, e := s.Client.AgentSpace.Get(ctx, userID)
	if e != nil {
		return adminapi.WorkspaceOperation{}, e
	}
	if space.BindingRevision != int64(input.ExpectedRevision) {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	if target.SpaceID == "" {
		target.SpaceID = space.AgentSpaceID
	}
	if target.SpaceID == "" && row.Action == "CREATE" && input.AgentSpaceId != nil && str(input.RemoteUsername) == target.Username && platformid.Validate(platformid.AgentSpace, *input.AgentSpaceId) == nil {
		target.SpaceID = *input.AgentSpaceId
	}
	if strings.TrimSpace(str(input.Evidence)) == "" || (row.Action == "CREATE" && target.SpaceID == "") {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	if row.Action == "DELETE" && remoteapi.IsNotFound(err) {
		row, err = s.Client.WorkspaceOperation.UpdateOneID(row.ID).SetStep("DONE").SetState("PENDING").SetEvidence(actor + ": " + *input.Evidence).Save(ctx)
		if err != nil {
			return adminapi.WorkspaceOperation{}, err
		}
		return operationView(row), nil
	}
	if err != nil || remote.AgentSpaceID != target.SpaceID {
		return adminapi.WorkspaceOperation{}, ErrUnavailable
	}
	user, userErr := s.Client.User.Get(ctx, userID)
	if userErr != nil && !ent.IsNotFound(userErr) {
		return adminapi.WorkspaceOperation{}, userErr
	}
	workspaceService, loadErr := s.Client.WorkspaceService.Get(ctx, space.WorkspaceServiceID)
	if loadErr != nil {
		return adminapi.WorkspaceOperation{}, loadErr
	}
	if user == nil || user.Status != "ACTIVE" || !workspaceService.Enabled || space.Intent == "DELETED" {
		// The administrator's evidence confirms that the old enabling request
		// ended. Retire it without replaying it; the current revoke/delete intent
		// is then handled by the same lifecycle reconciler.
		tx, e := s.Client.Tx(ctx)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		defer tx.Rollback()
		currentOp, e := tx.WorkspaceOperation.Get(ctx, row.ID)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		fresh, e := tx.AgentSpace.Get(ctx, userID)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		if currentOp.State != row.State || currentOp.Step != row.Step || fresh.BindingRevision != int64(input.ExpectedRevision) {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		if _, e = tx.AgentSpace.UpdateOneID(userID).SetAgentSpaceID(target.SpaceID).SetRemoteActive(remote.Status == "active").Save(ctx); e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		data, _ := json.Marshal(target)
		row, e = tx.WorkspaceOperation.UpdateOneID(row.ID).SetTargetJSON(data).SetState("COMPLETED").SetStep("REMOTE_QUIESCENCE_CONFIRMED").SetEvidence(actor + ": " + *input.Evidence).Save(ctx)
		if e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		if e = tx.Commit(); e != nil {
			return adminapi.WorkspaceOperation{}, e
		}
		return operationView(row), nil
	}
	if row.Action == "CREATE" && row.Step == "CREATE_SENT" {
		// Eligible users must use takeover, which revokes prior credentials.
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	step := strings.TrimSuffix(row.Step, "_SENT")
	switch step {
	case "DAV":
		step = "DAV_READY"
	case "DAV_REVOKE":
		step = "START"
	case "MCP":
		step = "MCP_READY"
	case "DISABLE":
		step = "CONTROL_DONE"
		if row.Action == "TAKEOVER" {
			step = "START"
		}
	case "DELETE":
		step = "CONTROL_DONE"
	case "ENABLE":
		step = "START"
		if row.Action == "TAKEOVER" {
			step = "TAKEOVER_WAIT"
		}
	case "START", "CONTROL", "CONTROL_DONE", "STOP_WAIT", "DELETE_WAIT", "TAKEOVER_WAIT", "MCP_READY", "DAV_READY", "DAV_PREPARE", "REMOTE_CREATED", "DONE":
	default:
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	defer tx.Rollback()
	currentOp, err := tx.WorkspaceOperation.Get(ctx, row.ID)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if currentOp.State != row.State || currentOp.Step != row.Step {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	fresh, err := tx.AgentSpace.Get(ctx, userID)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if fresh.BindingRevision != int64(input.ExpectedRevision) {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	// Recovery must restore the state consumed by the runtime compiler in the
	// same transaction as the operation. Otherwise a successful retry could
	// acknowledge an empty binding set and falsely report MCP as available.
	update := tx.AgentSpace.UpdateOneID(userID).ClearDiagnosticCode()
	if fresh.State == "NEEDS_ATTENTION" {
		switch row.Action {
		case "CREATE":
			update.SetState("CONNECTING")
		case "RESTORE", "TAKEOVER", "RESET_MCP":
			update.SetState("RESTORING")
		case "DISCONNECT", "SUSPEND":
			update.SetState("DISCONNECTING")
		case "DELETE":
			update.SetState("DELETING")
		}
	}
	if _, err = update.Save(ctx); err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	data, _ := json.Marshal(target)
	row, err = tx.WorkspaceOperation.UpdateOneID(row.ID).SetTargetJSON(data).SetStep(step).SetState("PENDING").SetEvidence(actor + ": " + *input.Evidence).ClearDiagnosticCode().SetUpdatedAt(s.Now().UTC()).Save(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	return operationView(row), nil
}

// Reconcile is called from the Hub's existing background loop. Unknown writes
// remain blocked; no page read or timer silently rotates credentials.
func (s *Service) Reconcile(ctx context.Context) error {
	if !s.mu.TryLock() {
		return nil
	}
	defer s.mu.Unlock()
	if err := s.syncIdentity(ctx); err != nil {
		return err
	}
	rows, err := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.StateIn("PENDING", "RUNNING")).Order(ent.Asc(workspaceoperation.FieldUpdatedAt)).Limit(50).All(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for step := 0; step < 12; step++ {
			current, e := s.Client.WorkspaceOperation.Get(ctx, row.ID)
			if e != nil {
				return e
			}
			if current.State != "PENDING" && current.State != "RUNNING" {
				break
			}
			advanced, e := s.advance(ctx, current)
			if e != nil {
				if current.Step == "CONTROL" {
					// Runtime Control owns the durable Activation and its ACK recovery.
					// Keep the binding intent intact while retrying that same operation;
					// NEEDS_ATTENTION here would invalidate its persisted descriptor.
					_, e = s.Client.WorkspaceOperation.UpdateOneID(current.ID).SetDiagnosticCode(diagnostic(e)).SetUpdatedAt(s.Now().UTC()).Save(ctx)
					if e != nil {
						return e
					}
					break
				}
				_ = s.fail(ctx, current, e)
				break
			}
			if !advanced {
				break
			}
		}
	}
	return nil
}
func (s *Service) step(ctx context.Context, row *ent.WorkspaceOperation, step string) error {
	_, err := s.Client.WorkspaceOperation.UpdateOneID(row.ID).SetStep(step).SetState("RUNNING").SetUpdatedAt(s.Now().UTC()).Save(ctx)
	return err
}
func (s *Service) fail(ctx context.Context, row *ent.WorkspaceOperation, err error) error {
	if fresh, e := s.Client.WorkspaceOperation.Get(ctx, row.ID); e == nil {
		row = fresh
	}
	var remote *remoteapi.Error
	state := "NEEDS_ATTENTION"
	if errors.As(err, &remote) && remote.Unknown || strings.HasSuffix(row.Step, "_SENT") {
		state = "UNKNOWN"
	}
	_, e := s.Client.WorkspaceOperation.UpdateOneID(row.ID).SetState(state).SetDiagnosticCode(diagnostic(err)).SetUpdatedAt(s.Now().UTC()).Save(ctx)
	if row.UserID != "" {
		update := s.Client.AgentSpace.UpdateOneID(row.UserID).SetDiagnosticCode(diagnostic(err))
		if row.Action != "SET_DAV" && row.Action != "REVOKE_DAV" && !strings.HasPrefix(row.Step, "DAV_") {
			update.SetState("NEEDS_ATTENTION")
		}
		_, _ = update.Save(ctx)
	} else if row.Action == "APPLY" && row.Step == "START" {
		// No remote write or Relay activation started. A failed read-only check
		// leaves the previously applied configuration authoritative and editable.
		var target operationTarget
		if json.Unmarshal(row.TargetJSON, &target) != nil {
			return ErrInvalid
		}
		if target.PreviousState == "" {
			target.PreviousState = "SAVED"
		}
		tx, txErr := s.Client.Tx(ctx)
		if txErr != nil {
			return txErr
		}
		defer tx.Rollback()
		if _, txErr = tx.WorkspaceService.UpdateOneID(row.WorkspaceServiceID).SetEnabled(target.PreviousEnabled).SetState(target.PreviousState).SetDiagnosticCode(diagnostic(err)).Save(ctx); txErr != nil {
			return txErr
		}
		if _, txErr = tx.WorkspaceOperation.UpdateOneID(row.ID).SetState("COMPLETED").SetStep("CHECK_FAILED").Save(ctx); txErr != nil {
			return txErr
		}
		return tx.Commit()
	}
	return e
}
func (s *Service) advance(ctx context.Context, op *ent.WorkspaceOperation) (bool, error) {
	var target operationTarget
	if json.Unmarshal(op.TargetJSON, &target) != nil {
		return false, ErrInvalid
	}
	client, err := s.adapter(ctx, target.Config)
	if err != nil {
		return false, err
	}
	if strings.HasSuffix(op.Step, "_SENT") {
		return false, &remoteapi.Error{Code: "management_result_unknown", Unknown: true}
	}
	if op.UserID == "" {
		return s.advanceWorkspaceService(ctx, op, client)
	}
	space, err := s.Client.AgentSpace.Get(ctx, op.UserID)
	if err != nil {
		return false, err
	}
	if space.BindingRevision != op.BindingRevision {
		return false, ErrConflict
	}
	if op.Action == "CREATE" || op.Action == "RESTORE" || op.Action == "TAKEOVER" || op.Action == "RESET_MCP" || op.Action == "SET_DAV" {
		user, e := s.Client.User.Get(ctx, space.ID)
		if e != nil && !ent.IsNotFound(e) {
			return false, e
		}
		workspaceService, loadErr := s.Client.WorkspaceService.Get(ctx, space.WorkspaceServiceID)
		if loadErr != nil {
			return false, loadErr
		}
		if user == nil || user.Status != "ACTIVE" || !workspaceService.Enabled || space.Intent != "CONNECTED" {
			return false, ErrUnavailable
		}
	}
	if target.SpaceID == "" && space.AgentSpaceID != "" {
		target.SpaceID = space.AgentSpaceID
	}
	switch op.Step {
	case "START":
		switch op.Action {
		case "CREATE":
			if err = s.step(ctx, op, "CREATE_SENT"); err != nil {
				return false, err
			}
			credential, e := client.Create(ctx, target.Username)
			if e != nil {
				return false, e
			}
			return true, s.saveMCP(ctx, op, credential.AgentSpaceID, credential.Token, "REMOTE_CREATED")
		case "RESTORE":
			remote, e := client.Get(ctx, target.Username)
			if e != nil {
				return false, e
			}
			if remote.AgentSpaceID != target.SpaceID || remote.StopPending {
				return false, ErrUnavailable
			}
			if err = s.step(ctx, op, "ENABLE_SENT"); err != nil {
				return false, err
			}
			if _, err = client.SetEnabled(ctx, target.Username, target.SpaceID, true); err != nil {
				return false, err
			}
			return true, s.step(ctx, op, "MCP_READY")
		case "TAKEOVER":
			remote, e := client.Get(ctx, target.Username)
			if e != nil {
				return false, e
			}
			if remote.AgentSpaceID != target.SpaceID {
				return false, ErrConflict
			}
			if err = s.step(ctx, op, "DISABLE_SENT"); err != nil {
				return false, err
			}
			if _, err = client.SetEnabled(ctx, target.Username, target.SpaceID, false); err != nil {
				return false, err
			}
			return true, s.step(ctx, op, "TAKEOVER_WAIT")
		case "SET_DAV":
			return true, s.prepareDAV(ctx, op, space)
		case "REVOKE_DAV":
			if err = s.step(ctx, op, "DAV_REVOKE_SENT"); err != nil {
				return false, err
			}
			if err = client.RevokeDAV(ctx, target.Username, target.SpaceID); err != nil {
				return false, err
			}
			return true, s.clearDAV(ctx, op)
		case "RESET_MCP":
			return true, s.step(ctx, op, "MCP_READY")
		default:
			return true, s.step(ctx, op, "CONTROL")
		}
	case "TAKEOVER_WAIT":
		remote, e := client.Get(ctx, target.Username)
		if e != nil {
			return false, e
		}
		if remote.AgentSpaceID != target.SpaceID {
			return false, ErrConflict
		}
		if remote.StopPending {
			return false, nil
		}
		if err = s.step(ctx, op, "ENABLE_SENT"); err != nil {
			return false, err
		}
		if _, err = client.SetEnabled(ctx, target.Username, target.SpaceID, true); err != nil {
			return false, err
		}
		return true, s.step(ctx, op, "MCP_READY")
	case "MCP_READY":
		if err = s.step(ctx, op, "MCP_SENT"); err != nil {
			return false, err
		}
		credential, e := client.RotateMCP(ctx, target.Username, target.SpaceID)
		if e != nil {
			return false, e
		}
		return true, s.saveMCP(ctx, op, target.SpaceID, credential.Token, "CONTROL")
	case "REMOTE_CREATED":
		return true, s.step(ctx, op, "CONTROL")
	case "DAV_PREPARE":
		if str(target.Config.DavOrigin) == "" {
			return true, s.step(ctx, op, "DONE")
		}
		return true, s.prepareDAV(ctx, op, space)
	case "DAV_READY":
		token, e := s.Secrets.ResolveSecret(ctx, op.CandidateSecretID, int(op.CandidateSecretVersion))
		if e != nil {
			return false, e
		}
		if err = s.step(ctx, op, "DAV_SENT"); err != nil {
			return false, err
		}
		_, err = client.SetDAV(ctx, target.Username, target.SpaceID, string(token))
		if err != nil {
			return false, err
		}
		tx, e := s.Client.Tx(ctx)
		if e != nil {
			return false, e
		}
		defer tx.Rollback()
		fresh, e := tx.AgentSpace.Get(ctx, space.ID)
		if e != nil {
			return false, e
		}
		if fresh.BindingRevision != op.BindingRevision {
			return false, ErrConflict
		}
		if _, e = tx.AgentSpace.UpdateOneID(space.ID).SetDavSecretID(op.CandidateSecretID).SetDavSecretVersion(op.CandidateSecretVersion).SetDavConfirmed(true).Save(ctx); e != nil {
			return false, e
		}
		next := "DONE"
		if _, e = tx.WorkspaceOperation.UpdateOneID(op.ID).SetStep(next).SetUpdatedAt(s.Now().UTC()).Save(ctx); e != nil {
			return false, e
		}
		return true, tx.Commit()
	case "CONTROL":
		if s.ApplyControl == nil {
			return false, ErrUnavailable
		}
		id, done, e := s.ApplyControl(ctx, op.CreatedByUserID, op.ID)
		if e != nil {
			if errors.Is(e, ErrPending) {
				return false, nil
			}
			return false, e
		}
		if !done {
			return false, nil
		}
		if id != "" {
			if _, e = s.Client.WorkspaceOperation.UpdateOneID(op.ID).SetActivationID(id).Save(ctx); e != nil {
				return false, e
			}
		}
		if op.Action == "CREATE" || op.Action == "RESTORE" || op.Action == "TAKEOVER" || op.Action == "RESET_MCP" {
			if op.Action == "CREATE" {
				revision := int64(0)
				if id != "" {
					activation, e := s.Client.Activation.Get(ctx, id)
					if e != nil {
						return false, e
					}
					revision = activation.ControlRevision
				}
				if _, e = s.Client.AgentSpace.UpdateOneID(space.ID).SetState("CONNECTED").SetAppliedControlRevision(revision).Save(ctx); e != nil {
					return false, e
				}
				return true, s.step(ctx, op, "DAV_PREPARE")
			}
			return true, s.step(ctx, op, "DONE")
		}
		return true, s.step(ctx, op, "CONTROL_DONE")
	case "CONTROL_DONE":
		if op.Action == "DELETE" {
			if err = s.step(ctx, op, "DELETE_SENT"); err != nil {
				return false, err
			}
			status, e := client.Delete(ctx, target.Username, target.SpaceID)
			if e != nil {
				return false, e
			}
			if status == 204 {
				return true, s.step(ctx, op, "DONE")
			}
			return true, s.step(ctx, op, "DELETE_WAIT")
		}
		if err = s.step(ctx, op, "DISABLE_SENT"); err != nil {
			return false, err
		}
		if _, err = client.SetEnabled(ctx, target.Username, target.SpaceID, false); err != nil {
			return false, err
		}
		return true, s.step(ctx, op, "STOP_WAIT")
	case "STOP_WAIT", "DELETE_WAIT":
		remote, e := client.Get(ctx, target.Username)
		if op.Step == "DELETE_WAIT" && remoteapi.IsNotFound(e) {
			return true, s.step(ctx, op, "DONE")
		}
		if e != nil {
			return false, e
		}
		if remote.AgentSpaceID != target.SpaceID {
			return false, ErrConflict
		}
		if op.Step == "DELETE_WAIT" || remote.StopPending {
			return false, nil
		}
		if remote.Status != "disabled" {
			return false, ErrUnavailable
		}
		return true, s.step(ctx, op, "DONE")
	case "DONE":
		return false, s.complete(ctx, op)
	}
	return false, ErrInvalid
}

func (s *Service) saveMCP(ctx context.Context, op *ent.WorkspaceOperation, spc, token, next string) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := tx.AgentSpace.Get(ctx, op.UserID)
	if err != nil {
		return err
	}
	if row.BindingRevision != op.BindingRevision {
		return ErrConflict
	}
	secret, err := s.Secrets.SaveSecretVersionTx(ctx, tx, op.CreatedByUserID, row.McpSecretID, "Remote workspace MCP", token)
	if err != nil {
		return err
	}
	if _, err = tx.AgentSpace.UpdateOneID(row.ID).SetAgentSpaceID(spc).SetMcpSecretID(secret.SecretID).SetMcpSecretVersion(int64(secret.SecretVersion)).SetRemoteActive(true).SetStopPending(false).SetObservedAt(s.Now().UTC()).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetStep(next).SetUpdatedAt(s.Now().UTC()).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) prepareDAV(ctx context.Context, op *ent.WorkspaceOperation, space *ent.AgentSpace) error {
	token, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	secret, err := s.Secrets.SaveSecretVersionTx(ctx, tx, op.CreatedByUserID, space.DavSecretID, "Remote workspace DAV", token)
	if err != nil {
		return err
	}
	if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetCandidateSecretID(secret.SecretID).SetCandidateSecretVersion(int64(secret.SecretVersion)).SetStep("DAV_READY").SetUpdatedAt(s.Now().UTC()).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) clearDAV(ctx context.Context, op *ent.WorkspaceOperation) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.AgentSpace.UpdateOneID(op.UserID).SetDavConfirmed(false).ClearDavSecretVersion().Save(ctx); err != nil {
		return err
	}
	if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetStep("DONE").Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) complete(ctx context.Context, op *ent.WorkspaceOperation) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if op.UserID != "" {
		row, e := tx.AgentSpace.Get(ctx, op.UserID)
		if e != nil {
			return e
		}
		if row.BindingRevision != op.BindingRevision {
			return ErrConflict
		}
		update := tx.AgentSpace.UpdateOneID(row.ID).SetUpdatedAt(s.Now().UTC()).ClearDiagnosticCode()
		switch op.Action {
		case "DELETE":
			ids := []string{row.McpSecretID, row.DavSecretID}
			candidates, e := tx.WorkspaceOperation.Query().Where(workspaceoperation.UserIDEQ(row.ID)).All(ctx)
			if e != nil {
				return e
			}
			for _, candidate := range candidates {
				ids = append(ids, candidate.CandidateSecretID)
			}
			seen := map[string]bool{}
			for _, id := range ids {
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				if _, e = tx.SecretVersion.Delete().Where(secretversion.SecretIDEQ(id)).Exec(ctx); e != nil {
					return e
				}
				if e = tx.Secret.DeleteOneID(id).Exec(ctx); e != nil && !ent.IsNotFound(e) {
					return e
				}
			}
			if err = tx.AgentSpace.DeleteOneID(row.ID).Exec(ctx); err != nil {
				return err
			}
		case "DISCONNECT", "SUSPEND":
			update.SetState("DISCONNECTED").SetRemoteActive(false).SetStopPending(false).SetDavConfirmed(false).ClearMcpSecretVersion().ClearDavSecretVersion()
			_, err = update.Save(ctx)
		case "CREATE", "RESTORE", "TAKEOVER", "RESET_MCP":
			revision := int64(0)
			if op.ActivationID != "" {
				activation, e := tx.Activation.Get(ctx, op.ActivationID)
				if e != nil {
					return e
				}
				revision = activation.ControlRevision
			}
			_, err = update.SetState("CONNECTED").SetAppliedControlRevision(revision).Save(ctx)
		default:
			_, err = update.Save(ctx)
		}
		if err != nil {
			return err
		}
	}
	if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetState("COMPLETED").SetStep("DONE").SetUpdatedAt(s.Now().UTC()).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) advanceWorkspaceService(ctx context.Context, op *ent.WorkspaceOperation, client *remoteapi.Client) (bool, error) {
	switch op.Step {
	case "START":
		if op.Action == "APPLY" {
			check, err := s.Check(ctx, op.WorkspaceServiceID)
			if err != nil {
				return false, err
			}
			if !check.ManagementReady {
				return false, ErrUnavailable
			}
		}
		return true, s.step(ctx, op, "CONTROL")
	case "CONTROL":
		if s.ApplyControl == nil {
			return false, ErrUnavailable
		}
		_, done, err := s.ApplyControl(ctx, op.CreatedByUserID, op.ID)
		if err != nil && !errors.Is(err, ErrPending) {
			return false, err
		}
		if err != nil || !done {
			return false, nil
		}
		return true, s.step(ctx, op, "REMOTE")
	case "REMOTE":
		if op.Action == "DISABLE" {
			rows, err := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(op.WorkspaceServiceID), agentspace.RemoteActiveEQ(true)).All(ctx)
			if err != nil {
				return false, err
			}
			if len(rows) > 0 {
				return false, nil
			}
		}
		tx, err := s.Client.Tx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		update := tx.WorkspaceService.UpdateOneID(op.WorkspaceServiceID).SetUpdatedAt(s.Now().UTC()).ClearDiagnosticCode()
		if op.Action == "APPLY" {
			update.SetState("ACTIVE").SetActiveConfigRevision(op.ConfigRevision)
		} else {
			update.SetState("DISABLED")
		}
		if _, err = update.Save(ctx); err != nil {
			return false, err
		}
		if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetState("COMPLETED").SetStep("DONE").SetUpdatedAt(s.Now().UTC()).Save(ctx); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	return false, ErrInvalid
}
