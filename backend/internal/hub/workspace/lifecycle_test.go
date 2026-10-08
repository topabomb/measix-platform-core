package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"measix/platform/ent"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

type lifecycleRemote struct {
	managementToken                      string
	url, name, space, token, dav         string
	exists, enabled, failDAV, failCreate bool
	failCheck                            bool
	creates, rotates, davWrites          int
	resources                            any
	onGet                                func()
}

func (f *lifecycleRemote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	management := f.managementToken
	if management == "" {
		management = "management-test"
	}
	if r.Header.Get("Authorization") != "Bearer "+management {
		w.WriteHeader(401)
		return
	}
	if r.URL.Path == "/admin/v1/status" {
		if f.failCheck {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ready": true})
		return
	}
	if r.URL.Path == "/admin/v1/users" && r.Method == "POST" {
		var in struct {
			Username string `json:"username"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		f.name = in.Username
		f.exists = true
		f.enabled = true
		f.creates++
		f.token = "mcp-created"
		if f.failCreate {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(f.identity(true))
		return
	}
	if !f.exists {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not_found"}`))
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/dav-token"):
		if r.Method == "DELETE" {
			f.dav = ""
			w.WriteHeader(204)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		f.dav = in.Token
		f.davWrites++
		if f.failDAV {
			w.WriteHeader(503)
			return
		}
		out := f.identity(false)
		out["token"] = f.dav
		out["davUrl"] = f.url + "/u/" + f.name + "/dav/"
		json.NewEncoder(w).Encode(out)
	case strings.HasSuffix(r.URL.Path, "/token"):
		f.rotates++
		f.token = "mcp-rotated"
		json.NewEncoder(w).Encode(map[string]string{"token": f.token, "keyId": "test"})
	case r.Method == "PATCH":
		var in struct {
			Enabled bool `json:"enabled"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		f.enabled = in.Enabled
		if !f.enabled {
			f.dav = ""
			f.token = ""
		}
		json.NewEncoder(w).Encode(f.identity(false))
	case r.Method == "DELETE":
		f.exists = false
		w.WriteHeader(202)
	default:
		if f.onGet != nil {
			f.onGet()
		}
		out := f.identity(false)
		if f.resources != nil {
			out["resources"] = f.resources
		}
		json.NewEncoder(w).Encode(out)
	}
}
func (f *lifecycleRemote) identity(credential bool) map[string]any {
	status := "disabled"
	if f.enabled {
		status = "active"
	}
	out := map[string]any{"username": f.name, "agentSpaceId": f.space, "mcpUrl": f.url + "/u/" + f.name + "/mcp", "status": status, "stopPending": false}
	if credential {
		out["token"] = f.token
	}
	return out
}
func lifecycleFixture(t *testing.T) (*Service, *lifecycleRemote, string, string) {
	t.Helper()
	ctx := context.Background()
	st := testutil.OpenStore(t)
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	secrets := upstream.NewService(st.Client, box)
	svc := NewService(st.Client, secrets)
	actor := platformid.New(platformid.User)
	user := platformid.New(platformid.User)
	now := time.Now().UTC()
	st.Client.User.Create().SetID(user).SetUsername(user).SetDisplayName("Workspace user").SetRole("USER").SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	remote := &lifecycleRemote{space: platformid.New(platformid.AgentSpace)}
	server := httptest.NewServer(remote)
	remote.url = server.URL
	t.Cleanup(server.Close)
	secret, e := secrets.CreateSecret(ctx, actor, "management", "management-test")
	if e != nil {
		t.Fatal(e)
	}
	cfg := adminapi.AgentSpaceConfig{AdminOrigin: server.URL, McpOrigin: server.URL, DavOrigin: &server.URL, ManagementSecret: adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}, ConnectTimeoutMs: 1000, IdleTimeoutMs: 1000}
	workspaceService, e := svc.Save(ctx, actor, platformid.New(platformid.Idempotency), "", adminapi.SaveWorkspaceServiceRequest{Name: "test", Config: cfg})
	if e != nil {
		t.Fatal(e)
	}
	st.Client.WorkspaceService.UpdateOneID(workspaceService.WorkspaceServiceId).SetEnabled(true).SetState("ACTIVE").SetActiveConfigRevision(1).SaveX(ctx)
	kind := adminapi.RuntimeBindingDefinitionTargetKind("REMOTE_WORKSPACE")
	content, _ := json.Marshal(adminapi.ManagedDraftContent{Mcp: []adminapi.McpDefinition{{McpServerId: workspaceService.McpServerId, Enabled: true}}, Bindings: []adminapi.RuntimeBindingDefinition{{ResourceId: workspaceService.McpServerId, TargetKind: &kind, WorkspaceServiceId: &workspaceService.WorkspaceServiceId}}})
	release := platformid.New(platformid.Release)
	st.Client.ManagedRelease.Create().SetID(release).SetManagedGeneration(1).SetStatus("ACTIVE").SetReleaseContentJSON(content).SetSnapshotJSON([]byte(`{}`)).SetSnapshotHash("test").SetSourceDraftRevision(1).SetCreatedByUserID(actor).SetCreatedAt(now).SaveX(ctx)
	st.Client.ManagedState.Create().SetID("current").SetActiveReleaseID(release).SetActiveManagedGeneration(1).SetDesiredControlRevision(1).SetManagedStateRevision(1).SetRuntimeStatus("READY").SetUpdatedAt(now).SaveX(ctx)
	svc.ApplyControl = func(ctx context.Context, actor, op string) (string, bool, error) {
		id := platformid.New(platformid.Activation)
		_, e := st.Client.Activation.Create().SetID(id).SetKind("WORKSPACE").SetState("COMPLETED").SetIdempotencyKey(op).SetRequestHash(op).SetControlRevision(2).SetBundleHash("test").SetTargetDescriptorJSON([]byte(`{}`)).SetSubjectID(op).SetCreatedByUserID(actor).SetCreatedAt(now).Save(ctx)
		return id, true, e
	}
	return svc, remote, actor, user
}
func runCommand(t *testing.T, s *Service, actor, user, action string) adminapi.WorkspaceOperation {
	t.Helper()
	ctx := context.Background()
	view, e := s.Projection(ctx, user)
	if e != nil {
		t.Fatal(e)
	}
	in := adminapi.WorkspaceCommand{Action: adminapi.WorkspaceCommandAction(action), ExpectedRevision: view.BindingRevision}
	if action == "DELETE" {
		in.Confirmation = view.AgentSpaceId
	}
	op, e := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, in)
	if e != nil {
		t.Fatal(action, e)
	}
	for i := 0; i < 3; i++ {
		if e = s.Reconcile(ctx); e != nil {
			t.Fatal(e)
		}
	}
	got, e := s.Operation(ctx, op.OperationId)
	if e != nil {
		t.Fatal(e)
	}
	return got
}

func TestLifecycleKeepsSpaceAndSeparatesDAVFromMCP(t *testing.T) {
	s, f, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	op := runCommand(t, s, actor, user, "CREATE")
	if op.State != "COMPLETED" {
		t.Fatalf("create: %+v", op)
	}
	view, _ := s.Projection(ctx, user)
	if !view.McpAvailable || !view.FilesAvailable || f.creates != 1 {
		t.Fatalf("create projection %+v", view)
	}
	original := *view.AgentSpaceId
	revision := view.BindingRevision
	op = runCommand(t, s, actor, user, "REVOKE_DAV")
	view, _ = s.Projection(ctx, user)
	if op.State != "COMPLETED" || !view.McpAvailable || view.FilesAvailable || view.BindingRevision != revision {
		t.Fatalf("revoke %+v %+v", op, view)
	}
	runCommand(t, s, actor, user, "DISCONNECT")
	view, _ = s.Projection(ctx, user)
	if view.State != "DISCONNECTED" || view.McpAvailable || view.FilesAvailable || f.enabled {
		t.Fatalf("disconnect %+v", view)
	}
	runCommand(t, s, actor, user, "RESTORE")
	view, _ = s.Projection(ctx, user)
	if !view.McpAvailable || view.FilesAvailable || *view.AgentSpaceId != original || f.creates != 1 || f.rotates != 1 {
		t.Fatalf("restore %+v", view)
	}
	runCommand(t, s, actor, user, "DELETE")
	if _, e := s.Client.AgentSpace.Get(ctx, user); !ent.IsNotFound(e) || f.exists {
		t.Fatalf("delete remote=%v local=%v", f.exists, e)
	}
}
func TestUnknownDAVKeepsMCPAndReusesDurableCandidate(t *testing.T) {
	s, f, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	f.failDAV = true
	op := runCommand(t, s, actor, user, "CREATE")
	view, _ := s.Projection(ctx, user)
	if op.State != "UNKNOWN" || !view.McpAvailable || view.FilesAvailable {
		t.Fatalf("DAV must not suppress MCP: %+v %+v", op, view)
	}
	candidate := f.dav
	s.Reconcile(ctx)
	if f.davWrites != 1 {
		t.Fatal("unknown write automatically replayed")
	}
	f.failDAV = false
	evidence := "Original management request has ended; target identity verified."
	_, e := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "CONTINUE", ExpectedRevision: view.BindingRevision, Evidence: &evidence})
	if e != nil {
		t.Fatal(e)
	}
	s.Reconcile(ctx)
	view, _ = s.Projection(ctx, user)
	if !view.FilesAvailable || !view.McpAvailable || f.dav != candidate || f.creates != 1 {
		t.Fatalf("recovery %+v", view)
	}
}

func TestControlFailureRetainsRuntimeBindingAndRetriesExistingOperation(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	runCommand(t, s, actor, user, "CREATE")
	original := s.ApplyControl
	s.ApplyControl = func(context.Context, string, string) (string, bool, error) { return "", false, ErrUnavailable }
	op := runCommand(t, s, actor, user, "RESET_MCP")
	if op.State != "RUNNING" || op.Step != "CONTROL" {
		t.Fatalf("expected control failure: %+v", op)
	}
	space := s.Client.AgentSpace.GetX(ctx, user)
	if space.State != "RESTORING" {
		t.Fatal(space.State)
	}
	workspaceService := s.Client.WorkspaceService.GetX(ctx, space.WorkspaceServiceID)
	s.ApplyControl = func(ctx context.Context, actor, op string) (string, bool, error) {
		_, bindings, e := s.RuntimeBindings(ctx, workspaceService.ID, workspaceService.McpServerID)
		if e != nil || len(bindings) != 1 {
			t.Fatalf("recovery compiled without user binding: %+v %v", bindings, e)
		}
		return original(ctx, actor, op)
	}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	view, _ := s.Projection(ctx, user)
	if !view.McpAvailable {
		t.Fatalf("recovered binding unavailable: %+v", view)
	}
}

func TestFailedServiceCheckPreservesPriorEnabledState(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "active"}[enabled], func(t *testing.T) {
			s, remote, actor, _ := lifecycleFixture(t)
			ctx := context.Background()
			service := s.Client.WorkspaceService.Query().OnlyX(ctx)
			state := "DISABLED"
			if enabled {
				state = "ACTIVE"
			}
			s.Client.WorkspaceService.UpdateOneID(service.ID).SetEnabled(enabled).SetState(state).ExecX(ctx)
			remote.failCheck = true
			op, err := s.WorkspaceServiceCommand(ctx, actor, platformid.New(platformid.Idempotency), service.ID, "APPLY")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			got := s.Client.WorkspaceService.GetX(ctx, service.ID)
			if got.Enabled != enabled || got.State != state {
				t.Fatalf("failed check changed availability: enabled=%v state=%s", got.Enabled, got.State)
			}
			result, err := s.Operation(ctx, op.OperationId)
			if err != nil || result.State != "COMPLETED" || result.Step != "CHECK_FAILED" {
				t.Fatalf("failed check cannot be corrected: %+v %v", result, err)
			}
		})
	}
}

func TestCannotIssueDAVWithoutConfiguredOrigin(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	cfg, err := s.config(ctx, service.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DavOrigin = nil
	_, err = s.Save(ctx, actor, platformid.New(platformid.Idempotency), service.ID, adminapi.SaveWorkspaceServiceRequest{Name: service.Name, ExpectedRevision: 1, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	s.Client.WorkspaceService.UpdateOneID(service.ID).SetActiveConfigRevision(2).ExecX(ctx)
	runCommand(t, s, actor, user, "CREATE")
	view, _ := s.Projection(ctx, user)
	before := s.Client.WorkspaceOperation.Query().CountX(ctx)
	_, err = s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "SET_DAV", ExpectedRevision: view.BindingRevision})
	if err != ErrInvalid {
		t.Fatalf("unconfigured DAV command accepted: %v", err)
	}
	if s.Client.WorkspaceOperation.Query().CountX(ctx) != before || remote.davWrites != 0 {
		t.Fatal("invalid command left a durable operation or remote write")
	}
}

func TestServiceControlFailureCanConvergeWithoutNewCommand(t *testing.T) {
	s, _, actor, _ := lifecycleFixture(t)
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	apply := s.ApplyControl
	s.ApplyControl = func(context.Context, string, string) (string, bool, error) { return "", false, ErrUnavailable }
	op, err := s.WorkspaceServiceCommand(ctx, actor, platformid.New(platformid.Idempotency), service.ID, "APPLY")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Operation(ctx, op.OperationId)
	if got.State != "RUNNING" || got.Step != "CONTROL" {
		t.Fatalf("control failure wedged service: %+v", got)
	}
	s.ApplyControl = apply
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Operation(ctx, op.OperationId)
	if got.State != "COMPLETED" {
		t.Fatalf("control did not converge: %+v", got)
	}
}

func TestDisableUserSupersedesUnknownDAVAndPreservesIntent(t *testing.T) {
	s, f, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	f.failDAV = true
	op := runCommand(t, s, actor, user, "CREATE")
	if op.State != "UNKNOWN" {
		t.Fatal(op)
	}
	s.Client.User.UpdateOneID(user).SetStatus("DISABLED").SaveX(ctx)
	for i := 0; i < 3; i++ {
		if err := s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	space := s.Client.AgentSpace.GetX(ctx, user)
	if f.enabled || f.dav != "" || space.State != "DISCONNECTED" || space.Intent != "CONNECTED" {
		t.Fatalf("disabled user cleanup blocked: %+v remote enabled=%v", space, f.enabled)
	}
	if f.davWrites != 1 {
		t.Fatal("unknown DAV write replayed")
	}
}

func TestLostCreateRequiresExplicitTakeoverOfOriginalSpace(t *testing.T) {
	s, f, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	f.failCreate = true
	op := runCommand(t, s, actor, user, "CREATE")
	if op.State != "UNKNOWN" || op.Step != "CREATE_SENT" {
		t.Fatalf("lost response: %+v", op)
	}
	for i := 0; i < 3; i++ {
		s.Reconcile(ctx)
	}
	if f.creates != 1 {
		t.Fatal("unknown create replayed")
	}
	view, _ := s.Projection(ctx, user)
	if view.McpAvailable || view.FilesAvailable {
		t.Fatal("unverified account became available")
	}
	evidence := "Original request completed; remote username and space manually verified"
	_, e := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "TAKEOVER", ExpectedRevision: view.BindingRevision, Evidence: &evidence, RemoteUsername: &f.name, AgentSpaceId: &f.space})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		if e = s.Reconcile(ctx); e != nil {
			t.Fatal(e)
		}
	}
	view, _ = s.Projection(ctx, user)
	if !view.McpAvailable || view.FilesAvailable || view.AgentSpaceId == nil || *view.AgentSpaceId != f.space || f.creates != 1 {
		t.Fatalf("takeover %+v", view)
	}
}

func TestVerifiedLostCreateCanBeCleanedAfterUserRevocation(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "deleted"}[deleted], func(t *testing.T) {
			s, remote, actor, user := lifecycleFixture(t)
			ctx := context.Background()
			remote.failCreate = true
			runCommand(t, s, actor, user, "CREATE")
			if deleted {
				s.Client.User.DeleteOneID(user).ExecX(ctx)
			} else {
				s.Client.User.UpdateOneID(user).SetStatus("DISABLED").ExecX(ctx)
			}
			view, _ := s.Projection(ctx, user)
			evidence := "Original create request ended; original account and space verified for cleanup"
			_, err := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "CONTINUE", ExpectedRevision: view.BindingRevision, Evidence: &evidence, RemoteUsername: &remote.name, AgentSpaceId: &remote.space})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if err = s.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if deleted {
				if remote.exists {
					t.Fatal("deleted user's remote account was not cleaned")
				}
			} else if remote.enabled {
				t.Fatal("disabled user's unknown create remained active")
			}
			if remote.creates != 1 || remote.rotates != 0 {
				t.Fatal("cleanup replayed creation or issued credentials")
			}
		})
	}
}

func TestVerifiedDisconnectRetiresUnknownDAVWithoutReplay(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	remote.failDAV = true
	old := runCommand(t, s, actor, user, "CREATE")
	view, _ := s.Projection(ctx, user)
	spaceID := str(view.AgentSpaceId)
	input := adminapi.WorkspaceCommand{Action: "DISCONNECT", ExpectedRevision: view.BindingRevision}
	if _, err := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, input); err != ErrPending {
		t.Fatalf("unverified disconnect must remain blocked: %v", err)
	}
	evidence := "Verified original account and space; the old DAV request has ended."
	input.Evidence = &evidence
	s.Client.WorkspaceOperation.UpdateOneID(old.OperationId).SetStep("CREATE_SENT").ExecX(ctx)
	if _, err := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, input); err != ErrPending {
		t.Fatalf("unknown create must not be superseded by disconnect: %v", err)
	}
	s.Client.WorkspaceOperation.UpdateOneID(old.OperationId).SetStep("DAV_SENT").ExecX(ctx)
	op, err := s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, input)
	if err != nil {
		t.Fatalf("verified disconnect must provide a recovery exit: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	retired, _ := s.Operation(ctx, old.OperationId)
	finished, _ := s.Operation(ctx, op.OperationId)
	view, _ = s.Projection(ctx, user)
	if retired.State != "COMPLETED" || retired.Step != "SUPERSEDED_BY_VERIFIED_DISCONNECT" || finished.State != "COMPLETED" || view.State != "DISCONNECTED" {
		t.Fatalf("recovery did not settle: %+v %+v %+v", retired, finished, view)
	}
	if !remote.exists || remote.enabled || remote.dav != "" || remote.davWrites != 1 || str(view.AgentSpaceId) != spaceID {
		t.Fatalf("exists=%v enabled=%v DAV present=%v writes=%d space=%s expected=%s", remote.exists, remote.enabled, remote.dav != "", remote.davWrites, str(view.AgentSpaceId), spaceID)
	}
}

func TestVerifiedContinueCanReplaceExpiredManagementCredential(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	remote.failDAV = true
	op := runCommand(t, s, actor, user, "CREATE")
	candidate := remote.dav
	remote.failDAV = false
	remote.managementToken = "replacement-management"
	secret, err := s.Secrets.CreateSecret(ctx, actor, "replacement", remote.managementToken)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := s.Projection(ctx, user)
	evidence := "Original request ended; same remote deployment verified; management credential rotated"
	_, err = s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "CONTINUE", ExpectedRevision: view.BindingRevision, Evidence: &evidence, ManagementSecret: &adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	result, _ := s.Operation(ctx, op.OperationId)
	if result.State != "COMPLETED" || remote.dav != candidate {
		t.Fatalf("credential recovery lost original operation/candidate: %+v", result)
	}
}

func TestProvisionFilesBeforeOptionalMCPPublication(t *testing.T) {
	s, _, actor, user := lifecycleFixture(t)
	ctx := context.Background()
	managed := s.Client.ManagedState.GetX(ctx, "current")
	s.Client.ManagedState.UpdateOneID("current").ClearActiveReleaseID().SetActiveManagedGeneration(0).SetDesiredControlRevision(0).ExecX(ctx)
	s.ApplyControl = func(context.Context, string, string) (string, bool, error) { return "", true, nil }
	runCommand(t, s, actor, user, "CREATE")
	view, err := s.Projection(ctx, user)
	if err != nil || view.State != "CONNECTED" || !view.FilesAvailable || view.McpAvailable {
		t.Fatalf("file-only provisioning: %+v %v", view, err)
	}
	// Publishing later supplies the binding in the normal ACKed release, without
	// rotating either credential or requiring another workspace operation.
	s.Client.ManagedState.UpdateOneID("current").SetActiveReleaseID(*managed.ActiveReleaseID).SetActiveManagedGeneration(1).SetDesiredControlRevision(1).ExecX(ctx)
	view, err = s.Projection(ctx, user)
	if err != nil || !view.McpAvailable || !view.FilesAvailable {
		t.Fatalf("later MCP publication: %+v %v", view, err)
	}
}
