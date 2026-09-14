package identity_test

import (
	"context"
	"errors"
	"measix/platform/internal/hub/identity"
	"measix/platform/pkg/platformid"
	"testing"
	"time"
)

func TestReenrollmentPreservesDeviceButNotSession(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	boot, err := s.Bootstrap(ctx, "Enterprise", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser(ctx, "member", "Member", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	installation := platformid.New(platformid.Installation)
	grant, _ := s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, time.Minute)
	first, err := s.ExchangeEnrollment(ctx, grant.Code, installation, "Device", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	grant, _ = s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, time.Minute)
	second, err := s.ExchangeEnrollment(ctx, grant.Code, installation, "Renamed", "2.0")
	if err != nil {
		t.Fatal(err)
	}
	if first.DeviceID != second.DeviceID || first.SessionID == second.SessionID {
		t.Fatal("invalid re-enrollment identity")
	}
	if _, err := s.Refresh(ctx, first.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrRevoked) {
		t.Fatalf("old credential: %v", err)
	}
	other, _ := s.CreateUser(ctx, "other", "Other", "MEMBER")
	grant, _ = s.CreateEnrollment(ctx, other.ID, boot.AdminUserID, time.Minute)
	if _, err := s.ExchangeEnrollment(ctx, grant.Code, installation, "Other", "1.0"); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("cross-user rebind: %v", err)
	}
	if _, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Other", "1.0"); err != nil {
		t.Fatalf("conflict consumed code: %v", err)
	}
	if _, err := s.Refresh(ctx, second.RefreshToken, platformid.New(platformid.Idempotency)); err != nil {
		t.Fatalf("cross-user attempt revoked victim: %v", err)
	}
}
