//go:build candidate

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	hubMetricsBefore := env.HubProcessMetrics()
	hubDBSizeBefore := dbFileSize(env.DBPath)
	t.Logf("BASELINE Hub idle RSS: %d bytes", hubMetricsBefore.RSSBytes)
	t.Logf("BASELINE Hub idle CPU: %.2f%%", hubMetricsBefore.CPUPercent)
	t.Logf("BASELINE Hub idle threads: %d", hubMetricsBefore.Threads)
	t.Logf("BASELINE Hub idle SQLite size: %d bytes", hubDBSizeBefore)

	// --- §17.2: Relay idle RSS/CPU ---
	// The Relay is a separate process; we measure its real RSS and spool DB size.
	relayMetricsBefore := env.RelayProcessMetrics()
	relaySpoolSizeBefore := dbFileSize(env.Root + "/relay-spool.db")
	t.Logf("BASELINE Relay idle RSS: %d bytes", relayMetricsBefore.RSSBytes)
	t.Logf("BASELINE Relay idle CPU: %.2f%%", relayMetricsBefore.CPUPercent)
	t.Logf("BASELINE Relay idle threads: %d", relayMetricsBefore.Threads)
	t.Logf("BASELINE Relay idle spool size: %d bytes", relaySpoolSizeBefore)

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
	// Measure Relay overhead by comparing direct Adapter TTFB vs Relay TTFB.
	// Direct adapter call: measure TTFB without Relay in the path.
	directAdapterStart := time.Now()
	var directAdapterTTFB time.Duration
	directReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, ad.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","stream":true}`))
	directReq.Header.Set("Content-Type", "application/json")
	directResp, err := http.DefaultClient.Do(directReq)
	if err != nil {
		t.Fatalf("direct adapter request: %v", err)
	}
	// Read just the first chunk to get TTFB
	buf := make([]byte, 256)
	n, _ := directResp.Body.Read(buf)
	directAdapterTTFB = time.Since(directAdapterStart)
	directResp.Body.Close()
	t.Logf("BASELINE Direct adapter TTFB: %v (%d bytes first chunk)", directAdapterTTFB, n)

	// Relay TTFB: measure time to first byte through Relay.
	relayTTFBStart := time.Now()
	var relayTTFB time.Duration
	err = tc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func(chunk []byte) {
		if relayTTFB == 0 {
			relayTTFB = time.Since(relayTTFBStart)
		}
	})
	if err != nil {
		t.Fatalf("stream first-byte: %v", err)
	}
	relayOverhead := relayTTFB - directAdapterTTFB
	t.Logf("BASELINE Relay first-byte overhead: %v (relay TTFB=%v, direct TTFB=%v)", relayOverhead, relayTTFB, directAdapterTTFB)

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
	// Run 10 concurrent streams and measure real Relay memory growth.
	relayMemBeforeConc := env.RelayProcessMetrics()
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
	relayMemAfterConc := env.RelayProcessMetrics()
	concMemGrowth := relayMemAfterConc.RSSBytes - relayMemBeforeConc.RSSBytes
	t.Logf("BASELINE concurrent streaming memory growth (10 streams): %d bytes (relay RSS before=%d after=%d)", concMemGrowth, relayMemBeforeConc.RSSBytes, relayMemAfterConc.RSSBytes)
	t.Logf("BASELINE concurrent streaming duration (10 streams): %v", concDuration)

	// --- §17.5b: 50-stream concurrent test ---
	// Run 50 concurrent streams to measure memory growth under heavier load.
	relayMemBeforeConc50 := env.RelayProcessMetrics()
	conc50Start := time.Now()
	conc50WG := make(chan error, 50)
	for i := 0; i < 50; i++ {
		go func() {
			tcConc50 := client.New(client.Options{
				RuntimeBaseURL:    env.RelayPubBaseURL,
				AccessToken:       clientToken,
				ManagedGeneration: generation,
				InteractionID:     platformid.New(platformid.Interaction),
			})
			err := tcConc50.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) {})
			conc50WG <- err
		}()
	}
	for i := 0; i < 50; i++ {
		if err := <-conc50WG; err != nil {
			t.Errorf("concurrent stream 50 #%d: %v", i, err)
		}
	}
	conc50Duration := time.Since(conc50Start)
	relayMemAfterConc50 := env.RelayProcessMetrics()
	conc50MemGrowth := relayMemAfterConc50.RSSBytes - relayMemBeforeConc50.RSSBytes
	t.Logf("BASELINE concurrent streaming memory growth (50 streams): %d bytes (relay RSS before=%d after=%d)", conc50MemGrowth, relayMemBeforeConc50.RSSBytes, relayMemAfterConc50.RSSBytes)
	t.Logf("BASELINE concurrent streaming duration (50 streams): %v", conc50Duration)

	// Measure TTS latency
	ttsStart := time.Now()
	_, _, err = tc.Speech(ctx, ids.tts, "/v1/audio/speech", `{"model":"tts-test","input":"hi","voice":"alloy"}`)
	if err != nil {
		t.Fatalf("tts: %v", err)
	}
	ttsLatency := time.Since(ttsStart)
	t.Logf("BASELINE TTS latency: %v", ttsLatency)

	// --- §17.6: Multipart temporary memory/disk behavior ---
	// Measure ASR multipart upload memory behavior with a realistic WAV payload.
	// Use a properly-sized WAV file (44 bytes header + 1KB data) rather than just "RIFF".
	wavData := makeRealisticWAV(44100, 1, 16, 1024) // 1KB of audio data
	relayMemBeforeMultipart := env.RelayProcessMetrics()
	asrStart := time.Now()
	asrResult, _, err := tc.Transcription(ctx, ids.asr, "/v1/audio/transcriptions", "whisper-test", "sample.wav", wavData)
	if err != nil {
		t.Fatalf("asr: %v", err)
	}
	asrLatency := time.Since(asrStart)
	relayMemAfterMultipart := env.RelayProcessMetrics()
	multipartMemGrowth := relayMemAfterMultipart.RSSBytes - relayMemBeforeMultipart.RSSBytes
	t.Logf("BASELINE ASR latency: %v", asrLatency)
	t.Logf("BASELINE multipart memory/disk behavior: %d bytes growth (payload=%d bytes, relay RSS before=%d after=%d)", multipartMemGrowth, len(wavData), relayMemBeforeMultipart.RSSBytes, relayMemAfterMultipart.RSSBytes)
	_ = asrResult

	// --- §17.6b: Large multipart upload ---
	// Test with a 1MB WAV file to verify no excessive buffering.
	largeWAVData := makeRealisticWAV(44100, 1, 16, 1024*1024) // ~1MB of audio data
	relayMemBeforeLargeMultipart := env.RelayProcessMetrics()
	largeMultipartStart := time.Now()
	_, _, err = tc.Transcription(ctx, ids.asr, "/v1/audio/transcriptions", "whisper-test", "large.wav", largeWAVData)
	if err != nil {
		t.Fatalf("large multipart: %v", err)
	}
	largeMultipartLatency := time.Since(largeMultipartStart)
	relayMemAfterLargeMultipart := env.RelayProcessMetrics()
	largeMultipartMemGrowth := relayMemAfterLargeMultipart.RSSBytes - relayMemBeforeLargeMultipart.RSSBytes
	t.Logf("BASELINE large multipart memory/disk behavior: %d bytes growth (payload=%d bytes, relay RSS before=%d after=%d)", largeMultipartMemGrowth, len(largeWAVData), relayMemBeforeLargeMultipart.RSSBytes, relayMemAfterLargeMultipart.RSSBytes)
	t.Logf("BASELINE large multipart latency: %v", largeMultipartLatency)

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
	// Verify the adapter actually observed the cancellation.
	ad.ClearCancelled()
	cancelStart := time.Now()
	cancelCtx, cancelFn := context.WithCancel(ctx)
	go func() {
		time.Sleep(100 * time.Millisecond) // Let the stream start
		cancelFn()
	}()
	_ = tc.ChatCompletionStream(cancelCtx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) {})
	cancelCleanupTime := time.Since(cancelStart)
	t.Logf("BASELINE cancel release time: %v", cancelCleanupTime)

	// Verify the adapter actually observed the client cancellation.
	// This proves cancel propagation through Relay, not just local context cancellation.
	cancelObserved := false
	for i := 0; i < 10; i++ {
		if ad.Cancelled() {
			cancelObserved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("BASELINE cancel adapter observed: %v", cancelObserved)

	// --- §17.3 cont: Usage query latency ---
	usageStart := time.Now()
	resp, err := admin.Get(ctx, "/api/admin/v1/usage/summary")
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	resp.Body.Close()
	usageLatency := time.Since(usageStart)
	t.Logf("BASELINE usage summary latency: %v", usageLatency)

	// --- §17.8: Usage backlog drain (Hub outage → spool → drain) ---
	// Stop the Hub to create a real usage backlog in the Relay spool.
	// Then restart Hub and verify the backlog drains.
	spoolPath := env.Root + "/relay-spool.db"
	spoolBeforeOutage := dbFileSize(spoolPath)

	// Generate usage events while Hub is down (they should spool locally).
	env.StopHub()
	time.Sleep(500 * time.Millisecond) // Let relay detect hub is down

	// Send a few requests — usage events will spool since Hub is unreachable.
	for i := 0; i < 3; i++ {
		_ = tc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) {})
	}
	spoolDuringOutage := dbFileSize(spoolPath)
	t.Logf("BASELINE spool size during outage: %d bytes (before=%d)", spoolDuringOutage, spoolBeforeOutage)

	// Restart Hub and measure drain time.
	drainStart := time.Now()
	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("restart hub for drain: %v", err)
	}
	// Wait for usage records to appear in Hub (proves spool drained).
	gp.waitUsageRecorded(ctx, admin, 15, 60*time.Second) // expect at least 15 requests total
	drainLatency := time.Since(drainStart)
	spoolAfterDrain := dbFileSize(spoolPath)
	t.Logf("BASELINE usage backlog drain: %v (spool after drain=%d bytes)", drainLatency, spoolAfterDrain)
	t.Logf("BASELINE Hub outage spool drain: outage_spool=%d drained_spool=%d", spoolDuringOutage, spoolAfterDrain)

	// --- §17.9: SQLite growth ---
	hubDBSizeAfter := dbFileSize(env.DBPath)
	relaySpoolSizeAfter := dbFileSize(env.Root + "/relay-spool.db")
	hubDBGrowth := hubDBSizeAfter - hubDBSizeBefore
	relaySpoolGrowth := relaySpoolSizeAfter - relaySpoolSizeBefore
	t.Logf("BASELINE SQLite growth (hub): %d bytes (before=%d after=%d)", hubDBGrowth, hubDBSizeBefore, hubDBSizeAfter)
	t.Logf("BASELINE SQLite growth (relay spool): %d bytes (before=%d after=%d)", relaySpoolGrowth, relaySpoolSizeBefore, relaySpoolSizeAfter)

	// --- §17.1/2 post-load: Hub/Relay RSS/CPU after load ---
	hubMetricsAfter := env.HubProcessMetrics()
	relayMetricsAfter := env.RelayProcessMetrics()
	t.Logf("BASELINE Hub post-load RSS: %d bytes", hubMetricsAfter.RSSBytes)
	t.Logf("BASELINE Hub post-load CPU: %.2f%%", hubMetricsAfter.CPUPercent)
	t.Logf("BASELINE Hub post-load threads: %d", hubMetricsAfter.Threads)
	t.Logf("BASELINE Relay post-load RSS: %d bytes", relayMetricsAfter.RSSBytes)
	t.Logf("BASELINE Relay post-load CPU: %.2f%%", relayMetricsAfter.CPUPercent)
	t.Logf("BASELINE Relay post-load threads: %d", relayMetricsAfter.Threads)

	// --- TTS buffering test ---
	// Measure TTS binary streaming behavior and Relay memory impact.
	relayMemBeforeTTS := env.RelayProcessMetrics()
	ttsBufStart := time.Now()
	ttsRespBody, _, err := tc.Speech(ctx, ids.tts, "/v1/audio/speech", `{"model":"tts-test","input":"hello","voice":"alloy"}`)
	if err != nil {
		t.Fatalf("tts buffering: %v", err)
	}
	ttsBufLatency := time.Since(ttsBufStart)
	relayMemAfterTTS := env.RelayProcessMetrics()
	ttsMemGrowth := relayMemAfterTTS.RSSBytes - relayMemBeforeTTS.RSSBytes
	t.Logf("BASELINE TTS buffering: %v (payload=%d bytes, relay RSS growth=%d bytes)", ttsBufLatency, len(ttsRespBody), ttsMemGrowth)

	// Summary — typed JSON metric output for collect-baseline.mjs to parse.
	t.Log("=== BASELINE SUMMARY ===")
	t.Logf("Hub idle RSS:            %d bytes", hubMetricsBefore.RSSBytes)
	t.Logf("Hub idle CPU:            %.2f%%", hubMetricsBefore.CPUPercent)
	t.Logf("Hub idle threads:        %d", hubMetricsBefore.Threads)
	t.Logf("Hub idle SQLite size:    %d bytes", hubDBSizeBefore)
	t.Logf("Relay idle RSS:          %d bytes", relayMetricsBefore.RSSBytes)
	t.Logf("Relay idle CPU:          %.2f%%", relayMetricsBefore.CPUPercent)
	t.Logf("Relay idle threads:      %d", relayMetricsBefore.Threads)
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
	t.Logf("Direct adapter TTFB:     %v", directAdapterTTFB)
	t.Logf("Relay TTFB:              %v", relayTTFB)
	t.Logf("First-byte overhead:     %v", relayOverhead)
	t.Logf("Runtime (model):         %v", runtimeLatency)
	t.Logf("Streaming:               %v", streamLatency)
	t.Logf("Conc stream mem growth:  %d bytes", concMemGrowth)
	t.Logf("TTS:                     %v", ttsLatency)
	t.Logf("TTS buffering:           %v", ttsBufLatency)
	t.Logf("ASR:                     %v", asrLatency)
	t.Logf("Multipart mem growth:    %d bytes", multipartMemGrowth)
	t.Logf("MCP:                     %v", mcpLatency)
	t.Logf("Cancel release time:     %v", cancelCleanupTime)
	t.Logf("Cancel adapter observed: %v", cancelObserved)
	t.Logf("Usage Summary:           %v", usageLatency)
	t.Logf("Usage backlog drain:     %v", drainLatency)
	t.Logf("Hub outage spool drain:  outage=%d drained=%d", spoolDuringOutage, spoolAfterDrain)
	t.Logf("SQLite growth (hub):    %d bytes", hubDBGrowth)
	t.Logf("SQLite growth (spool):  %d bytes", relaySpoolGrowth)
	t.Logf("Hub post-load RSS:       %d bytes", hubMetricsAfter.RSSBytes)
	t.Logf("Hub post-load CPU:       %.2f%%", hubMetricsAfter.CPUPercent)
	t.Logf("Relay post-load RSS:     %d bytes", relayMetricsAfter.RSSBytes)
	t.Logf("Relay post-load CPU:     %.2f%%", relayMetricsAfter.CPUPercent)

	// Output typed JSON metrics for collect-baseline.mjs to parse.
	metricsJSON := map[string]interface{}{
		"hub_idle_rss_bytes":                    hubMetricsBefore.RSSBytes,
		"hub_idle_cpu_percent":                  hubMetricsBefore.CPUPercent,
		"hub_idle_threads":                      hubMetricsBefore.Threads,
		"relay_idle_rss_bytes":                  relayMetricsBefore.RSSBytes,
		"relay_idle_cpu_percent":                relayMetricsBefore.CPUPercent,
		"relay_idle_threads":                    relayMetricsBefore.Threads,
		"relay_idle_spool_size":                 relaySpoolSizeBefore,
		"direct_adapter_ttfb_ms":                directAdapterTTFB.Milliseconds(),
		"relay_ttfb_ms":                         relayTTFB.Milliseconds(),
		"first_byte_overhead_ms":                relayOverhead.Milliseconds(),
		"concurrent_stream_mem_growth_bytes":    concMemGrowth,
		"concurrent_stream_50_mem_growth_bytes": conc50MemGrowth,
		"multipart_mem_growth_bytes":            multipartMemGrowth,
		"large_multipart_mem_growth_bytes":      largeMultipartMemGrowth,
		"tts_buffering_latency_ms":              ttsBufLatency.Milliseconds(),
		"tts_buffering_mem_growth_bytes":        ttsMemGrowth,
		"cancel_release_time_ms":                cancelCleanupTime.Milliseconds(),
		"cancel_adapter_observed":               cancelObserved,
		"hub_outage_spool_during_bytes":         spoolDuringOutage,
		"hub_outage_spool_drained_bytes":        spoolAfterDrain,
		"usage_backlog_drain_ms":                drainLatency.Milliseconds(),
		"sqlite_growth_hub_bytes":               hubDBGrowth,
		"sqlite_growth_spool_bytes":             relaySpoolGrowth,
		"hub_post_load_rss_bytes":               hubMetricsAfter.RSSBytes,
		"relay_post_load_rss_bytes":             relayMetricsAfter.RSSBytes,
	}
	metricsJSONBytes, _ := json.Marshal(metricsJSON)
	t.Logf("BASELINE_JSON_METRICS: %s", string(metricsJSONBytes))
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

// makeRealisticWAV creates a minimal but valid WAV file with the given parameters.
// This is used for ASR multipart testing to ensure a realistic binary payload
// rather than just 4 bytes of "RIFF".
func makeRealisticWAV(sampleRate, channels, bitsPerSample, dataLen int) []byte {
	// RIFF header
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	totalSize := 36 + dataLen
	header[4] = byte(totalSize & 0xFF)
	header[5] = byte((totalSize >> 8) & 0xFF)
	header[6] = byte((totalSize >> 16) & 0xFF)
	header[7] = byte((totalSize >> 24) & 0xFF)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	header[16] = 16 // fmt chunk size
	header[20] = 1  // PCM format
	header[22] = byte(channels)
	header[24] = byte(sampleRate & 0xFF)
	header[25] = byte((sampleRate >> 8) & 0xFF)
	byteRate := sampleRate * channels * bitsPerSample / 8
	header[28] = byte(byteRate & 0xFF)
	header[29] = byte((byteRate >> 8) & 0xFF)
	blockAlign := channels * bitsPerSample / 8
	header[32] = byte(blockAlign)
	header[34] = byte(bitsPerSample)
	copy(header[36:40], []byte("data"))
	header[40] = byte(dataLen & 0xFF)
	header[41] = byte((dataLen >> 8) & 0xFF)
	header[42] = byte((dataLen >> 16) & 0xFF)
	header[43] = byte((dataLen >> 24) & 0xFF)

	result := make([]byte, 44+dataLen)
	copy(result[0:44], header)
	// Fill with silence (zeros) — deterministic
	return result
}
