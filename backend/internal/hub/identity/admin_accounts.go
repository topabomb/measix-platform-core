package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"measix/platform/ent"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/ent/session"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
)

var (
	ErrOwnPasswordReset      = errors.New("use the own password change flow")
	ErrCurrentAdmin          = errors.New("cannot remove the current administrator")
	ErrLastAdmin             = errors.New("cannot remove the last login-capable administrator")
	ErrRoleConflict          = errors.New("user role changed")
	ErrAdminPasswordRequired = errors.New("administrator password is required")
	ErrPasswordConfirmation  = errors.New("password confirmation mismatch")
	ErrDeletionInProgress    = errors.New("user deletion is in progress")
)

// AdminAccountAuthorization is request-local; it is never persisted or logged.
type AdminAccountAuthorization struct{ UserID, Password, Source string }

type verifiedAdmin struct {
	userID       string
	passwordHash *string
}

func (s *Service) verifyAccountActor(ctx context.Context, auth AdminAccountAuthorization, required bool) (verifiedAdmin, error) {
	proof := verifiedAdmin{userID: auth.UserID}
	if !required {
		return proof, nil
	}
	if auth.Password == "" {
		return proof, ErrInvalidInput
	}
	if !s.adminLoginLimiter.tryBeginVerification() {
		return proof, &LoginThrottledError{RetryAfter: adminPasswordVerificationBusyRetry}
	}
	defer s.adminLoginLimiter.endVerification()
	actor, err := s.Client.User.Get(ctx, auth.UserID)
	if ent.IsNotFound(err) {
		return proof, ErrNotAuthorized
	}
	if err != nil {
		return proof, err
	}
	now := s.Now().UTC()
	if wait := s.adminLoginLimiter.retryAfter(actor.Username, auth.Source, now); wait > 0 {
		return proof, &LoginThrottledError{RetryAfter: wait}
	}
	if actor.Status != "ACTIVE" || actor.Role != "ADMIN" {
		return proof, ErrNotAuthorized
	}
	if !security.VerifyPasswordOrDummy(actor.PasswordHash, auth.Password) {
		s.adminLoginLimiter.failure(actor.Username, auth.Source, now)
		return proof, ErrCurrentPassword
	}
	s.adminLoginLimiter.success(actor.Username, now)
	proof.passwordHash = actor.PasswordHash
	return proof, nil
}

// RequireActiveAdmin rechecks authorization inside the caller's mutation transaction.
func RequireActiveAdmin(ctx context.Context, tx *ent.Tx, actorID string) error {
	actor, err := writableAccount(ctx, tx, actorID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrDeletionInProgress) {
			return ErrNotAuthorized
		}
		return err
	}
	if actor.Role != "ADMIN" || actor.Status != "ACTIVE" || actor.PasswordHash == nil || *actor.PasswordHash == "" {
		return ErrNotAuthorized
	}
	return nil
}

func (proof verifiedAdmin) recheck(ctx context.Context, tx *ent.Tx) error {
	if err := RequireActiveAdmin(ctx, tx, proof.userID); err != nil {
		return err
	}
	if proof.passwordHash != nil {
		actor, err := tx.User.Get(ctx, proof.userID)
		if err != nil {
			return err
		}
		if actor.PasswordHash == nil || *actor.PasswordHash != *proof.passwordHash {
			return ErrCurrentPassword
		}
	}
	return nil
}

func writableAccount(ctx context.Context, tx *ent.Tx, id string) (*ent.User, error) {
	deleted, err := tx.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(id)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if deleted {
		return nil, ErrDeletionInProgress
	}
	row, err := tx.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return row, err
}

// GuardAdminRemoval counts only accounts that can still log in, in the same
// write transaction as role/disable/delete. SQLite BEGIN IMMEDIATE serializes
// competing account mutations without introducing a persisted lock or revision.
func GuardAdminRemoval(ctx context.Context, tx *ent.Tx, actorID string, target *ent.User) error {
	if actorID == target.ID {
		return ErrCurrentAdmin
	}
	if target.Role != "ADMIN" || target.Status != "ACTIVE" || target.PasswordHash == nil || *target.PasswordHash == "" {
		return nil
	}
	count, err := tx.User.Query().Where(user.IDNEQ(target.ID), user.RoleEQ("ADMIN"), user.StatusEQ("ACTIVE"), user.PasswordHashNotNil(), user.PasswordHashNEQ(""), func(sel *sql.Selector) {
		deleted := sql.Table(deletedprincipal.Table)
		sel.Where(sql.Not(sql.Exists(sql.Select(deleted.C(deletedprincipal.FieldID)).From(deleted).
			Where(sql.ColumnsEQ(deleted.C(deletedprincipal.FieldID), sel.C(user.FieldID))))))
	}).Count(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrLastAdmin
	}
	return nil
}

func accountPasswordHash(password, confirmation string) (*string, error) {
	if password == "" {
		return nil, nil
	}
	if password != confirmation {
		return nil, ErrPasswordConfirmation
	}
	hash, err := security.HashPassword(password)
	if errors.Is(err, security.ErrInvalidPassword) {
		return nil, ErrInvalidInput
	}
	if err != nil {
		return nil, err
	}
	return &hash, nil
}

func revokeAdminSessions(ctx context.Context, tx *ent.Tx, userID string, now time.Time) error {
	_, err := tx.Session.Update().Where(session.UserIDEQ(userID), session.ChannelEQ("ADMIN_WEB"), session.StatusEQ("ACTIVE")).
		SetStatus("REVOKED").SetRevokedAt(now).ClearPreviousRefreshDigest().ClearRefreshRequestKey().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().Save(ctx)
	return err
}

func (s *Service) CreateAccount(ctx context.Context, auth AdminAccountAuthorization, username, displayName, role, password, confirmation string) (UserView, error) {
	username, displayName = NormalizeUsername(username), strings.TrimSpace(displayName)
	if username == "" || displayName == "" || (role != "ADMIN" && role != "MEMBER") {
		return UserView{}, ErrInvalidInput
	}
	if role == "ADMIN" && password == "" {
		return UserView{}, ErrAdminPasswordRequired
	}
	if role == "MEMBER" && (password != "" || confirmation != "") {
		return UserView{}, ErrInvalidInput
	}
	proof, err := s.verifyAccountActor(ctx, auth, role == "ADMIN")
	if err != nil {
		return UserView{}, err
	}
	hash, err := accountPasswordHash(password, confirmation)
	if err != nil {
		return UserView{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return UserView{}, err
	}
	defer tx.Rollback()
	if err := proof.recheck(ctx, tx); err != nil {
		return UserView{}, err
	}
	now := s.Now().UTC()
	row, err := tx.User.Create().SetID(platformid.New(platformid.User)).SetUsername(username).SetDisplayName(displayName).SetRole(role).
		SetNillablePasswordHash(hash).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if ent.IsConstraintError(err) {
		return UserView{}, ErrConflict
	}
	if err != nil {
		return UserView{}, err
	}
	if err := tx.Commit(); err != nil {
		return UserView{}, err
	}
	return userView(row), nil
}

func (s *Service) ResetAccountPassword(ctx context.Context, auth AdminAccountAuthorization, targetID, password, confirmation string) error {
	if auth.UserID == targetID {
		return ErrOwnPasswordReset
	}
	proof, err := s.verifyAccountActor(ctx, auth, true)
	if err != nil {
		return err
	}
	hash, err := accountPasswordHash(password, confirmation)
	if err != nil {
		return err
	}
	if hash == nil {
		return ErrInvalidInput
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := proof.recheck(ctx, tx); err != nil {
		return err
	}
	if _, err := writableAccount(ctx, tx, targetID); err != nil {
		return err
	}
	now := s.Now().UTC()
	if _, err := tx.User.UpdateOneID(targetID).SetPasswordHash(*hash).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	if err := revokeAdminSessions(ctx, tx, targetID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) SetAccountRole(ctx context.Context, auth AdminAccountAuthorization, targetID, role, expectedRole, password, confirmation string) (UserView, error) {
	return s.updateAccount(ctx, auth, targetID, nil, role, expectedRole, password, confirmation, true)
}

type accountProfile struct{ username, displayName string }

func (s *Service) UpdateAccount(ctx context.Context, auth AdminAccountAuthorization, targetID, username, displayName, role, expectedRole, password, confirmation string) (UserView, error) {
	profile := &accountProfile{NormalizeUsername(username), strings.TrimSpace(displayName)}
	if profile.username == "" || profile.displayName == "" {
		return UserView{}, ErrInvalidInput
	}
	return s.updateAccount(ctx, auth, targetID, profile, role, expectedRole, password, confirmation, auth.Password != "")
}

func (s *Service) updateAccount(ctx context.Context, auth AdminAccountAuthorization, targetID string, profile *accountProfile, role, expectedRole, password, confirmation string, verify bool) (UserView, error) {
	if role != "ADMIN" && role != "MEMBER" {
		return UserView{}, ErrInvalidInput
	}
	if expectedRole != "" && expectedRole != "ADMIN" && expectedRole != "MEMBER" {
		return UserView{}, ErrInvalidInput
	}
	proof, err := s.verifyAccountActor(ctx, auth, verify)
	if err != nil {
		return UserView{}, err
	}
	hash, err := accountPasswordHash(password, confirmation)
	if err != nil {
		return UserView{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return UserView{}, err
	}
	defer tx.Rollback()
	if err := proof.recheck(ctx, tx); err != nil {
		return UserView{}, err
	}
	row, err := writableAccount(ctx, tx, targetID)
	if err != nil {
		return UserView{}, err
	}
	if expectedRole != "" && row.Role != expectedRole {
		return UserView{}, ErrRoleConflict
	}
	changed := row.Role != role
	if changed && (proof.passwordHash == nil || expectedRole == "") {
		return UserView{}, ErrInvalidInput
	}
	if !changed && (hash != nil || confirmation != "") {
		return UserView{}, ErrInvalidInput
	}
	if changed && role == "MEMBER" {
		if hash != nil || confirmation != "" {
			return UserView{}, ErrInvalidInput
		}
		if err := GuardAdminRemoval(ctx, tx, auth.UserID, row); err != nil {
			return UserView{}, err
		}
	}
	if changed && role == "ADMIN" {
		if row.PasswordHash == nil || *row.PasswordHash == "" {
			if hash == nil {
				return UserView{}, ErrAdminPasswordRequired
			}
		} else if hash != nil {
			return UserView{}, ErrInvalidInput
		}
	}
	now := s.Now().UTC()
	mutation := tx.User.UpdateOneID(targetID).SetRole(role).SetUpdatedAt(now)
	if profile != nil {
		mutation.SetUsername(profile.username).SetDisplayName(profile.displayName)
	}
	if hash != nil {
		mutation.SetPasswordHash(*hash)
	}
	updated, err := mutation.Save(ctx)
	if ent.IsConstraintError(err) {
		return UserView{}, ErrConflict
	}
	if err != nil {
		return UserView{}, err
	}
	if changed {
		if err := revokeAdminSessions(ctx, tx, targetID, now); err != nil {
			return UserView{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return UserView{}, err
	}
	return userView(updated), nil
}
