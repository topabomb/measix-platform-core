package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
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
		state, err := s.stateFromActivationDescriptor(ctx, pending)
		if err != nil {
			_ = s.setRuntimeStatus(ctx, "DEGRADED")
			return nil, err
		}
		generation := int64(state.ActiveManagedGeneration)
		if relayMatches(status, pending.ControlRevision, pending.BundleHash, &generation) {
			return s.finalizePending(ctx, pending.ID)
		}
		if !status.Ready || status.AppliedControlRevision < int(pending.ControlRevision) {
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
	if !status.Ready || status.AppliedControlRevision < int(managed.DesiredControlRevision) {
		if err := s.rehydrateActive(ctx, int(managed.ActiveManagedGeneration), int(managed.DesiredControlRevision), stringPointer(managed.DesiredBundleHash)); err != nil {
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
	switch row.Kind {
	case "PUBLISH":
		if row.SubjectID == nil || row.TargetGeneration == nil {
			return nil, fmt.Errorf("publish activation missing target")
		}
		if err := s.finalizePublish(ctx, row.ID, *row.SubjectID, int(*row.TargetGeneration), int(row.ControlRevision), row.BundleHash); err != nil {
			return nil, err
		}
	case "RUNTIME_CONFIG":
		if row.SubjectID == nil {
			return nil, fmt.Errorf("runtime config activation missing target")
		}
		var pending struct {
			TargetRevision int `json:"targetRevision"`
		}
		if row.PendingOperationJSON == nil || len(*row.PendingOperationJSON) == 0 || json.Unmarshal(*row.PendingOperationJSON, &pending) != nil || pending.TargetRevision < 1 {
			return nil, fmt.Errorf("runtime config activation missing persisted revision")
		}
		if err := s.finalizeUpstreamApply(ctx, row.ID, *row.SubjectID, pending.TargetRevision, int(row.ControlRevision), row.BundleHash); err != nil {
			return nil, err
		}
	case "SECURITY_CHANGE":
		if row.SubjectID == nil {
			return nil, fmt.Errorf("security activation missing subject")
		}
		var pending struct {
			Operation string `json:"operation"`
			SubjectID string `json:"subjectId"`
		}
		if row.PendingOperationJSON == nil || len(*row.PendingOperationJSON) == 0 || json.Unmarshal(*row.PendingOperationJSON, &pending) != nil || pending.SubjectID != *row.SubjectID {
			return nil, fmt.Errorf("security activation missing persisted operation")
		}
		if err := s.finalizeSecurityChange(ctx, row.ID, securitySubject(pending.Operation), pending.SubjectID, int(row.ControlRevision), row.BundleHash); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported pending activation kind %q", row.Kind)
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
	state, err := s.stateFromActivationDescriptor(ctx, row)
	if err != nil {
		return err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil || string(hash) != row.BundleHash {
		_ = s.setRuntimeStatus(ctx, "DEGRADED")
		return ErrRelayDiverged
	}
	state.BundleHash = hash
	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		_ = s.markUnknown(ctx, row.ID, "relay_reapply_unknown")
		return err
	}
	if ack.AppliedControlRevision != int(row.ControlRevision) || string(ack.BundleHash) != row.BundleHash || ack.ActiveManagedGeneration != state.ActiveManagedGeneration {
		_ = s.markFailed(ctx, row.ID, "relay_ack_mismatch")
		return ErrRelayAckMismatch
	}
	return nil
}

func (s *Service) stateFromActivationDescriptor(ctx context.Context, row *ent.Activation) (relaycontrolapi.RuntimeControlState, error) {
	var state relaycontrolapi.RuntimeControlState
	if len(row.TargetDescriptorJSON) == 0 || json.Unmarshal(row.TargetDescriptorJSON, &state) != nil {
		return state, fmt.Errorf("invalid persisted activation descriptor")
	}
	if state.ControlRevision != int(row.ControlRevision) {
		return state, ErrRelayDiverged
	}
	for i := range state.Upstreams {
		value := &state.Upstreams[i]
		if value.Auth.AdditionalProperties == nil {
			value.Auth.AdditionalProperties = map[string]interface{}{}
		}
		if value.Auth.Type == relaycontrolapi.NONE {
			continue
		}
		if value.SecretRef == nil {
			return state, fmt.Errorf("persisted upstream %s lacks secret reference", value.UpstreamId)
		}
		secret, err := s.Upstream.ResolveSecret(ctx, value.SecretRef.SecretId, value.SecretRef.SecretVersion)
		if err != nil {
			return state, err
		}
		switch value.Auth.Type {
		case relaycontrolapi.BEARER:
			value.Auth.AdditionalProperties["token"] = string(secret)
		case relaycontrolapi.STATICHEADER:
			value.Auth.AdditionalProperties["value"] = string(secret)
		case relaycontrolapi.BASIC:
			value.Auth.AdditionalProperties["password"] = string(secret)
		default:
			return state, fmt.Errorf("unsupported persisted auth type %s", value.Auth.Type)
		}
	}
	return state, nil
}

func (s *Service) rehydrateActive(ctx context.Context, generation, revision int, expectedHash string) error {
	if expectedHash == "" {
		return fmt.Errorf("desired runtime state cannot be reconstructed")
	}
	content, err := s.activeReleaseContent(ctx)
	if err != nil {
		return err
	}
	state, err := s.compileOperationalState(ctx, content, generation, revision, nil)
	if err != nil {
		return err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil || string(hash) != expectedHash {
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
