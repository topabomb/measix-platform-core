package identity

import (
	"context"
	"errors"
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
	ErrInvalidInput    = errors.New("invalid identity input")
	ErrRefreshConflict = errors.New("refresh conflict")
	ErrNotFound        = errors.New("identity not found")
	ErrConflict        = errors.New("identity conflict")
	ErrCredential      = errors.New("invalid credential")
	ErrExpired         = errors.New("credential expired")
	ErrRevoked         = errors.New("identity revoked")
	ErrAlreadyUsed     = errors.New("enrollment already used")
	ErrNotAuthorized   = errors.New("not authorized")
)

type Service struct {
	BootstrapTimezone string
	Client            *ent.Client
	Signer            *security.AccessSigner
	CSRFKey           []byte
	Now               func() time.Time
	Random            func(int) (string, error)
}

func New(client *ent.Client, signer *security.AccessSigner, csrfKey []byte) *Service {
	return &Service{
		BootstrapTimezone: "UTC",
		Client:            client,
		Signer:            signer,
		CSRFKey:           append([]byte(nil), csrfKey...),
		Now:               time.Now,
		Random:            security.RandomToken,
	}
}

func NormalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) CreateUser(ctx context.Context, username, displayName, role string) (*ent.User, error) {
	return s.createUser(ctx, username, displayName, role, nil)
}

func (s *Service) CreateAdmin(ctx context.Context, username, displayName, password string) (*ent.User, error) {
	hash, err := security.HashPassword(password)
	if errors.Is(err, security.ErrInvalidPassword) {
		return nil, ErrInvalidInput
	}
	if err != nil {
		return nil, err
	}
	return s.createUser(ctx, username, displayName, "ADMIN", &hash)
}

func (s *Service) createUser(ctx context.Context, username, displayName, role string, passwordHash *string) (*ent.User, error) {
	username = NormalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || displayName == "" || (role != "ADMIN" && role != "MEMBER") {
		return nil, ErrInvalidInput
	}
	now := s.Now().UTC()
	u, err := s.Client.User.Create().
		SetID(platformid.New(platformid.User)).
		SetUsername(username).
		SetDisplayName(displayName).
		SetRole(role).
		SetNillablePasswordHash(passwordHash).
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

func (s *Service) UpdateUser(ctx context.Context, userID, username, displayName, role string) (*ent.User, error) {
	username = NormalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || displayName == "" || (role != "ADMIN" && role != "MEMBER") {
		return nil, ErrInvalidInput
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
	if errors.Is(err, security.ErrInvalidPassword) {
		return ErrInvalidInput
	}
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
		return EnrollmentGrant{}, ErrInvalidInput
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return EnrollmentGrant{}, err
	}
	if u.Status != "ACTIVE" {
		return EnrollmentGrant{}, ErrRevoked
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

func (s *Service) ExchangeEnrollment(ctx context.Context, code, installationID, deviceName, appVersion string) (ExchangeResult, error) {
	if err := platformid.Validate(platformid.Installation, installationID); err != nil || strings.TrimSpace(deviceName) == "" || strings.TrimSpace(appVersion) == "" || code == "" {
		return ExchangeResult{}, ErrInvalidInput
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
	existing, queryErr := tx.Device.Query().Where(device.InstallationIDEQ(installationID)).Only(ctx)
	if queryErr != nil && !ent.IsNotFound(queryErr) {
		return rollback(queryErr)
	}
	if existing != nil {
		if existing.UserID != e.UserID {
			return rollback(ErrConflict)
		}
		if existing.Status != "ACTIVE" {
			return rollback(ErrRevoked)
		}
	}
	refreshToken, err := s.Random(32)
	if err != nil {
		return rollback(err)
	}
	deviceID := platformid.New(platformid.Device)
	sessionID := platformid.New(platformid.Session)
	refreshExpiresAt := now.Add(sessionIdleTTL)
	if existing == nil {
		if _, err := tx.Device.Create().
			SetID(deviceID).
			SetUserID(e.UserID).
			SetInstallationID(installationID).
			SetName(strings.TrimSpace(deviceName)).
			SetStatus("ACTIVE").
			SetNillableAppVersion(optionalString(appVersion)).
			SetCreatedAt(now).
			Save(ctx); err != nil {
			return rollback(err)
		}
	} else {
		deviceID = existing.ID
		if _, err := tx.Device.UpdateOneID(deviceID).SetName(strings.TrimSpace(deviceName)).SetAppVersion(strings.TrimSpace(appVersion)).Save(ctx); err != nil {
			return rollback(err)
		}
		if _, err := tx.Session.Update().Where(session.DeviceIDEQ(deviceID), session.StatusEQ("ACTIVE")).SetStatus("REVOKED").SetRevokedAt(now).
			ClearPreviousRefreshDigest().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().Save(ctx); err != nil {
			return rollback(err)
		}
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
	accessToken, accessExpiresAt, err := s.Signer.Sign(e.UserID, deviceID, sessionID)
	if err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
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
	if ent.IsNotFound(err) {
		return AccessPrincipal{}, ErrRevoked
	}
	if err != nil {
		return AccessPrincipal{}, err
	}
	if !s.Now().UTC().Before(se.ExpiresAt) {
		return AccessPrincipal{}, ErrExpired
	}
	if se.Status != "ACTIVE" || se.Channel != "ANDROID" {
		return AccessPrincipal{}, ErrRevoked
	}
	if se.UserID != claims.Subject || se.DeviceID == nil || *se.DeviceID != claims.DeviceID {
		return AccessPrincipal{}, ErrCredential
	}
	u, err := s.Client.User.Get(ctx, claims.Subject)
	if ent.IsNotFound(err) {
		return AccessPrincipal{}, ErrRevoked
	}
	if err != nil {
		return AccessPrincipal{}, err
	}
	if u.Status != "ACTIVE" {
		return AccessPrincipal{}, ErrRevoked
	}
	d, err := s.Client.Device.Get(ctx, claims.DeviceID)
	if ent.IsNotFound(err) {
		return AccessPrincipal{}, ErrRevoked
	}
	if err != nil {
		return AccessPrincipal{}, err
	}
	if d.Status != "ACTIVE" || d.UserID != u.ID {
		return AccessPrincipal{}, ErrRevoked
	}
	return AccessPrincipal{
		DeploymentID: claims.DeploymentID,
		UserID:       u.ID,
		DeviceID:     d.ID,
		SessionID:    se.ID,
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	digest := security.DigestToken(refreshToken)
	se, err := s.Client.Session.Query().Where(session.ChannelEQ("ANDROID"), session.Or(session.RefreshDigestEQ(digest), session.And(session.PreviousRefreshDigestEQ(digest), session.RefreshReplayUntilGT(s.Now().UTC())))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return err
	}
	_, err = s.Client.Session.UpdateOneID(se.ID).
		SetStatus("REVOKED").
		ClearPreviousRefreshDigest().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().
		SetRevokedAt(s.Now().UTC()).
		Save(ctx)
	return err
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

func (s *Service) LoginAdmin(ctx context.Context, username, password string) (AdminSessionResult, error) {
	u, err := s.Client.User.Query().Where(user.UsernameEQ(NormalizeUsername(username))).Only(ctx)
	if ent.IsNotFound(err) {
		return AdminSessionResult{}, ErrCredential
	}
	if err != nil {
		return AdminSessionResult{}, err
	}
	if u.Role != "ADMIN" || u.Status != "ACTIVE" || u.PasswordHash == nil || !security.VerifyPassword(*u.PasswordHash, password) {
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
	if ent.IsNotFound(err) {
		return nil, nil, ErrNotAuthorized
	}
	if err != nil {
		return nil, nil, err
	}
	if se.Channel != "ADMIN_WEB" || se.Status != "ACTIVE" || !s.Now().UTC().Before(se.ExpiresAt) {
		return nil, nil, ErrNotAuthorized
	}
	u, err := s.Client.User.Get(ctx, se.UserID)
	if ent.IsNotFound(err) {
		return nil, nil, ErrNotAuthorized
	}
	if err != nil {
		return nil, nil, err
	}
	if u.Status != "ACTIVE" || u.Role != "ADMIN" {
		return nil, nil, ErrNotAuthorized
	}
	return u, se, nil
}

func (s *Service) LogoutAdmin(ctx context.Context, cookieSecret string) error {
	se, err := s.Client.Session.Query().Where(session.ChannelEQ("ADMIN_WEB"), session.RefreshDigestEQ(security.DigestToken(cookieSecret))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return err
	}
	_, err = s.Client.Session.UpdateOneID(se.ID).
		SetStatus("REVOKED").
		ClearPreviousRefreshDigest().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().
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

func (s *Service) ListUserViews(ctx context.Context, limit int, after string) ([]UserView, error) {
	rows, err := s.Client.User.Query().Where(user.IDGT(after)).Order(ent.Asc(user.FieldID)).Limit(limit).All(ctx)
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

func (s *Service) ListDeviceViews(ctx context.Context, userID string, limit int, after string) ([]DeviceView, error) {
	if _, err := s.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.Client.Device.Query().Where(device.UserIDEQ(userID), device.IDGT(after)).Order(ent.Asc(device.FieldID)).Limit(limit).All(ctx)
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
