package identity_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
)

func TestHUBID002RenamePreservesStableUserID(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	_ = boot

	u, err := s.CreateUser(ctx, "alice", "Alice", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateUser(ctx, u.ID, " Alice.Renamed ", "Alice Renamed", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != u.ID {
		t.Fatalf("rename changed stable ID: got %s want %s", updated.ID, u.ID)
	}
	if updated.Username != "alice.renamed" {
		t.Fatalf("normalized renamed username=%q", updated.Username)
	}
}

func TestHUBID003EnrollmentExpiryAndCredentialDigestOnly(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "alice", "Alice", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}

	grant, err := s.CreateEnrollment(ctx, u.ID, boot.AdminUserID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	storedEnrollment, err := s.Client.Enrollment.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantEnrollmentDigest := security.DigestToken(grant.Code)
	if !bytes.Equal(storedEnrollment.TokenDigest, wantEnrollmentDigest) {
		t.Fatal("enrollment digest mismatch")
	}
	if bytes.Equal(storedEnrollment.TokenDigest, []byte(grant.Code)) {
		t.Fatal("enrollment plaintext was persisted")
	}

	now := storedEnrollment.ExpiresAt
	s.Now = func() time.Time { return now }
	s.Signer.Now = s.Now
	if _, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Test device", "1.0.0"); !errors.Is(err, identity.ErrExpired) {
		t.Fatalf("exchange at expiry err=%v, want ErrExpired", err)
	}
}

func TestHUBID003EnrollmentTTLContract(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "ttl-user", "TTL User", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	now := s.Now().UTC()
	defaultGrant, err := s.CreateEnrollment(ctx, u.ID, boot.AdminUserID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(time.Hour); !defaultGrant.ExpiresAt.Equal(want) {
		t.Fatalf("default expiry=%s, want %s", defaultGrant.ExpiresAt, want)
	}
	if _, err := s.CreateEnrollment(ctx, u.ID, boot.AdminUserID, 24*time.Hour); err != nil {
		t.Fatalf("24 hour enrollment rejected: %v", err)
	}
	for _, ttl := range []time.Duration{time.Minute - time.Second, 24*time.Hour + time.Second} {
		if _, err := s.CreateEnrollment(ctx, u.ID, boot.AdminUserID, ttl); !errors.Is(err, identity.ErrInvalidInput) {
			t.Fatalf("ttl %s err=%v, want ErrInvalidInput", ttl, err)
		}
	}
}

func TestHUBID005RefreshCredentialDigestOnly(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "alice", "Alice", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.CreateEnrollment(ctx, u.ID, boot.AdminUserID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Test device", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	storedSession, err := s.Client.Session.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if storedSession.RefreshDigest == nil {
		t.Fatal("missing persisted refresh digest")
	}
	wantDigest := security.DigestToken(exchange.RefreshToken)
	if !bytes.Equal(*storedSession.RefreshDigest, wantDigest) {
		t.Fatal("refresh digest mismatch")
	}
	if bytes.Equal(*storedSession.RefreshDigest, []byte(exchange.RefreshToken)) {
		t.Fatal("refresh plaintext was persisted")
	}
}
