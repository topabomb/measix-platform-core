package runtimecontrol_test

import (
	"context"
	"errors"
	"testing"

	"measix/platform/ent/activation"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

// Every control writer compiles outside its transaction. A workspace command
// accepted in that gap must reject the stale compile, not strand an immutable
// descriptor which can no longer pass recovery validation after an ACK loss.
func TestControlIntentRejectsConcurrentDAVPermissionChange(t *testing.T) {
	for _, writer := range []string{"publish", "republish", "upstream", "workspace", "security"} {
		t.Run(writer, func(t *testing.T) {
			ctx := context.Background()
			st, svc, relayStore, relayServer, now, actor, upstreamID, _ := newRuntimeControlEnv(t)
			defer relayServer.Close()
			ws := workspace.NewService(st.Client, svc.Upstream)
			svc.Workspace, ws.ApplyControl = ws, svc.ApplyWorkspace
			secret, err := svc.Upstream.CreateSecret(ctx, actor, "workspace", "test-workspace-key")
			if err != nil {
				t.Fatal(err)
			}
			origin := "http://127.0.0.1:1"
			service, err := ws.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "DAV", Config: adminapi.AgentSpaceConfig{AdminOrigin: origin, McpOrigin: origin, DavOrigin: &origin, ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
			if err != nil {
				t.Fatal(err)
			}
			st.Client.WorkspaceService.UpdateOneID(service.WorkspaceServiceId).SetActiveConfigRevision(1).SetEnabled(true).SetState("ACTIVE").ExecX(ctx)
			st.Client.AgentSpace.Create().SetID(actor).SetWorkspaceServiceID(service.WorkspaceServiceId).SetRemoteUsername("alice").SetAgentSpaceID(platformid.New(platformid.AgentSpace)).SetBindingRevision(1).SetIntent("CONNECTED").SetState("CONNECTED").SetRemoteActive(true).SetStopPending(false).SetDavConfirmed(true).SetFileAccessIntent("AUTHORIZED").SetDavAccessMode("FULL").SetDavBindingRevision(1).SetDavSecretID(secret.SecretID).SetDavSecretVersion(int64(secret.SecretVersion)).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
			if err = ws.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			draft, err := svc.Capability.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			publishAndFinalize(t, svc, actor, draft.DraftRevision)
			managed := st.Client.ManagedState.GetX(ctx, "current")
			releaseID := *managed.ActiveReleaseID
			deviceID := platformid.New(platformid.Device)
			st.Client.Device.Create().SetID(deviceID).SetUserID(actor).SetStatus("ACTIVE").SetCreatedAt(now).SaveX(ctx)
			key, operationID := platformid.New(platformid.Idempotency), platformid.New(platformid.WorkspaceOperation)
			call := func() error {
				switch writer {
				case "publish":
					_, e := svc.Publish(ctx, runtimecontrol.PublishRequest{AdminUserID: actor, IdempotencyKey: key, ExpectedDraftRevision: draft.DraftRevision})
					return e
				case "republish":
					_, e := svc.Republish(ctx, actor, key, releaseID)
					return e
				case "upstream":
					_, e := svc.ApplyUpstream(ctx, actor, key, upstreamID)
					return e
				case "workspace":
					_, _, e := svc.ApplyWorkspace(ctx, actor, operationID)
					return e
				default:
					_, e := svc.RevokeDevice(ctx, actor, key, deviceID)
					return e
				}
			}
			originalRelay := svc.Relay
			svc.Relay = &davLostControl{store: relayStore, fail: true}
			var permissionOperation string
			svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{BeforeIntentCommit: func(ctx context.Context) error {
				row := st.Client.AgentSpace.GetX(ctx, actor)
				mode, revision := adminapi.DAVAccessMode("READ_ONLY"), row.DavBindingRevision
				op, e := ws.Command(ctx, actor, platformid.New(platformid.Idempotency), actor, adminapi.WorkspaceCommand{Action: "SET_DAV_ACCESS", ExpectedRevision: int(row.BindingRevision), ExpectedDAVRevision: &revision, DavAccessMode: &mode})
				permissionOperation = op.OperationId
				return e
			}})
			beforeActivations, beforeReleases := st.Client.Activation.Query().CountX(ctx), st.Client.ManagedRelease.Query().CountX(ctx)
			if err = call(); !errors.Is(err, runtimecontrol.ErrActivationInProgress) {
				t.Fatalf("stale DAV binding was persisted instead of rejected: %v", err)
			}
			if st.Client.Activation.Query().CountX(ctx) != beforeActivations || st.Client.ManagedRelease.Query().CountX(ctx) != beforeReleases || st.Client.ManagedState.GetX(ctx, "current").DesiredControlRevision != managed.DesiredControlRevision {
				t.Fatal("stale control intent left durable state")
			}
			svc.SetTestBarrierHooks(runtimecontrol.PublishBarrierHooks{})
			svc.Relay = originalRelay
			for range 3 {
				if err = ws.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if st.Client.WorkspaceOperation.GetX(ctx, permissionOperation).State != "COMPLETED" {
				t.Fatal("permission operation stranded")
			}
			svc.Relay = &davLostControl{store: relayStore, fail: true}
			if err = call(); err != nil {
				t.Fatal("fresh retry rejected", err)
			}
			if st.Client.Activation.Query().Where(activation.StateEQ("UNKNOWN")).CountX(ctx) != 1 {
				t.Fatal("missing simulated lost control")
			}
			if _, err = svc.Reconcile(ctx); err != nil {
				t.Fatal("fresh intent did not recover", err)
			}
			if st.Client.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).CountX(ctx) != 0 {
				t.Fatal("pending activation remains")
			}
			for _, binding := range relayStore.Current().DAVBindings {
				if binding.AccessMode != "READ_ONLY" {
					t.Fatal("permission regressed", binding.AccessMode)
				}
			}
		})
	}
}
