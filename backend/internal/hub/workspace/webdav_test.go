package workspace

import (
	"context"
	"encoding/json"
	"testing"

	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestDAVActivationStagesOnceAndKeepsExplicitDisable(t *testing.T) {
	s, _, actor, _ := lifecycleFixture(t)
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	if platformid.Validate(platformid.WebDAV, service.DavResourceID) != nil {
		t.Fatal("workspace has no stable DAV identity")
	}
	s.Client.ManagedDraft.Create().SetID("current").SetDraftRevision(8).SetContentJSON([]byte(`{"mcp":[],"bindings":[],"providers":[{"providerId":"unpublished-edit","displayName":"retain"}]}`)).SetUpdatedByUserID(actor).SetUpdatedAt(s.Now()).SaveX(ctx)
	apply := func() {
		t.Helper()
		op, err := s.WorkspaceServiceCommand(ctx, actor, platformid.New(platformid.Idempotency), service.ID, "APPLY")
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		result, err := s.Operation(ctx, op.OperationId)
		if err != nil || result.State != "COMPLETED" {
			t.Fatalf("apply: %+v %v", result, err)
		}
	}
	apply()
	draft := s.Client.ManagedDraft.GetX(ctx, "current")
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(draft.ContentJSON, &content); err != nil {
		t.Fatal(err)
	}
	if content.Webdav == nil || len(*content.Webdav) != 1 || !(*content.Webdav)[0].Enabled || (*content.Webdav)[0].ResourceId != service.DavResourceID || len(content.Providers) != 1 || len(content.Mcp) != 0 || draft.DraftRevision != 9 {
		t.Fatal("activation did not preserve draft and stage DAV")
	}
	if s.Client.ManagedRelease.Query().CountX(ctx) != 1 {
		t.Fatal("activation published draft")
	}
	(*content.Webdav)[0].Enabled = false
	raw, _ := json.Marshal(content)
	s.Client.ManagedDraft.UpdateOneID(draft.ID).SetContentJSON(raw).SetDraftRevision(10).ExecX(ctx)
	apply()
	draft = s.Client.ManagedDraft.GetX(ctx, "current")
	json.Unmarshal(draft.ContentJSON, &content)
	if draft.DraftRevision != 10 || (*content.Webdav)[0].Enabled {
		t.Fatal("reactivation reset explicit disable")
	}
}

func TestDAVPermissionChangeIsIndependentAndWaitsForActivation(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	before := s.Client.AgentSpace.GetX(ctx, user)
	mode := adminapi.DAVAccessMode("READ_ONLY")
	revision := before.DavBindingRevision
	key := platformid.New(platformid.Idempotency)
	input := adminapi.WorkspaceCommand{Action: "SET_DAV_ACCESS", ExpectedRevision: int(before.BindingRevision), DavAccessMode: &mode, ExpectedDAVRevision: &revision}
	op, err := s.Command(ctx, actor, key, user, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Command(ctx, actor, key, user, input); err != nil {
		t.Fatal("idempotent retry", err)
	}
	pending := s.Client.AgentSpace.GetX(ctx, user)
	if pending.DavAccessMode != "READ_ONLY" || pending.DavBindingRevision != revision+1 || pending.BindingRevision != before.BindingRevision || !pending.DavConfirmed {
		t.Fatal("permission mutation altered Files/MCP or failed its independent revision")
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	done, err := s.Operation(ctx, op.OperationId)
	after := s.Client.AgentSpace.GetX(ctx, user)
	if err != nil || done.State != "COMPLETED" || after.DavAppliedRevision != after.DavBindingRevision || after.DavAppliedAccessMode != "READ_ONLY" || after.DavSecretVersion != before.DavSecretVersion || remote.davWrites != 1 {
		t.Fatalf("permission apply: %+v %v", done, err)
	}
	if _, err = s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, input); err != ErrConflict {
		t.Fatalf("stale permission command: %v", err)
	}
}

func TestDAVAuthorizedRestoreIsAutomaticButCredentialMismatchNeedsVerification(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	space := s.Client.AgentSpace.GetX(ctx, user)
	if space.FileAccessIntent != "AUTHORIZED" {
		t.Fatal("creation omitted file intent")
	}
	runCommand(t, s, actor, user, "DISCONNECT")
	runCommand(t, s, actor, user, "RESTORE")
	if remote.davWrites != 2 || !s.Client.AgentSpace.GetX(ctx, user).DavConfirmed {
		t.Fatal("authorized restore requires manual credential renewal")
	}
	s.Client.AgentSpace.UpdateOneID(user).SetDavConfirmed(false).SetDiagnosticCode("dav_credential_unavailable").ExecX(ctx)
	for range 3 {
		if err := s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if remote.davWrites != 2 || s.Client.AgentSpace.GetX(ctx, user).DavConfirmed {
		t.Fatal("credential mismatch was overwritten without verifying external ownership")
	}
	runCommand(t, s, actor, user, "REVOKE_DAV")
	for range 3 {
		if err := s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if remote.davWrites != 2 || s.Client.AgentSpace.GetX(ctx, user).DavConfirmed {
		t.Fatal("explicit revoke was automatically undone")
	}
}

func TestDAVProjectionDoesNotClaimAppliedPermissionWhenControlIsUnconfirmed(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	managed := s.Client.ManagedState.GetX(ctx, "current")
	kind := adminapi.RuntimeBindingDefinitionTargetKind("REMOTE_WORKSPACE")
	defs := []adminapi.WebDAVDefinition{{ResourceId: service.DavResourceID, Enabled: true}}
	content, _ := json.Marshal(adminapi.ManagedDraftContent{Webdav: &defs, Bindings: []adminapi.RuntimeBindingDefinition{{ResourceId: service.DavResourceID, TargetKind: &kind, WorkspaceServiceId: &service.ID}}})
	s.Client.ManagedRelease.UpdateOneID(*managed.ActiveReleaseID).SetReleaseContentJSON(content).ExecX(ctx)
	for _, status := range []string{"READY", "ACTIVATING", "DEGRADED"} {
		s.Client.ManagedState.UpdateOneID("current").SetRuntimeStatus(status).ExecX(ctx)
		view, err := s.Projection(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if status == "READY" {
			if !view.Webdav.Available {
				t.Fatal("ready binding unavailable")
			}
		} else if view.Webdav.Available || view.Webdav.AccessMode != nil || view.Webdav.LastConfirmedAccessMode == nil || view.Webdav.Reason != "dav_activation_pending" {
			t.Fatalf("unconfirmed control claims effective permission: %+v", view.Webdav)
		}
	}
}

func TestDAVExistingActiveServiceStagesOnReconcileWithoutPublishing(t *testing.T) {
	s, _, actor, _ := lifecycleFixture(t)
	ctx := context.Background()
	s.Client.ManagedDraft.Create().SetID("current").SetDraftRevision(7).SetContentJSON([]byte(`{"mcp":[],"bindings":[]}`)).SetUpdatedByUserID(actor).SetUpdatedAt(s.Now()).SaveX(ctx)
	for range 2 {
		if err := s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	draft := s.Client.ManagedDraft.GetX(ctx, "current")
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(draft.ContentJSON, &content); err != nil {
		t.Fatal(err)
	}
	if draft.DraftRevision != 8 || content.Webdav == nil || len(*content.Webdav) != 1 || !(*content.Webdav)[0].Enabled {
		t.Fatal("upgraded active service did not stage DAV exactly once")
	}
	if s.Client.ManagedRelease.Query().CountX(ctx) != 1 {
		t.Fatal("background reconciliation published a draft")
	}
}

func TestDAVRejectedCredentialReplayKeepsAuthorizationAndIdentityChecks(t *testing.T) {
	for _, change := range []string{"health-only", "revoked", "disconnected", "permission", "revision", "secret", "user-disabled", "service-disabled"} {
		t.Run(change, func(t *testing.T) {
			s, _, actor, user := lifecycleFixture(t)
			ctx := context.Background()
			runCommand(t, s, actor, user, "CREATE")
			service := s.Client.WorkspaceService.Query().OnlyX(ctx)
			_, bindings, err := s.DAVRuntimeBindings(ctx, service.ID, service.DavResourceID)
			if err != nil || len(bindings) != 1 {
				t.Fatalf("binding: %v %d", err, len(bindings))
			}
			binding := bindings[0]
			s.Client.AgentSpace.UpdateOneID(user).SetDavConfirmed(false).SetDiagnosticCode("dav_credential_unavailable").ExecX(ctx)
			switch change {
			case "revoked":
				s.Client.AgentSpace.UpdateOneID(user).SetFileAccessIntent("REVOKED").ExecX(ctx)
			case "disconnected":
				s.Client.AgentSpace.UpdateOneID(user).SetIntent("DISCONNECTED").ExecX(ctx)
			case "permission":
				s.Client.AgentSpace.UpdateOneID(user).SetDavAccessMode("READ_ONLY").ExecX(ctx)
			case "revision":
				s.Client.AgentSpace.UpdateOneID(user).AddDavBindingRevision(1).ExecX(ctx)
			case "secret":
				binding.SecretRef.SecretVersion++
			case "user-disabled":
				s.Client.User.UpdateOneID(user).SetStatus("DISABLED").ExecX(ctx)
			case "service-disabled":
				s.Client.WorkspaceService.UpdateOneID(service.ID).SetEnabled(false).ExecX(ctx)
			}
			if err := s.ValidateDAVRuntimeBinding(ctx, binding); err == nil {
				t.Fatal("normal compilation accepted a rejected credential")
			}
			err = s.ValidateDAVReplayBinding(ctx, binding)
			if change == "health-only" && err != nil {
				t.Fatal(err)
			}
			if change != "health-only" && err == nil {
				t.Fatal("pending replay bypassed authorization or binding identity")
			}
		})
	}
}
