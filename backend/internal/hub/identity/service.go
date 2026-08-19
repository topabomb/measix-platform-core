package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/device"
	"measix/platform/ent/enrollment"
	"measix/platform/ent/session"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
)

var (
	ErrNotFound      = errors.New("identity not found")
	ErrConflict      = errors.New("identity conflict")
	ErrCredential    = errors.New("invalid credential")
	ErrExpired       = errors.New("credential expired")
	ErrRevoked       = errors.New("identity revoked")
	ErrAlreadyUsed   = errors.New("enrollment already used")
	ErrNotAuthorized = errors.New("not authorized")
)

type Service struct {
	Client  *ent.Client
	Signer  *security.AccessSigner
	CSRFKey []byte
	Now     func() time.Time
	Random  func(int) (string, error)
}

func New(client *ent.Client, signer *security.AccessSigner, csrfKey []byte) *Service {
	return &Service{
		Client:  client,
		Signer:  signer,
		CSRFKey: append([]byte(nil), csrfKey...),
		Now:     time.Now,
		Random:  security.RandomToken,
	}
}

func NormalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) CreateUser(ctx context.Context, username, displayName, role string) (*ent.User, error) {
	username = NormalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || displayName == "" || (role != "ADMIN" && role != "MEMBER") {
		return nil, fmt.Errorf("invalid user")
	}
	now := s.Now().UTC()
	u, err := s.Client.User.Create().
		SetID(platformid.New(platformid.User)).
		SetUsername(username).
		SetDisplayName(displayName).
		SetRole(role).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, ErrConflict
	}
	return u, err
}

func (s *Service) GetUser(ctx context.Context, userID string) (*ent.User, error) {
	u, err := s.Client.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Service) ListUsers(ctx context.Context, limit int) ([]*ent.User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Client.User.Query().Order(ent.Asc(user.FieldUsername)).Limit(limit).All(ctx)
}

func (s *Service) UpdateUser(ctx context.Context, userID, username, displayName, role string) (*ent.User, error) {
	username = NormalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || displayName == "" || (role != "ADMIN" && role != "MEMBER") {
		return nil, fmt.Errorf("invalid user")
	}
	n, err := s.Client.User.Update().Where(user.IDEQ(userID)).
		SetUsername(username).
		SetDisplayName(displayName).
		SetRole(role).
		SetUpdatedAt(s.Now().UTC()).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, ErrNotFound
	}
	return s.GetUser(ctx, userID)
}

func (s *Service) SetPassword(ctx context.Context, userID, password string) error {
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	n, err := s.Client.User.Update().Where(user.IDEQ(userID)).
		SetPasswordHash(hash).
		SetUpdatedAt(s.Now().UTC()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) CreateEnrollment(ctx context.Context, userID, createdBy string, ttl time.Duration) (EnrollmentGrant, error) {
	if ttl == 0 {
		ttl = 10 * time.Minute
	}
	if ttl < time.Minute || ttl > time.Hour {
		return EnrollmentGrant{}, fmt.Errorf("enrollment TTL out of range")
	}
	if _, err := s.GetUser(ctx, userID); err != nil {
		return EnrollmentGrant{}, err
	}
	code, err := s.Random(16)
	if err != nil {
		return EnrollmentGrant{}, err
	}
	now := s.Now().UTC()
	id := platformid.New(platformid.Enrollment)
	expiresAt := now.Add(ttl)
	_, err = s.Client.Enrollment.Create().
		SetID(id).
		SetUserID(userID).
		SetTokenDigest(security.DigestToken(code)).
		SetExpiresAt(expiresAt).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return EnrollmentGrant{}, err
	}
	return EnrollmentGrant{EnrollmentID: id, Code: code, ExpiresAt: expiresAt}, nil
}

func (s *Service) ExchangeEnrollment(ctx context.Context, code, installationID, appVersion string) (ExchangeResult, error) {
	if err := platformid.Validate(platformid.Installation, installationID); err != nil {
		return ExchangeResult{}, fmt.Errorf("invalid installation id")
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return ExchangeResult{}, err
	}
	rollback := func(cause error) (ExchangeResult, error) {
		_ = tx.Rollback()
		return ExchangeResult{}, cause
	}
	e, err := tx.Enrollment.Query().Where(enrollment.TokenDigestEQ(security.DigestToken(code))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(ErrCredential)
		}
		return rollback(err)
	}
	now := s.Now().UTC()
	if e.ConsumedAt != nil {
		return rollback(ErrAlreadyUsed)
	}
	if !now.Before(e.ExpiresAt) {
		return rollback(ErrExpired)
	}
	u, err := tx.User.Get(ctx, e.UserID)
	if err != nil {
		return rollback(err)
	}
	if u.Status != "ACTIVE" {
		return rollback(ErrRevoked)
	}
	if existing, queryErr := tx.Device.Query().Where(device.InstallationIDEQ(installationID)).Only(ctx); queryErr == nil && existing.ID != "" {
		return rollback(ErrConflict)
	} else if queryErr != nil && !ent.IsNotFound(queryErr) {
		return rollback(queryErr)
	}
	refreshToken, err := s.Random(32)
	if err != nil {
		return rollback(err)
	}
	deviceID := platformid.New(platformid.Device)
	sessionID := platformid.New(platformid.Session)
	refreshExpiresAt := now.Add(30 * 24 * time.Hour)
	if _, err := tx.Device.Create().
		SetID(deviceID).
		SetUserID(e.UserID).
		SetInstallationID(installationID).
		SetStatus("ACTIVE").
		SetNillableAppVersion(optionalString(appVersion)).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.Session.Create().
		SetID(sessionID).
		SetUserID(e.UserID).
		SetDeviceID(deviceID).
		SetChannel("ANDROID").
		SetRefreshDigest(security.DigestToken(refreshToken)).
		SetExpiresAt(refreshExpiresAt).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.Enrollment.UpdateOneID(e.ID).SetConsumedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return ExchangeResult{}, err
	}
	accessToken, accessExpiresAt, err := s.Signer.Sign(e.UserID, deviceID, sessionID)
	if err != nil {
		return ExchangeResult{}, err
	}
	return ExchangeResult{
		DeploymentID:         s.Signer.DeploymentID,
		UserID:               e.UserID,
		DeviceID:             deviceID,
		SessionID:            sessionID,
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessExpiresAt,
		RefreshToken:         refreshToken,
		RefreshExpiresAt:     refreshExpiresAt,
	}, nil
}

func (s *Service) AuthenticateAccess(ctx context.Context, token string) (AccessPrincipal, error) {
	claims, err := s.Signer.Verify(token)
	if err != nil {
		return AccessPrincipal{}, ErrCredential
	}
	se, err := s.Client.Session.Get(ctx, claims.SessionID)
	if err != nil || se.Status != "ACTIVE" || se.Channel != "ANDROID" || !s.Now().UTC().Before(se.ExpiresAt) {
		return AccessPrincipal{}, ErrRevoked
	}
	if se.UserID != claims.Subject || se.DeviceID == nil || *se.DeviceID != claims.DeviceID {
		return AccessPrincipal{}, ErrCredential
	}
	u, err := s.Client.User.Get(ctx, claims.Subject)
	if err != nil || u.Status != "ACTIVE" {
		return AccessPrincipal{}, ErrRevoked
	}
	d, err := s.Client.Device.Get(ctx, claims.DeviceID)
	if err != nil || d.Status != "ACTIVE" || d.UserID != u.ID {
		return AccessPrincipal{}, ErrRevoked
	}
	return AccessPrincipal{
		DeploymentID: claims.DeploymentID,
		UserID:       u.ID,
		DeviceID:     d.ID,
		SessionID:    se.ID,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, time.Time, error) {
	se, err := s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(refreshToken))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", time.Time{}, ErrCredential
		}
		return "", time.Time{}, err
	}
	if se.Channel != "ANDROID" || se.Status != "ACTIVE" {
		return "", time.Time{}, ErrRevoked
	}
	if !s.Now().UTC().Before(se.ExpiresAt) {
		return "", time.Time{}, ErrExpired
	}
	u, err := s.Client.User.Get(ctx, se.UserID)
	if err != nil || u.Status != "ACTIVE" {
		return "", time.Time{}, ErrRevoked
	}
	if se.DeviceID == nil {
		return "", time.Time{}, ErrCredential
	}
	d, err := s.Client.Device.Get(ctx, *se.DeviceID)
	if err != nil || d.Status != "ACTIVE" {
		return "", time.Time{}, ErrRevoked
	}
	_, _ = s.Client.Session.UpdateOneID(se.ID).SetLastUsedAt(s.Now().UTC()).Save(ctx)
	return s.Signer.Sign(se.UserID, d.ID, se.ID)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	se, err := s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(refreshToken))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return err
	}
	_, err = s.Client.Session.UpdateOneID(se.ID).
		SetStatus("REVOKED").
		SetRevokedAt(s.Now().UTC()).
		Save(ctx)
	return err
}

func (s *Service) DisableUser(ctx context.Context, userID string) error {
	now := s.Now().UTC()
	n, err := s.Client.User.Update().Where(user.IDEQ(userID)).
		SetStatus("DISABLED").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	_, err = s.Client.Session.Update().Where(session.UserIDEQ(userID), session.StatusEQ("ACTIVE")).
		SetStatus("REVOKED").
		SetRevokedAt(now).
		Save(ctx)
	return err
}

func (s *Service) EnableUserLocal(ctx context.Context, userID string) error {
	n, err := s.Client.User.Update().Where(user.IDEQ(userID)).
		SetStatus("ACTIVE").
		SetUpdatedAt(s.Now().UTC()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListDevices(ctx context.Context, userID string, limit int) ([]*ent.Device, error) {
	if _, err := s.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Client.Device.Query().Where(device.UserIDEQ(userID)).Limit(limit).All(ctx)
}

func (s *Service) RevokeDevice(ctx context.Context, deviceID string) error {
	now := s.Now().UTC()
	n, err := s.Client.Device.Update().Where(device.IDEQ(deviceID)).
		SetStatus("REVOKED").
		SetRevokedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	_, err = s.Client.Session.Update().Where(session.DeviceIDEQ(deviceID), session.StatusEQ("ACTIVE")).
		SetStatus("REVOKED").
		SetRevokedAt(now).
		Save(ctx)
	return err
}

func (s *Service) LoginAdmin(ctx context.Context, username, password string) (AdminSessionResult, error) {
	u, err := s.Client.User.Query().Where(user.UsernameEQ(NormalizeUsername(username))).Only(ctx)
	if err != nil || u.Role != "ADMIN" || u.Status != "ACTIVE" || u.PasswordHash == nil || !security.VerifyPassword(*u.PasswordHash, password) {
		return AdminSessionResult{}, ErrCredential
	}
	cookieSecret, err := s.Random(32)
	if err != nil {
		return AdminSessionResult{}, err
	}
	now := s.Now().UTC()
	expiresAt := now.Add(12 * time.Hour)
	_, err = s.Client.Session.Create().
		SetID(platformid.New(platformid.Session)).
		SetUserID(u.ID).
		SetChannel("ADMIN_WEB").
		SetRefreshDigest(security.DigestToken(cookieSecret)).
		SetExpiresAt(expiresAt).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return AdminSessionResult{}, err
	}
	return AdminSessionResult{
		UserID:       u.ID,
		DisplayName:  u.DisplayName,
		Role:         u.Role,
		CookieSecret: cookieSecret,
		CSRFToken:    security.CSRFToken(cookieSecret, s.CSRFKey),
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *Service) AuthenticateAdmin(ctx context.Context, cookieSecret, csrfToken string, requireCSRF bool) (*ent.User, *ent.Session, error) {
	if cookieSecret == "" || (requireCSRF && !security.VerifyCSRF(cookieSecret, csrfToken, s.CSRFKey)) {
		return nil, nil, ErrNotAuthorized
	}
	se, err := s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(cookieSecret))).Only(ctx)
	if err != nil || se.Channel != "ADMIN_WEB" || se.Status != "ACTIVE" || !s.Now().UTC().Before(se.ExpiresAt) {
		return nil, nil, ErrNotAuthorized
	}
	u, err := s.Client.User.Get(ctx, se.UserID)
	if err != nil || u.Status != "ACTIVE" || u.Role != "ADMIN" {
		return nil, nil, ErrNotAuthorized
	}
	return u, se, nil
}

func (s *Service) LogoutAdmin(ctx context.Context, cookieSecret string) error {
	se, err := s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(cookieSecret))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return err
	}
	_, err = s.Client.Session.UpdateOneID(se.ID).
		SetStatus("REVOKED").
		SetRevokedAt(s.Now().UTC()).
		Save(ctx)
	return err
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func userView(row *ent.User) UserView {
	return UserView{ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, Role: row.Role, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func deviceView(row *ent.Device) DeviceView {
	return DeviceView{ID: row.ID, UserID: row.UserID, InstallationID: row.InstallationID, AppVersion: row.AppVersion, LastSeenAt: row.LastSeenAt, Status: row.Status}
}

func (s *Service) CreateUserView(ctx context.Context, username, displayName, role string) (UserView, error) {
	row, err := s.CreateUser(ctx, username, displayName, role)
	if err != nil {
		return UserView{}, err
	}
	return userView(row), nil
}

func (s *Service) GetUserView(ctx context.Context, userID string) (UserView, error) {
	row, err := s.GetUser(ctx, userID)
	if err != nil {
		return UserView{}, err
	}
	return userView(row), nil
}

func (s *Service) ListUserViews(ctx context.Context, limit int) ([]UserView, error) {
	rows, err := s.ListUsers(ctx, limit)
	if err != nil {
		return nil, err
	}
	views := make([]UserView, 0, len(rows))
	for _, row := range rows {
		views = append(views, userView(row))
	}
	return views, nil
}

func (s *Service) UpdateUserView(ctx context.Context, userID, username, displayName, role string) (UserView, error) {
	row, err := s.UpdateUser(ctx, userID, username, displayName, role)
	if err != nil {
		return UserView{}, err
	}
	return userView(row), nil
}

func (s *Service) ListDeviceViews(ctx context.Context, userID string, limit int) ([]DeviceView, error) {
	rows, err := s.ListDevices(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	views := make([]DeviceView, 0, len(rows))
	for _, row := range rows {
		views = append(views, deviceView(row))
	}
	return views, nil
}

func (s *Service) AuthenticateAdminView(ctx context.Context, cookieSecret, csrfToken string, requireCSRF bool) (AdminPrincipalView, error) {
	u, se, err := s.AuthenticateAdmin(ctx, cookieSecret, csrfToken, requireCSRF)
	if err != nil {
		return AdminPrincipalView{}, err
	}
	return AdminPrincipalView{UserID: u.ID, DisplayName: u.DisplayName, Role: u.Role, SessionID: se.ID, ExpiresAt: se.ExpiresAt}, nil
}
