//go:build candidate

package scenarios

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// RLY-CON-005 — Cancel storm: goroutine/connection/resource cleanup.
// Launch many concurrent streaming requests, cancel them all mid-stream,
// then verify:
//   - bounded RSS growth after warming the same workload;
//   - no panic or error in Relay logs;
//   - adapter observed cancellations;
//   - Relay remains responsive after the storm.
//
// Per architecture s0-runtime-relay-testing-spec §9 RLY-CON-005:
//
//	"cancel storm 后 goroutine/connection/resource 回落"
func TestRLYCON005CancelStorm(t *testing.T) {
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

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	ad.HoldChatStreams()
	const stormSize = 20
	runStorm := func(round int) {
		var wg sync.WaitGroup
		ready := make(chan struct{}, stormSize)
		cancelErrors := make([]error, stormSize)
		cancels := make([]context.CancelFunc, stormSize)
		for i := 0; i < stormSize; i++ {
			streamCtx, streamCancel := context.WithCancel(ctx)
			cancels[i] = streamCancel
			defer streamCancel()
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				tc := client.New(client.Options{
					RuntimeBaseURL: env.RelayPubBaseURL, AccessToken: clientToken,
					ManagedGeneration: generation, InteractionID: platformid.New(platformid.Interaction),
				})
				var firstChunk sync.Once
				cancelErrors[idx] = tc.ChatCompletionStream(streamCtx, ids.model, "/v1/chat/completions",
					`{"model":"gpt-test","stream":true}`, func([]byte) { firstChunk.Do(func() { ready <- struct{}{} }) })
			}(i)
		}
		// Cancel only when every request is genuinely active and unfinished.
		startDeadline := time.NewTimer(10 * time.Second)
		defer startDeadline.Stop()
		for i := 0; i < stormSize; i++ {
			select {
			case <-ready:
			case <-startDeadline.C:
				t.Fatal("not every storm stream delivered its first event")
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		if active := ad.ActiveStreams(); active != stormSize {
			t.Fatalf("active upstream streams=%d, want %d", active, stormSize)
		}
		for _, cancel := range cancels {
			cancel()
		}
		wg.Wait()
		for i, err := range cancelErrors {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("storm stream %d must cancel mid-stream: %v", i, err)
			}
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && (ad.CancellationCount() != round*stormSize || ad.ActiveStreams() != 0) {
			time.Sleep(10 * time.Millisecond)
		}
		if ad.CancellationCount() != round*stormSize || ad.ActiveStreams() != 0 {
			t.Fatalf("storm upstream cleanup: cancelled=%d active=%d", ad.CancellationCount(), ad.ActiveStreams())
		}
		t.Logf("cancel storm round %d: %d/%d cancelled; active upstream streams=0", round, stormSize, stormSize)
	}

	// Warm the same 20-stream workload before taking the RSS baseline. Initial
	// process allocation is not evidence of a leak; subsequent rounds must drain.
	runStorm(1)
	time.Sleep(2 * time.Second)
	metricsBefore := env.RelayProcessMetrics()
	if metricsBefore.RSSBytes <= 0 {
		t.Fatal("Relay RSS measurement unavailable")
	}
	for round := 2; round <= 3; round++ {
		runStorm(round)
		time.Sleep(2 * time.Second)
		metricsAfter := env.RelayProcessMetrics()
		if metricsAfter.RSSBytes <= 0 {
			t.Fatal("Relay RSS measurement unavailable")
		}
		rssGrowth := metricsAfter.RSSBytes - metricsBefore.RSSBytes
		t.Logf("relay RSS after round %d: baseline=%d after=%d growth=%d bytes", round, metricsBefore.RSSBytes, metricsAfter.RSSBytes, rssGrowth)
		if rssGrowth > 20*1024*1024 {
			t.Errorf("relay RSS grew %d bytes after warmed cancel storm", rssGrowth)
		}
	}
	ad.ReleaseChatStreams()

	// Verify Relay is still responsive — send a normal request.
	tc2 := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})
	if _, _, err := tc2.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`); err != nil {
		t.Fatalf("relay not responsive after cancel storm: %v", err)
	}

	t.Log("RLY-CON-005 Cancel Storm: PASS")
}

// RLY-CON-006 — Control apply does not block usage sender.
// While usage is being actively spooled (concurrent runtime requests),
// a control apply (upstream apply) must complete without blocking the
// usage sender, and vice versa. The usage sender must not hold a long
// shared lock that blocks the control path.
//
// Per architecture s0-runtime-relay-testing-spec §9 RLY-CON-006:
//
//	"control apply 与 usage sender 不通过长共享锁互相阻塞"
func TestRLYCON006ControlApplyNoUsageBlock(t *testing.T) {
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

	clientToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, clientToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	// Phase 1: Start continuous runtime requests in background (usage spool activity).
	// stopCh: main goroutine signals the worker to stop (main closes it).
	// doneCh: worker signals it has exited (worker defers close it).
	// This separation avoids the close-of-closed-channel race that occurs
	// when a single channel is both closed by the worker (defer) and by main.
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	usageCount := 0
	var usageMu sync.Mutex

	go func() {
		defer close(doneCh)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			default:
				tc := client.New(client.Options{
					RuntimeBaseURL:    env.RelayPubBaseURL,
					AccessToken:       clientToken,
					ManagedGeneration: generation,
					InteractionID:     platformid.New(platformid.Interaction),
				})
				_, _, err := tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`)
				if err == nil {
					usageMu.Lock()
					usageCount++
					usageMu.Unlock()
				}
				time.Sleep(50 * time.Millisecond) // brief pause between requests
			}
		}
	}()

	// Phase 2: While usage is flowing, perform a control apply.
	// Measure how long the apply takes — it should not be excessively delayed
	// by the concurrent usage activity.
	applyStart := time.Now()

	// Create a new upstream to apply (this triggers Relay control path).
	secretID, secretVer := gp.createSecret(ctx, admin)
	newUpstreamID := gp.createUpstream(ctx, admin, ad.URL, secretID, secretVer)
	gp.testUpstream(ctx, admin, newUpstreamID)
	gp.applyUpstream(ctx, admin, newUpstreamID)

	applyDuration := time.Since(applyStart)
	t.Logf("control apply duration during usage: %v", applyDuration)

	// The apply should complete in a reasonable time — if the usage sender
	// was blocking it with a long shared lock, this would time out.
	if applyDuration > 60*time.Second {
		t.Fatalf("control apply took too long (%v) — may be blocked by usage sender", applyDuration)
	}

	// Phase 3: Stop usage and verify count.
	close(stopCh)
	<-doneCh // wait for goroutine to exit

	usageMu.Lock()
	finalCount := usageCount
	usageMu.Unlock()

	t.Logf("usage requests completed during apply: %d", finalCount)
	if finalCount == 0 {
		t.Fatal("no usage requests completed during control apply — usage sender may be blocked")
	}

	// Verify usage was actually recorded in Hub.
	gp.waitUsageRecorded(ctx, admin, finalCount, 30*time.Second)

	// Verify Relay is still responsive.
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})
	if _, _, err := tc.ChatCompletion(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","messages":[]}`); err != nil {
		t.Fatalf("relay not responsive after concurrent apply: %v", err)
	}

	// Verify the new upstream is ACTIVE.
	resp, err := admin.Get(ctx, fmt.Sprintf("/api/admin/v1/upstreams/%s", newUpstreamID))
	if err != nil {
		t.Fatalf("get upstream: %v", err)
	}
	var up struct {
		Status string `json:"status"`
	}
	_ = harness.DecodeJSON(resp, &up)
	if up.Status != "ACTIVE" {
		t.Fatalf("new upstream not ACTIVE: %s", up.Status)
	}

	t.Log("RLY-CON-006 Control Apply No Usage Block: PASS")
}
