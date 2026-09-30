package runtimecontrol

import (
	"context"
	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

// ApplyWorkspace participates in the existing global Activation protocol. The
// workspace operation owns its remote IO and resumes after this short phase.
func (s *Service) ApplyWorkspace(ctx context.Context, actor, operationID string) (string, bool, error) {
	existing, err := s.Client.Activation.Query().Where(activation.KindEQ("WORKSPACE"), activation.SubjectIDEQ(operationID)).Only(ctx)
	if err == nil {
		return existing.ID, existing.State == "COMPLETED", nil
	}
	if !ent.IsNotFound(err) {
		return "", false, err
	}
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return "", false, err
	}
	if managed.ActiveReleaseID == nil {
		return "", true, nil
	}
	content, err := s.activeReleaseContent(ctx)
	if err != nil {
		return "", false, err
	}
	revision := int(managed.DesiredControlRevision) + 1
	state, err := s.compileState(ctx, content, int(managed.ActiveManagedGeneration), revision, nil)
	if err != nil {
		return "", false, err
	}
	descriptor, err := relaystate.DescriptorJSON(state)
	if err != nil {
		return "", false, err
	}
	digest, err := relaystate.HashDescriptor(state)
	if err != nil {
		return "", false, err
	}
	state.BundleHash = digest
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	busy, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Exist(ctx)
	if err != nil {
		return "", false, err
	}
	if busy {
		return "", false, ErrActivationInProgress
	}
	current, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		return "", false, err
	}
	if current.DesiredControlRevision != managed.DesiredControlRevision {
		return "", false, ErrActivationInProgress
	}
	id := platformid.New(platformid.Activation)
	now := s.Now().UTC()
	_, err = tx.Activation.Create().SetID(id).SetKind("WORKSPACE").SetState("APPLYING").SetIdempotencyKey(operationID).SetRequestHash(hashOperation(operationID)).SetControlRevision(int64(revision)).SetBundleHash(string(digest)).SetTargetDescriptorJSON(descriptor).SetSubjectID(operationID).SetCreatedByUserID(actor).SetCreatedAt(now).Save(ctx)
	if err != nil {
		return "", false, err
	}
	_, err = tx.ManagedState.UpdateOneID("current").SetDesiredControlRevision(int64(revision)).SetDesiredBundleHash(string(digest)).SetRuntimeStatus("ACTIVATING").SetManagedStateRevision(current.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return "", false, err
	}
	if err = tx.Commit(); err != nil {
		return "", false, err
	}
	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		_ = s.markUnknown(ctx, id, "relay_apply_unknown")
		return id, false, nil
	}
	if !ackProtocolMatches(state, ack) || ack.AppliedControlRevision != revision || ack.BundleHash != string(digest) || ack.ActiveManagedGeneration != state.ActiveManagedGeneration {
		_ = s.markUnknown(ctx, id, "relay_ack_mismatch")
		return id, false, ErrRelayAckMismatch
	}
	if err = s.finalizeWorkspace(ctx, id); err != nil {
		return id, false, err
	}
	return id, true, nil
}

func (s *Service) finalizeWorkspace(ctx context.Context, id string) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := tx.Activation.Get(ctx, id)
	if err != nil {
		return err
	}
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		return err
	}
	if managed.DesiredControlRevision != row.ControlRevision || managed.DesiredBundleHash == nil || *managed.DesiredBundleHash != row.BundleHash {
		return ErrRelayAckMismatch
	}
	now := s.Now().UTC()
	if _, err = tx.Activation.UpdateOneID(id).SetState("COMPLETED").SetCompletedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.ManagedState.UpdateOneID("current").SetRuntimeStatus("READY").SetManagedStateRevision(managed.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
