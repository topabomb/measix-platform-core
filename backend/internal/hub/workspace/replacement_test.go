package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"measix/platform/ent/workspaceoperation"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestChangedOriginsOnlyPrepareConfiguration(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	cfg, _ := s.config(ctx, service.ID, service.ConfigRevision)
	cfg.AdminOrigin, cfg.McpOrigin = "http://127.0.0.1:1", "http://127.0.0.1:1"
	out, err := s.Save(ctx, actor, platformid.New(platformid.Idempotency), service.ID, adminapi.SaveWorkspaceServiceRequest{Name: service.Name, ExpectedRevision: 1, Config: cfg})
	if err != nil || out.ConfigRevision != 2 || *out.ActiveConfigRevision != 1 {
		t.Fatalf("prepare new address: %+v %v", out, err)
	}
	if remote.creates != 1 || s.Client.AgentSpace.GetX(ctx, user).AgentSpaceID != remote.space {
		t.Fatal("saving addresses changed original space")
	}
	if !out.McpPublished || out.DavResourceId == nil || *out.DavResourceId != service.DavResourceID {
		t.Fatal("save response discarded read-only resource metadata", out)
	}
}

func TestApplyPinsConfirmedConfigAndReconcilesOriginalOperation(t *testing.T) {
	s, _, actor, _ := lifecycleFixture(t)
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	if _, err := s.ApplyAtRevision(ctx, actor, platformid.New(platformid.Idempotency), service.ID, 2); err != ErrConflict {
		t.Fatal("applied an unconfirmed revision", err)
	}
	key := platformid.New(platformid.Idempotency)
	op, err := s.ApplyAtRevision(ctx, actor, key, service.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.Client.WorkspaceOperation.UpdateOneID(op.OperationId).SetState("COMPLETED").SaveX(ctx)
	s.Client.WorkspaceService.UpdateOneID(service.ID).SetConfigRevision(2).SaveX(ctx)
	if retry, err := s.ApplyAtRevision(ctx, actor, key, service.ID, 1); err != nil || retry.OperationId != op.OperationId {
		t.Fatal("idempotent retry did not retain its original revision", retry, err)
	}
}

func TestMissingDeploymentCheckFailsClosedOnManagementFailure(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	remote.exists = false
	check, err := s.Check(ctx, service.ID)
	if err != nil || check.ReplacementAvailable == nil || !*check.ReplacementAvailable || check.MissingWorkspaces == nil || len(*check.MissingWorkspaces) != 1 {
		t.Fatalf("missing binding preview: %+v %v", check, err)
	}
	remote.failCheck = true
	check, err = s.Check(ctx, service.ID)
	if err != nil || (check.ReplacementAvailable != nil && *check.ReplacementAvailable) {
		t.Fatalf("management failure offered replacement: %+v %v", check, err)
	}
}

func TestReplacementWaitsForACKAndResumesWithoutDuplicateCreation(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	old := s.Client.AgentSpace.GetX(ctx, user)
	s.Client.AgentSpace.UpdateOneID(user).SetDavAccessMode("READ_ONLY").SetDavBindingRevision(old.DavBindingRevision + 1).SaveX(ctx)
	old = s.Client.AgentSpace.GetX(ctx, user)
	release := s.Client.ManagedRelease.Query().OnlyX(ctx)
	remote.exists, remote.space = false, platformid.New(platformid.AgentSpace)
	request := adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: int(service.ConfigRevision), ConfirmedDestroyedDeployment: true, Bindings: []adminapi.WorkspaceReplacementBinding{{UserId: user, AgentSpaceId: old.AgentSpaceID, BindingRevision: old.BindingRevision, DavBindingRevision: old.DavBindingRevision}}}
	key := platformid.New(platformid.Idempotency)
	ack := s.ApplyControl
	s.ApplyControl = func(context.Context, string, string) (string, bool, error) { return "", false, nil }
	op, err := s.ReplaceDeployment(ctx, actor, key, service.ID, request)
	if err != nil {
		t.Fatalf("replacement rejected: %v", err)
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if remote.creates != 1 || remote.exists {
		t.Fatal("replacement created before old access was acknowledged removed")
	}
	_, bindings, err := s.RuntimeBindings(ctx, service.ID, service.McpServerID)
	if err != nil || len(bindings) != 0 {
		t.Fatal("old binding remained admitted", err)
	}
	resumed := NewService(s.Client, s.Secrets)
	resumed.ApplyControl = ack
	for i := 0; i < 8; i++ {
		if err = resumed.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if remote.creates != 2 || s.Client.WorkspaceOperation.GetX(ctx, op.OperationId).State != "COMPLETED" {
		t.Fatal("replacement did not durably finish")
	}
	current := s.Client.AgentSpace.GetX(ctx, user)
	if current.AgentSpaceID == old.AgentSpaceID || current.State != "CONNECTED" || current.DavAccessMode != "READ_ONLY" {
		t.Fatal("new space identity/permission", current)
	}
	if !bytes.Equal(s.Client.ManagedRelease.GetX(ctx, release.ID).ReleaseContentJSON, release.ReleaseContentJSON) {
		t.Fatal("replacement changed historical release")
	}
	if _, err = resumed.ReplaceDeployment(ctx, actor, key, service.ID, request); err != nil {
		t.Fatal("same request could not be reconciled idempotently", err)
	}
	if s.Client.WorkspaceOperation.Query().Where(workspaceoperation.ActionEQ("CREATE")).CountX(ctx) != 2 || remote.creates != 2 {
		t.Fatal("retry queued another creation")
	}
}

func TestReplacementRejectsChangedDAVIntentAndUnconfirmedDestruction(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	old := s.Client.AgentSpace.GetX(ctx, user)
	remote.exists = false
	request := adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 1, Bindings: []adminapi.WorkspaceReplacementBinding{{UserId: user, AgentSpaceId: old.AgentSpaceID, BindingRevision: old.BindingRevision, DavBindingRevision: old.DavBindingRevision}}}
	if _, err := s.ReplaceDeployment(ctx, actor, platformid.New(platformid.Idempotency), service.ID, request); err != ErrInvalid {
		t.Fatal("missing destruction confirmation", err)
	}
	request.ConfirmedDestroyedDeployment = true
	remote.onMissingGet = func() {
		remote.onMissingGet = nil
		s.Client.AgentSpace.UpdateOneID(user).SetDavBindingRevision(old.DavBindingRevision + 1).SetDavAccessMode("READ_ONLY").SaveX(ctx)
	}
	if _, err := s.ReplaceDeployment(ctx, actor, platformid.New(platformid.Idempotency), service.ID, request); err != ErrConflict {
		t.Fatal("stale DAV preview accepted", err)
	}
	if s.Client.WorkspaceOperation.Query().Where(workspaceoperation.ActionEQ("REPLACE")).CountX(ctx) != 0 || s.Client.AgentSpace.GetX(ctx, user).State != "CONNECTED" {
		t.Fatal("rejected replacement mutated state")
	}
}

func TestReplacementPreservesRevokedFileAccessAndUnknownCreateRecovery(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	old := s.Client.AgentSpace.UpdateOneID(user).SetFileAccessIntent("REVOKED").SetDavConfirmed(false).SaveX(ctx)
	remote.exists = false
	remote.space = platformid.New(platformid.AgentSpace)
	remote.failCreate = true
	request := adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 1, ConfirmedDestroyedDeployment: true, Bindings: []adminapi.WorkspaceReplacementBinding{{UserId: user, AgentSpaceId: old.AgentSpaceID, BindingRevision: old.BindingRevision, DavBindingRevision: old.DavBindingRevision}}}
	op, err := s.ReplaceDeployment(ctx, actor, platformid.New(platformid.Idempotency), service.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := s.Operation(ctx, op.OperationId)
	if err != nil || progress.ReplacementProgress == nil || progress.ReplacementProgress.NeedsAttention != 1 {
		t.Fatal("unknown child not surfaced", progress, err)
	}
	creates := remote.creates
	for i := 0; i < 3; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if remote.creates != creates {
		t.Fatal("unknown create replayed")
	}
	remote.failCreate = false
	child := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.UserIDEQ(user), workspaceoperation.StateEQ("UNKNOWN")).OnlyX(ctx)
	view, _ := s.Projection(ctx, user)
	evidence := "Original request ended; verified created account and space"
	name, space := remote.name, remote.space
	if _, err = s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "TAKEOVER", ExpectedRevision: view.BindingRevision, RemoteUsername: &name, AgentSpaceId: &space, Evidence: &evidence}); err != nil {
		t.Fatal("batch blocked verified recovery of its original child", err)
	}
	// The original child is explicitly superseded and the batch follows that
	// verified takeover. No unknown create is replayed automatically.
	if child.Step != "CREATE_SENT" {
		t.Fatal(child.Step)
	}
	if s.Client.AgentSpace.GetX(ctx, user).FileAccessIntent != "REVOKED" {
		t.Fatal("revoked files restored")
	}
	for i := 0; i < 6; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if s.Client.WorkspaceOperation.GetX(ctx, op.OperationId).State != "COMPLETED" || s.Client.AgentSpace.GetX(ctx, user).State != "CONNECTED" || remote.creates != creates {
		t.Fatal("verified recovery did not finish the same replacement")
	}
}

func TestReplacementBatchSkipsDisabledAndDisconnectedUsers(t *testing.T) {
	s, original, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	now := time.Now()
	for i := 0; i < 4; i++ {
		id := platformid.New(platformid.User)
		status, intent := "ACTIVE", "CONNECTED"
		if i == 2 {
			status = "DISABLED"
		}
		if i == 3 {
			intent = "DISCONNECTED"
		}
		s.Client.User.Create().SetID(id).SetUsername(id).SetDisplayName(id).SetRole("USER").SetStatus(status).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
		name, _ := remoteapi.Username(id)
		s.Client.AgentSpace.Create().SetID(id).SetWorkspaceServiceID(service.ID).SetRemoteUsername(name).SetAgentSpaceID(platformid.New(platformid.AgentSpace)).SetBindingRevision(7).SetDavBindingRevision(8).SetDavAccessMode("READ_ONLY").SetFileAccessIntent("REVOKED").SetIntent(intent).SetState("DISCONNECTED").SetRemoteActive(false).SetStopPending(false).SetDavConfirmed(false).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	}
	accounts := map[string]*lifecycleRemote{}
	var origin string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/v1/status" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ready":true}`))
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/admin/v1/users/")
		name = strings.Split(name, "/")[0]
		if r.Method == "POST" && r.URL.Path == "/admin/v1/users" {
			var in struct {
				Username string `json:"username"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			name = in.Username
			if accounts[name] != nil {
				w.WriteHeader(409)
				return
			}
			accounts[name] = &lifecycleRemote{url: origin, name: name, space: platformid.New(platformid.AgentSpace)}
			r.Body = io.NopCloser(strings.NewReader(`{"username":"` + name + `"}`))
		}
		if accounts[name] == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"not_found"}`))
			return
		}
		accounts[name].ServeHTTP(w, r)
	}))
	defer remote.Close()
	origin = remote.URL
	cfg, _ := s.config(ctx, service.ID, 1)
	cfg.AdminOrigin, cfg.McpOrigin, cfg.DavOrigin = origin, origin, &origin
	if _, err := s.Save(ctx, actor, platformid.New(platformid.Idempotency), service.ID, adminapi.SaveWorkspaceServiceRequest{Name: service.Name, ExpectedRevision: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	old := s.Client.AgentSpace.GetX(ctx, user)
	accounts[old.RemoteUsername] = &lifecycleRemote{url: origin, name: old.RemoteUsername, space: old.AgentSpaceID, exists: true, enabled: true}
	partial, err := s.Check(ctx, service.ID)
	if err != nil || *partial.ReplacementAvailable || len(*partial.MissingWorkspaces) != 4 {
		t.Fatal("partially missing accounts offered replacement", partial, err)
	}
	accounts[old.RemoteUsername].space = platformid.New(platformid.AgentSpace)
	mismatch, err := s.Check(ctx, service.ID)
	if err != nil || *mismatch.ReplacementAvailable || str(mismatch.DiagnosticCode) != "workspace_target_mismatch" {
		t.Fatal("identity mismatch offered replacement", mismatch, err)
	}
	delete(accounts, old.RemoteUsername)
	check, err := s.Check(ctx, service.ID)
	if err != nil || check.ReplacementAvailable == nil || !*check.ReplacementAvailable || len(*check.MissingWorkspaces) != 5 {
		t.Fatal(check, err)
	}
	op, err := s.ReplaceDeployment(ctx, actor, platformid.New(platformid.Idempotency), service.ID, adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 2, Bindings: *check.MissingWorkspaces, ConfirmedDestroyedDeployment: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.Operation(ctx, op.OperationId)
	if err != nil || result.State != "COMPLETED" || result.ReplacementProgress.Total != 3 || result.ReplacementProgress.Completed != 3 || len(accounts) != 3 || original.creates != 1 {
		t.Fatal(result, err, len(accounts))
	}
	for _, row := range s.Client.AgentSpace.Query().AllX(ctx) {
		if row.ID != user && (row.BindingRevision != 9 || row.DavBindingRevision != 10 || row.DavAccessMode != "READ_ONLY" || row.FileAccessIntent != "REVOKED") {
			t.Fatal("lost revisions/permission", row)
		}
	}
	for _, account := range accounts {
		if account.creates != 1 {
			t.Fatal("duplicate create")
		}
	}
}

func TestReplacementWaitsForConcurrentActivationWithoutFailing(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	remote.exists, remote.space = false, platformid.New(platformid.AgentSpace)
	check, err := s.Check(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := s.ReplaceDeployment(ctx, actor, platformid.New(platformid.Idempotency), service.ID, adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 1, Bindings: *check.MissingWorkspaces, ConfirmedDestroyedDeployment: true})
	if err != nil {
		t.Fatal(err)
	}
	ack := s.ApplyControl
	var busyID string
	s.ApplyControl = func(ctx context.Context, actor, operation string) (string, bool, error) {
		id, done, err := ack(ctx, actor, operation)
		if operation == op.OperationId && busyID == "" {
			busyID = platformid.New(platformid.Activation)
			s.Client.Activation.Create().SetID(busyID).SetKind("DEVICE").SetState("APPLYING").SetIdempotencyKey(busyID).SetRequestHash(busyID).SetControlRevision(3).SetBundleHash("test").SetTargetDescriptorJSON([]byte(`{}`)).SetCreatedByUserID(actor).SetCreatedAt(time.Now()).SaveX(ctx)
		}
		return id, done, err
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	waiting := s.Client.WorkspaceOperation.GetX(ctx, op.OperationId)
	if waiting.State != "RUNNING" || waiting.Step != "REMOTE" || remote.creates != 1 {
		t.Fatalf("temporary control contention failed replacement: state=%s step=%s code=%s", waiting.State, waiting.Step, waiting.DiagnosticCode)
	}
	s.Client.Activation.UpdateOneID(busyID).SetState("COMPLETED").ExecX(ctx)
	for i := 0; i < 8; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if s.Client.WorkspaceOperation.GetX(ctx, op.OperationId).State != "COMPLETED" || remote.creates != 2 {
		t.Fatal("replacement did not continue after control contention")
	}
}

func TestConcurrentReplacementRetryReturnsOriginalOperation(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	remote.exists = false
	check, err := s.Check(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 1, Bindings: *check.MissingWorkspaces, ConfirmedDestroyedDeployment: true}
	key := platformid.New(platformid.Idempotency)
	type outcome struct {
		op  adminapi.WorkspaceOperation
		err error
	}
	accepted := make(chan outcome, 1)
	remote.onMissingGet = func() {
		remote.onMissingGet = nil
		op, err := s.ReplaceDeployment(ctx, actor, key, service.ID, input)
		accepted <- outcome{op, err}
	}
	retry, retryErr := s.ReplaceDeployment(ctx, actor, key, service.ID, input)
	first := <-accepted
	if first.err != nil || retryErr != nil || retry.OperationId != first.op.OperationId {
		t.Fatalf("concurrent identical request did not reconcile original: first=%v retry=%v", first.err, retryErr)
	}
	if s.Client.WorkspaceOperation.Query().Where(workspaceoperation.ActionEQ("REPLACE")).CountX(ctx) != 1 {
		t.Fatal("concurrent retry duplicated replacement")
	}
}

func TestReplacementHonorsUserDeletionBeforeRecreation(t *testing.T) {
	s, remote, actor, user := lifecycleFixture(t)
	runCommand(t, s, actor, user, "CREATE")
	ctx := context.Background()
	service := s.Client.WorkspaceService.Query().OnlyX(ctx)
	remote.exists = false
	check, err := s.Check(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := s.ReplaceDeployment(ctx, actor, platformid.New(platformid.Idempotency), service.ID, adminapi.ReplaceWorkspaceDeploymentRequest{ExpectedConfigRevision: 1, Bindings: *check.MissingWorkspaces, ConfirmedDestroyedDeployment: true})
	if err != nil {
		t.Fatal(err)
	}
	ack := s.ApplyControl
	s.ApplyControl = func(context.Context, string, string) (string, bool, error) { return "", false, nil }
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = PreserveDeletionTx(ctx, tx, user, time.Now()); err != nil {
		t.Fatal(err)
	}
	tx.User.DeleteOneID(user).ExecX(ctx)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s.ApplyControl = ack
	for i := 0; i < 10; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.Operation(ctx, op.OperationId)
	if err != nil || result.State != "COMPLETED" || result.ReplacementProgress.Total != 0 || remote.creates != 1 {
		t.Fatal("user deletion stalled or recreated the replacement", result, err)
	}
	// Deletion of the already-destroyed remote returned 404. Its original
	// unknown-write verification remains independent of the replacement.
	cleanup := s.Client.WorkspaceOperation.Query().Where(workspaceoperation.UserIDEQ(user), workspaceoperation.ActionEQ("DELETE")).OnlyX(ctx)
	if cleanup.State != "UNKNOWN" || cleanup.Step != "DELETE_SENT" {
		t.Fatal("replacement lost the original deletion recovery", cleanup.State, cleanup.Step)
	}
	row := s.Client.AgentSpace.GetX(ctx, user)
	evidence := "Confirmed original delete request ended and original account is absent"
	if _, err = s.Command(ctx, actor, platformid.New(platformid.Idempotency), user, adminapi.WorkspaceCommand{Action: "CONTINUE", ExpectedRevision: int(row.BindingRevision), Evidence: &evidence}); err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(ctx); err != nil || s.Client.AgentSpace.Query().CountX(ctx) != 0 || remote.creates != 1 {
		t.Fatal("original identity cleanup could not finish", err)
	}
}
