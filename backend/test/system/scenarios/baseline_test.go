//go:build candidate

package scenarios

import (
	"context"
	"fmt"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// TestBaseline measures S0.1 resource and latency baselines.
// It records timing for key operations: login, CRUD, publish, runtime, convergence.
// This is not a pass/fail test — it produces a baseline report.
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

	// Measure login latency
	loginStart := time.Now()
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}
	loginLatency := time.Since(loginStart)
	t.Logf("BASELINE login latency: %v", loginLatency)

	// Measure user creation latency
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

	// Publish + activation
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

	// Measure runtime request latency (model)
	runtimeStart := time.Now()
	_, _, err = tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	runtimeLatency := time.Since(runtimeStart)
	t.Logf("BASELINE runtime request latency (model): %v", runtimeLatency)

	// Measure streaming latency
	streamStart := time.Now()
	chunkCount := 0
	err = tc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) { chunkCount++ })
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	streamLatency := time.Since(streamStart)
	t.Logf("BASELINE streaming latency (model, %d chunks): %v", chunkCount, streamLatency)

	// Measure TTS latency
	ttsStart := time.Now()
	_, _, err = tc.Speech(ctx, ids.tts, "/v1/audio/speech", `{"input":"hi","voice":"alloy"}`)
	if err != nil {
		t.Fatalf("tts: %v", err)
	}
	ttsLatency := time.Since(ttsStart)
	t.Logf("BASELINE TTS latency: %v", ttsLatency)

	// Measure ASR latency
	asrStart := time.Now()
	_, _, err = tc.Transcription(ctx, ids.asr, "/v1/audio/transcriptions", "whisper-test", "sample.wav", []byte("RIFF"))
	if err != nil {
		t.Fatalf("asr: %v", err)
	}
	asrLatency := time.Since(asrStart)
	t.Logf("BASELINE ASR latency: %v", asrLatency)

	// Measure MCP latency
	mcpStart := time.Now()
	_, _, err = tc.MCP(ctx, ids.mcp, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if err != nil {
		t.Fatalf("mcp: %v", err)
	}
	mcpLatency := time.Since(mcpStart)
	t.Logf("BASELINE MCP latency: %v", mcpLatency)

	// Measure usage query latency
	usageStart := time.Now()
	resp, err := admin.Get(ctx, "/api/admin/v1/usage/summary")
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	resp.Body.Close()
	usageLatency := time.Since(usageStart)
	t.Logf("BASELINE usage summary latency: %v", usageLatency)

	// Summary
	t.Log("=== BASELINE SUMMARY ===")
	t.Logf("Login:           %v", loginLatency)
	t.Logf("Create User:     %v", userLatency)
	t.Logf("Create Enroll:    %v", enrollLatency)
	t.Logf("Create Secret:   %v", secretLatency)
	t.Logf("Create Upstream: %v", upstreamLatency)
	t.Logf("Test Upstream:   %v", testLatency)
	t.Logf("Apply Upstream:  %v", applyLatency)
	t.Logf("Put Draft:       %v", putLatency)
	t.Logf("Validate Draft:  %v", validateLatency)
	t.Logf("Preview Draft:   %v", previewLatency)
	t.Logf("Publish+Activate:%v", publishLatency)
	t.Logf("Convergence:     %v", convergeLatency)
	t.Logf("Runtime (model): %v", runtimeLatency)
	t.Logf("Streaming:       %v", streamLatency)
	t.Logf("TTS:             %v", ttsLatency)
	t.Logf("ASR:             %v", asrLatency)
	t.Logf("MCP:             %v", mcpLatency)
	t.Logf("Usage Summary:   %v", usageLatency)
	t.Log("=== BASELINE COMPLETE ===")

	_ = fmt.Sprintf // keep import if needed
}
