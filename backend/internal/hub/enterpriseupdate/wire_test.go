package enterpriseupdate_test

import (
	"testing"

	"measix/platform/internal/hub/enterpriseupdate"
)

// TestGetReturnsCorrectFields verifies that Get returns all fields correctly
// including contentFormat, category, severity.
func TestGetReturnsCorrectFields(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Get Title", "Get Content", "MARKDOWN", "ANNOUNCEMENT", "CRITICAL")
	if err != nil {
		t.Fatal(err)
	}
	fetched, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, fetched.ID)
	}
	if fetched.Title != "Get Title" {
		t.Fatalf("expected title 'Get Title', got %s", fetched.Title)
	}
	if fetched.Content != "Get Content" {
		t.Fatalf("expected content 'Get Content', got %s", fetched.Content)
	}
	if fetched.ContentFormat != "MARKDOWN" {
		t.Fatalf("expected contentFormat 'MARKDOWN', got %s", fetched.ContentFormat)
	}
	if fetched.Category != "ANNOUNCEMENT" {
		t.Fatalf("expected category 'ANNOUNCEMENT', got %s", fetched.Category)
	}
	if fetched.Severity != "CRITICAL" {
		t.Fatalf("expected severity 'CRITICAL', got %s", fetched.Severity)
	}
	if fetched.Status != "DRAFT" {
		t.Fatalf("expected status 'DRAFT', got %s", fetched.Status)
	}
	if fetched.FeedRevision != 0 {
		t.Fatal("draft must not carry public revision")
	}
}

// TestGetNotFound verifies that Get returns ErrNotFound for missing IDs.
func TestGetNotFound(t *testing.T) {
	svc, ctx, _ := setupService(t)
	_, err := svc.Get(ctx, "eup_00000000-0000-4000-8000-000000000000")
	if err != enterpriseupdate.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestListEmpty verifies that List on an empty feed returns no items and revision 0.
func TestListEmpty(t *testing.T) {
	svc, ctx, _ := setupService(t)
	items, rev, err := svc.List(ctx, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
	if rev != 0 {
		t.Fatalf("expected revision 0 for empty feed, got %d", rev)
	}
}

// TestListLimitClamped verifies that List clamps invalid limits to default 50.
func TestListLimitClamped(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
		if err != nil {
			t.Fatal(err)
		}
	}
	// limit 0 — should clamp to 50 and return all 3
	items, _, err := svc.List(ctx, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// limit 201 — should clamp to 50 and return all 3
	items, _, err = svc.List(ctx, 201, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

// TestUpdatePreservesFeedRevision verifies that updating a draft does not change
// the feedRevision (only publish/withdraw do).
func TestUpdatePreservesFeedRevision(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	revBefore := created.FeedRevision
	updated, err := svc.Update(ctx, created.ID, "New Title", "New Content", "MARKDOWN", "ANNOUNCEMENT", "WARNING")
	if err != nil {
		t.Fatal(err)
	}
	if updated.FeedRevision != revBefore {
		t.Fatalf("feedRevision changed on update: before=%d after=%d", revBefore, updated.FeedRevision)
	}
}

// TestPublishWithdrawIncreaseFeedRevision verifies that each state transition
// (publish and withdraw) increases the feedRevision.
func TestPublishWithdrawIncreaseFeedRevision(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	created, err := svc.Create(ctx, adminID, "Title", "Content", "PLAIN", "NOTICE", "INFO")
	if err != nil {
		t.Fatal(err)
	}
	revCreate := created.FeedRevision

	published, err := svc.Publish(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if published.FeedRevision <= revCreate {
		t.Fatalf("feedRevision should increase on publish: before=%d after=%d", revCreate, published.FeedRevision)
	}

	withdrawn, err := svc.Withdraw(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if withdrawn.FeedRevision <= published.FeedRevision {
		t.Fatalf("feedRevision should increase on withdraw: before=%d after=%d", published.FeedRevision, withdrawn.FeedRevision)
	}
}

// TestCreateWithAllCategorySeverityCombos verifies all valid enum combinations work.
func TestCreateWithAllCategorySeverityCombos(t *testing.T) {
	svc, ctx, adminID := setupService(t)
	categories := []string{"ANNOUNCEMENT", "MAINTENANCE", "NOTICE"}
	severities := []string{"INFO", "WARNING", "CRITICAL"}
	formats := []string{"PLAIN", "MARKDOWN"}
	for _, fmt := range formats {
		for _, cat := range categories {
			for _, sev := range severities {
				item, err := svc.Create(ctx, adminID, "Title", "Content", fmt, cat, sev)
				if err != nil {
					t.Fatalf("Create(%s, %s, %s) failed: %v", fmt, cat, sev, err)
				}
				if item.ContentFormat != fmt || item.Category != cat || item.Severity != sev {
					t.Fatalf("round-trip mismatch: got fmt=%s cat=%s sev=%s, want fmt=%s cat=%s sev=%s",
						item.ContentFormat, item.Category, item.Severity, fmt, cat, sev)
				}
			}
		}
	}
}
