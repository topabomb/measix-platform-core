package identity

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	AdminSessionTTL                      = 12 * time.Hour
	AdminRememberedSessionTTL            = 30 * 24 * time.Hour
	adminLoginQuietReset                 = 30 * time.Minute
	adminSourceWindow                    = 10 * time.Minute
	adminSourceBlock                     = 10 * time.Minute
	adminSourceFailureLimit              = 20
	adminLoginKeyLimit                   = 4096
	adminPasswordVerificationConcurrency = 4
	adminPasswordVerificationBusyRetry   = time.Second
)

var ErrLoginThrottled = fmt.Errorf("admin login throttled")

type AdminLoginOptions struct {
	RememberMe bool
	Source     string
}

type LoginThrottledError struct {
	RetryAfter time.Duration
}

func (e *LoginThrottledError) Error() string {
	return fmt.Sprintf("%s for %s", ErrLoginThrottled, e.RetryAfter)
}

func (e *LoginThrottledError) Unwrap() error { return ErrLoginThrottled }

type adminIdentityAttempts struct {
	Failures     int
	LastFailure  time.Time
	BlockedUntil time.Time
}

type adminSourceAttempts struct {
	Failures     int
	WindowStart  time.Time
	BlockedUntil time.Time
}

// adminLoginLimiter is intentionally process-local: the current Hub is a
// single authority process. Keys are hashed and maps are bounded so arbitrary
// usernames or sources cannot turn the protection itself into a memory sink.
type adminLoginLimiter struct {
	mu                sync.Mutex
	identities        map[string]adminIdentityAttempts
	sources           map[string]adminSourceAttempts
	verificationSlots chan struct{}
	operations        uint64
}

func newAdminLoginLimiter() *adminLoginLimiter {
	return &adminLoginLimiter{
		identities:        make(map[string]adminIdentityAttempts),
		sources:           make(map[string]adminSourceAttempts),
		verificationSlots: make(chan struct{}, adminPasswordVerificationConcurrency),
	}
}

func adminLoginKey(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return string(digest[:])
}

func (l *adminLoginLimiter) identityKey(username string) string {
	key := adminLoginKey(NormalizeUsername(username))
	if _, exists := l.identities[key]; exists {
		return key
	}
	if _, overflowExists := l.identities["identity-overflow"]; !overflowExists && len(l.identities) < adminLoginKeyLimit-1 {
		return key
	}
	return "identity-overflow"
}

func (l *adminLoginLimiter) sourceKey(source string) string {
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	key := adminLoginKey(source)
	if _, exists := l.sources[key]; exists {
		return key
	}
	if _, overflowExists := l.sources["source-overflow"]; !overflowExists && len(l.sources) < adminLoginKeyLimit-1 {
		return key
	}
	return "source-overflow"
}

func (l *adminLoginLimiter) tryBeginVerification() bool {
	select {
	case l.verificationSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *adminLoginLimiter) endVerification() {
	<-l.verificationSlots
}

func (l *adminLoginLimiter) retryAfter(username, source string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maintain(now)
	want := time.Duration(0)
	if state, exists := l.identities[l.identityKey(username)]; exists && now.Before(state.BlockedUntil) {
		want = state.BlockedUntil.Sub(now)
	}
	if state, exists := l.sources[l.sourceKey(source)]; exists && now.Before(state.BlockedUntil) {
		if sourceWait := state.BlockedUntil.Sub(now); sourceWait > want {
			want = sourceWait
		}
	}
	return want
}

func (l *adminLoginLimiter) failure(username, source string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maintain(now)

	identityKey := l.identityKey(username)
	identityState := l.identities[identityKey]
	if identityState.LastFailure.IsZero() || now.Sub(identityState.LastFailure) >= adminLoginQuietReset {
		identityState = adminIdentityAttempts{}
	}
	identityState.Failures++
	identityState.LastFailure = now
	if wait := adminIdentityWait(identityState.Failures); wait > 0 {
		identityState.BlockedUntil = now.Add(wait)
	}
	l.identities[identityKey] = identityState

	sourceKey := l.sourceKey(source)
	sourceState := l.sources[sourceKey]
	if sourceState.WindowStart.IsZero() || now.Sub(sourceState.WindowStart) >= adminSourceWindow {
		sourceState = adminSourceAttempts{WindowStart: now}
	}
	sourceState.Failures++
	if sourceState.Failures >= adminSourceFailureLimit {
		sourceState.BlockedUntil = now.Add(adminSourceBlock)
	}
	l.sources[sourceKey] = sourceState
}

func (l *adminLoginLimiter) success(username string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maintain(now)
	delete(l.identities, l.identityKey(username))
}

func (l *adminLoginLimiter) maintain(now time.Time) {
	l.operations++
	if l.operations%128 != 0 && len(l.identities) < adminLoginKeyLimit && len(l.sources) < adminLoginKeyLimit {
		return
	}
	for key, state := range l.identities {
		if !state.LastFailure.IsZero() && now.Sub(state.LastFailure) >= adminLoginQuietReset && !now.Before(state.BlockedUntil) {
			delete(l.identities, key)
		}
	}
	for key, state := range l.sources {
		if !state.WindowStart.IsZero() && now.Sub(state.WindowStart) >= adminSourceWindow && !now.Before(state.BlockedUntil) {
			delete(l.sources, key)
		}
	}
}

func adminIdentityWait(failures int) time.Duration {
	switch failures {
	case 5:
		return 5 * time.Second
	case 6:
		return 30 * time.Second
	case 7:
		return 2 * time.Minute
	default:
		if failures >= 8 {
			return 5 * time.Minute
		}
		return 0
	}
}
