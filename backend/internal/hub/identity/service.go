package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/topabomb/measix-platform-core/backend/ent"
	"github.com/topabomb/measix-platform-core/backend/ent/device"
	"github.com/topabomb/measix-platform-core/backend/ent/enrollment"
	"github.com/topabomb/measix-platform-core/backend/ent/session"
	"github.com/topabomb/measix-platform-core/backend/ent/user"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
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

type EnrollmentGrant struct { EnrollmentID, Code string; ExpiresAt time.Time }
type ExchangeResult struct {
	DeploymentID, UserID, DeviceID, SessionID string
	AccessToken string
	AccessTokenExpiresAt time.Time
	RefreshToken string
	RefreshExpiresAt time.Time
}
type AdminSessionResult struct {
	UserID, DisplayName, Role, CookieSecret, CSRFToken string
	ExpiresAt time.Time
}

func New(client *ent.Client, signer *security.AccessSigner, csrfKey []byte) *Service {
	return &Service{Client: client, Signer: signer, CSRFKey: append([]byte(nil), csrfKey...), Now: time.Now, Random: security.RandomToken}
}
func NormalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func (s *Service) CreateUser(ctx context.Context, username, displayName, role string) (*ent.User, error) {
	username = NormalizeUsername(username)
	if username == "" || strings.TrimSpace(displayName) == "" || (role != "ADMIN" && role != "MEMBER") { return nil, fmt.Errorf("invalid user") }
	now := s.Now().UTC()
	u, err := s.Client.User.Create().SetID(platformid.New(platformid.User)).SetUsername(username).SetDisplayName(strings.TrimSpace(displayName)).SetRole(role).SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if ent.IsConstraintError(err) { return nil, ErrConflict }
	return u, err
}

func (s *Service) SetPassword(ctx context.Context, userID, password string) error {
	hash, err := security.HashPassword(password); if err != nil { return err }
	n, err := s.Client.User.Update().Where(user.IDEQ(userID)).SetPasswordHash(hash).SetUpdatedAt(s.Now().UTC()).Save(ctx)
	if err != nil { return err }; if n != 1 { return ErrNotFound }; return nil
}

func (s *Service) CreateEnrollment(ctx context.Context, userID, createdBy string, ttl time.Duration) (EnrollmentGrant, error) {
	if ttl == 0 { ttl = 10*time.Minute }; if ttl < time.Minute || ttl > time.Hour { return EnrollmentGrant{}, fmt.Errorf("enrollment TTL out of range") }
	if _, err := s.Client.User.Get(ctx, userID); err != nil { if ent.IsNotFound(err) { return EnrollmentGrant{}, ErrNotFound }; return EnrollmentGrant{}, err }
	code, err := s.Random(16); if err != nil { return EnrollmentGrant{}, err }
	now := s.Now().UTC(); id := platformid.New(platformid.Enrollment)
	if _, err := s.Client.Enrollment.Create().SetID(id).SetUserID(userID).SetTokenDigest(security.DigestToken(code)).SetExpiresAt(now.Add(ttl)).SetCreatedByUserID(createdBy).SetCreatedAt(now).Save(ctx); err != nil { return EnrollmentGrant{}, err }
	return EnrollmentGrant{EnrollmentID:id, Code:code, ExpiresAt:now.Add(ttl)}, nil
}

func (s *Service) ExchangeEnrollment(ctx context.Context, code, installationID, appVersion string) (ExchangeResult, error) {
	if err := platformid.Validate(platformid.Installation, installationID); err != nil { return ExchangeResult{}, fmt.Errorf("invalid installation id") }
	tx, err := s.Client.Tx(ctx); if err != nil { return ExchangeResult{}, err }
	rollback := func(e error)(ExchangeResult,error){ _=tx.Rollback(); return ExchangeResult{},e }
	e, err := tx.Enrollment.Query().Where(enrollment.TokenDigestEQ(security.DigestToken(code))).Only(ctx)
	if err != nil { if ent.IsNotFound(err){ return rollback(ErrCredential) }; return rollback(err) }
	now := s.Now().UTC(); if e.ConsumedAt != nil { return rollback(ErrAlreadyUsed) }; if !now.Before(e.ExpiresAt){ return rollback(ErrExpired) }
	u, err := tx.User.Get(ctx,e.UserID); if err != nil { return rollback(err) }; if u.Status!="ACTIVE" { return rollback(ErrRevoked) }
	if existing, qerr := tx.Device.Query().Where(device.InstallationIDEQ(installationID)).Only(ctx); qerr==nil && existing.ID!="" { return rollback(ErrConflict) } else if qerr!=nil && !ent.IsNotFound(qerr) { return rollback(qerr) }
	refresh, err := s.Random(32); if err != nil { return rollback(err) }
	deviceID, sessionID := platformid.New(platformid.Device), platformid.New(platformid.Session); refreshExpires:=now.Add(30*24*time.Hour)
	if _,err:=tx.Device.Create().SetID(deviceID).SetUserID(e.UserID).SetInstallationID(installationID).SetStatus("ACTIVE").SetNillableAppVersion(optionalString(appVersion)).SetCreatedAt(now).Save(ctx); err!=nil { return rollback(err) }
	if _,err:=tx.Session.Create().SetID(sessionID).SetUserID(e.UserID).SetDeviceID(deviceID).SetChannel("ANDROID").SetRefreshDigest(security.DigestToken(refresh)).SetExpiresAt(refreshExpires).SetStatus("ACTIVE").SetCreatedAt(now).Save(ctx); err!=nil { return rollback(err) }
	if _,err:=tx.Enrollment.UpdateOneID(e.ID).SetConsumedAt(now).Save(ctx); err!=nil { return rollback(err) }
	if err:=tx.Commit(); err!=nil { return ExchangeResult{},err }
	access, accessExpires, err := s.Signer.Sign(e.UserID,deviceID,sessionID); if err!=nil { return ExchangeResult{},err }
	return ExchangeResult{DeploymentID:s.Signer.DeploymentID,UserID:e.UserID,DeviceID:deviceID,SessionID:sessionID,AccessToken:access,AccessTokenExpiresAt:accessExpires,RefreshToken:refresh,RefreshExpiresAt:refreshExpires},nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string)(string,time.Time,error){
	se,err:=s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(refreshToken))).Only(ctx); if err!=nil { if ent.IsNotFound(err){return "",time.Time{},ErrCredential}; return "",time.Time{},err }
	if se.Channel!="ANDROID"||se.Status!="ACTIVE"{return "",time.Time{},ErrRevoked}; now:=s.Now().UTC(); if !now.Before(se.ExpiresAt){return "",time.Time{},ErrExpired}
	u,err:=s.Client.User.Get(ctx,se.UserID); if err!=nil||u.Status!="ACTIVE"{return "",time.Time{},ErrRevoked}; if se.DeviceID==nil{return "",time.Time{},ErrCredential}
	d,err:=s.Client.Device.Get(ctx,*se.DeviceID); if err!=nil||d.Status!="ACTIVE"{return "",time.Time{},ErrRevoked}; _,_=s.Client.Session.UpdateOneID(se.ID).SetLastUsedAt(now).Save(ctx)
	return s.Signer.Sign(se.UserID,d.ID,se.ID)
}
func (s *Service) Logout(ctx context.Context, refreshToken string) error { se,err:=s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(refreshToken))).Only(ctx); if err!=nil {if ent.IsNotFound(err){return nil};return err}; _,err=s.Client.Session.UpdateOneID(se.ID).SetStatus("REVOKED").SetRevokedAt(s.Now().UTC()).Save(ctx);return err }
func (s *Service) DisableUser(ctx context.Context,userID string)error{now:=s.Now().UTC();n,err:=s.Client.User.Update().Where(user.IDEQ(userID)).SetStatus("DISABLED").SetUpdatedAt(now).Save(ctx);if err!=nil{return err};if n!=1{return ErrNotFound};_,err=s.Client.Session.Update().Where(session.UserIDEQ(userID),session.StatusEQ("ACTIVE")).SetStatus("REVOKED").SetRevokedAt(now).Save(ctx);return err}
func (s *Service) EnableUserLocal(ctx context.Context,userID string)error{n,err:=s.Client.User.Update().Where(user.IDEQ(userID)).SetStatus("ACTIVE").SetUpdatedAt(s.Now().UTC()).Save(ctx);if err!=nil{return err};if n!=1{return ErrNotFound};return nil}
func (s *Service) RevokeDevice(ctx context.Context,deviceID string)error{now:=s.Now().UTC();n,err:=s.Client.Device.Update().Where(device.IDEQ(deviceID)).SetStatus("REVOKED").SetRevokedAt(now).Save(ctx);if err!=nil{return err};if n!=1{return ErrNotFound};_,err=s.Client.Session.Update().Where(session.DeviceIDEQ(deviceID),session.StatusEQ("ACTIVE")).SetStatus("REVOKED").SetRevokedAt(now).Save(ctx);return err}

func (s *Service) LoginAdmin(ctx context.Context,username,password string)(AdminSessionResult,error){
	u,err:=s.Client.User.Query().Where(user.UsernameEQ(NormalizeUsername(username))).Only(ctx);if err!=nil||u.Role!="ADMIN"||u.Status!="ACTIVE"||u.PasswordHash==nil||!security.VerifyPassword(*u.PasswordHash,password){return AdminSessionResult{},ErrCredential}
	cookie,err:=s.Random(32);if err!=nil{return AdminSessionResult{},err};now:=s.Now().UTC();expires:=now.Add(12*time.Hour)
	if _,err:=s.Client.Session.Create().SetID(platformid.New(platformid.Session)).SetUserID(u.ID).SetChannel("ADMIN_WEB").SetRefreshDigest(security.DigestToken(cookie)).SetExpiresAt(expires).SetStatus("ACTIVE").SetCreatedAt(now).Save(ctx);err!=nil{return AdminSessionResult{},err}
	return AdminSessionResult{UserID:u.ID,DisplayName:u.DisplayName,Role:u.Role,CookieSecret:cookie,CSRFToken:security.CSRFToken(cookie,s.CSRFKey),ExpiresAt:expires},nil
}
func (s *Service) AuthenticateAdmin(ctx context.Context,cookieSecret,csrfToken string,requireCSRF bool)(*ent.User,*ent.Session,error){if cookieSecret==""||(requireCSRF&&!security.VerifyCSRF(cookieSecret,csrfToken,s.CSRFKey)){return nil,nil,ErrNotAuthorized};se,err:=s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(cookieSecret))).Only(ctx);if err!=nil||se.Channel!="ADMIN_WEB"||se.Status!="ACTIVE"||!s.Now().UTC().Before(se.ExpiresAt){return nil,nil,ErrNotAuthorized};u,err:=s.Client.User.Get(ctx,se.UserID);if err!=nil||u.Status!="ACTIVE"||u.Role!="ADMIN"{return nil,nil,ErrNotAuthorized};return u,se,nil}
func (s *Service) LogoutAdmin(ctx context.Context,cookieSecret string)error{se,err:=s.Client.Session.Query().Where(session.RefreshDigestEQ(security.DigestToken(cookieSecret))).Only(ctx);if err!=nil{if ent.IsNotFound(err){return nil};return err};_,err=s.Client.Session.UpdateOneID(se.ID).SetStatus("REVOKED").SetRevokedAt(s.Now().UTC()).Save(ctx);return err}
func optionalString(v string)*string{v=strings.TrimSpace(v);if v==""{return nil};return &v}
