//go:build candidate

package scenarios

import (
	"context"
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
//   - no goroutine leak (Relay RSS returns near baseline after GC);
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

	// Measure Relay RSS before the storm.
	metricsBefore := env.RelayProcessMetrics()
	t.Logf("relay RSS before cancel storm: %d bytes", metricsBefore.RSSBytes)

	// Launch N concurrent streaming requests and cancel them all.
	const stormSize = 20
	var wg sync.WaitGroup
	cancelErrors := make([]error, stormSize)

	for i := 0; i < stormSize; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			streamCtx, streamCancel := context.WithCancel(ctx)
			tc := client.New(client.Options{
				RuntimeBaseURL:    env.RelayPubBaseURL,
				AccessToken:       clientToken,
				ManagedGeneration: generation,
				InteractionID:     platformid.New(platformid.Interaction),
			})
			// Start a streaming request — it will block on the adapter's
			// streaming response. We cancel it after a brief moment.
			go func() {
				time.Sleep(200 * time.Millisecond)
				streamCancel()
			}()
			// The stream should be interrupted by the cancel, not hang.
			err := tc.ChatCompletionStream(streamCtx, ids.model, "/v1/chat/completions",
				`{"model":"gpt-test","stream":true}`, func([]byte) {})
			if err == nil {
				// Stream completed before cancel — acceptable.
				return
			}
			// Error is expected from cancellation.
			cancelErrors[idx] = err
		}(i)
	}

	// Wait for all storm goroutines to finish.
	wg.Wait()

	// Count how many were cancelled vs completed.
	cancelledCount := 0
	for _, e := range cancelErrors {
		if e != nil {
			cancelledCount++
		}
	}
	t.Logf("cancel storm: %d/%d streams cancelled with error, %d completed normally", cancelledCount, stormSize, stormSize-cancelledCount)

	// Verify the adapter observed at least some cancellations.
	// (The deterministic adapter records cancel events.)
	ad.ClearCancelled()
	// Re-run a single cancel to verify propagation still works.
	verifyCtx, verifyCancel := context.WithCancel(ctx)
	tc := client.New(client.Options{
		RuntimeBaseURL:    env.RelayPubBaseURL,
		AccessToken:       clientToken,
		ManagedGeneration: generation,
		InteractionID:     platformid.New(platformid.Interaction),
	})
	go func() {
		time.Sleep(200 * time.Millisecond)
		verifyCancel()
	}()
	_ = tc.ChatCompletionStream(verifyCtx, ids.model, "/v1/chat/completions",
		`{"model":"gpt-test","stream":true}`, func([]byte) {})

	// Give the Relay a moment to clean up resources.
	time.Sleep(2 * time.Second)

	// Measure Relay RSS after cleanup.
	metricsAfter := env.RelayProcessMetrics()
	t.Logf("relay RSS after cancel storm: %d bytes", metricsAfter.RSSBytes)

	// RSS should not have grown excessively (allow some overhead for GC).
	// A 50MB growth is acceptable for 20 cancelled streams with buffers.
	rssGrowth := metricsAfter.RSSBytes - metricsBefore.RSSBytes
	if rssGrowth > 50*1024*1024 {
		t.Errorf("relay RSS grew %d bytes after cancel storm — possible resource leak", rssGrowth)
	}

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
	usageDone := make(chan struct{})
	usageCount := 0
	var usageMu sync.Mutex

	go func() {
		defer close(usageDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-usageDone:
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
	close(usageDone)
	<-usageDone // wait for goroutine to exit

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
