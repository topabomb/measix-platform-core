package runtimecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"measix/platform/ent/activation"
	"measix/platform/ent/predicate"
	"measix/platform/ent/session"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

type securitySubject string

const (
	securityUserDisable   securitySubject = "USER_DISABLE"
	securityUserEnable    securitySubject = "USER_ENABLE"
	securityDeviceRevoke  securitySubject = "DEVICE_REVOKE"
	securitySessionRevoke securitySubject = "SESSION_REVOKE"
)

func (s *Service) DisableUser(ctx context.Context, adminUserID, idempotencyKey, userID string) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityUserDisable), userID)
}

func (s *Service) EnableUser(ctx context.Context, adminUserID, idempotencyKey, userID string) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityUserEnable), userID)
}

func (s *Service) RevokeDevice(ctx context.Context, adminUserID, idempotencyKey, deviceID string) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityDeviceRevoke), deviceID)
}

func (s *Service) securityChange(ctx context.Context, adminUserID, idempotencyKey, operation, subjectID string) (ActivationResult, error) {
	if s.Client == nil || s.Signer == nil || s.Relay == nil || platformid.Validate(platformid.User, adminUserID) != nil || platformid.Validate(platformid.Idempotency, idempotencyKey) != nil {
		return ActivationResult{}, fmt.Errorf("invalid security change request")
	}
	var path string
	switch securitySubject(operation) {
	case securityUserDisable:
		if platformid.Validate(platformid.User, subjectID) != nil {
			return ActivationResult{}, fmt.Errorf("invalid user id")
		}
		if _, err := s.Client.User.Get(ctx, subjectID); err != nil {
			return ActivationResult{}, err
		}
		path = "/api/admin/v1/users/" + subjectID + ":disable"
	case securityUserEnable:
		if platformid.Validate(platformid.User, subjectID) != nil {
			return ActivationResult{}, fmt.Errorf("invalid user id")
		}
		if _, err := s.Client.User.Get(ctx, subjectID); err != nil {
			return ActivationResult{}, err
		}
		path = "/api/admin/v1/users/" + subjectID + ":enable"
	case securityDeviceRevoke:
		if platformid.Validate(platformid.Device, subjectID) != nil {
			return ActivationResult{}, fmt.Errorf("invalid device id")
		}
		if _, err := s.Client.Device.Get(ctx, subjectID); err != nil {
			return ActivationResult{}, err
		}
		path = "/api/admin/v1/devices/" + subjectID + ":revoke"
	case securitySessionRevoke:
		if platformid.Validate(platformid.Session, subjectID) != nil {
			return ActivationResult{}, fmt.Errorf("invalid session id")
		}
		row, err := s.Client.Session.Get(ctx, subjectID)
		if err != nil {
			return ActivationResult{}, err
		}
		if row.Status != "REVOKED" || row.UserID != adminUserID || row.Channel != "ANDROID" {
			return ActivationResult{}, fmt.Errorf("session revoke is not durable")
		}
		path = "internal:session-revoke/" + subjectID
	default:
		return ActivationResult{}, fmt.Errorf("unknown security change")
	}

	requestHash := hashOperation(struct {
		Operation string `json:"operation"`
		SubjectID string `json:"subjectId"`
	}{operation, subjectID})
	if existing, found, err := s.findOperationIdempotent(ctx, adminUserID, path, idempotencyKey, requestHash); err != nil || found {
		return existing, err
	}

	if securitySubject(operation) == securityUserEnable {
		row, err := s.Client.User.Get(ctx, subjectID)
		if err != nil {
			return ActivationResult{}, err
		}
		if row.Status != "DISABLED" {
			return ActivationResult{}, fmt.Errorf("user is not disabled")
		}
	}
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return ActivationResult{}, err
	}
	content, err := s.activeReleaseContent(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	revision := int(managed.DesiredControlRevision) + 1
	generation := int(managed.ActiveManagedGeneration)
	state, err := s.compileOperationalState(ctx, content, generation, revision, nil)
	if err != nil {
		return ActivationResult{}, err
	}
	applySecurityPrincipal(&state.PrincipalState, securitySubject(operation), subjectID)
	activationID := platformid.New(platformid.Activation)
	now := s.Now().UTC()
	pendingJSON, _ := json.Marshal(map[string]string{"operation": operation, "subjectId": subjectID})

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
	if err != nil || fresh.DesiredControlRevision+1 != int64(revision) {
		return ActivationResult{}, ErrActivationInProgress
	}
	// deny-first: local Hub state changes before Relay network I/O. Enable is deliberately allow-last.
	switch securitySubject(operation) {
	case securityUserDisable:
		if _, err := tx.User.UpdateOneID(subjectID).SetStatus("DISABLED").SetUpdatedAt(now).Save(ctx); err != nil {
			return ActivationResult{}, err
		}
	case securityDeviceRevoke:
		if _, err := tx.Device.UpdateOneID(subjectID).SetStatus("REVOKED").SetRevokedAt(now).Save(ctx); err != nil {
			return ActivationResult{}, err
		}
	}

	if securitySubject(operation) == securityUserDisable || securitySubject(operation) == securityDeviceRevoke {
		var subject predicate.Session = session.UserIDEQ(subjectID)
		if securitySubject(operation) == securityDeviceRevoke {
			subject = session.DeviceIDEQ(subjectID)
		}
		sessions, err := tx.Session.Query().Where(subject).All(ctx)
		if err != nil {
			return ActivationResult{}, err
		}
		for _, se := range sessions {
			state.PrincipalState.RevokedSessionIds = addSorted(state.PrincipalState.RevokedSessionIds, se.ID)
		}
		if _, err := tx.Session.Update().Where(subject, session.StatusEQ("ACTIVE")).SetStatus("REVOKED").SetRevokedAt(now).
			ClearPreviousRefreshDigest().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().Save(ctx); err != nil {
			return ActivationResult{}, err
		}
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
	if _, err := tx.Activation.Create().
		SetID(activationID).SetKind("SECURITY_CHANGE").SetState("APPLYING").
		SetIdempotencyKey(idempotencyKey).SetRequestHash(requestHash).SetControlRevision(int64(revision)).
		SetBundleHash(string(hash)).SetTargetDescriptorJSON(descriptor).SetSubjectID(subjectID).
		SetPendingOperationJSON(pendingJSON).SetCreatedByUserID(adminUserID).SetCreatedAt(now).Save(ctx); err != nil {
		return ActivationResult{}, err
	}
	if _, err := tx.IdempotencyRecord.Create().SetAdminUserID(adminUserID).SetMethod("POST").SetNormalizedPath(path).
		SetIdempotencyKey(idempotencyKey).SetRequestHash(requestHash).SetActivationID(activationID).SetCreatedAt(now).Save(ctx); err != nil {
		return ActivationResult{}, err
	}
	if _, err := tx.ManagedState.UpdateOneID("current").SetDesiredControlRevision(int64(revision)).SetDesiredBundleHash(string(hash)).
		SetRuntimeStatus("ACTIVATING").SetManagedStateRevision(fresh.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
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
	if ack.AppliedControlRevision != revision || string(ack.BundleHash) != string(hash) || ack.ActiveManagedGeneration != generation {
		_ = s.markFailed(ctx, activationID, "relay_ack_mismatch")
		return ActivationResult{}, ErrRelayAckMismatch
	}
	if err := s.finalizeSecurityChange(ctx, activationID, securitySubject(operation), subjectID, revision, string(hash)); err != nil {
		return ActivationResult{}, err
	}
	return s.loadActivation(ctx, activationID)
}

func applySecurityPrincipal(principal *relaycontrolapi.PrincipalState, operation securitySubject, subjectID string) {
	switch operation {
	case securityUserDisable:
		principal.DisabledUserIds = addSorted(principal.DisabledUserIds, subjectID)
	case securityUserEnable:
		principal.DisabledUserIds = removeSorted(principal.DisabledUserIds, subjectID)
	case securitySessionRevoke:
		principal.RevokedSessionIds = addSorted(principal.RevokedSessionIds, subjectID)
	case securityDeviceRevoke:
		principal.RevokedDeviceIds = addSorted(principal.RevokedDeviceIds, subjectID)
	}
}

func addSorted(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	result := append(append([]string(nil), values...), value)
	sort.Strings(result)
	return result
}

func removeSorted(values []string, value string) []string {
	result := make([]string, 0, len(values))
	for _, existing := range values {
		if existing != value {
			result = append(result, existing)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Service) finalizeSecurityChange(ctx context.Context, activationID string, operation securitySubject, subjectID string, revision int, bundleHash string) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil || managed.DesiredControlRevision != int64(revision) || managed.DesiredBundleHash == nil || *managed.DesiredBundleHash != bundleHash {
		return ErrRelayAckMismatch
	}
	now := s.Now().UTC()
	if operation == securityUserEnable {
		if _, err := tx.User.UpdateOneID(subjectID).SetStatus("ACTIVE").SetUpdatedAt(now).Save(ctx); err != nil {
			return err
		}
	}
	if _, err := tx.Activation.UpdateOneID(activationID).SetState("COMPLETED").SetCompletedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := tx.ManagedState.UpdateOneID("current").SetRuntimeStatus("READY").SetManagedStateRevision(managed.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
