package identity

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestAdminLoginLimiterBoundsArbitraryKeys(t *testing.T) {
	limiter := newAdminLoginLimiter()
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	for index := 0; index < adminLoginKeyLimit*2; index++ {
		limiter.failure(fmt.Sprintf("user-%d", index), fmt.Sprintf("source-%d", index), now)
	}
	if got := len(limiter.identities); got > adminLoginKeyLimit {
		t.Fatalf("identity limiter keys=%d, want <=%d", got, adminLoginKeyLimit)
	}
	if got := len(limiter.sources); got > adminLoginKeyLimit {
		t.Fatalf("source limiter keys=%d, want <=%d", got, adminLoginKeyLimit)
	}
}

func TestAdminLoginVerificationConcurrencyIsBounded(t *testing.T) {
	limiter := newAdminLoginLimiter()
	const attempts = 100
	results := make(chan bool, attempts)
	release := make(chan struct{})
	var group sync.WaitGroup
	group.Add(attempts)
	for range attempts {
		go func() {
			defer group.Done()
			acquired := limiter.tryBeginVerification()
			results <- acquired
			if !acquired {
				return
			}
			<-release
			limiter.endVerification()
		}()
	}

	acquired := 0
	for range attempts {
		if <-results {
			acquired++
		}
	}
	if acquired != adminPasswordVerificationConcurrency {
		t.Fatalf("concurrent password verifications=%d, want %d", acquired, adminPasswordVerificationConcurrency)
	}
	close(release)
	group.Wait()
}

func TestAdminLoginLimiterWaitScheduleAndRecovery(t *testing.T) {
	limiter := newAdminLoginLimiter()
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	waits := []time.Duration{0, 0, 0, 0, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 5 * time.Minute}
	for index, want := range waits {
		limiter.failure("admin", "source-a", now)
		if got := limiter.retryAfter("admin", "source-b", now); got != want {
			t.Fatalf("failure %d retry=%v, want %v", index+1, got, want)
		}
		if want > 0 {
			now = now.Add(want)
		}
	}

	limiter.success("admin", now)
	if got := limiter.retryAfter("admin", "source-b", now); got != 0 {
		t.Fatalf("successful login left username wait=%v", got)
	}

	limiter.failure("quiet-user", "quiet-source", now)
	now = now.Add(adminLoginQuietReset)
	for range 4 {
		limiter.failure("quiet-user", "quiet-source", now)
	}
	if got := limiter.retryAfter("quiet-user", "other-source", now); got != 0 {
		t.Fatalf("quiet reset retained stale failures: %v", got)
	}
}

func TestAdminLoginSourceWindowSurvivesSuccessAndDoesNotExtendBlock(t *testing.T) {
	limiter := newAdminLoginLimiter()
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	for index := 0; index < adminSourceFailureLimit-1; index++ {
		limiter.failure(fmt.Sprintf("unknown-%d", index), "sprayer", now)
	}
	limiter.success("admin", now)
	limiter.failure("last-unknown", "sprayer", now)
	if got := limiter.retryAfter("admin", "sprayer", now); got != adminSourceBlock {
		t.Fatalf("source block=%v, want %v", got, adminSourceBlock)
	}

	now = now.Add(5 * time.Minute)
	if got := limiter.retryAfter("admin", "sprayer", now); got != 5*time.Minute {
		t.Fatalf("source block was extended or shortened: %v", got)
	}
	if got := limiter.retryAfter("admin", "sprayer", now); got != 5*time.Minute {
		t.Fatalf("repeated blocked request changed deadline: %v", got)
	}
	now = now.Add(5 * time.Minute)
	if got := limiter.retryAfter("admin", "sprayer", now); got != 0 {
		t.Fatalf("source did not recover after window: %v", got)
	}
}
