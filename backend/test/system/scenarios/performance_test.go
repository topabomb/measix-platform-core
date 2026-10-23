//go:build candidate

package scenarios

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

const performanceConcurrency = 100

func TestAdminLoginCandidatePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("performance test requires a real Hub process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	env, err := harness.NewHubEnv(ctx)
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	defer env.Cleanup()
	if err := env.StartHub(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	normalDurations := make([]time.Duration, 5)
	for index := range normalDurations {
		started := time.Now()
		request, requestErr := adminLoginRequest(ctx, env.HubBaseURL, "admin", env.AdminPassword)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if requestErr = expectHTTPStatus(httpClient, request, http.StatusOK); requestErr != nil {
			t.Fatal(requestErr)
		}
		normalDurations[index] = time.Since(started)
		if normalDurations[index] > 2*time.Second {
			t.Fatalf("normal admin login %d took %v, want <=2s", index+1, normalDurations[index])
		}
	}
	sort.Slice(normalDurations, func(i, j int) bool { return normalDurations[i] < normalDurations[j] })
	t.Logf("PERF admin-login-normal samples=%d p50=%v p95=%v", len(normalDurations), normalDurations[2], normalDurations[4])

	before := env.HubProcessMetrics()
	stopSampling := make(chan struct{})
	peakRSS := make(chan int64, 1)
	go func() {
		peak := before.RSSBytes
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if current := env.HubProcessMetrics().RSSBytes; current > peak {
					peak = current
				}
			case <-stopSampling:
				peakRSS <- peak
				return
			}
		}
	}()

	var unauthorized atomic.Int64
	var throttled atomic.Int64
	metrics := runConcurrentLoad(t, "admin-login-invalid-burst", func(index int) error {
		request, requestErr := adminLoginRequest(ctx, env.HubBaseURL, fmt.Sprintf("unknown-%03d", index), "wrong password value")
		if requestErr != nil {
			return requestErr
		}
		response, requestErr := httpClient.Do(request)
		if requestErr != nil {
			return requestErr
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		switch response.StatusCode {
		case http.StatusUnauthorized:
			unauthorized.Add(1)
		case http.StatusTooManyRequests:
			throttled.Add(1)
		default:
			return fmt.Errorf("admin login returned %d", response.StatusCode)
		}
		return nil
	})
	close(stopSampling)
	peak := <-peakRSS
	after := env.HubProcessMetrics()
	if after.RSSBytes > peak {
		peak = after.RSSBytes
	}
	growth := peak - before.RSSBytes
	t.Logf("PERF admin-login-invalid-burst unauthorized=%d throttled=%d hub_rss_before=%d hub_rss_peak=%d hub_rss_growth=%d",
		unauthorized.Load(), throttled.Load(), before.RSSBytes, peak, growth)
	if unauthorized.Load() == 0 || throttled.Load() == 0 {
		t.Fatalf("burst did not exercise both verification and fast throttle: unauthorized=%d throttled=%d", unauthorized.Load(), throttled.Load())
	}
	if metrics.total > 30*time.Second {
		t.Fatalf("admin login burst took %v, want <=30s", metrics.total)
	}
	if before.RSSBytes > 0 && growth > 512*1024*1024 {
		t.Fatalf("admin login burst grew Hub RSS by %d bytes, want <=512 MiB", growth)
	}
}

func adminLoginRequest(ctx context.Context, baseURL, username, password string) (*http.Request, error) {
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/admin/v1/session/login", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

// TestRuntimeRelayHundredEnterpriseUsers exercises the real hot path:
// Android-visible client reads plus Hub budget admission/lifecycle, Relay
// durable spool, transparent streaming transport, and deterministic Adapter.
// Every worker has a distinct enterprise user/session/device and interaction.
func TestRuntimeRelayHundredEnterpriseUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("performance test requires real Hub and Relay processes")
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
	if err := admin.Login(ctx, "admin", env.AdminPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	gp := &goldenPathTest{t: t}
	gp.fullSetup(ctx, admin, ad, env)
	tokens := make([]string, 0, performanceConcurrency)
	firstToken, generation := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, gp.lastEnrollmentCode)
	tokens = append(tokens, firstToken)
	stamp := time.Now().UnixNano()
	for index := 1; index < performanceConcurrency; index++ {
		response, err := admin.Post(ctx, "/api/admin/v1/users", map[string]any{
			"username": fmt.Sprintf("load-%d-%03d", stamp, index), "displayName": fmt.Sprintf("Load User %03d", index), "role": "MEMBER",
		})
		if err != nil {
			t.Fatalf("create load user %d: %v", index, err)
		}
		var user struct {
			UserID string `json:"userId"`
		}
		if err := harness.DecodeJSON(response, &user); err != nil {
			t.Fatalf("decode load user %d: %v", index, err)
		}
		code := gp.createEnrollment(ctx, admin, user.UserID)
		token, userGeneration := gp.exchangeEnrollmentAndBootstrap(ctx, env.HubBaseURL, code)
		if userGeneration != generation {
			t.Fatalf("user %d generation=%d, want %d", index, userGeneration, generation)
		}
		tokens = append(tokens, token)
	}
	ids := gp.getSnapshotResourceIDs(ctx, env.HubBaseURL, firstToken, generation, gp.lastModelID, gp.lastTtsID, gp.lastAsrID, gp.lastMcpID)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 256
	transport.MaxIdleConnsPerHost = 128
	httpClient := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	bootstrapMetrics := runConcurrentLoad(t, "bootstrap", func(index int) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, env.HubBaseURL+"/api/client/v1/bootstrap", nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+tokens[index])
		return expectHTTPStatus(httpClient, request, http.StatusOK)
	})

	stateMetrics := runConcurrentLoad(t, "managed-state", func(index int) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, env.HubBaseURL+"/api/client/v1/managed/state", nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+tokens[index])
		request.Header.Set("X-Measix-Applied-Managed-Generation", fmt.Sprint(generation))
		return expectHTTPStatus(httpClient, request, http.StatusOK)
	})

	snapshotURL := fmt.Sprintf("%s/api/client/v1/managed/snapshots/%d", env.HubBaseURL, generation)
	preflight, _ := http.NewRequestWithContext(ctx, http.MethodGet, snapshotURL, nil)
	preflight.Header.Set("Authorization", "Bearer "+firstToken)
	preflightResponse, err := httpClient.Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, preflightResponse.Body)
	_ = preflightResponse.Body.Close()
	etag := preflightResponse.Header.Get("ETag")
	if preflightResponse.StatusCode != http.StatusOK || etag == "" {
		t.Fatalf("snapshot preflight status=%d etag=%q", preflightResponse.StatusCode, etag)
	}
	snapshotMetrics := runConcurrentLoad(t, "snapshot-304", func(index int) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, snapshotURL, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+tokens[index])
		request.Header.Set("If-None-Match", etag)
		return expectHTTPStatus(httpClient, request, http.StatusNotModified)
	})

	runtimeMetrics := runConcurrentLoad(t, "runtime-stream", func(index int) error {
		runtimeClient := client.New(client.Options{
			RuntimeBaseURL: env.RelayPubBaseURL, AccessToken: tokens[index], ManagedGeneration: generation,
			InteractionID: platformid.New(platformid.Interaction), HTTPClient: httpClient,
		})
		return runtimeClient.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true,"stream_options":{"include_usage":true}}`, func([]byte) {})
	})
	gp.waitUsageRecorded(ctx, admin, performanceConcurrency, 60*time.Second)

	for _, result := range []loadMetrics{bootstrapMetrics, stateMetrics, snapshotMetrics, runtimeMetrics} {
		if result.total > 10*time.Second || result.p95 > 5*time.Second {
			t.Fatalf("%s load exceeded target: total=%v p95=%v", result.name, result.total, result.p95)
		}
	}
}

type loadMetrics struct {
	name                 string
	total, p50, p95, p99 time.Duration
}

func runConcurrentLoad(t *testing.T, name string, operation func(int) error) loadMetrics {
	t.Helper()
	start := make(chan struct{})
	errors := make(chan error, performanceConcurrency)
	durations := make([]time.Duration, performanceConcurrency)
	var group sync.WaitGroup
	group.Add(performanceConcurrency)
	totalStarted := time.Now()
	for index := range performanceConcurrency {
		index := index
		go func() {
			defer group.Done()
			<-start
			started := time.Now()
			err := operation(index)
			durations[index] = time.Since(started)
			errors <- err
		}()
	}
	close(start)
	group.Wait()
	total := time.Since(totalStarted)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	result := loadMetrics{name: name, total: total, p50: durations[49], p95: durations[94], p99: durations[98]}
	t.Logf("PERF %s users=%d total=%v throughput=%.1f req/s p50=%v p95=%v p99=%v",
		name, performanceConcurrency, total, performanceConcurrency/total.Seconds(), result.p50, result.p95, result.p99)
	return result
}

func expectHTTPStatus(client *http.Client, request *http.Request, expected int) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, readErr := io.Copy(io.Discard, response.Body)
	if readErr != nil {
		return readErr
	}
	if response.StatusCode != expected {
		return fmt.Errorf("%s returned %d, want %d", request.URL.Path, response.StatusCode, expected)
	}
	return nil
}
