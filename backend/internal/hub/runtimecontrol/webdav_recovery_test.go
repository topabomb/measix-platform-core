package runtimecontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"measix/platform/ent/activation"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

type davLostControl struct {
	store         *control.Store
	fail, applied bool
}

func (r *davLostControl) Status(context.Context) (relaycontrolapi.ControlStatus, error) {
	return r.store.Status(), nil
}
func (r *davLostControl) Apply(_ context.Context, state relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error) {
	if r.fail {
		r.fail = false
		if r.applied {
			if _, err := r.store.Apply(state); err != nil {
				return relaycontrolapi.ControlAck{}, err
			}
		}
		return relaycontrolapi.ControlAck{}, context.DeadlineExceeded
	}
	return r.store.Apply(state)
}

func TestDAVCredentialRejectionDoesNotDeadlockPendingActivation(t *testing.T) {
	for _, kind := range []string{"PUBLISH", "WORKSPACE"} {
		for _, applied := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/before-apply", true: "/lost-ack"}[applied], func(t *testing.T) {
				ctx := context.Background()
				st, svc, relayStore, relayServer, _, actor, _, _ := newRuntimeControlEnv(t)
				defer relayServer.Close()
				ws := workspace.NewService(st.Client, svc.Upstream)
				svc.Workspace = ws
				ws.ApplyControl = svc.ApplyWorkspace
				spaceID, username, remoteURL, writes := platformid.New(platformid.AgentSpace), "", "", 0
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/admin/v1/status" {
						_, _ = w.Write([]byte(`{"ready":true}`))
						return
					}
					if r.URL.Path == "/admin/v1/users" {
						var body struct{ Username string }
						_ = json.NewDecoder(r.Body).Decode(&body)
						username = body.Username
					}
					out := map[string]any{"username": username, "agentSpaceId": spaceID, "mcpUrl": remoteURL + "/u/" + username + "/mcp", "davUrl": remoteURL + "/u/" + username + "/dav/", "status": "active", "stopPending": false, "token": "mcp-test"}
					if strings.HasSuffix(r.URL.Path, "/dav-token") {
						var body struct{ Token string }
						_ = json.NewDecoder(r.Body).Decode(&body)
						out["token"] = body.Token
						writes++
					}
					_ = json.NewEncoder(w).Encode(out)
				}))
				defer remote.Close()
				remoteURL = remote.URL
				secret, err := svc.Upstream.CreateSecret(ctx, actor, "management", "management-test")
				if err != nil {
					t.Fatal(err)
				}
				service, err := ws.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "DAV", Config: adminapi.AgentSpaceConfig{AdminOrigin: remote.URL, McpOrigin: remote.URL, DavOrigin: &remoteURL, ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = ws.WorkspaceServiceCommand(ctx, actor, platformid.New(platformid.Idempotency), service.WorkspaceServiceId, "APPLY"); err != nil {
					t.Fatal(err)
				}
				for range 3 {
					if err = ws.Reconcile(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if _, err = ws.Command(ctx, actor, platformid.New(platformid.Idempotency), actor, adminapi.WorkspaceCommand{Action: "CREATE", ExpectedRevision: 0}); err != nil {
					t.Fatal(err)
				}
				for range 3 {
					if err = ws.Reconcile(ctx); err != nil {
						t.Fatal(err)
					}
				}
				draft, err := svc.Capability.GetDraft(ctx)
				if err != nil {
					t.Fatal(err)
				}
				publishAndFinalize(t, svc, actor, draft.DraftRevision)
				lost := &davLostControl{store: relayStore, fail: true, applied: applied}
				svc.Relay = lost
				var activationID string
				if kind == "PUBLISH" {
					result := publishAndFinalize(t, svc, actor, draft.DraftRevision)
					activationID = result.ActivationID
				} else {
					row := st.Client.AgentSpace.GetX(ctx, actor)
					mode := adminapi.DAVAccessMode("READ_ONLY")
					rev := row.DavBindingRevision
					op, e := ws.Command(ctx, actor, platformid.New(platformid.Idempotency), actor, adminapi.WorkspaceCommand{Action: "SET_DAV_ACCESS", ExpectedRevision: int(row.BindingRevision), ExpectedDAVRevision: &rev, DavAccessMode: &mode})
					if e != nil {
						t.Fatal(e)
					}
					if e = ws.Reconcile(ctx); e != nil {
						t.Fatal(e)
					}
					activationID = st.Client.Activation.Query().Where(activation.SubjectIDEQ(op.OperationId + ":DAV_APPLY")).OnlyX(ctx).ID
				}
				if activationID == "" {
					t.Fatal("missing pending activation")
				}
				pending := st.Client.Activation.GetX(ctx, activationID)
				if pending.State != "UNKNOWN" {
					t.Fatal(pending.State)
				}
				original := append([]byte(nil), pending.TargetDescriptorJSON...)
				st.Client.AgentSpace.UpdateOneID(actor).SetDavConfirmed(false).SetDiagnosticCode("dav_credential_unavailable").ExecX(ctx)
				row := st.Client.AgentSpace.GetX(ctx, actor)
				if _, err = ws.Command(ctx, actor, platformid.New(platformid.Idempotency), actor, adminapi.WorkspaceCommand{Action: "SET_DAV", ExpectedRevision: int(row.BindingRevision)}); err != workspace.ErrPending {
					t.Fatal("pending barrier bypass", err)
				}
				if _, err = svc.Reconcile(ctx); err != nil {
					t.Fatal("failed to converge old activation after current key rejection", err)
				}
				for range 4 {
					if err = ws.Reconcile(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if writes != 1 || st.Client.AgentSpace.GetX(ctx, actor).DavConfirmed {
					t.Fatal("rejection automatically overwrote external key")
				}
				if len(relayStore.Current().DAVBindings) != 0 {
					t.Fatal("rejected binding not withdrawn")
				}
				if !bytes.Equal(st.Client.Activation.GetX(ctx, activationID).TargetDescriptorJSON, original) {
					t.Fatal("persisted descriptor mutated")
				}
				row = st.Client.AgentSpace.GetX(ctx, actor)
				if _, err = ws.Command(ctx, actor, platformid.New(platformid.Idempotency), actor, adminapi.WorkspaceCommand{Action: "SET_DAV", ExpectedRevision: int(row.BindingRevision)}); err != nil {
					t.Fatal(err)
				}
				for range 4 {
					if err = ws.Reconcile(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if writes != 2 || !st.Client.AgentSpace.GetX(ctx, actor).DavConfirmed || len(relayStore.Current().DAVBindings) != 1 {
					t.Fatal("verified administrator recovery failed")
				}
			})
		}
	}
}
