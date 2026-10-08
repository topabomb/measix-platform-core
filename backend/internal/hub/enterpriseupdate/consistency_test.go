package enterpriseupdate_test

import (
	"sync"
	"testing"
	"time"
)

func TestConcurrentPublishesAdvanceDistinctRevisions(t *testing.T) {
	s, ctx, admin := setupService(t)
	ids := make([]string, 8)
	for i := range ids {
		row, err := s.Create(ctx, admin, "Title", "Body", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = row.ID
	}
	var wg sync.WaitGroup
	revisions := make([]int64, len(ids))
	errs := make([]error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			row, err := s.Publish(ctx, id)
			revisions[i], errs[i] = row.FeedRevision, err
		}(i, id)
	}
	wg.Wait()
	seen := map[int64]bool{}
	for i, rev := range revisions {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if rev < 1 || seen[rev] {
			t.Fatalf("duplicate/invalid revision %d", rev)
		}
		seen[rev] = true
	}
}

func TestFeedTimezoneDSTAndQueryETag(t *testing.T) {
	s, ctx, admin := setupService(t)
	d, _ := s.Client.Deployment.Query().Only(ctx)
	if _, err := s.Client.Deployment.UpdateOneID(d.ID).SetTimezone("America/New_York").Save(ctx); err != nil {
		t.Fatal(err)
	}
	for _, at := range []string{"2026-03-08T05:00:00Z", "2026-03-09T03:30:00Z", "2026-03-09T04:30:00Z"} {
		now, _ := time.Parse(time.RFC3339, at)
		s.Now = func() time.Time { return now }
		row, err := s.Create(ctx, admin, "Title", "Body", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Publish(ctx, row.ID); err != nil {
			t.Fatal(err)
		}
	}
	day, _ := time.Parse("2006-01-02", "2026-03-08")
	items, _, meta, err := s.ListPublished(ctx, &day, &day, 10)
	if err != nil || len(items) != 2 || meta.Timezone != "America/New_York" {
		t.Fatalf("DST calendar query: count=%d timezone=%s err=%v", len(items), meta.Timezone, err)
	}
	_, _, limited, err := s.ListPublished(ctx, &day, &day, 1)
	if err != nil || limited.ETag == meta.ETag {
		t.Fatalf("limit not included in ETag: %v", err)
	}
	now, _ := time.Parse(time.RFC3339, "2026-03-10T03:59:00Z")
	s.Now = func() time.Time { return now }
	_, _, before, err := s.ListPublished(ctx, &day, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	_, _, after, err := s.ListPublished(ctx, &day, nil, 10)
	if err != nil || before.ETag == after.ETag {
		t.Fatalf("start-only query failed to re-evaluate midnight: %v", err)
	}
}
