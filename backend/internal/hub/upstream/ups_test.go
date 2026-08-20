package upstream

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/adminapi"
)

func newUpstreamService(t *testing.T) (*Service, *testutil.StoreHandle, string, time.Time) {
	t.Helper()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Bootstrap identity
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(context.Background(), "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(st.Client, box)
	svc.Now = func() time.Time { return now }
	return svc, st, boot.AdminUserID, now
}

func testConfig(secretID string, secretVersion int) adminapi.UpstreamConfig {
	return adminapi.UpstreamConfig{
		Name:                  "Adapter",
		BaseUrl:               "https://adapter.example",
		TransportCapabilities: []string{"HTTP", "SSE", "BINARY", "MULTIPART", "MCP_STREAMABLE_HTTP"},
		Auth: adminapi.UpstreamConfig_Auth{
			Type: adminapi.UpstreamConfigAuthTypeBEARER,
			AdditionalProperties: map[string]interface{}{
				"secretRef": map[string]interface{}{"secretId": secretID, "secretVersion": secretVersion},
			},
		},
		CorrelationMode:      "HEADER",
		UsageCapabilityLevel: adminapi.LEVEL0,
		TimeoutDefaults:      adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
	}
}

// HUB-UPS-001: candidate config revision must be separate from active
// config revision. CreateUpstream sets ConfigRevision=1 but
// ActiveConfigRevision is nil.
func TestHUBUPS001CandidateAndActiveConfigRevisionsAreSeparate(t *testing.T) {
	ctx := context.Background()
	svc, st, adminID, _ := newUpstreamService(t)
	defer st.Close()
	secret, err := svc.CreateSecret(ctx, adminID, "token", "value")
	if err != nil {
		t.Fatal(err)
	}
	up, err := svc.CreateUpstream(ctx, adminID, testConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	if up.ConfigRevision != 1 {
		t.Fatalf("candidate revision=%d want=1", up.ConfigRevision)
	}
	if up.ActiveConfigRevision != nil {
		t.Fatalf("active revision should be nil, got=%d", *up.ActiveConfigRevision)
	}
	if up.Status != "INACTIVE" {
		t.Fatalf("status=%s want=INACTIVE", up.Status)
	}

	// Update config — candidate revision bumps, active stays nil
	updated := testConfig(secret.SecretID, secret.SecretVersion)
	updated.Name = "Adapter-v2"
	up2, err := svc.UpdateUpstream(ctx, adminID, up.UpstreamID, 1, updated)
	if err != nil {
		t.Fatal(err)
	}
	if up2.ConfigRevision != 2 {
		t.Fatalf("candidate revision=%d want=2", up2.ConfigRevision)
	}
	if up2.ActiveConfigRevision != nil {
		t.Fatalf("active revision should still be nil after update, got=%d", *up2.ActiveConfigRevision)
	}

	// Manually activate
	if _, err := st.Client.Upstream.UpdateOneID(up.UpstreamID).SetActiveConfigRevision(2).SetStatus("ACTIVE").Save(ctx); err != nil {
		t.Fatal(err)
	}
	up3, err := svc.GetUpstream(ctx, up.UpstreamID)
	if err != nil {
		t.Fatal(err)
	}
	if up3.ConfigRevision != 2 || up3.ActiveConfigRevision == nil || *up3.ActiveConfigRevision != 2 {
		t.Fatalf("after activate: candidate=%d active=%v", up3.ConfigRevision, up3.ActiveConfigRevision)
	}
}

// HUB-UPS-002: Test Connection must not auto-apply. CreateUpstream + validate
// should not change status or active revision.
func TestHUBUPS002ValidateConfigDoesNotApply(t *testing.T) {
	ctx := context.Background()
	svc, st, adminID, _ := newUpstreamService(t)
	defer st.Close()
	secret, err := svc.CreateSecret(ctx, adminID, "token", "value")
	if err != nil {
		t.Fatal(err)
	}
	config := testConfig(secret.SecretID, secret.SecretVersion)
	// ValidateConfig should not mutate anything
	err = ValidateConfig(ctx, st.Client, config)
	if err != nil {
		t.Fatal(err)
	}
	// No upstream should exist yet
	count, err := st.Client.Upstream.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("ValidateConfig created upstream: %d", count)
	}
	// Now actually create
	up, err := svc.CreateUpstream(ctx, adminID, config)
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != "INACTIVE" || up.ActiveConfigRevision != nil {
		t.Fatalf("CreateUpstream auto-applied: status=%s active=%v", up.Status, up.ActiveConfigRevision)
	}
}

// HUB-UPS-004: upstreamConfig must precisely reference secretId+secretVersion.
// Invalid secretId or version must be rejected.
func TestHUBUPS004PreciseSecretReference(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID, _ := newUpstreamService(t)
	secret, err := svc.CreateSecret(ctx, adminID, "token", "value")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("valid secretRef is accepted", func(t *testing.T) {
		_, err := svc.CreateUpstream(ctx, adminID, testConfig(secret.SecretID, secret.SecretVersion))
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("nonexistent secretId is rejected", func(t *testing.T) {
		bad := testConfig(secret.SecretID, secret.SecretVersion)
		bad.Auth.AdditionalProperties["secretRef"] = map[string]interface{}{
			"secretId": "sec_nonexistent", "secretVersion": 1,
		}
		_, err := svc.CreateUpstream(ctx, adminID, bad)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for nonexistent secret, got: %v", err)
		}
	})
	t.Run("stale secretVersion is rejected", func(t *testing.T) {
		bad := testConfig(secret.SecretID, 999)
		_, err := svc.CreateUpstream(ctx, adminID, bad)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for stale version, got: %v", err)
		}
	})
}

// HUB-UPS-005: apply failure must retain old active revision.
// (This is tested at the runtimecontrol level, but here we verify that
// UpdateUpstream with a bad config does not change ConfigRevision.)
func TestHUBUPS005UpdateFailureRetainsOldRevision(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID, _ := newUpstreamService(t)
	secret, err := svc.CreateSecret(ctx, adminID, "token", "value")
	if err != nil {
		t.Fatal(err)
	}
	up, err := svc.CreateUpstream(ctx, adminID, testConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	// Try update with wrong expected revision
	_, err = svc.UpdateUpstream(ctx, adminID, up.UpstreamID, 999, testConfig(secret.SecretID, secret.SecretVersion))
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected ErrRevisionConflict, got: %v", err)
	}
	// Verify candidate revision unchanged
	up2, err := svc.GetUpstream(ctx, up.UpstreamID)
	if err != nil {
		t.Fatal(err)
	}
	if up2.ConfigRevision != 1 {
		t.Fatalf("revision changed after failed update: %d", up2.ConfigRevision)
	}
}

// HUB-UPS-006: secret rotation must produce a new SecretVersion but
// must not by itself produce a new managedGeneration (that is a
// runtimecontrol concern). Here we verify ReplaceSecret creates a new
// version and the old version is still intact.
func TestHUBUPS006SecretRotationProducesNewVersion(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID, _ := newUpstreamService(t)
	secret, err := svc.CreateSecret(ctx, adminID, "token", "first-value")
	if err != nil {
		t.Fatal(err)
	}
	// Rotate
	replaced, err := svc.ReplaceSecret(ctx, adminID, secret.SecretID, 1, "second-value")
	if err != nil {
		t.Fatal(err)
	}
	if replaced.SecretVersion != 2 {
		t.Fatalf("new version=%d want=2", replaced.SecretVersion)
	}
	// Old version should still be resolvable
	firstValue, err := svc.ResolveSecret(ctx, secret.SecretID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstValue) != "first-value" {
		t.Fatalf("old version value mismatch: %s", firstValue)
	}
	// New version should resolve to new value
	secondValue, err := svc.ResolveSecret(ctx, secret.SecretID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if string(secondValue) != "second-value" {
		t.Fatalf("new version value mismatch: %s", secondValue)
	}
}
