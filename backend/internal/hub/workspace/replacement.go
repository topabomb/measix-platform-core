package workspace

import (
	"context"
	"encoding/json"
	"sort"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/workspaceoperation"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

type replacementChild struct {
	UserID      string `json:"userId"`
	OperationID string `json:"operationId"`
}

func replacementBindingView(row *ent.AgentSpace, name string) adminapi.WorkspaceReplacementBinding {
	return adminapi.WorkspaceReplacementBinding{UserId: row.ID, AgentSpaceId: row.AgentSpaceID, BindingRevision: row.BindingRevision, DavBindingRevision: row.DavBindingRevision, DisplayName: ptr(name)}
}

func replacementBindingsMatch(rows []*ent.AgentSpace, expected []adminapi.WorkspaceReplacementBinding) bool {
	if len(rows) != len(expected) || len(rows) == 0 {
		return false
	}
	want := map[string]adminapi.WorkspaceReplacementBinding{}
	for _, value := range expected {
		if _, duplicate := want[value.UserId]; duplicate {
			return false
		}
		want[value.UserId] = value
	}
	for _, row := range rows {
		value, ok := want[row.ID]
		if !ok || value.AgentSpaceId != row.AgentSpaceID || value.BindingRevision != row.BindingRevision || value.DavBindingRevision != row.DavBindingRevision {
			return false
		}
	}
	return true
}

// ReplaceDeployment only archives local bindings. The administrator explicitly
// attests destruction of the old deployment; this never deletes remote data or
// treats transport/authentication failure as proof of missing accounts.
func (s *Service) ReplaceDeployment(ctx context.Context, actor, key, id string, input adminapi.ReplaceWorkspaceDeploymentRequest) (adminapi.WorkspaceOperation, error) {
	if platformid.Validate(platformid.User, actor) != nil || platformid.Validate(platformid.Idempotency, key) != nil || !input.ConfirmedDestroyedDeployment || input.ExpectedConfigRevision < 1 || len(input.Bindings) == 0 {
		return adminapi.WorkspaceOperation{}, ErrInvalid
	}
	requestHash := hash(struct {
		ServiceID string
		Input     adminapi.ReplaceWorkspaceDeploymentRequest
	}{id, input})
	existing := func() (adminapi.WorkspaceOperation, bool, error) {
		prior, err := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.CreatedByUserIDEQ(actor), workspaceoperation.IdempotencyKeyEQ(key)).Only(ctx)
		if ent.IsNotFound(err) {
			return adminapi.WorkspaceOperation{}, false, nil
		}
		if err != nil {
			return adminapi.WorkspaceOperation{}, false, err
		}
		if prior.Action != "REPLACE" || prior.WorkspaceServiceID != id || prior.RequestHash != requestHash {
			return adminapi.WorkspaceOperation{}, true, ErrConflict
		}
		out, err := s.Operation(ctx, prior.ID)
		return out, true, err
	}
	if out, found, err := existing(); found || err != nil {
		return out, err
	}
	service, err := s.Client.WorkspaceService.Get(ctx, id)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if service.ConfigRevision != int64(input.ExpectedConfigRevision) {
		if out, found, err := existing(); found || err != nil {
			return out, err
		}
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	check, checkErr := s.Check(ctx, id)
	rows, rowsErr := s.Client.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id)).All(ctx)
	// An identical request can commit while these read-only checks are in
	// flight. Return its receipt before interpreting the now-stale preview.
	if out, found, err := existing(); found || err != nil {
		return out, err
	}
	if checkErr != nil {
		return adminapi.WorkspaceOperation{}, checkErr
	}
	if rowsErr != nil {
		return adminapi.WorkspaceOperation{}, rowsErr
	}
	if check.ConfigRevision == nil || *check.ConfigRevision != input.ExpectedConfigRevision || check.ReplacementAvailable == nil || !*check.ReplacementAvailable {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	if !replacementBindingsMatch(rows, input.Bindings) || check.MissingWorkspaces == nil || !replacementBindingsMatch(rows, *check.MissingWorkspaces) {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	cfg, err := s.config(ctx, id, service.ConfigRevision)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	defer tx.Rollback()
	// A concurrent retry may have committed while this request was checking the
	// remote. Reconcile that same operation before comparing changed bindings.
	if existing, e := tx.WorkspaceOperation.Query().Where(workspaceoperation.CreatedByUserIDEQ(actor), workspaceoperation.IdempotencyKeyEQ(key)).Only(ctx); e == nil {
		if existing.Action != "REPLACE" || existing.RequestHash != requestHash || existing.WorkspaceServiceID != id {
			return adminapi.WorkspaceOperation{}, ErrConflict
		}
		return operationView(existing), nil
	} else if !ent.IsNotFound(e) {
		return adminapi.WorkspaceOperation{}, e
	}
	current, err := tx.WorkspaceService.Get(ctx, id)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if current.ConfigRevision != service.ConfigRevision {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	currentRows, err := tx.AgentSpace.Query().Where(agentspace.WorkspaceServiceIDEQ(id)).All(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if !replacementBindingsMatch(currentRows, input.Bindings) {
		return adminapi.WorkspaceOperation{}, ErrConflict
	}
	busy, err := tx.WorkspaceOperation.Query().Where(workspaceoperation.WorkspaceServiceIDEQ(id), workspaceoperation.StateNEQ("COMPLETED")).Exist(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	controlBusy, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Exist(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if busy || controlBusy {
		return adminapi.WorkspaceOperation{}, ErrPending
	}
	sort.Slice(currentRows, func(i, j int) bool { return currentRows[i].ID < currentRows[j].ID })
	now := s.Now().UTC()
	target := operationTarget{Config: cfg, PreviousEnabled: current.Enabled, PreviousState: current.State, Replacement: currentRows}
	for _, row := range currentRows {
		if _, err = tx.AgentSpace.UpdateOneID(row.ID).SetIntent("DISCONNECTED").SetState("DISCONNECTING").SetRemoteActive(false).SetDavConfirmed(false).SetBindingRevision(row.BindingRevision + 1).SetDavBindingRevision(row.DavBindingRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
			return adminapi.WorkspaceOperation{}, err
		}
	}
	if _, err = tx.WorkspaceService.UpdateOneID(id).SetEnabled(true).SetState("APPLYING").SetUpdatedAt(now).Save(ctx); err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	data, _ := json.Marshal(target)
	row, err := tx.WorkspaceOperation.Create().SetID(platformid.New(platformid.WorkspaceOperation)).SetWorkspaceServiceID(id).SetAction("REPLACE").SetIdempotencyKey(key).SetRequestHash(requestHash).SetConfigRevision(service.ConfigRevision).SetBindingRevision(0).SetTargetJSON(data).SetState("PENDING").SetStep("START").SetCreatedByUserID(actor).SetEvidence("Confirmed old deployment destroyed and old management requests ended; create empty spaces").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return adminapi.WorkspaceOperation{}, err
	}
	return operationView(row), nil
}

// This transaction runs only after the old bindings' exact Control ACK. Fresh
// CREATE intents are queued atomically through the ordinary command owner.
func (s *Service) queueReplacement(ctx context.Context, op *ent.WorkspaceOperation, target operationTarget) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	service, err := tx.WorkspaceService.Get(ctx, op.WorkspaceServiceID)
	if err != nil {
		return err
	}
	if service.ConfigRevision != op.ConfigRevision || service.State != "APPLYING" {
		return ErrConflict
	}
	skipped := map[string]bool{}
	for _, old := range target.Replacement {
		row, e := tx.AgentSpace.Get(ctx, old.ID)
		if ent.IsNotFound(e) {
			_, userErr := tx.User.Get(ctx, old.ID)
			if ent.IsNotFound(userErr) {
				// Independent identity cleanup already removed this old binding.
				skipped[old.ID] = true
				continue
			}
			if userErr != nil {
				return userErr
			}
			return ErrConflict
		}
		if e != nil {
			return e
		}
		if row.AgentSpaceID != old.AgentSpaceID {
			return ErrConflict
		}
		if row.Intent == "DELETED" {
			// Keep the pinned identity-deletion operation and its binding intact.
			skipped[old.ID] = true
			continue
		}
		if row.BindingRevision != old.BindingRevision+1 || row.DavBindingRevision != old.DavBindingRevision+1 || row.Intent != "DISCONNECTED" {
			return ErrConflict
		}
		if err = tx.AgentSpace.DeleteOneID(old.ID).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = tx.WorkspaceService.UpdateOneID(service.ID).SetActiveConfigRevision(op.ConfigRevision).SetState("ACTIVE").ClearDiagnosticCode().SetUpdatedAt(s.Now()).Save(ctx); err != nil {
		return err
	}
	for _, old := range target.Replacement {
		if skipped[old.ID] {
			continue
		}
		user, e := tx.User.Get(ctx, old.ID)
		if ent.IsNotFound(e) {
			continue
		}
		if e != nil {
			return e
		}
		if user.Status != "ACTIVE" || old.Intent != "CONNECTED" {
			continue
		}
		input := adminapi.WorkspaceCommand{Action: "CREATE", ExpectedRevision: 0}
		childTarget := operationTarget{Config: target.Config, Username: old.RemoteUsername, InitialDAVRevision: old.DavBindingRevision + 2}
		child, e := s.beginTx(ctx, tx, op.CreatedByUserID, platformid.New(platformid.Idempotency), service.ID, old.ID, "CREATE", op.ConfigRevision, old.BindingRevision+1, childTarget, &input, op.ID)
		if e != nil {
			return e
		}
		if _, e = tx.AgentSpace.UpdateOneID(old.ID).SetDavAccessMode(old.DavAccessMode).SetFileAccessIntent(old.FileAccessIntent).Save(ctx); e != nil {
			return e
		}
		target.Recreated = append(target.Recreated, replacementChild{UserID: old.ID, OperationID: child.ID})
	}
	data, _ := json.Marshal(target)
	if _, err = tx.WorkspaceOperation.UpdateOneID(op.ID).SetTargetJSON(data).SetState("RUNNING").SetStep("RECREATING").SetUpdatedAt(s.Now()).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) replacementProgress(ctx context.Context, target operationTarget) (adminapi.WorkspaceReplacementProgress, error) {
	progress := adminapi.WorkspaceReplacementProgress{Total: len(target.Recreated)}
	for _, child := range target.Recreated {
		row, err := s.Client.WorkspaceOperation.Get(ctx, child.OperationID)
		if err != nil {
			return progress, err
		}
		if row.UserID != child.UserID {
			return progress, ErrConflict
		}
		if row.State == "COMPLETED" {
			progress.Completed++
		}
		if row.State == "UNKNOWN" || row.State == "NEEDS_ATTENTION" {
			progress.NeedsAttention++
		}
	}
	return progress, nil
}
