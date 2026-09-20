package identity_test

import (
	"context"
	"errors"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/store"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
	"testing"
	"time"
)

func TestPortalParentLifetimeAndRestart(t *testing.T) {
	for _, cause := range []string{"user_disabled", "session_revoked", "expiry"} {
		t.Run(cause, func(t *testing.T) {
			ctx := context.Background()
			st := testutil.OpenStore(t)
			s := testutil.NewIdentityService(t, st, time.Now().UTC())
			s.SetPublicOrigin("https://platform.example")
			boot, err := s.Bootstrap(ctx, "Enterprise", "admin", "Admin", "correct horse battery staple")
			if err != nil {
				t.Fatal(err)
			}
			member, err := s.CreateUser(ctx, "alice", "Alice", "MEMBER")
			if err != nil {
				t.Fatal(err)
			}
			grant, err := s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			native, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Phone", "1")
			if err != nil {
				t.Fatal(err)
			}
			now := s.Now()
			s.Now = func() time.Time { return now }
			s.Signer.Now = s.Now
			deadline := now.Add(2 * time.Minute)
			if _, err := s.Client.Session.UpdateOneID(native.SessionID).SetExpiresAt(deadline).Save(ctx); err != nil {
				t.Fatal(err)
			}
			ticket, err := s.CreatePortalGrant(ctx, native.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			cookie, expiry, err := s.ExchangePortalGrant(ctx, ticket.Ticket)
			if err != nil {
				t.Fatal(err)
			}
			if !expiry.Equal(deadline) {
				t.Fatal("Portal outlived parent")
			}
			var seq int
			var name, path string
			if err := st.DB.QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
				t.Fatal(err)
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := store.OpenEnt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			s.Client = reopened.Client
			restarted := identity.New(reopened.Client, s.Signer, s.CSRFKey)
			restarted.SetPublicOrigin(s.PublicOrigin())
			restarted.Now = s.Now
			if _, _, err := restarted.ExchangePortalGrant(ctx, ticket.Ticket); err == nil {
				t.Fatal("restart allowed replay")
			}
			if _, err := restarted.AuthenticatePortal(ctx, cookie); err != nil {
				t.Fatal(err)
			}
			switch cause {
			case "user_disabled":
				_, err = s.Client.User.UpdateOneID(member.ID).SetStatus("DISABLED").Save(ctx)
			case "session_revoked":
				_, err = s.Client.Session.UpdateOneID(native.SessionID).SetStatus("REVOKED").Save(ctx)
			case "expiry":
				now = deadline
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.AuthenticatePortal(ctx, cookie); err == nil {
				t.Fatal("parent invalidation ignored")
			}
		})
	}
}

func TestPublicOriginPolicy(t *testing.T) {
	for _, raw := range []string{"https://portal.example", "http://portal.example:9000", "http://192.168.1.20:9000", "http://203.0.113.20:9000", "https://203.0.113.20:9443", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if err := identity.ValidatePublicOrigin(raw); err != nil {
			t.Fatalf("valid origin %s", raw)
		}
	}
	for _, raw := range []string{"", "http://:9000", "http://platform.example:0", "http://platform.example:65536", "http://platform.example?", "http://platform.example#", "https://evil@portal.example", "https://portal.example/path", "https://portal.example?redirect=evil", "https://portal.example#x", "javascript:alert(1)"} {
		if identity.ValidatePublicOrigin(raw) == nil {
			t.Fatalf("unsafe origin %s", raw)
		}
	}
}

func TestPortalGrantRequiresServedStaticArtifact(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStore(t)
	s := testutil.NewIdentityService(t, st, time.Now().UTC())
	s.SetPublicOrigin("https://platform.example")
	s.PortalStaticAvailable = false
	boot, err := s.Bootstrap(ctx, "Enterprise", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser(ctx, "alice", "Alice", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	native, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Phone", "1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePortalGrant(ctx, native.AccessToken); !errors.Is(err, identity.ErrPortalUnavailable) {
		t.Fatalf("expected portal unavailable, got %v", err)
	}
}
