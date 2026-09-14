package identity

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/session"
	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
)

const sessionIdleTTL = 7 * 24 * time.Hour
const refreshRecoveryTTL = 2 * time.Minute

type RefreshResult struct {
	AccessToken          string
	AccessTokenExpiresAt time.Time
	RefreshToken         string
	RefreshExpiresAt     time.Time
	SessionIdleExpiresAt time.Time
}

type refreshRecovery struct {
	SessionID  string
	RequestKey string
	Result     RefreshResult
}

// The key is domain-separated from Admin CSRF. The response is persisted only
// encrypted; its authenticated payload also binds it to the session and request.
func (s *Service) refreshBox() (*security.SecretBox, error) {
	if len(s.CSRFKey) < 32 {
		return nil, fmt.Errorf("refresh encryption key is too short")
	}
	mac := hmac.New(sha256.New, s.CSRFKey)
	_, _ = mac.Write([]byte("measix:refresh-recovery:v1"))
	return security.NewSecretBox(mac.Sum(nil), 1)
}

func (s *Service) Refresh(ctx context.Context, refreshToken, requestKey string) (RefreshResult, error) {
	if refreshToken == "" || platformid.Validate(platformid.Idempotency, requestKey) != nil {
		return RefreshResult{}, ErrInvalidInput
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return RefreshResult{}, err
	}
	defer tx.Rollback()
	digest := security.DigestToken(refreshToken)
	se, err := tx.Session.Query().Where(session.ChannelEQ("ANDROID"), session.Or(session.RefreshDigestEQ(digest), session.PreviousRefreshDigestEQ(digest))).Only(ctx)
	if ent.IsNotFound(err) {
		return RefreshResult{}, ErrCredential
	}
	if err != nil {
		return RefreshResult{}, err
	}
	now := s.Now().UTC()
	if se.Status != "ACTIVE" {
		return RefreshResult{}, ErrRevoked
	}
	if !now.Before(se.ExpiresAt) {
		return RefreshResult{}, ErrExpired
	}
	u, err := tx.User.Get(ctx, se.UserID)
	if err != nil {
		return RefreshResult{}, err
	}
	if u.Status != "ACTIVE" {
		return RefreshResult{}, ErrRevoked
	}
	if se.DeviceID == nil {
		return RefreshResult{}, ErrCredential
	}
	d, err := tx.Device.Get(ctx, *se.DeviceID)
	if err != nil {
		return RefreshResult{}, err
	}
	if d.Status != "ACTIVE" || d.UserID != u.ID {
		return RefreshResult{}, ErrRevoked
	}
	box, err := s.refreshBox()
	if err != nil {
		return RefreshResult{}, err
	}
	current := se.RefreshDigest != nil && bytes.Equal(*se.RefreshDigest, digest)
	if !current {
		if se.RefreshReplayUntil == nil || !now.Before(*se.RefreshReplayUntil) {
			return RefreshResult{}, ErrCredential
		}
		if se.RefreshRequestKey == nil || *se.RefreshRequestKey != requestKey {
			return RefreshResult{}, ErrRefreshConflict
		}
		if se.RefreshResponseCiphertext == nil {
			return RefreshResult{}, fmt.Errorf("missing refresh recovery")
		}
		plain, err := box.Decrypt(*se.RefreshResponseCiphertext)
		if err != nil {
			return RefreshResult{}, err
		}
		var recovery refreshRecovery
		if err := json.Unmarshal(plain, &recovery); err != nil {
			return RefreshResult{}, err
		}
		if recovery.SessionID != se.ID || recovery.RequestKey != requestKey {
			return RefreshResult{}, fmt.Errorf("invalid refresh recovery binding")
		}
		return recovery.Result, nil
	}
	if se.RefreshRequestKey != nil && *se.RefreshRequestKey == requestKey {
		return RefreshResult{}, ErrRefreshConflict
	}
	result := RefreshResult{RefreshExpiresAt: now.Add(sessionIdleTTL), SessionIdleExpiresAt: now.Add(sessionIdleTTL)}
	result.RefreshToken, err = s.Random(32)
	if err != nil {
		return RefreshResult{}, err
	}
	result.AccessToken, result.AccessTokenExpiresAt, err = s.Signer.Sign(u.ID, d.ID, se.ID)
	if err != nil {
		return RefreshResult{}, err
	}
	plain, err := json.Marshal(refreshRecovery{SessionID: se.ID, RequestKey: requestKey, Result: result})
	if err != nil {
		return RefreshResult{}, err
	}
	ciphertext, err := box.Encrypt(plain)
	if err != nil {
		return RefreshResult{}, err
	}
	_, err = tx.Session.UpdateOneID(se.ID).
		SetPreviousRefreshDigest(digest).SetRefreshRequestKey(requestKey).
		SetRefreshReplayUntil(now.Add(refreshRecoveryTTL)).SetRefreshResponseCiphertext(ciphertext).
		SetRefreshDigest(security.DigestToken(result.RefreshToken)).SetExpiresAt(result.SessionIdleExpiresAt).
		SetLastUsedAt(now).Save(ctx)
	if err != nil {
		return RefreshResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RefreshResult{}, err
	}
	return result, nil
}

// SweepRefreshRecovery bounds durable recovery retention, including idle sessions.
// Keep the most recent request key until the next rotation to reject its reuse.
func (s *Service) SweepRefreshRecovery(ctx context.Context) error {
	_, err := s.Client.Session.Update().Where(session.Or(session.RefreshReplayUntilLTE(s.Now().UTC()), session.StatusNEQ("ACTIVE"))).
		ClearPreviousRefreshDigest().ClearRefreshReplayUntil().ClearRefreshResponseCiphertext().Save(ctx)
	return err
}
