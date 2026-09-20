package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
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
	if after.Name != "Renamed enterprise" || after.PublicOrigin != "https://new.example" || s.PublicOrigin() != after.PublicOrigin || after.Timezone != before.Timezone || !after.UpdatedAt.Equal(now) {
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
}
