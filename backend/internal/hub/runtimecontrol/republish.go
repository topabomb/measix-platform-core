package runtimecontrol

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/topabomb/measix-platform-core/backend/ent"
	"github.com/topabomb/measix-platform-core/backend/ent/activation"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/capability"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/adminapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaystate"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func (s *Service) Republish(ctx context.Context, adminUserID, idempotencyKey, sourceReleaseID string) (ActivationResult, error) {
	if platformid.Validate(platformid.User, adminUserID) != nil || platformid.Validate(platformid.Idempotency, idempotencyKey) != nil || platformid.Validate(platformid.Release, sourceReleaseID) != nil {
		return ActivationResult{}, fmt.Errorf("invalid republish request")
	}
	source, err := s.Client.ManagedRelease.Get(ctx, sourceReleaseID)
	if err != nil {
		return ActivationResult{}, err
	}
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(source.ReleaseContentJSON, &content); err != nil {
		return ActivationResult{}, err
	}
	path := "/api/admin/v1/releases/" + sourceReleaseID + ":republish"
	requestHash := hashOperation(struct {
		ReleaseID string `json:"releaseId"`
	}{sourceReleaseID})
	if existing, found, err := s.findOperationIdempotent(ctx, adminUserID, path, idempotencyKey, requestHash); err != nil || found {
		return existing, err
	}
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return ActivationResult{}, err
	}
	generation, err := s.nextGeneration(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	controlRevision := int(managed.DesiredControlRevision) + 1
	newReleaseID := platformid.New(platformid.Release)
	activationID := platformid.New(platformid.Activation)
	now := s.Now().UTC()
	snapshot, snapshotHash, err := s.Capability.CompileSnapshot(capability.SnapshotInput{
		DeploymentID: s.Signer.DeploymentID, ReleaseID: newReleaseID, ManagedGeneration: generation,
		Content: content, PublishedAt: now, PublishedByUserID: adminUserID,
	})
	if err != nil {
		return ActivationResult{}, err
	}
	state, err := s.compileOperationalState(ctx, content, generation, controlRevision, nil)
	if err != nil {
		return ActivationResult{}, err
	}
	descriptor, err := relaystate.DescriptorJSON(state)
	if err != nil {
		return ActivationResult{}, err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		return ActivationResult{}, err
	}
	state.BundleHash = hash
	snapshotJSON, _ := json.Marshal(snapshot)

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	defer tx.Rollback()
	pending, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Count(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	if pending != 0 {
		return ActivationResult{}, ErrActivationInProgress
	}
	fresh, err := tx.ManagedState.Get(ctx, "current")
	if err != nil || fresh.DesiredControlRevision+1 != int64(controlRevision) {
		return ActivationResult{}, ErrActivationInProgress
	}
	if _, err := tx.ManagedRelease.Create().
		SetID(newReleaseID).SetManagedGeneration(int64(generation)).SetStatus("STAGED").
		SetReleaseContentJSON(append([]byte(nil), source.ReleaseContentJSON...)).
		SetSnapshotSchemaVersion(1).SetSnapshotJSON(snapshotJSON).SetSnapshotHash(snapshotHash).
		SetSourceDraftRevision(source.SourceDraftRevision).SetCreatedByUserID(adminUserID).SetCreatedAt(now).Save(ctx); err != nil {
		return ActivationResult{}, err
	}
	if _, err := tx.Activation.Create().
		SetID(activationID).SetKind("PUBLISH").SetState("APPLYING").SetIdempotencyKey(idempotencyKey).SetRequestHash(requestHash).
		SetControlRevision(int64(controlRevision)).SetBundleHash(string(hash)).SetTargetGeneration(int64(generation)).
		SetTargetDescriptorJSON(descriptor).SetSubjectID(newReleaseID).SetCreatedByUserID(adminUserID).SetCreatedAt(now).Save(ctx); err != nil {
		return ActivationResult{}, err
	}
	if _, err := tx.IdempotencyRecord.Create().
		SetAdminUserID(adminUserID).SetMethod("POST").SetNormalizedPath(path).SetIdempotencyKey(idempotencyKey).
		SetRequestHash(requestHash).SetActivationID(activationID).SetCreatedAt(now).Save(ctx); err != nil {
		return ActivationResult{}, err
	}
	if _, err := tx.ManagedState.UpdateOneID("current").
		SetDesiredControlRevision(int64(controlRevision)).SetDesiredBundleHash(string(hash)).SetRuntimeStatus("ACTIVATING").
		SetManagedStateRevision(fresh.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		return ActivationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ActivationResult{}, err
	}

	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		_ = s.markUnknown(ctx, activationID, "relay_apply_unknown")
		return s.loadActivation(ctx, activationID)
	}
	if ack.AppliedControlRevision != controlRevision || string(ack.BundleHash) != string(hash) || ack.ActiveManagedGeneration != generation {
		_ = s.markFailed(ctx, activationID, "relay_ack_mismatch")
		return ActivationResult{}, ErrRelayAckMismatch
	}
	if err := s.finalizePublish(ctx, activationID, newReleaseID, generation, controlRevision, string(hash)); err != nil {
		return ActivationResult{}, err
	}
	return s.loadActivation(ctx, activationID)
}

var _ = ent.IsNotFound
