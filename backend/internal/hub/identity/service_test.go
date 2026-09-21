package identity_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func newService(t *testing.T) (*identity.Service, string) {
	t.Helper()
	st := testutil.OpenStore(t)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := platformid.New(platformid.Deployment)
	signer, err := security.NewAccessSigner(privateKey, deploymentID, "test-key", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s := identity.New(st.Client, signer, []byte("01234567890123456789012345678901"))
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	signer.Now = s.Now
	return s, deploymentID
}

func TestI1IdentityEnrollmentRefreshAndRevoke(t *testing.T) {
	ctx := context.Background()
	s, deploymentID := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "Root.Admin", "Root Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if boot.DeploymentID != deploymentID || boot.AdminUserID == "" || boot.DraftID == "" {
		t.Fatalf("invalid bootstrap result: %+v", boot)
	}
	row, err := s.Client.ManagedDraft.Get(ctx, boot.DraftID)
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]json.RawMessage
	if err := json.Unmarshal(row.ContentJSON, &content); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"assistants", "starters"} {
		if value, present := content[field]; !present || string(value) != "[]" {
			t.Fatalf("bootstrap draft must contain explicit empty %s array, got %s", field, value)
		}
	}
	member, err := s.CreateUser(ctx, " Alice ", "Alice", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	if member.Username != "alice" {
		t.Fatalf("normalized username=%q", member.Username)
	}
	if _, err := s.CreateUser(ctx, "ALICE", "Other", "MEMBER"); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("duplicate username err=%v", err)
	}
	grant, err := s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Test device", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if exchange.UserID != member.ID || exchange.DeploymentID != deploymentID || exchange.RefreshToken == "" || exchange.AccessToken == "" {
		t.Fatalf("invalid exchange: %+v", exchange)
	}
	if _, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Test device", "1.0.0"); !errors.Is(err, identity.ErrAlreadyUsed) {
		t.Fatalf("second exchange err=%v", err)
	}
	refreshed, err := s.Refresh(ctx, exchange.RefreshToken, platformid.New(platformid.Idempotency))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// Seed the deny fact directly; production revocation is owned by runtimecontrol.
	if _, err := s.Client.Device.UpdateOneID(exchange.DeviceID).SetStatus("REVOKED").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(ctx, refreshed.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, identity.ErrDeviceRevoked) {
		t.Fatalf("refresh after revoke err=%v", err)
	}
}

func TestClientDenialsPreserveRevokedOwner(t *testing.T) {
	for _, test := range []struct {
		name string
		deny func(context.Context, *identity.Service, identity.ExchangeResult) error
		want error
	}{
		{
			name: "user disabled",
			deny: func(ctx context.Context, service *identity.Service, exchange identity.ExchangeResult) error {
				_, err := service.Client.User.UpdateOneID(exchange.UserID).SetStatus("DISABLED").Save(ctx)
				return err
			},
			want: identity.ErrUserDisabled,
		},
		{
			name: "device revoked",
			deny: func(ctx context.Context, service *identity.Service, exchange identity.ExchangeResult) error {
				_, err := service.Client.Device.UpdateOneID(exchange.DeviceID).SetStatus("REVOKED").Save(ctx)
				return err
			},
			want: identity.ErrDeviceRevoked,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, exchange := enrolled(t)
			ctx := context.Background()
			if err := test.deny(ctx, service, exchange); err != nil {
				t.Fatal(err)
			}
			if _, err := service.AuthenticateAccess(ctx, exchange.AccessToken); !errors.Is(err, test.want) {
				t.Fatalf("access denial=%v, want %v", err, test.want)
			}
			if _, err := service.Refresh(ctx, exchange.RefreshToken, platformid.New(platformid.Idempotency)); !errors.Is(err, test.want) {
				t.Fatalf("refresh denial=%v, want %v", err, test.want)
			}
		})
	}
}

func TestAdminSessionCookieAndCSRF(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	if _, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	login, err := s.LoginAdmin(ctx, " ADMIN ", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateAdmin(ctx, login.CookieSecret, "wrong", true); !errors.Is(err, identity.ErrNotAuthorized) {
		t.Fatalf("wrong csrf err=%v", err)
	}
	u, _, err := s.AuthenticateAdmin(ctx, login.CookieSecret, login.CSRFToken, true)
	if err != nil || u.Role != "ADMIN" {
		t.Fatalf("authenticate user=%v err=%v", u, err)
	}
	if err := s.LogoutAdmin(ctx, login.CookieSecret); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateAdmin(ctx, login.CookieSecret, login.CSRFToken, false); !errors.Is(err, identity.ErrNotAuthorized) {
		t.Fatalf("session remained active: %v", err)
	}
}

func TestAdminSessionLifetimeAndPasswordResetRevocation(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	now := s.Now().UTC()
	short, err := s.LoginAdminWithOptions(ctx, "admin", "correct horse battery staple", identity.AdminLoginOptions{Source: "browser-a"})
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(12 * time.Hour); !short.ExpiresAt.Equal(want) {
		t.Fatalf("ordinary admin expiry=%s want=%s", short.ExpiresAt, want)
	}
	remembered, err := s.LoginAdminWithOptions(ctx, "admin", "correct horse battery staple", identity.AdminLoginOptions{RememberMe: true, Source: "browser-b"})
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(30 * 24 * time.Hour); !remembered.ExpiresAt.Equal(want) {
		t.Fatalf("remembered admin expiry=%s want=%s", remembered.ExpiresAt, want)
	}
	if err := s.SetPassword(ctx, boot.AdminUserID, "new correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	for _, login := range []identity.AdminSessionResult{short, remembered} {
		if _, _, err := s.AuthenticateAdmin(ctx, login.CookieSecret, "", false); !errors.Is(err, identity.ErrNotAuthorized) {
			t.Fatalf("password reset left admin session active: %v", err)
		}
	}
}

func TestAdminLoginProgressiveAndSourceThrottling(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	if _, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	now := s.Now().UTC()
	s.Now = func() time.Time { return now }

	for range 5 {
		if _, err := s.LoginAdminWithOptions(ctx, "admin", "wrong password value", identity.AdminLoginOptions{Source: "browser-a"}); !errors.Is(err, identity.ErrCredential) {
			t.Fatalf("wrong credential error=%v", err)
		}
	}
	_, err := s.LoginAdminWithOptions(ctx, "admin", "correct horse battery staple", identity.AdminLoginOptions{Source: "browser-a"})
	var throttled *identity.LoginThrottledError
	if !errors.As(err, &throttled) || throttled.RetryAfter != 5*time.Second {
		t.Fatalf("fifth failure throttle=%v", err)
	}
	now = now.Add(5 * time.Second)
	if _, err := s.LoginAdminWithOptions(ctx, "admin", "correct horse battery staple", identity.AdminLoginOptions{Source: "browser-a"}); err != nil {
		t.Fatalf("login did not recover after account wait: %v", err)
	}

	s2, _ := newService(t)
	if _, err := s2.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		username := fmt.Sprintf("unknown-%d", i)
		if _, err := s2.LoginAdminWithOptions(ctx, username, "wrong password value", identity.AdminLoginOptions{Source: "sprayer"}); !errors.Is(err, identity.ErrCredential) {
			t.Fatalf("source failure %d error=%v", i+1, err)
		}
	}
	_, err = s2.LoginAdminWithOptions(ctx, "admin", "correct horse battery staple", identity.AdminLoginOptions{Source: "sprayer"})
	throttled = nil
	if !errors.As(err, &throttled) || throttled.RetryAfter != 10*time.Minute {
		t.Fatalf("source throttle=%v", err)
	}
}
