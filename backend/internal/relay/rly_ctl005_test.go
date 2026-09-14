package relay_test

import (
	"testing"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// RLY-CTL-005: An invalid control state (bad route/upstream/reference/JWK
// or malformed descriptor) must be fully rejected — the old Current State
// must remain completely unchanged. No partial apply.
func TestRLYCTL005InvalidControlStateFullyRejected(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	store := control.NewStore(func() time.Time { return now })

	// Establish a valid current state first
	valid := minimalControlState(t, 5, 2)
	if _, err := store.Apply(valid); err != nil {
		t.Fatal(err)
	}
	original := store.Status()

	t.Run("invalid bundle hash is rejected", func(t *testing.T) {
		bad := minimalControlState(t, 6, 3)
		bad.BundleHash = "sha256:invalid-hash-that-does-not-match"
		_, err := store.Apply(bad)
		if err == nil {
			t.Fatal("expected error for mismatched bundle hash")
		}
		current := store.Status()
		if current.AppliedControlRevision != original.AppliedControlRevision {
			t.Fatalf("current state changed after invalid apply: rev=%d", current.AppliedControlRevision)
		}
	})

	t.Run("invalid deployment ID is rejected", func(t *testing.T) {
		bad := minimalControlState(t, 7, 3)
		bad.DeploymentId = "not-a-valid-deployment-id"
		bad.BundleHash, _ = control.HashDescriptor(bad)
		_, err := store.Apply(bad)
		if err == nil {
			t.Fatal("expected error for invalid deployment ID")
		}
		current := store.Status()
		if current.AppliedControlRevision != original.AppliedControlRevision {
			t.Fatalf("current state changed after invalid apply: rev=%d", current.AppliedControlRevision)
		}
	})

	t.Run("invalid route referencing nonexistent upstream is rejected", func(t *testing.T) {
		bad := minimalControlState(t, 8, 3)
		routeID := platformid.New(platformid.Route)
		upstreamID := platformid.New(platformid.Upstream)
		bad.Routes = []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID,
			AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1"},
			TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE,
			TimeoutPolicy:   relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}}
		// No upstream defined — route references missing upstream
		bad.BundleHash, _ = control.HashDescriptor(bad)
		_, err := store.Apply(bad)
		if err == nil {
			t.Fatal("expected error for route referencing missing upstream")
		}
		current := store.Status()
		if current.AppliedControlRevision != original.AppliedControlRevision {
			t.Fatalf("current state changed after invalid apply: rev=%d", current.AppliedControlRevision)
		}
	})

	t.Run("invalid JWK is rejected", func(t *testing.T) {
		bad := minimalControlState(t, 9, 3)
		bad.AuthKeys = []relaycontrolapi.PublicJwk{{
			Kty: "RSA", // wrong key type
			Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA, Use: relaycontrolapi.Sig,
			Kid: "bad-key", X: "invalid-base64",
		}}
		bad.BundleHash, _ = control.HashDescriptor(bad)
		_, err := store.Apply(bad)
		if err == nil {
			t.Fatal("expected error for invalid JWK")
		}
		current := store.Status()
		if current.AppliedControlRevision != original.AppliedControlRevision {
			t.Fatalf("current state changed after invalid apply: rev=%d", current.AppliedControlRevision)
		}
	})

	t.Run("stale revision is rejected", func(t *testing.T) {
		bad := minimalControlState(t, 3, 2) // lower revision than current 5
		bad.BundleHash, _ = control.HashDescriptor(bad)
		_, err := store.Apply(bad)
		if err == nil {
			t.Fatal("expected error for stale revision")
		}
		current := store.Status()
		if current.AppliedControlRevision != original.AppliedControlRevision {
			t.Fatalf("current state changed after stale apply: rev=%d", current.AppliedControlRevision)
		}
	})

	t.Run("zero max request bytes is rejected", func(t *testing.T) {
		bad := minimalControlState(t, 10, 3)
		bad.OperationalLimits = relaycontrolapi.OperationalLimits{MaxRequestBytes: 0}
		bad.BundleHash, _ = control.HashDescriptor(bad)
		_, err := store.Apply(bad)
		if err == nil {
			t.Fatal("expected error for zero max request bytes")
		}
		current := store.Status()
		if current.AppliedControlRevision != original.AppliedControlRevision {
			t.Fatalf("current state changed after invalid apply: rev=%d", current.AppliedControlRevision)
		}
	})
}
