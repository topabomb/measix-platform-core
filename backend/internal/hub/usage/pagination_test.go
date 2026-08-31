package usage

import (
	"context"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"testing"
	"time"
)

func TestRequestPaginationAndInvalidFilters(t *testing.T) {
	st := testutil.OpenStore(t)
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	userID, up := seedUsageParents(t, st.Client, now)
	s := NewService(st.Client)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		e := validRequestUsageEvent(now, userID, up)
		if _, err := s.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{e}}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListRequests(ctx, Filter{}, 2)
	if err != nil || len(rows) != 2 {
		t.Fatalf("first page: %v %v", rows, err)
	}
	next, err := s.ListRequests(ctx, Filter{After: rows[1].CompletedAt.UTC().Format(time.RFC3339Nano) + "|" + rows[1].RequestID}, 2)
	if err != nil || len(next) != 1 || next[0].RequestID <= rows[1].RequestID {
		t.Fatalf("next page: %v %v", next, err)
	}
	for _, f := range []Filter{{After: "bad"}, {Status: "bogus"}, {Completeness: "KNOWN"}, {ResourceKind: "bogus"}} {
		if _, err := s.ListRequests(ctx, f, 2); err == nil {
			t.Fatalf("invalid filter accepted: %+v", f)
		}
	}
}
