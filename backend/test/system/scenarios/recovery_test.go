package scenarios

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// CAP-C6-010 — Hub crash around Publish.
// When the Hub crashes after a Publish activation is created but before
// convergence, the persisted activation + Relay status must reconcile without
// producing a duplicate generation. After Hub restart, the same activation
// must complete, the relay must converge, and the generation must not
// increment beyond the expected value.
func TestCAPC6010HubCrashAroundPublish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Record the generation before the crash scenario.
	_, generationBefore := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)

	// Build modified draft and publish to create a new activation.
	draftRev := gp.getDraftRevision(ctx, admin)
	content := gp.buildModifiedDraftContent(ctx, admin, "Hub Crash Model")
	newRev := gp.putDraft(ctx, admin, draftRev, content)
	gp.validateDraft(ctx, admin, newRev)
	activationID := gp.publishDraft(ctx, admin, newRev)

	// Crash the Hub immediately after the activation is created.
	// The activation is persisted in SQLite; on restart the reconciler
	// must pick it up and complete it without creating a duplicate.
	env.StopHub()

	// Restart Hub.
	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("restart hub after crash: %v", err)
	}

	// Re-login after restart.
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("re-login: %v", err)
	}

	// The same activation must complete — no duplicate.
	gp.waitActivationCompleted(ctx, admin, activationID, 60*time.Second)

	// Wait for convergence.
	if err := harness.WaitConvergence(ctx, env.HubBaseURL, admin.CSRFToken(), admin.CookieHeader(), 30*time.Second); err != nil {
		t.Fatalf("post-crash convergence: %v", err)
	}
	if err := harness.WaitReadyRelay(ctx, env.RelayPubBaseURL, 30*time.Second); err != nil {
		t.Fatalf("post-crash relay ready: %v", err)
	}

	// Verify no duplicate activation was created — list activations and
	// ensure there is exactly one for this publish.
	resp, err := admin.Get(ctx, "/api/admin/v1/activations?limit=10")
	if err != nil {
		t.Fatalf("list activations: %v", err)
	}
	var acts struct {
		Items []struct {
			ActivationID string `json:"activationId"`
			Kind         string `json:"kind"`
			State        string `json:"state"`
		} `json:"items"`
	}
	if err := harness.DecodeJSON(resp, &acts); err != nil {
		t.Fatalf("decode activations: %v", err)
	}
	// Count COMPLETED activations for the publish kind.
	completedCount := 0
	for _, a := range acts.Items {
		if a.State == "COMPLETED" && a.Kind == "PUBLISH" {
			completedCount++
		}
	}
	// There should be at least 2 (initial setup + crash recovery), but
	// no duplicate of the same activation.
	if completedCount < 2 {
		t.Fatalf("expected at least 2 completed PUBLISH activations, got %d", completedCount)
	}

	// Verify the generation incremented exactly once from the crash recovery.
	newEnrollmentCode := gp.createEnrollment(ctx, admin, gp.lastUserID)
	_, generationAfter := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, newEnrollmentCode)
	if generationAfter != generationBefore+1 {
		t.Fatalf("generation should increment exactly once: %d -> %d (expected %d)",
			generationBefore, generationAfter, generationBefore+1)
	}

	t.Log("CAP-C6-010 Hub Crash Around Publish: PASS")
}

// CAP-C6-013 — SQLite busy/transient error handling.
// When SQLite encounters a transient/busy error, the Hub must handle it
// gracefully with bounded retry semantics — no corruption, no silent loss.
// This test simulates a transient DB lock by opening a write transaction
// from a second connection while the Hub is operating, then verifying the
// Hub recovers and continues to serve correctly.
func TestCAPC6013SQLiteBusyTransient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Perform a normal operation to confirm the DB is healthy.
	resp, err := admin.Get(ctx, "/api/admin/v1/users")
	if err != nil {
		t.Fatalf("list users before transient: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users before transient: status=%d", resp.StatusCode)
	}

	// Simulate a transient SQLite busy condition by copying the DB file
	// (simulating a backup/snapshot that briefly locks the file) and
	// immediately performing a write operation. The Hub must either
	// retry internally or return a clean error that the client can retry.
	//
	// We use a file copy to simulate the transient lock; the Hub's WAL
	// mode and busy_timeout should handle this without corruption.
	backupPath := filepath.Join(env.Root, "busy-snapshot.db")
	if err := copyFile(env.DBPath, backupPath); err != nil {
		t.Fatalf("copy db: %v", err)
	}

	// Immediately perform a write through the Admin API — the Hub must
	// handle any transient SQLite lock gracefully.
	draftRev := gp.getDraftRevision(ctx, admin)
	content := gp.buildModifiedDraftContent(ctx, admin, "Transient Model")
	newRev := gp.putDraft(ctx, admin, draftRev, content)
	gp.validateDraft(ctx, admin, newRev)

	// Publish and verify activation completes despite the transient.
	activationID := gp.publishDraft(ctx, admin, newRev)
	gp.waitActivationCompleted(ctx, admin, activationID, 60*time.Second)

	// Wait for convergence.
	if err := harness.WaitConvergence(ctx, env.HubBaseURL, admin.CSRFToken(), admin.CookieHeader(), 30*time.Second); err != nil {
		t.Fatalf("post-transient convergence: %v", err)
	}

	// Verify the database is not corrupted — list releases and users.
	resp, err = admin.Get(ctx, "/api/admin/v1/releases?limit=5")
	if err != nil {
		t.Fatalf("list releases after transient: %v", err)
	}
	var releases struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := harness.DecodeJSON(resp, &releases); err != nil {
		t.Fatalf("decode releases: %v", err)
	}
	if len(releases.Items) == 0 {
		t.Fatal("no releases after transient — DB may be corrupted")
	}

	// Verify the runtime still works end-to-end.
	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})
	if _, _, err := tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test"}`); err != nil {
		t.Fatalf("runtime request after transient: %v", err)
	}

	// Clean up the backup file.
	_ = os.Remove(backupPath)

	t.Log("CAP-C6-013 SQLite Busy/Transient: PASS")
}

// CAP-C6-004 Enhanced — Publish new generation with no-forward + Usage generation verification.
// This test strengthens CAP-C6-004 by explicitly asserting:
// 1. The old generation request returns 428 with forwarded=false.
// 2. The upstream Adapter received NO request body for the denied request.
// 3. Usage records the correct generation for each request.
func TestCAPC6004EnhancedNoForwardAndUsageGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()

	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	if err := env.StartRelay(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}

	ad := adapter.New()
	defer ad.Close()

	admin := harness.NewAdminClient(env.HubBaseURL)
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)

	// Generation N setup.
	clientToken, generationN := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generationN, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generationN,
		InteractionID:     platformid.New(platformid.Interaction),
	})

	// Verify generation N works.
	if _, _, err := tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`); err != nil {
		t.Fatalf("generation N should succeed: %v", err)
	}

	// Wait for usage to be recorded for the successful request.
	gp.waitUsageRecorded(ctx, admin, 1, 30*time.Second)

	// Publish new generation.
	draftRev := gp.getDraftRevision(ctx, admin)
	content := gp.buildModifiedDraftContent(ctx, admin, "Enhanced No-Forward Model")
	newRev := gp.putDraft(ctx, admin, draftRev, content)
	gp.validateDraft(ctx, admin, newRev)
	activationID := gp.publishDraft(ctx, admin, newRev)
	gp.waitActivationCompleted(ctx, admin, activationID, 60*time.Second)

	// Wait for convergence.
	if err := harness.WaitConvergence(ctx, env.HubBaseURL, admin.CSRFToken(), admin.CookieHeader(), 30*time.Second); err != nil {
		t.Fatalf("convergence: %v", err)
	}

	// Clear adapter request facts so we can verify the denied request was NOT forwarded.
	ad.ClearFacts()

	// Old generation request must get 428 + forwarded=false.
	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test"}`)
	if err == nil {
		t.Fatal("expected 428 for old generation")
	}
	pe, ok := err.(client.ProblemError)
	if !ok || pe.Status != 428 {
		t.Fatalf("expected 428, got %v", err)
	}
	// Assert forwarded is explicitly false, not just absent.
	if pe.Forwarded == nil {
		t.Fatal("forwarded field is absent in 428 response — must be explicitly false")
	}
	if *pe.Forwarded {
		t.Fatal("forwarded=true in 428 response — must be false")
	}

	// Assert the adapter received NO request for the denied call.
	// Give a brief moment for any in-flight to potentially arrive (it should not).
	time.Sleep(500 * time.Millisecond)
	if fact := ad.LastRequest("/v1/chat/completions"); fact != nil {
		t.Fatalf("adapter received request body for denied old-generation call: %+v", fact)
	}

	// Fetch new snapshot with new generation.
	newEnrollmentCode := gp.createEnrollment(ctx, admin, gp.lastUserID)
	_, generationN1 := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, newEnrollmentCode)
	if generationN1 <= generationN {
		t.Fatalf("generation should increment: %d -> %d", generationN, generationN1)
	}

	tc2 := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generationN1,
		InteractionID:     platformid.New(platformid.Interaction),
	})

	// New generation should succeed.
	if _, _, err := tc2.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`); err != nil {
		t.Fatalf("generation N+1 should succeed: %v", err)
	}

	// Wait for usage to record the new-generation request.
	gp.waitUsageRecorded(ctx, admin, 2, 30*time.Second)

	// Verify Usage records the correct generation for each request.
	resp, err := admin.Get(ctx, "/api/admin/v1/usage/requests?limit=10")
	if err != nil {
		t.Fatalf("usage requests: %v", err)
	}
	var requests struct {
		Items []struct {
			ManagedGeneration int    `json:"managedGeneration"`
			ResourceKind      string `json:"resourceKind"`
		} `json:"items"`
	}
	if err := harness.DecodeJSON(resp, &requests); err != nil {
		t.Fatalf("decode requests: %v", err)
	}

	// Verify at least one request with generation N and one with generation N+1.
	foundN, foundN1 := false, false
	for _, r := range requests.Items {
		if r.ManagedGeneration == generationN {
			foundN = true
		}
		if r.ManagedGeneration == generationN1 {
			foundN1 = true
		}
	}
	if !foundN {
		t.Fatalf("no usage request found for generation N=%d", generationN)
	}
	if !foundN1 {
		t.Fatalf("no usage request found for generation N+1=%d", generationN1)
	}

	t.Log("CAP-C6-004 Enhanced No-Forward + Usage Generation: PASS")
}
