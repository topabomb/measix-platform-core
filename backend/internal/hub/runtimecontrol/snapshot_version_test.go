package runtimecontrol_test

import (
	"context"
	"encoding/json"
	"testing"

	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

// ERX-C0-002: the production activation path persists the compiler's schema.
func TestPublishAndRepublishPersistSnapshotVersion(t *testing.T) {
	ctx := context.Background()
	st, svc, _, relayServer, _, adminID, _, draftRevision := newRuntimeControlEnv(t)
	defer relayServer.Close()
	first := publishAndFinalize(t, svc, adminID, draftRevision)
	second, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), first.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"publish": first.ReleaseID, "republish": second.ReleaseID} {
		t.Run(name, func(t *testing.T) {
			row, err := st.Client.ManagedRelease.Get(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot clientapi.ManagedSnapshot
			if err := json.Unmarshal(row.SnapshotJSON, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.SchemaVersion != 4 {
				t.Fatalf("unexpected persisted snapshot version %d", snapshot.SchemaVersion)
			}
		})
	}
}
