package runtimecontrol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/relaycontrolapi"
)

func TestDAVAcknowledgementPinsAppliedPermission(t *testing.T) {
	for _, kind := range []string{"WORKSPACE", "PUBLISH"} {
		for _, changed := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/current", true: "/superseded"}[changed], func(t *testing.T) {
				ctx := context.Background()
				st := testutil.OpenStoreHandle(t)
				now := time.Now().UTC()
				svc := &Service{Client: st.Client, Now: func() time.Time { return now }}
				st.Client.WorkspaceService.Create().SetID("workspace").SetName("Space").SetConfigRevision(1).SetEnabled(true).SetState("ACTIVE").SetMcpServerID("mcp").SetRuntimeRouteID("route").SetDavResourceID("dav").SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
				st.Client.ManagedState.Create().SetID("current").SetActiveManagedGeneration(0).SetDesiredControlRevision(2).SetDesiredBundleHash("hash").SetManagedStateRevision(1).SetRuntimeStatus("ACTIVATING").SetUpdatedAt(now).SaveX(ctx)
				st.Client.AgentSpace.Create().SetID("user").SetWorkspaceServiceID("workspace").SetRemoteUsername("remote").SetAgentSpaceID("space").SetBindingRevision(1).SetIntent("CONNECTED").SetState("CONNECTED").SetDavSecretID("secret").SetDavSecretVersion(1).SetDavBindingRevision(3).SetDavAccessMode("FULL").SetFileAccessIntent("AUTHORIZED").SetDavConfirmed(true).SetRemoteActive(true).SetStopPending(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
				bindings := []relaycontrolapi.UserDAVBinding{{UserId: "user", ResourceId: "dav", Target: relaycontrolapi.WorkspaceTarget{WorkspaceServiceId: "workspace", AgentSpaceId: "space", RemoteUsername: "remote", BindingRevision: 3}, SecretRef: relaycontrolapi.SecretRef{SecretId: "secret", SecretVersion: 1}, AccessMode: "FULL"}}
				descriptor, err := json.Marshal(relaycontrolapi.RuntimeControlState{DavBindings: &bindings})
				if err != nil {
					t.Fatal(err)
				}
				st.Client.Activation.Create().SetID("activation").SetKind(kind).SetState("APPLYING").SetIdempotencyKey("key").SetRequestHash("request").SetControlRevision(2).SetBundleHash("hash").SetTargetGeneration(1).SetTargetDescriptorJSON(descriptor).SetCreatedByUserID("admin").SetCreatedAt(now).SaveX(ctx)
				if changed {
					st.Client.AgentSpace.UpdateOneID("user").SetDavBindingRevision(4).SetDavAccessMode("READ_ONLY").ExecX(ctx)
				}
				if kind == "PUBLISH" {
					st.Client.ManagedRelease.Create().SetID("release").SetManagedGeneration(1).SetStatus("PENDING").SetReleaseContentJSON([]byte(`{}`)).SetSnapshotJSON([]byte(`{}`)).SetSnapshotHash("snapshot").SetSourceDraftRevision(1).SetCreatedByUserID("admin").SetCreatedAt(now).SaveX(ctx)
					err = svc.finalizePublish(ctx, "activation", "release", 1, 2, "hash")
				} else {
					err = svc.finalizeWorkspace(ctx, "activation")
				}
				if err != nil {
					t.Fatal(err)
				}
				space := st.Client.AgentSpace.GetX(ctx, "user")
				if !changed && (space.DavAppliedRevision != 3 || space.DavAppliedAccessMode != "FULL") {
					t.Fatal("ACK did not confirm migrated DAV binding")
				}
				if changed && space.DavAppliedRevision != 0 {
					t.Fatal("late ACK confirmed superseded permission")
				}
			})
		}
	}
}
