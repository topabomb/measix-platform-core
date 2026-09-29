package workspace

import (
	"context"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"testing"
)

func TestSaveConfigurationDoesNotContactRemoteAndIsCAS(t *testing.T) {
	st := testutil.OpenStore(t)
	box, err := security.NewSecretBox(make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	secrets := upstream.NewService(st.Client, box)
	actor := platformid.New(platformid.User)
	secret, err := secrets.CreateSecret(context.Background(), actor, "test workspace service", "not-a-production-secret")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(st.Client, secrets)
	st.Client.ManagedDraft.Create().SetID("current").SetDraftRevision(1).SetContentJSON([]byte(`{"mcp":[],"bindings":[]}`)).SetUpdatedByUserID(actor).SetUpdatedAt(svc.Now()).SaveX(context.Background())
	request := adminapi.SaveWorkspaceServiceRequest{Name: "Test", Config: adminapi.AgentSpaceConfig{AdminOrigin: "http://127.0.0.1:1", McpOrigin: "http://127.0.0.1:1", ReleaseIdentity: "3ea01c167fb263f8ef2467b5fe3103353f9a5ddc", ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 10000, IdleTimeoutMs: 60000}}
	got, err := svc.Save(context.Background(), actor, platformid.New(platformid.Idempotency), "", request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.ActiveConfigRevision != nil || got.McpPublished {
		t.Fatalf("save activated workspace service: %+v", got)
	}
	if st.Client.ManagedDraft.GetX(context.Background(), "current").DraftRevision != 1 {
		t.Fatal("saving workspace configuration changed the optional MCP draft")
	}
	request.Name = "stale"
	if _, err := svc.Save(context.Background(), actor, platformid.New(platformid.Idempotency), got.WorkspaceServiceId, request); err != ErrConflict {
		t.Fatalf("stale update: %v", err)
	}
}

func TestUnconfiguredWorkspaceIsStructuredAndNoSideEffects(t *testing.T) {
	st := testutil.OpenStore(t)
	svc := NewService(st.Client, nil)
	view, err := svc.Projection(context.Background(), platformid.New(platformid.User))
	if err != nil || view.State != "UNPROVISIONED" || view.FilesAvailable || view.McpAvailable {
		t.Fatalf("%+v %v", view, err)
	}
}

func TestWorkspaceMCPDraftIsExplicitCASAndIdempotent(t *testing.T) {
	s, _, actor, _ := lifecycleFixture(t)
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	s.Client.ManagedDraft.Create().SetID("current").SetDraftRevision(1).SetContentJSON([]byte(`{"mcp":[],"bindings":[]}`)).SetUpdatedByUserID(actor).SetUpdatedAt(s.Now()).SaveX(ctx)
	if err := s.StageMCP(ctx, actor, platformid.New(platformid.Idempotency), service.ID, 2); err != ErrConflict {
		t.Fatalf("stale draft: %v", err)
	}
	key := platformid.New(platformid.Idempotency)
	if err := s.StageMCP(ctx, actor, key, service.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.StageMCP(ctx, actor, key, service.ID, 1); err != nil {
		t.Fatal(err)
	}
	draft := s.Client.ManagedDraft.GetX(ctx, "current")
	if draft.DraftRevision != 2 {
		t.Fatal("retry duplicated draft mutation")
	}
	s.Client.WorkspaceService.UpdateOneID(service.ID).SetEnabled(false).ExecX(ctx)
	if err := s.StageMCP(ctx, actor, platformid.New(platformid.Idempotency), service.ID, 2); err != ErrUnavailable {
		t.Fatalf("disabled service staged MCP: %v", err)
	}
}
