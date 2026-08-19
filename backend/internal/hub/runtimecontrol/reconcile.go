package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/topabomb/measix-platform-core/backend/ent"
	"github.com/topabomb/measix-platform-core/backend/ent/activation"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/adminapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaystate"
)

var ErrRelayDiverged = errors.New("relay applied state is unexpectedly newer or different")

func IsRelayDiverged(err error) bool { return errors.Is(err, ErrRelayDiverged) }

func (s *Service) Reconcile(ctx context.Context) (*ActivationResult, error) {
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return nil, err
	}
	status, err := s.Relay.Status(ctx)
	if err != nil {
		_ = s.setRuntimeStatus(ctx, "DEGRADED")
		return nil, err
	}
	if relayUnexpected(managed.DesiredControlRevision, managed.DesiredBundleHash, status) {
		_ = s.setRuntimeStatus(ctx, "DEGRADED")
		return nil, ErrRelayDiverged
	}

	pending, err := s.Client.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		if pending.ControlRevision != managed.DesiredControlRevision || managed.DesiredBundleHash == nil || pending.BundleHash != *managed.DesiredBundleHash {
			_ = s.setRuntimeStatus(ctx, "DEGRADED")
			return nil, ErrRelayDiverged
		}
		if relayMatches(status, pending.ControlRevision, pending.BundleHash, pending.TargetGeneration) {
			return s.finalizePending(ctx, pending.ID)
		}
		if !status.Ready || status.AppliedControlRevision < pending.ControlRevision {
			if err := s.reapplyPending(ctx, pending.ID); err != nil {
				return nil, err
			}
			return s.finalizePending(ctx, pending.ID)
		}
		_ = s.setRuntimeStatus(ctx, "DEGRADED")
		return nil, ErrRelayDiverged
	}

	if managed.DesiredControlRevision == 0 {
		if status.AppliedControlRevision != 0 {
			_ = s.setRuntimeStatus(ctx, "DEGRADED")
			return nil, ErrRelayDiverged
		}
		return nil, nil
	}
	if relayMatches(status, managed.DesiredControlRevision, stringPointer(managed.DesiredBundleHash), &managed.ActiveManagedGeneration) {
		if managed.RuntimeStatus != "READY" {
			if err := s.setRuntimeStatus(ctx, "READY"); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if !status.Ready || status.AppliedControlRevision < managed.DesiredControlRevision {
		if err := s.rehydrateActive(ctx, managed.ActiveReleaseID, int(managed.ActiveManagedGeneration), int(managed.DesiredControlRevision), stringPointer(managed.DesiredBundleHash)); err != nil {
			_ = s.setRuntimeStatus(ctx, "DEGRADED")
			return nil, err
		}
		if err := s.setRuntimeStatus(ctx, "READY"); err != nil {
			return nil, err
		}
		return nil, nil
	}
	_ = s.setRuntimeStatus(ctx, "DEGRADED")
	return nil, ErrRelayDiverged
}

func relayUnexpected(desiredRevision int64, desiredHash *string, status relaycontrolapi.ControlStatus) bool {
	if status.AppliedControlRevision > int(desiredRevision) {
		return true
	}
	if status.AppliedControlRevision == int(desiredRevision) && desiredRevision != 0 && desiredHash != nil && status.BundleHash != *desiredHash {
		return true
	}
	return false
}

func relayMatches(status relaycontrolapi.ControlStatus, revision int64, hash string, generation *int64) bool {
	if !status.Ready || status.AppliedControlRevision != int(revision) || status.BundleHash != hash {
		return false
	}
	if generation != nil && status.ActiveManagedGeneration != int(*generation) {
		return false
	}
	return true
}

func (s *Service) finalizePending(ctx context.Context, activationID string) (*ActivationResult, error) {
	row, err := s.Client.Activation.Get(ctx, activationID)
	if err != nil {
		return nil, err
	}
	if row.Kind != "PUBLISH" || row.SubjectID == nil || row.TargetGeneration == nil {
		return nil, fmt.Errorf("unsupported pending activation kind %q", row.Kind)
	}
	if err := s.finalizePublish(ctx, row.ID, *row.SubjectID, int(*row.TargetGeneration), int(row.ControlRevision), row.BundleHash); err != nil {
		return nil, err
	}
	result, err := s.loadActivation(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) reapplyPending(ctx context.Context, activationID string) error {
	row, err := s.Client.Activation.Get(ctx, activationID)
	if err != nil {
		return err
	}
	if row.Kind != "PUBLISH" || row.SubjectID == nil || row.TargetGeneration == nil {
		return fmt.Errorf("unsupported pending activation kind %q", row.Kind)
	}
	release, err := s.Client.ManagedRelease.Get(ctx, *row.SubjectID)
	if err != nil {
		return err
	}
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(release.ReleaseContentJSON, &content); err != nil {
		return err
	}
	state, err := s.compileState(ctx, content, int(*row.TargetGeneration), int(row.ControlRevision))
	if err != nil {
		return err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		return err
	}
	if string(hash) != row.BundleHash {
		_ = s.setRuntimeStatus(ctx, "DEGRADED")
		return ErrRelayDiverged
	}
	state.BundleHash = hash
	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		_ = s.markUnknown(ctx, row.ID, "relay_reapply_unknown")
		return err
	}
	if ack.AppliedControlRevision != int(row.ControlRevision) || string(ack.BundleHash) != row.BundleHash || ack.ActiveManagedGeneration != int(*row.TargetGeneration) {
		_ = s.markFailed(ctx, row.ID, "relay_ack_mismatch")
		return ErrRelayAckMismatch
	}
	return nil
}

func (s *Service) rehydrateActive(ctx context.Context, releaseID *string, generation, revision int, expectedHash string) error {
	if releaseID == nil || expectedHash == "" {
		return fmt.Errorf("desired runtime state cannot be reconstructed")
	}
	release, err := s.Client.ManagedRelease.Get(ctx, *releaseID)
	if err != nil {
		return err
	}
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(release.ReleaseContentJSON, &content); err != nil {
		return err
	}
	state, err := s.compileState(ctx, content, generation, revision)
	if err != nil {
		return err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		return err
	}
	if string(hash) != expectedHash {
		return ErrRelayDiverged
	}
	state.BundleHash = hash
	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		return err
	}
	if ack.AppliedControlRevision != revision || string(ack.BundleHash) != expectedHash || ack.ActiveManagedGeneration != generation {
		return ErrRelayAckMismatch
	}
	return nil
}

func (s *Service) setRuntimeStatus(ctx context.Context, status string) error {
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return err
	}
	if managed.RuntimeStatus == status {
		return nil
	}
	_, err = s.Client.ManagedState.UpdateOneID("current").
		SetRuntimeStatus(status).
		SetManagedStateRevision(managed.ManagedStateRevision + 1).
		SetUpdatedAt(s.Now().UTC()).
		Save(ctx)
	return err
}

func stringPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
