package identity

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/ent/device"
	"measix/platform/ent/enrollment"
	"measix/platform/ent/session"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
)

const (
	minimumEnrollmentTTL = time.Minute
	defaultEnrollmentTTL = time.Hour
	maximumEnrollmentTTL = 24 * time.Hour
)

var (
	ErrInvalidInput      = errors.New("invalid identity input")
	ErrRefreshConflict   = errors.New("refresh conflict")
	ErrNotFound          = errors.New("identity not found")
	ErrConflict          = errors.New("identity conflict")
	ErrCredential        = errors.New("invalid credential")
	ErrCurrentPassword   = errors.New("invalid current password")
	ErrExpired           = errors.New("credential expired")
	ErrRevoked           = errors.New("identity revoked")
	ErrUserDisabled      = errors.New("user disabled")
	ErrDeviceRevoked     = errors.New("device revoked")
	ErrAlreadyUsed       = errors.New("enrollment already used")
	ErrNotAuthorized     = errors.New("not authorized")
	ErrPortalUnavailable = errors.New("portal unavailable")
	ErrIdentityDeleted   = errors.New("enterprise identity deleted")
)

type Service struct {
	deploymentSettingsMu  sync.Mutex
	adminLoginLimiter     *adminLoginLimiter
	publicOriginMu        sync.RWMutex
	publicOrigin          string
	PortalStaticAvailable bool
	BootstrapTimezone     string
	Client                *ent.Client
	Signer                *security.AccessSigner
	CSRFKey               []byte
	Now                   func() time.Time
	Random                func(int) (string, error)
}

// PublicOrigin returns the deployment-owned canonical address advertised to
// clients. It is mutable at runtime, so every request takes a consistent
// snapshot instead of racing with an Admin settings update.
func (s *Service) PublicOrigin() string {
	s.publicOriginMu.RLock()
	defer s.publicOriginMu.RUnlock()
	return s.publicOrigin
}

func (s *Service) SetPublicOrigin(value string) {
	s.publicOriginMu.Lock()
	s.publicOrigin = value
	s.publicOriginMu.Unlock()
}

func New(client *ent.Client, signer *security.AccessSigner, csrfKey []byte) *Service {
	return &Service{
		adminLoginLimiter:     newAdminLoginLimiter(),
		BootstrapTimezone:     "UTC",
		PortalStaticAvailable: true,
		Client:                client,
		Signer:                signer,
		CSRFKey:               append([]byte(nil), csrfKey...),
		Now:                   time.Now,
		Random:                security.RandomToken,
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
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	n, err := tx.User.Update().Where(user.IDEQ(userID)).
		SetPasswordHash(hash).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return rollback(err)
	}
	if n != 1 {
		return rollback(ErrNotFound)
	}
	_, err = tx.Session.Update().Where(
		session.UserIDEQ(userID),
		session.ChannelEQ("ADMIN_WEB"),
		session.StatusEQ("ACTIVE"),
	).SetStatus("REVOKED").SetRevokedAt(now).
		ClearPreviousRefreshDigest().ClearRefreshRequestKey().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().
		Save(ctx)
	if err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

// ChangeOwnPassword verifies the currently stored credential and atomically
// replaces it while revoking every Admin Web session owned by the caller.
func (s *Service) ChangeOwnPassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	current, err := s.Client.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current.PasswordHash == nil || !security.VerifyPassword(*current.PasswordHash, currentPassword) {
		return ErrCurrentPassword
	}
	hash, err := security.HashPassword(newPassword)
	if errors.Is(err, security.ErrInvalidPassword) {
		return ErrInvalidInput
	}
	if err != nil {
		return err
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	fresh, err := tx.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(ErrNotFound)
		}
		return rollback(err)
	}
	if fresh.PasswordHash == nil || *fresh.PasswordHash != *current.PasswordHash {
		return rollback(ErrCurrentPassword)
	}
	if _, err = tx.User.UpdateOneID(userID).SetPasswordHash(hash).SetUpdatedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err = tx.Session.Update().Where(
		session.UserIDEQ(userID),
		session.ChannelEQ("ADMIN_WEB"),
		session.StatusEQ("ACTIVE"),
	).SetStatus("REVOKED").SetRevokedAt(now).
		ClearPreviousRefreshDigest().ClearRefreshRequestKey().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().
		Save(ctx); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func (s *Service) CreateEnrollment(ctx context.Context, userID, createdBy string, ttl time.Duration) (EnrollmentGrant, error) {
	if ttl == 0 {
		ttl = defaultEnrollmentTTL
	}
	if ttl < minimumEnrollmentTTL || ttl > maximumEnrollmentTTL {
		return EnrollmentGrant{}, ErrInvalidInput
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return EnrollmentGrant{}, err
	}
	if u.Status != "ACTIVE" {
		return EnrollmentGrant{}, ErrUserDisabled
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
		return rollback(ErrUserDisabled)
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
			return rollback(ErrDeviceRevoked)
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
	p, _, _, _, err := s.authenticateAccessDetails(ctx, token)
	return p, err
}

func (s *Service) authenticateAccessDetails(ctx context.Context, token string) (AccessPrincipal, *ent.User, *ent.Device, *ent.Session, error) {
	claims, err := s.Signer.Verify(token)
	if err != nil {
		return AccessPrincipal{}, nil, nil, nil, ErrCredential
	}
	p := AccessPrincipal{DeploymentID: claims.DeploymentID, UserID: claims.Subject, DeviceID: claims.DeviceID, SessionID: claims.SessionID}
	u, d, se, err := s.accessPrincipalDetails(ctx, p)
	if err != nil {
		return AccessPrincipal{}, nil, nil, nil, err
	}
	return p, u, d, se, nil
}

func (s *Service) validateAccessPrincipal(ctx context.Context, p AccessPrincipal) error {
	_, _, _, err := s.accessPrincipalDetails(ctx, p)
	return err
}

func (s *Service) accessPrincipalDetails(ctx context.Context, p AccessPrincipal) (*ent.User, *ent.Device, *ent.Session, error) {
	deleted, err := s.Client.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(p.UserID)).Exist(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if deleted {
		return nil, nil, nil, ErrIdentityDeleted
	}
	se, err := s.Client.Session.Get(ctx, p.SessionID)
	if ent.IsNotFound(err) {
		return nil, nil, nil, ErrRevoked
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if se.Channel != "ANDROID" || se.UserID != p.UserID || se.DeviceID == nil || *se.DeviceID != p.DeviceID {
		return nil, nil, nil, ErrCredential
	}
	u, err := s.Client.User.Get(ctx, p.UserID)
	if ent.IsNotFound(err) {
		return nil, nil, nil, ErrRevoked
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if u.Status != "ACTIVE" {
		return nil, nil, nil, ErrUserDisabled
	}
	d, err := s.Client.Device.Get(ctx, p.DeviceID)
	if ent.IsNotFound(err) {
		return nil, nil, nil, ErrRevoked
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if d.UserID != u.ID {
		return nil, nil, nil, ErrCredential
	}
	if d.Status != "ACTIVE" {
		return nil, nil, nil, ErrDeviceRevoked
	}
	if !s.Now().UTC().Before(se.ExpiresAt) {
		return nil, nil, nil, ErrExpired
	}
	if se.Status != "ACTIVE" {
		return nil, nil, nil, ErrRevoked
	}
	return u, d, se, nil
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
	return s.LoginAdminWithOptions(ctx, username, password, AdminLoginOptions{})
}

func (s *Service) LoginAdminWithOptions(ctx context.Context, username, password string, options AdminLoginOptions) (AdminSessionResult, error) {
	now := s.Now().UTC()
	if wait := s.adminLoginLimiter.retryAfter(username, options.Source, now); wait > 0 {
		return AdminSessionResult{}, &LoginThrottledError{RetryAfter: wait}
	}
	if !s.adminLoginLimiter.tryBeginVerification() {
		return AdminSessionResult{}, &LoginThrottledError{RetryAfter: adminPasswordVerificationBusyRetry}
	}
	defer s.adminLoginLimiter.endVerification()
	u, err := s.Client.User.Query().Where(user.UsernameEQ(NormalizeUsername(username))).Only(ctx)
	if ent.IsNotFound(err) {
		security.VerifyPasswordOrDummy(nil, password)
		s.adminLoginLimiter.failure(username, options.Source, now)
		return AdminSessionResult{}, ErrCredential
	}
	if err != nil {
		return AdminSessionResult{}, err
	}
	passwordValid := security.VerifyPasswordOrDummy(u.PasswordHash, password)
	if u.Role != "ADMIN" || u.Status != "ACTIVE" || !passwordValid {
		s.adminLoginLimiter.failure(username, options.Source, now)
		return AdminSessionResult{}, ErrCredential
	}
	s.adminLoginLimiter.success(username, now)
	cookieSecret, err := s.Random(32)
	if err != nil {
		return AdminSessionResult{}, err
	}
	ttl := AdminSessionTTL
	if options.RememberMe {
		ttl = AdminRememberedSessionTTL
	}
	expiresAt := now.Add(ttl)
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
	return DeviceView{Name: row.Name, ID: row.ID, UserID: row.UserID, InstallationID: row.InstallationID, AppVersion: row.AppVersion, LastSeenAt: row.LastSeenAt, Status: row.Status}
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

// ListUserViews pages users by id and optionally narrows them by a search term.
// The term matches either the login name or the display name so an operator can
// find an account without knowing its identifier; the cursor stays bound to the
// whole query, so changing the term starts a new page sequence.
func (s *Service) ListUserViews(ctx context.Context, search string, limit int, after string) ([]UserView, error) {
	q := s.Client.User.Query().Where(user.IDGT(after))
	if term := strings.TrimSpace(search); term != "" {
		q = q.Where(user.Or(user.UsernameContainsFold(term), user.DisplayNameContainsFold(term)))
	}
	rows, err := q.Order(ent.Asc(user.FieldID)).Limit(limit).All(ctx)
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
	owner, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Client.Device.Query().Where(device.UserIDEQ(userID), device.IDGT(after)).Order(ent.Asc(device.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]DeviceView, 0, len(rows))
	if len(rows) == 0 {
		return views, nil
	}
	state, err := s.ManagedState(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	sessions, err := s.Client.Session.Query().Where(session.UserIDEQ(userID), session.DeviceIDIn(ids...), session.ChannelEQ("ANDROID"), session.StatusEQ("ACTIVE"), session.ExpiresAtGT(s.Now())).Order(ent.Desc(session.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	current := make(map[string]*ent.Session, len(sessions))
	for _, row := range sessions {
		if row.DeviceID != nil && current[*row.DeviceID] == nil {
			current[*row.DeviceID] = row
		}
	}
	for _, row := range rows {
		view := deviceView(row)
		view.TargetManagedGeneration = state.ActiveManagedGeneration
		view.ApplicationState = "UNKNOWN"
		if state.ActiveManagedGeneration == 0 {
			view.ApplicationState = "UNPUBLISHED"
		}
		if report := current[row.ID]; owner.Status == "ACTIVE" && row.Status == "ACTIVE" && report != nil && report.AppliedManagedGeneration != nil {
			generation := int(*report.AppliedManagedGeneration)
			view.AppliedManagedGeneration = &generation
			view.AppliedReportedAt = report.AppliedReportedAt
			view.ApplicationState = "PENDING"
			if generation == state.ActiveManagedGeneration {
				view.ApplicationState = "APPLIED"
			}
		}
		views = append(views, view)
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
