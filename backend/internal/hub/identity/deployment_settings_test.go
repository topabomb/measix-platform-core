package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/pkg/platformid"
)

func TestDeploymentSettingsUpdateIsRevisionProtectedAndAudited(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	boot, err := s.Bootstrap(ctx, "Original enterprise", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Client.Deployment.UpdateOneID(boot.DeploymentID).SetPublicOrigin("https://old.example").Save(ctx); err != nil {
		t.Fatal(err)
	}
	s.SetPublicOrigin("https://old.example")
	member, err := s.CreateUser(ctx, "alice", "Alice", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	client, err := s.ExchangeEnrollment(ctx, grant.Code, platformid.New(platformid.Installation), "Alice's phone", "1")
	if err != nil {
		t.Fatal(err)
	}
	portal, err := s.Client.PortalSession.Create().
		SetID("portal-settings-test").
		SetSessionID("session-settings-test").
		SetOrigin("https://old.example").
		SetTicketDigest([]byte("settings-test-ticket")).
		SetGrantExpiresAt(time.Now().Add(time.Hour)).
		SetExpiresAt(time.Now().Add(time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.DeploymentSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := before.UpdatedAt.Add(time.Second)
	s.Now = func() time.Time { return now }
	after, err := s.UpdateDeploymentSettings(ctx, "  Renamed enterprise  ", "https://new.example", before.UpdatedAt, boot.AdminUserID)
	if err != nil {
		t.Fatal(err)
	}
	if after.DeploymentID != before.DeploymentID || after.Name != "Renamed enterprise" || after.PublicOrigin != "https://new.example" || s.PublicOrigin() != after.PublicOrigin || after.Timezone != before.Timezone || !after.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected settings: %+v", after)
	}
	audits, err := s.Client.DeploymentSettingAudit.Query().All(ctx)
	if err != nil || len(audits) != 1 || audits[0].ActorUserID != boot.AdminUserID || audits[0].OldName != before.Name || audits[0].NewName != after.Name || audits[0].OldPublicOrigin != before.PublicOrigin || audits[0].NewPublicOrigin != after.PublicOrigin {
		t.Fatalf("unexpected audit: %+v, %v", audits, err)
	}
	portal, err = s.Client.PortalSession.Get(ctx, portal.ID)
	if err != nil || !portal.Revoked {
		t.Fatalf("origin change must revoke existing Portal sessions: %+v, %v", portal, err)
	}
	if _, err = s.UpdateDeploymentSettings(ctx, "Stale write", "https://stale.example", before.UpdatedAt, boot.AdminUserID); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	principal, err := s.AuthenticateAccess(ctx, client.AccessToken)
	if err != nil {
		t.Fatalf("address change invalidated Android access: %v", err)
	}
	if principal.DeploymentID != client.DeploymentID || principal.UserID != client.UserID || principal.DeviceID != client.DeviceID || principal.SessionID != client.SessionID {
		t.Fatalf("address change altered Android principal: %+v", principal)
	}
	s.PortalStaticAvailable = true
	newPortal, err := s.CreatePortalGrant(ctx, client.AccessToken)
	if err != nil || newPortal.ExchangeUrl != "https://new.example/portal/session/exchange" {
		t.Fatalf("new Portal grant did not use changed address: %+v err=%v", newPortal, err)
	}
	if _, err := s.Refresh(ctx, client.RefreshToken, platformid.New(platformid.Idempotency)); err != nil {
		t.Fatalf("address change invalidated Android refresh: %v", err)
	}
}

func TestDeploymentSettingsCanonicalEquivalentOriginIsNoOp(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	boot, err := s.Bootstrap(ctx, "Enterprise", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Client.Deployment.UpdateOneID(boot.DeploymentID).SetPublicOrigin("https://core.example").Save(ctx); err != nil {
		t.Fatal(err)
	}
	s.SetPublicOrigin("https://core.example")
	portal, err := s.Client.PortalSession.Create().
		SetID("portal-canonical-no-op").
		SetSessionID("session-canonical-no-op").
		SetOrigin("https://core.example").
		SetTicketDigest([]byte("canonical-no-op-ticket")).
		SetGrantExpiresAt(time.Now().Add(time.Hour)).
		SetExpiresAt(time.Now().Add(time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.DeploymentSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.UpdateDeploymentSettings(ctx, before.Name, " HTTPS://Core.Example:443/ ", before.UpdatedAt, boot.AdminUserID)
	if err != nil {
		t.Fatal(err)
	}
	if after.PublicOrigin != before.PublicOrigin || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("canonical-equivalent update changed settings: before=%+v after=%+v", before, after)
	}
	if audits, err := s.Client.DeploymentSettingAudit.Query().Count(ctx); err != nil || audits != 0 {
		t.Fatalf("canonical-equivalent update created audit: count=%d err=%v", audits, err)
	}
	portal, err = s.Client.PortalSession.Get(ctx, portal.ID)
	if err != nil || portal.Revoked {
		t.Fatalf("canonical-equivalent update revoked Portal session: %+v err=%v", portal, err)
	}
}
