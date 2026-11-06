package identity_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
)

func TestDAVSessionDeadlineMatchesCommittedSessionAndRefreshReplay(t *testing.T) {
	s, exchange := enrolled(t)
	ctx := context.Background()
	check := func(token string) {
		t.Helper()
		claims, err := s.Signer.Verify(token)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(claims)
		var fields map[string]any
		json.Unmarshal(raw, &fields)
		deadline, ok := fields["sessionValidUntil"].(float64)
		row := s.Client.Session.GetX(ctx, exchange.SessionID)
		if !ok || int64(deadline) != row.ExpiresAt.Unix() {
			t.Fatal("DAV deadline was not signed from persisted session lifetime")
		}
		if !claims.ExpiresAt.Time.Before(row.ExpiresAt) {
			t.Fatal("short access expiry was extended")
		}
	}
	check(exchange.AccessToken)
	now := s.Now().Add(time.Hour)
	s.Now = func() time.Time { return now }
	s.Signer.Now = s.Now
	key := platformid.New(platformid.Idempotency)
	result, err := s.Refresh(ctx, exchange.RefreshToken, key)
	if err != nil {
		t.Fatal(err)
	}
	check(result.AccessToken)
	replay, err := s.Refresh(ctx, exchange.RefreshToken, key)
	if err != nil || replay.AccessToken != result.AccessToken {
		t.Fatal("refresh replay changed signed deadline", err)
	}
}
