package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/deletedcredential"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/ent/predicate"
	"measix/platform/ent/session"
	"measix/platform/ent/user"
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
	securityUserDelete    securitySubject = "USER_DELETE"
)

type DeleteUserInput struct {
	ConfirmationUsername string
	Reason               string
}

var (
	ErrDeleteConfirmation = errors.New("user deletion confirmation does not match")
	ErrDeleteSelf         = errors.New("an administrator cannot delete the active account")
	ErrDeleteLastAdmin    = errors.New("cannot delete the last administrator")
	ErrDeleteInFlight     = errors.New("user deletion is waiting for active requests to finish")
)

func (s *Service) DisableUser(ctx context.Context, adminUserID, idempotencyKey, userID string) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityUserDisable), userID, nil)
}

func (s *Service) EnableUser(ctx context.Context, adminUserID, idempotencyKey, userID string) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityUserEnable), userID, nil)
}

func (s *Service) RevokeDevice(ctx context.Context, adminUserID, idempotencyKey, deviceID string) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityDeviceRevoke), deviceID, nil)
}

func (s *Service) DeleteUser(ctx context.Context, adminUserID, idempotencyKey, userID string, input DeleteUserInput) (ActivationResult, error) {
	return s.securityChange(ctx, adminUserID, idempotencyKey, string(securityUserDelete), userID, &input)
}

func (s *Service) securityChange(ctx context.Context, adminUserID, idempotencyKey, operation, subjectID string, deleteInput *DeleteUserInput) (ActivationResult, error) {
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
	case securityUserDelete:
		if subjectID == adminUserID {
			return ActivationResult{}, ErrDeleteSelf
		}
		if platformid.Validate(platformid.User, subjectID) != nil || deleteInput == nil || strings.TrimSpace(deleteInput.Reason) == "" {
			return ActivationResult{}, fmt.Errorf("invalid user deletion request")
		}
		path = "/api/admin/v1/users/" + subjectID
	default:
		return ActivationResult{}, fmt.Errorf("unknown security change")
	}

	requestHash := hashOperation(struct {
		Operation            string `json:"operation"`
		SubjectID            string `json:"subjectId"`
		ConfirmationUsername string `json:"confirmationUsername,omitempty"`
		Reason               string `json:"reason,omitempty"`
	}{operation, subjectID, func() string {
		if deleteInput == nil {
			return ""
		}
		return deleteInput.ConfirmationUsername
	}(), func() string {
		if deleteInput == nil {
			return ""
		}
		return strings.TrimSpace(deleteInput.Reason)
	}()})
	if existing, found, err := s.findOperationIdempotent(ctx, adminUserID, path, idempotencyKey, requestHash); err != nil || found {
		return existing, err
	}
	if securitySubject(operation) == securityUserDelete {
		row, err := s.Client.User.Get(ctx, subjectID)
		if err != nil {
			return ActivationResult{}, err
		}
		if row.Username != deleteInput.ConfirmationUsername {
			return ActivationResult{}, ErrDeleteConfirmation
		}
		if row.Role == "ADMIN" {
			count, err := s.Client.User.Query().Where(user.RoleEQ("ADMIN"), user.StatusEQ("ACTIVE")).Count(ctx)
			if err != nil {
				return ActivationResult{}, err
			}
			if count <= 1 {
				return ActivationResult{}, ErrDeleteLastAdmin
			}
		}
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
	state, err := s.compileState(ctx, content, generation, revision, nil)
	if err != nil {
		return ActivationResult{}, err
	}
	applySecurityPrincipal(&state.PrincipalState, securitySubject(operation), subjectID)
	activationID := platformid.New(platformid.Activation)
	now := s.Now().UTC()
	pendingOperation := map[string]string{"operation": operation, "subjectId": subjectID}
	if deleteInput != nil {
		pendingOperation["reason"] = strings.TrimSpace(deleteInput.Reason)
	}
	pendingJSON, _ := json.Marshal(pendingOperation)

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
	case securityUserDelete:
		// The in-flight check and tombstone write share this transaction. That
		// closes the admission race: a request is either already visible here
		// and deletion is rejected, or the tombstone wins and later admissions
		// fail closed.
		active, err := tx.BudgetRequest.Query().Where(
			budgetrequest.UserIDEQ(subjectID),
			budgetrequest.StateIn(budgetrequest.StateADMITTED, budgetrequest.StateSTARTED, budgetrequest.StateRECONCILIATION),
		).Exist(ctx)
		if err != nil {
			return ActivationResult{}, err
		}
		if active {
			return ActivationResult{}, ErrDeleteInFlight
		}
		if _, err := tx.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(subjectID)).Only(ctx); err == nil {
			// An existing tombstone means an earlier idempotent deletion already
			// established the durable deny fact.
		} else if !ent.IsNotFound(err) {
			return ActivationResult{}, err
		} else if _, err := tx.DeletedPrincipal.Create().SetID(subjectID).SetDeletedAt(now).Save(ctx); err != nil {
			return ActivationResult{}, err
		}
	}

	if securitySubject(operation) == securityUserDisable || securitySubject(operation) == securityDeviceRevoke || securitySubject(operation) == securityUserDelete {
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
			if securitySubject(operation) == securityUserDelete {
				for _, digest := range []*[]byte{se.RefreshDigest, se.PreviousRefreshDigest} {
					if digest == nil || len(*digest) == 0 {
						continue
					}
					exists, err := tx.DeletedCredential.Query().Where(deletedcredential.DigestEQ(*digest)).Exist(ctx)
					if err != nil {
						return ActivationResult{}, err
					}
					if !exists {
						if _, err := tx.DeletedCredential.Create().SetDigest(*digest).SetDeletedAt(now).Save(ctx); err != nil {
							return ActivationResult{}, err
						}
					}
				}
			}
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
	case securityUserDelete:
		principal.DeletedUserIds = addSorted(principal.DeletedUserIds, subjectID)
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
	if operation == securityUserDelete {
		if err := purgeUserData(ctx, tx, subjectID, activationID); err != nil {
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
