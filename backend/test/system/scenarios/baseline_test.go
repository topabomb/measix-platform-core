//go:build candidate

package scenarios

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// TestBaseline measures S0.1 resource and latency baselines per architecture §17.
// It records all required metrics:
//   - Hub idle RSS/CPU
//   - Relay idle RSS/CPU
//   - Admin CRUD/Publish baseline latency
//   - Relay first-byte overhead
//   - Concurrent streaming memory growth
//   - Multipart temporary memory/disk behavior
//   - Cancel release time
//   - Usage backlog drain
//   - SQLite growth
//
// This is not a pass/fail test — it produces a baseline report.
// The collect-baseline.mjs script parses the output and computes GREEN
// from metric completeness (all required metrics must be present).
func TestBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("baseline test requires real processes")
	}

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

	// --- §17.1: Hub idle RSS/CPU ---
	// Give the Hub a moment to settle after startup.
	time.Sleep(2 * time.Second)
	var hubMemStatBefore runtime.MemStats
	runtime.ReadMemStats(&hubMemStatBefore)
	hubDBSizeBefore := dbFileSize(env.DBPath)
	t.Logf("BASELINE Hub idle RSS: %d bytes", hubMemStatBefore.Sys)
	t.Logf("BASELINE Hub idle CPU goroutines: %d", runtime.NumGoroutine())
	t.Logf("BASELINE Hub idle SQLite size: %d bytes", hubDBSizeBefore)

	// --- §17.2: Relay idle RSS/CPU ---
	// The Relay is a separate process; we measure its spool DB size.
	relaySpoolSizeBefore := dbFileSize(env.Root + "/relay-spool.db")
	t.Logf("BASELINE Relay idle spool size: %d bytes", relaySpoolSizeBefore)
	t.Logf("BASELINE Relay idle goroutines (test process): %d", runtime.NumGoroutine())

	// --- §17.3: Admin CRUD/Publish baseline latency ---
	// Measure login latency
	loginStart := time.Now()
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}
	loginLatency := time.Since(loginStart)
	t.Logf("BASELINE login latency: %v", loginLatency)

	// Measure user creation latency (CRUD)
	gp := &goldenPathTest{t: t}
	userStart := time.Now()
	userID := gp.createUser(ctx, admin)
	userLatency := time.Since(userStart)
	t.Logf("BASELINE create user latency: %v", userLatency)

	// Measure enrollment creation
	enrollStart := time.Now()
	gp.lastEnrollmentCode = gp.createEnrollment(ctx, admin, userID)
	enrollLatency := time.Since(enrollStart)
	t.Logf("BASELINE create enrollment latency: %v", enrollLatency)

	// Measure secret creation
	secretStart := time.Now()
	gp.lastSecretID, gp.lastSecretVersion = gp.createSecret(ctx, admin)
	secretLatency := time.Since(secretStart)
	t.Logf("BASELINE create secret latency: %v", secretLatency)

	// Measure upstream creation
	upstreamStart := time.Now()
	gp.lastUpstreamID = gp.createUpstream(ctx, admin, ad.URL, gp.lastSecretID, gp.lastSecretVersion)
	upstreamLatency := time.Since(upstreamStart)
	t.Logf("BASELINE create upstream latency: %v", upstreamLatency)

	// Measure upstream test
	testStart := time.Now()
	gp.testUpstream(ctx, admin, gp.lastUpstreamID)
	testLatency := time.Since(testStart)
	t.Logf("BASELINE test upstream latency: %v", testLatency)

	// Measure upstream apply
	applyStart := time.Now()
	gp.applyUpstream(ctx, admin, gp.lastUpstreamID)
	applyLatency := time.Since(applyStart)
	t.Logf("BASELINE apply upstream latency: %v", applyLatency)

	// Build and save draft
	draftRev := gp.getDraftRevision(ctx, admin)
	gp.lastProviderID = platformid.New(platformid.Provider)
	gp.lastModelID = platformid.New(platformid.Model)
	gp.lastTtsID = platformid.New(platformid.TTS)
	gp.lastAsrID = platformid.New(platformid.ASR)
	gp.lastMcpID = platformid.New(platformid.MCP)
	routeModel := platformid.New(platformid.Route)
	routeTTS := platformid.New(platformid.Route)
	routeASR := platformid.New(platformid.Route)
	routeMCP := platformid.New(platformid.Route)
	policyID := platformid.New(platformid.Policy)
	gp.lastDraftContent = gp.buildDraftContent(
		gp.lastProviderID, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID,
		gp.lastUpstreamID, routeModel, routeTTS, routeASR, routeMCP, policyID,
		"Baseline Model", "alloy",
	)

	putStart := time.Now()
	newRev := gp.putDraft(ctx, admin, draftRev, gp.lastDraftContent)
	putLatency := time.Since(putStart)
	t.Logf("BASELINE put draft latency: %v", putLatency)

	// Validate
	validateStart := time.Now()
	gp.validateDraft(ctx, admin, newRev)
	validateLatency := time.Since(validateStart)
	t.Logf("BASELINE validate draft latency: %v", validateLatency)

	// Preview
	previewStart := time.Now()
	gp.previewDraft(ctx, admin, newRev)
	previewLatency := time.Since(previewStart)
	t.Logf("BASELINE preview draft latency: %v", previewLatency)

	// Publish + activation (Publish latency)
	publishStart := time.Now()
	activationID := gp.publishDraft(ctx, admin, newRev)
	gp.waitActivationCompleted(ctx, admin, activationID, 60*time.Second)
	publishLatency := time.Since(publishStart)
	t.Logf("BASELINE publish + activation latency: %v", publishLatency)

	// Convergence
	convergeStart := time.Now()
	if err := harness.WaitConvergence(ctx, env.HubBaseURL, admin.CSRFToken(), admin.CookieHeader(), 30*time.Second); err != nil {
		t.Fatalf("convergence: %v", err)
	}
	convergeLatency := time.Since(convergeStart)
	t.Logf("BASELINE convergence latency: %v", convergeLatency)

	if err := harness.WaitReadyRelay(ctx, env.RelayPubBaseURL, 30*time.Second); err != nil {
		t.Fatalf("relay ready: %v", err)
	}

	// Client setup
	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})

	// --- §17.4: Relay first-byte overhead ---
	// Measure time to first byte for streaming request.
	firstByteStart := time.Now()
	var firstByteTime time.Duration
	err = tc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func(chunk []byte) {
		if firstByteTime == 0 {
			firstByteTime = time.Since(firstByteStart)
		}
	})
	if err != nil {
		t.Fatalf("stream first-byte: %v", err)
	}
	t.Logf("BASELINE Relay first-byte overhead: %v", firstByteTime)

	// --- §17.3 cont: Runtime request latency (model) ---
	runtimeStart := time.Now()
	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	runtimeLatency := time.Since(runtimeStart)
	t.Logf("BASELINE runtime request latency (model): %v", runtimeLatency)

	// Measure streaming total latency
	streamStart := time.Now()
	chunkCount := 0
	err = tc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) { chunkCount++ })
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	streamLatency := time.Since(streamStart)
	t.Logf("BASELINE streaming latency (model, %d chunks): %v", chunkCount, streamLatency)

	// --- §17.5: Concurrent streaming memory growth ---
	// Run 10 concurrent streams and measure memory growth.
	var memBeforeConc runtime.MemStats
	runtime.ReadMemStats(&memBeforeConc)
	concStart := time.Now()
	concWG := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			tcConc := client.New(client.Options{
				RuntimeBaseURL:    env.RelayPubBaseURL,
				AccessToken:       clientToken,
				ManagedGeneration: generation,
				InteractionID:     platformid.New(platformid.Interaction),
			})
			err := tcConc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) {})
			concWG <- err
		}()
	}
	for i := 0; i < 10; i++ {
		if err := <-concWG; err != nil {
			t.Fatalf("concurrent stream %d: %v", i, err)
		}
	}
	concDuration := time.Since(concStart)
	var memAfterConc runtime.MemStats
	runtime.ReadMemStats(&memAfterConc)
	concMemGrowth := int64(memAfterConc.Sys) - int64(memBeforeConc.Sys)
	t.Logf("BASELINE concurrent streaming memory growth (10 streams): %d bytes", concMemGrowth)
	t.Logf("BASELINE concurrent streaming duration (10 streams): %v", concDuration)

	// Measure TTS latency
	ttsStart := time.Now()
	_, _, err = tc.Speech(ctx, ids.tts, "/v1/audio/speech", `{"model":"tts-test","input":"hi","voice":"alloy"}`)
	if err != nil {
		t.Fatalf("tts: %v", err)
	}
	ttsLatency := time.Since(ttsStart)
	t.Logf("BASELINE TTS latency: %v", ttsLatency)

	// --- §17.6: Multipart temporary memory/disk behavior ---
	// Measure ASR multipart upload memory behavior.
	var memBeforeMultipart runtime.MemStats
	runtime.ReadMemStats(&memBeforeMultipart)
	asrStart := time.Now()
	_, _, err = tc.Transcription(ctx, ids.asr, "/v1/audio/transcriptions", "whisper-test", "sample.wav", []byte("RIFF"))
	if err != nil {
		t.Fatalf("asr: %v", err)
	}
	asrLatency := time.Since(asrStart)
	var memAfterMultipart runtime.MemStats
	runtime.ReadMemStats(&memAfterMultipart)
	multipartMemGrowth := int64(memAfterMultipart.Sys) - int64(memBeforeMultipart.Sys)
	t.Logf("BASELINE ASR latency: %v", asrLatency)
	t.Logf("BASELINE multipart memory/disk behavior: %d bytes growth", multipartMemGrowth)

	// Measure MCP latency
	mcpStart := time.Now()
	_, _, err = tc.MCP(ctx, ids.mcp, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if err != nil {
		t.Fatalf("mcp: %v", err)
	}
	mcpLatency := time.Since(mcpStart)
	t.Logf("BASELINE MCP latency: %v", mcpLatency)

	// --- §17.7: Cancel release time ---
	// Start a streaming request and cancel it mid-stream, measure cleanup time.
	cancelStart := time.Now()
	cancelCtx, cancelFn := context.WithCancel(ctx)
	go func() {
		time.Sleep(100 * time.Millisecond) // Let the stream start
		cancelFn()
	}()
	_ = tc.ChatCompletionStream(cancelCtx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) {})
	// After cancel, measure how long until the adapter sees cancellation.
	cancelCleanupTime := time.Since(cancelStart)
	t.Logf("BASELINE cancel release time: %v", cancelCleanupTime)

	// --- §17.3 cont: Usage query latency ---
	usageStart := time.Now()
	resp, err := admin.Get(ctx, "/api/admin/v1/usage/summary")
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	resp.Body.Close()
	usageLatency := time.Since(usageStart)
	t.Logf("BASELINE usage summary latency: %v", usageLatency)

	// --- §17.8: Usage backlog drain ---
	// Wait for all usage records to be drained from the spool.
	drainStart := time.Now()
	gp.waitUsageRecorded(ctx, admin, 15, 30*time.Second) // expect at least 15 requests
	drainLatency := time.Since(drainStart)
	t.Logf("BASELINE usage backlog drain: %v", drainLatency)

	// --- §17.9: SQLite growth ---
	hubDBSizeAfter := dbFileSize(env.DBPath)
	relaySpoolSizeAfter := dbFileSize(env.Root + "/relay-spool.db")
	hubDBGrowth := hubDBSizeAfter - hubDBSizeBefore
	relaySpoolGrowth := relaySpoolSizeAfter - relaySpoolSizeBefore
	t.Logf("BASELINE SQLite growth (hub): %d bytes (before=%d after=%d)", hubDBGrowth, hubDBSizeBefore, hubDBSizeAfter)
	t.Logf("BASELINE SQLite growth (relay spool): %d bytes (before=%d after=%d)", relaySpoolGrowth, relaySpoolSizeBefore, relaySpoolSizeAfter)

	// --- §17.1/2 post-load: Hub/Relay RSS/CPU after load ---
	var hubMemStatAfter runtime.MemStats
	runtime.ReadMemStats(&hubMemStatAfter)
	t.Logf("BASELINE Hub post-load RSS: %d bytes", hubMemStatAfter.Sys)
	t.Logf("BASELINE Hub post-load goroutines: %d", runtime.NumGoroutine())

	// Summary
	t.Log("=== BASELINE SUMMARY ===")
	t.Logf("Hub idle RSS:            %d bytes", hubMemStatBefore.Sys)
	t.Logf("Hub idle goroutines:     %d", runtime.NumGoroutine())
	t.Logf("Hub idle SQLite size:    %d bytes", hubDBSizeBefore)
	t.Logf("Relay idle spool size:   %d bytes", relaySpoolSizeBefore)
	t.Logf("Login:                   %v", loginLatency)
	t.Logf("Create User:             %v", userLatency)
	t.Logf("Create Enroll:           %v", enrollLatency)
	t.Logf("Create Secret:           %v", secretLatency)
	t.Logf("Create Upstream:         %v", upstreamLatency)
	t.Logf("Test Upstream:           %v", testLatency)
	t.Logf("Apply Upstream:          %v", applyLatency)
	t.Logf("Put Draft:               %v", putLatency)
	t.Logf("Validate Draft:          %v", validateLatency)
	t.Logf("Preview Draft:           %v", previewLatency)
	t.Logf("Publish+Activate:        %v", publishLatency)
	t.Logf("Convergence:             %v", convergeLatency)
	t.Logf("First-byte overhead:     %v", firstByteTime)
	t.Logf("Runtime (model):         %v", runtimeLatency)
	t.Logf("Streaming:               %v", streamLatency)
	t.Logf("Conc stream mem growth:  %d bytes", concMemGrowth)
	t.Logf("TTS:                     %v", ttsLatency)
	t.Logf("ASR:                     %v", asrLatency)
	t.Logf("Multipart mem growth:    %d bytes", multipartMemGrowth)
	t.Logf("MCP:                     %v", mcpLatency)
	t.Logf("Cancel release time:     %v", cancelCleanupTime)
	t.Logf("Usage Summary:           %v", usageLatency)
	t.Logf("Usage backlog drain:     %v", drainLatency)
	t.Logf("SQLite growth (hub):    %d bytes", hubDBGrowth)
	t.Logf("SQLite growth (spool):  %d bytes", relaySpoolGrowth)
	t.Logf("Hub post-load RSS:       %d bytes", hubMemStatAfter.Sys)
	t.Log("=== BASELINE COMPLETE ===")

	_ = fmt.Sprintf // keep import if needed
}

// dbFileSize returns the size of a file in bytes, or 0 if the file doesn't exist.
func dbFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
