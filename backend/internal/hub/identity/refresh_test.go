package identity_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/pkg/platformid"
)

func enrolled(t *testing.T) (*identity.Service, identity.ExchangeResult) {
	t.Helper()
	s, _ := newService(t)
	ctx := context.Background()
	boot, err := s.Bootstrap(ctx, "Example", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.CreateEnrollment(ctx, boot.AdminUserID, boot.AdminUserID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Device", "test")
	if err != nil {
		t.Fatal(err)
	}
	return s, exchange
}

func TestRefreshConcurrentRecoveryAndRotation(t *testing.T) {
	s, exchange := enrolled(t)
	ctx := context.Background()
	now := s.Now().Add(6 * 24 * time.Hour)
	s.Now = func() time.Time { return now }
	s.Signer.Now = s.Now
	key := platformid.New(platformid.Idempotency)
	var wg sync.WaitGroup
	results := make([]identity.RefreshResult, 8)
	errs := make([]error, len(results))
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = s.Refresh(ctx, exchange.RefreshToken, key) }(i)
	}
	wg.Wait()
	for i, result := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if result != results[0] {
			t.Fatal("duplicate refresh did not return identical result")
		}
		if result.RefreshToken == exchange.RefreshToken || !result.SessionIdleExpiresAt.Equal(now.Add(7*24*time.Hour)) {
			t.Fatal("missing rotation/idle renewal")
		}
	}
	if _, err := s.Refresh(ctx, exchange.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrRefreshConflict) {
		t.Fatalf("old token/new key: %v", err)
	}
	if _, err := s.Refresh(ctx, results[0].RefreshToken, key); !errors.Is(err, identity.ErrRefreshConflict) {
		t.Fatalf("new token/old key: %v", err)
	}
	row, err := s.Client.Session.Get(ctx, exchange.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if row.RefreshResponseCiphertext == nil || bytes.Contains(*row.RefreshResponseCiphertext, []byte(results[0].RefreshToken)) {
		t.Fatal("recovery is not encrypted")
	}
	// A new service instance recovers the committed result, not an in-memory cache.
	restarted := identity.New(s.Client, s.Signer, s.CSRFKey)
	restarted.Now = s.Now
	replayed, err := restarted.Refresh(ctx, exchange.RefreshToken, key)
	if err != nil || replayed != results[0] {
		t.Fatalf("restart recovery: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := s.Refresh(ctx, exchange.RefreshToken, key); !errors.Is(err, identity.ErrCredential) {
		t.Fatalf("expired replay: %v", err)
	}
	if err := s.SweepRefreshRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = s.Client.Session.Get(ctx, exchange.SessionID)
	if row.RefreshReplayUntil != nil || (row.RefreshResponseCiphertext != nil && len(*row.RefreshResponseCiphertext) != 0) || (row.PreviousRefreshDigest != nil && len(*row.PreviousRefreshDigest) != 0) {
		t.Fatal("expired recovery retained")
	}
	if _, err := s.Refresh(ctx, results[0].RefreshToken, platformid.New(platformid.Idempotency)); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshExpiryAndLogoutIsolation(t *testing.T) {
	s, exchange := enrolled(t)
	ctx := context.Background()
	admin, err := s.LoginAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(ctx, admin.CookieSecret); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateAdmin(ctx, admin.CookieSecret, "", false); err != nil {
		t.Fatalf("Android logout revoked admin: %v", err)
	}
	if err := s.LogoutAdmin(ctx, exchange.RefreshToken); err != nil {
		t.Fatal(err)
	}
	key := platformid.New(platformid.Idempotency)
	refreshed, err := s.Refresh(ctx, exchange.RefreshToken, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(ctx, exchange.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(ctx, refreshed.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrRevoked) {
		t.Fatalf("logout after lost response: %v", err)
	}
	if _, err := s.Refresh(ctx, exchange.RefreshToken, key); err == nil {
		t.Fatal("revoked response recovered")
	}
}

func TestOrdinaryAuthenticationDoesNotRenewIdle(t *testing.T) {
	s, exchange := enrolled(t)
	ctx := context.Background()
	if _, err := s.AuthenticateAccess(ctx, exchange.AccessToken); err != nil {
		t.Fatal(err)
	}
	row, _ := s.Client.Session.Get(ctx, exchange.SessionID)
	if !row.ExpiresAt.Equal(exchange.RefreshExpiresAt) {
		t.Fatal("access renewed idle lease")
	}
	now := exchange.RefreshExpiresAt
	s.Now = func() time.Time { return now }
	s.Signer.Now = s.Now
	if _, err := s.Refresh(ctx, exchange.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrExpired) {
		t.Fatalf("idle boundary: %v", err)
	}
}
