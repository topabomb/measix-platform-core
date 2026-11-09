package maintenance_test

import (
	"bytes"
	"context"
	"measix/platform/internal/hub/maintenance"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/store"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceMigrationKeepsUnconfiguredCoreIndependent(t *testing.T) {
	s := testutil.OpenStore(t)
	for _, table := range []string{"workspace_services", "workspace_service_configs", "agent_spaces", "workspace_operations", "workspace_audits"} {
		var n int
		if err := s.DB.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("%s: %v", table, err)
		} else if n != 0 {
			t.Errorf("migration provisioned %s", table)
		}
	}
}

func TestWorkspaceBackupRetainsUnknownOperationAndEncryptedCredential(t *testing.T) {
	ctx := context.Background()
	s := testutil.OpenStore(t)
	now := time.Now().UTC()
	box, err := security.NewSecretBox(make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	secrets := upstream.NewService(s.Client, box)
	credential, err := secrets.CreateSecret(ctx, "actor", "candidate DAV", "test-backup-token")
	if err != nil {
		t.Fatal(err)
	}
	s.Client.WorkspaceService.Create().SetID("integration").SetName("independent service").SetConfigRevision(1).SetEnabled(true).SetState("READY").SetMcpServerID("mcp").SetRuntimeRouteID("route").SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	config := []byte(`{"managementSecret":{"secretId":"fixed","secretVersion":1}}`)
	s.Client.WorkspaceServiceConfig.Create().SetWorkspaceServiceID("integration").SetRevision(1).SetConfigJSON(config).SetCreatedByUserID("actor").SetCreatedAt(now).SaveX(ctx)
	s.Client.AgentSpace.Create().SetID("deleted-core-user").SetWorkspaceServiceID("integration").SetRemoteUsername("immutable-remote-user").SetAgentSpaceID("original-space").SetBindingRevision(7).SetIntent("DELETED").SetState("DELETING").SetDavConfirmed(false).SetRemoteActive(false).SetStopPending(true).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	target := []byte(`{"agentSpaceId":"original-space","configRevision":1}`)
	s.Client.WorkspaceOperation.Create().SetID("operation").SetWorkspaceServiceID("integration").SetUserID("deleted-core-user").SetAction("DELETE").SetIdempotencyKey("idempotency").SetRequestHash("hash").SetConfigRevision(1).SetBindingRevision(7).SetTargetJSON(target).SetState("UNKNOWN").SetStep("DELETE_SENT").SetCandidateSecretID(credential.SecretID).SetCandidateSecretVersion(int64(credential.SecretVersion)).SetCreatedByUserID("actor").SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	backup := filepath.Join(t.TempDir(), "restored.db")
	if _, err := maintenance.Backup(ctx, s.DB, backup, "workspace-test", now); err != nil {
		t.Fatal(err)
	}
	restored, err := store.OpenEnt(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := maintenance.Check(ctx, restored.DB); err != nil {
		t.Fatal(err)
	}
	op := restored.Client.WorkspaceOperation.GetX(ctx, "operation")
	if op.State != "UNKNOWN" || op.Step != "DELETE_SENT" || !bytes.Equal(op.TargetJSON, target) || op.BindingRevision != 7 {
		t.Fatalf("lost immutable operation: %+v", op)
	}
	binding := restored.Client.AgentSpace.GetX(ctx, "deleted-core-user")
	if binding.AgentSpaceID != "original-space" || binding.Intent != "DELETED" || !binding.StopPending {
		t.Fatalf("lost cleanup authority: %+v", binding)
	}
	payload, err := upstream.NewService(restored.Client, box).ResolveSecret(ctx, op.CandidateSecretID, int(op.CandidateSecretVersion))
	if err != nil || string(payload) != "test-backup-token" {
		t.Fatalf("candidate credential restore: %v", err)
	}
	if !bytes.Equal(restored.Client.WorkspaceServiceConfig.Query().OnlyX(ctx).ConfigJSON, config) {
		t.Fatal("configuration revision changed")
	}
}
