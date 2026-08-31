package identity_test

import (
	"context"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
)

// HUB-ID-009: the durable session deadline is the seven-day idle authority.
func TestSessionEnrollmentUsesSevenDayIdleDeadline(t *testing.T) {
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
	result, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Test device", "test")
	if err != nil {
		t.Fatal(err)
	}
	want := s.Now().UTC().Add(7 * 24 * time.Hour)
	if !result.RefreshExpiresAt.Equal(want) {
		t.Fatalf("refresh deadline=%v, want %v", result.RefreshExpiresAt, want)
	}
	row, err := s.Client.Session.Get(ctx, result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.ExpiresAt.Equal(want) {
		t.Fatalf("durable deadline=%v, want %v", row.ExpiresAt, want)
	}
}
func TestInvalidAdminPasswordDoesNotCreatePartialUser(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.CreateAdmin(ctx, "admin", "Admin", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	count, err := s.Client.User.Query().Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("partial user left behind: %d %v", count, err)
	}
}
