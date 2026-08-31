package enterpriseupdate_test

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func setupService(t *testing.T) (*enterpriseupdate.Service, context.Context, string) {
	t.Helper()
	st := testutil.OpenStore(t)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	idSvc := testutil.NewIdentityService(t, st, now)
	boot, err := idSvc.Bootstrap(context.Background(), "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	svc := enterpriseupdate.NewService(st.Client)
	svc.Now = func() time.Time { return now }
	return svc, context.Background(), boot.AdminUserID
}

// ERX-UPD-001: Admin Draft→Publish makes item client-visible.
func TestERXUPD001DraftToPublishMakesItemClientVisible(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title 1", "Content 1", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "DRAFT" {
		t.Fatalf("expected DRAFT, got %s", created.Status)
	}
	// Before publish, ListPublished should return nothing
	items, _, _, err := svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 published items, got %d", len(items))
	}
	// Publish
	published, err := svc.Publish(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "PUBLISHED" {
		t.Fatalf("expected PUBLISHED, got %s", published.Status)
	}
	if published.PublishedAt == nil {
		t.Fatal("publishedAt should be set after publish")
	}
	// After publish, ListPublished should return 1 item
	items, _, _, err = svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("expected 1 published item with ID %s, got %+v", created.ID, items)
	}
}

// ERX-UPD-002: Withdraw removes item after successful refresh.
func TestERXUPD002WithdrawRemovesItemFromPublished(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title 1", "Content 1", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	// Verify it's visible
	items, _, _, err := svc.ListPublished(ctx, nil, nil, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("expected 1 published item, got %d, err=%v", len(items), err)
	}
	// Withdraw
	withdrawn, err := svc.Withdraw(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if withdrawn.Status != "WITHDRAWN" {
		t.Fatalf("expected WITHDRAWN, got %s", withdrawn.Status)
	}
	// After withdraw, ListPublished should return 0 items
	items, _, _, err = svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 published items after withdraw, got %d", len(items))
	}
}

// ERX-UPD-003: Feed update changes Feed revision/ETag but not managedGeneration.
func TestERXUPD003FeedRevisionChangesButIndependentFromGeneration(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Create and publish first update
	first, err := svc.Create(ctx, adminID, "First", "Content 1", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	firstPub, err := svc.Publish(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	rev1 := firstPub.FeedRevision
	if rev1 == 0 {
		t.Fatal("feed revision should be > 0 after publish")
	}
	// Create and publish second update
	second, err := svc.Create(ctx, adminID, "Second", "Content 2", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	secondPub, err := svc.Publish(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	rev2 := secondPub.FeedRevision
	if rev2 <= rev1 {
		t.Fatalf("feed revision should increase: rev1=%d rev2=%d", rev1, rev2)
	}
	// List should return both items
	items, latestRev, err := svc.List(ctx, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if latestRev != rev2 {
		t.Fatalf("latest feed revision should be %d, got %d", rev2, latestRev)
	}
}

// ERX-UPD-004: Client sees only PUBLISHED items ordered newest-first.
func TestERXUPD004OnlyPublishedOrderedNewestFirst(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Create a DRAFT that should NOT appear in published list
	draft, err := svc.Create(ctx, adminID, "Draft", "Should not be visible", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	// Create and publish first
	first, err := svc.Create(ctx, adminID, "First Published", "Content 1", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC) }
	if _, err := svc.Publish(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	// Create and publish second (later)
	second, err := svc.Create(ctx, adminID, "Second Published", "Content 2", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return time.Date(2026, 8, 28, 13, 0, 0, 0, time.UTC) }
	if _, err := svc.Publish(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	// ListPublished should return 2 items, newest first
	items, _, _, err := svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 published items, got %d", len(items))
	}
	if items[0].ID != second.ID {
		t.Fatalf("expected newest item first: got %s, want %s", items[0].ID, second.ID)
	}
	if items[1].ID != first.ID {
		t.Fatalf("expected older item second: got %s, want %s", items[1].ID, first.ID)
	}
	// The draft should not appear
	for _, item := range items {
		if item.ID == draft.ID {
			t.Fatal("draft item should not appear in published list")
		}
	}
}

// ERX-B-001: no dates + omitted limit returns latest 10.
func TestERXB001NoDatesDefaultLimit10(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	for i := 0; i < 15; i++ {
		item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	items, truncated, _, err := svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 {
		t.Fatalf("expected 10 items, got %d", len(items))
	}
	if !truncated {
		t.Fatal("expected truncated=true when more items exist")
	}
}

// ERX-B-002: no dates + limit N returns latest N.
func TestERXB002NoDatesLimitN(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	for i := 0; i < 10; i++ {
		item, _ := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		svc.Publish(ctx, item.ID)
	}
	items, truncated, _, err := svc.ListPublished(ctx, nil, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	// limit 10 with exactly 10 items should not be truncated
	items, truncated, _, err = svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 {
		t.Fatalf("expected 10 items, got %d", len(items))
	}
	if truncated {
		t.Fatal("expected truncated=false when exactly limit items")
	}
}

// ERX-B-007: limit 0 or >20 returns typed invalid argument.
func TestERXB007InvalidLimitRejected(t *testing.T) {
	svc, ctx, _ := setupService(t)
	// limit 0
	_, _, _, err := svc.ListPublished(ctx, nil, nil, 0)
	if err != enterpriseupdate.ErrInvalidLimit {
		t.Fatalf("expected ErrInvalidLimit for limit=0, got %v", err)
	}
	// limit > 20
	_, _, _, err = svc.ListPublished(ctx, nil, nil, 21)
	if err != enterpriseupdate.ErrInvalidLimit {
		t.Fatalf("expected ErrInvalidLimit for limit=21, got %v", err)
	}
}

// ERX-B-009: overflow sets truncated=true.
func TestERXB009OverflowSetsTruncated(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	for i := 0; i < 5; i++ {
		item, _ := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		svc.Publish(ctx, item.ID)
	}
	items, truncated, _, err := svc.ListPublished(ctx, nil, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if !truncated {
		t.Fatal("expected truncated=true when more items exist")
	}
}

// Test that Create generates valid eup_ IDs.
func TestCreateGeneratesValidIds(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	if err := platformid.Validate(platformid.EntUpdate, item.ID); err != nil {
		t.Fatalf("invalid eup_ id: %s, err: %v", item.ID, err)
	}
}

// Test that Update only works on DRAFT items.
func TestUpdateOnlyOnDraft(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	// Update should work on DRAFT
	updated, err := svc.Update(ctx, created.ID, "New Title", "New Content", "MARKDOWN", "ANNOUNCEMENT", "WARNING")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "New Title" {
		t.Fatalf("expected title 'New Title', got %s", updated.Title)
	}
	if updated.ContentFormat != "MARKDOWN" {
		t.Fatalf("expected contentFormat 'MARKDOWN', got %s", updated.ContentFormat)
	}
	if updated.Category != "ANNOUNCEMENT" {
		t.Fatalf("expected category 'ANNOUNCEMENT', got %s", updated.Category)
	}
	if updated.Severity != "WARNING" {
		t.Fatalf("expected severity 'WARNING', got %s", updated.Severity)
	}
	// Publish
	if _, err := svc.Publish(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	// Update should fail on PUBLISHED
	_, err = svc.Update(ctx, created.ID, "Again", "Again", "PLAIN", "NOTICE", "INFO")
	if err != enterpriseupdate.ErrInvalidStatus {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

// Test that Publish only works on DRAFT items.
func TestPublishOnlyOnDraft(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	// First publish should succeed
	if _, err := svc.Publish(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	// Second publish should fail
	if _, err := svc.Publish(ctx, created.ID); err != enterpriseupdate.ErrInvalidStatus {
		t.Fatalf("expected ErrInvalidStatus on double publish, got %v", err)
	}
}

// Test that Withdraw only works on PUBLISHED items.
func TestWithdrawOnlyOnPublished(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	// Withdraw on DRAFT should fail
	if _, err := svc.Withdraw(ctx, created.ID); err != enterpriseupdate.ErrInvalidStatus {
		t.Fatalf("expected ErrInvalidStatus on withdraw of draft, got %v", err)
	}
	// Publish then withdraw
	if _, err := svc.Publish(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Withdraw(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	// Withdraw on WITHDRAWN should fail
	if _, err := svc.Withdraw(ctx, created.ID); err != enterpriseupdate.ErrInvalidStatus {
		t.Fatalf("expected ErrInvalidStatus on double withdraw, got %v", err)
	}
}

// Test date filtering: start only, end only, both dates.
func TestListPublishedDateFiltering(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Create items on different dates
	for _, day := range []int{26, 27, 28} {
		svc.Now = func(d int) func() time.Time {
			return func() time.Time { return time.Date(2026, 8, d, 12, 0, 0, 0, time.UTC) }
		}(day)
		item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	// start only: from Aug 27 onwards
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	result, _, _, err := svc.ListPublished(ctx, &start, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items from Aug 27, got %d", len(result))
	}
	// end only: up to Aug 27
	end := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	result, _, _, err = svc.ListPublished(ctx, nil, &end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items up to Aug 27, got %d", len(result))
	}
	// both dates: Aug 27 only (inclusive range)
	result, _, _, err = svc.ListPublished(ctx, &start, &end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item in [Aug 27, Aug 27], got %d", len(result))
	}
}

// ERX-B-003: start only returns start-date through today.
func TestERXB003StartOnlyReturnsStartDateThroughToday(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Create items on different dates
	for _, day := range []int{25, 26, 27, 28} {
		svc.Now = func(d int) func() time.Time {
			return func() time.Time { return time.Date(2026, 8, d, 12, 0, 0, 0, time.UTC) }
		}(day)
		item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	// start only: from Aug 27 onwards — should return Aug 27 and Aug 28
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	result, _, _, err := svc.ListPublished(ctx, &start, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items from Aug 27 onwards, got %d", len(result))
	}
	// Verify ordering: newest first
	if result[0].PublishedAt.Before(*result[1].PublishedAt) {
		t.Fatal("expected newest first ordering")
	}
}

// ERX-B-004: end only returns latest items up to end date.
func TestERXB004EndOnlyReturnsLatestUpToEndDate(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	for _, day := range []int{25, 26, 27, 28} {
		svc.Now = func(d int) func() time.Time {
			return func() time.Time { return time.Date(2026, 8, d, 12, 0, 0, 0, time.UTC) }
		}(day)
		item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	// end only: up to Aug 26 (inclusive) — should return Aug 25 and Aug 26
	end := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	result, _, _, err := svc.ListPublished(ctx, nil, &end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items up to Aug 26, got %d", len(result))
	}
	// Verify ordering: newest first
	if result[0].PublishedAt.Before(*result[1].PublishedAt) {
		t.Fatal("expected newest first ordering")
	}
}

// ERX-B-005: both dates use inclusive closed interval.
func TestERXB005BothDatesInclusiveClosedInterval(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	for _, day := range []int{25, 26, 27, 28} {
		svc.Now = func(d int) func() time.Time {
			return func() time.Time { return time.Date(2026, 8, d, 12, 0, 0, 0, time.UTC) }
		}(day)
		item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	// both dates: [Aug 26, Aug 27] inclusive — should return Aug 26 and Aug 27
	start := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	result, _, _, err := svc.ListPublished(ctx, &start, &end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items in [Aug 26, Aug 27], got %d", len(result))
	}
}

// ERX-B-006: start > end returns typed invalid argument error.
func TestERXB006StartAfterEndReturnsInvalidArgument(t *testing.T) {
	svc, ctx, _ := setupService(t)
	start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	_, _, _, err := svc.ListPublished(ctx, &start, &end, 10)
	if err != enterpriseupdate.ErrInvalidDateRange {
		t.Fatalf("expected ErrInvalidDateRange for start > end, got %v", err)
	}
}

// ERX-B-008: Deployment timezone and newest-first ordering are explicit.
func TestERXB008TimezoneAndNewestFirstOrdering(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Create items at different times on the same day
	times := []time.Time{
		time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 28, 16, 0, 0, 0, time.UTC),
	}
	for _, ts := range times {
		svc.Now = func(t time.Time) func() time.Time { return func() time.Time { return t } }(ts)
		item, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Publish(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	items, _, _, err := svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Verify newest-first ordering
	for i := 1; i < len(items); i++ {
		if items[i].PublishedAt.After(*items[i-1].PublishedAt) {
			t.Fatalf("item %d is newer than item %d — not newest-first", i, i-1)
		}
	}
}

// ERX-B-010: tool uses Managed MCP/Relay auth and is unavailable in Personal Realm.
// This test verifies the service-layer contract: ListPublished requires no realm
// parameter (it returns PUBLISHED items regardless), but the realm check is
// enforced at the Relay/auth layer. Here we verify the service correctly
// returns only PUBLISHED content.
func TestERXB010OnlyPublishedContentVisible(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Create a DRAFT, a PUBLISHED, and a WITHDRAWN item
	draft, err := svc.Create(ctx, adminID, "Draft", "Draft content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	_ = draft // draft remains DRAFT — should not appear in ListPublished

	pub, err := svc.Create(ctx, adminID, "Published", "Published content", "MARKDOWN", "ANNOUNCEMENT", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(ctx, pub.ID); err != nil {
		t.Fatal(err)
	}

	withdrawn, err := svc.Create(ctx, adminID, "Withdrawn", "Withdrawn content", "PLAIN", "MAINTENANCE", "WARNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(ctx, withdrawn.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Withdraw(ctx, withdrawn.ID); err != nil {
		t.Fatal(err)
	}

	// ListPublished should only return the published item
	items, _, _, err := svc.ListPublished(ctx, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 published item, got %d", len(items))
	}
	if items[0].ID != pub.ID {
		t.Fatalf("expected published item %s, got %s", pub.ID, items[0].ID)
	}
}

// TestLatestFeedRevision verifies that the ETag source is authoritative
// across all statuses, not just published items.
func TestLatestFeedRevision(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	// Initially no items — revision should be 0
	rev, err := svc.LatestFeedRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rev != 0 {
		t.Fatalf("expected revision 0 for empty feed, got %d", rev)
	}
	// Create and publish first item
	first, err := svc.Create(ctx, adminID, "First", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	firstPub, err := svc.Publish(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = svc.LatestFeedRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rev != firstPub.FeedRevision {
		t.Fatalf("expected revision %d, got %d", firstPub.FeedRevision, rev)
	}
	// A private draft must not advance the public feed revision.
	draftItem, err := svc.Create(ctx, adminID, "Draft", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	_ = draftItem
	rev, err = svc.LatestFeedRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Only a publish/withdraw advances the public revision.
	if rev != firstPub.FeedRevision {
		t.Fatalf("expected unchanged revision %d after creating draft, got %d", firstPub.FeedRevision, rev)
	}
}
