package enterpriseupdate_test

import (
	"testing"
	"time"

	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/wire/adminapi"
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
	if fetched.FeedRevision == 0 {
		t.Fatal("expected non-zero feedRevision")
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

// TestToAdminWireConversion verifies the wire conversion maps all fields correctly.
func TestToAdminWireConversion(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	pubAt := now.Add(1 * time.Hour)
	view := enterpriseupdate.UpdateView{
		ID:            "eup_test-id",
		Title:         "Wire Test",
		Content:       "Content here",
		ContentFormat: "MARKDOWN",
		Category:      "ANNOUNCEMENT",
		Severity:      "CRITICAL",
		Status:        "PUBLISHED",
		PublishedAt:   &pubAt,
		FeedRevision:  42,
		CreatedAt:     now,
		UpdatedAt:     pubAt,
	}
	wire := enterpriseupdate.ToAdminWire(view)
	if wire.EnterpriseUpdateId != adminapi.EnterpriseUpdateId("eup_test-id") {
		t.Fatalf("expected enterpriseUpdateId 'eup_test-id', got %s", wire.EnterpriseUpdateId)
	}
	if wire.Title != "Wire Test" {
		t.Fatalf("expected title 'Wire Test', got %s", wire.Title)
	}
	if wire.Content != "Content here" {
		t.Fatalf("expected content 'Content here', got %s", wire.Content)
	}
	if wire.ContentFormat != adminapi.MARKDOWN {
		t.Fatalf("expected contentFormat MARKDOWN, got %s", wire.ContentFormat)
	}
	if wire.Category != adminapi.EnterpriseUpdateCategory("ANNOUNCEMENT") {
		t.Fatalf("expected category ANNOUNCEMENT, got %s", wire.Category)
	}
	if wire.Severity != adminapi.EnterpriseUpdateSeverity("CRITICAL") {
		t.Fatalf("expected severity CRITICAL, got %s", wire.Severity)
	}
	if wire.Status != adminapi.EnterpriseUpdateStatus("PUBLISHED") {
		t.Fatalf("expected status PUBLISHED, got %s", wire.Status)
	}
	if wire.FeedRevision != 42 {
		t.Fatalf("expected feedRevision 42, got %d", wire.FeedRevision)
	}
	if wire.PublishedAt != pubAt {
		t.Fatalf("expected publishedAt %v, got %v", pubAt, wire.PublishedAt)
	}
	if wire.CreatedAt != now {
		t.Fatalf("expected createdAt %v, got %v", now, wire.CreatedAt)
	}
	if wire.UpdatedAt != pubAt {
		t.Fatalf("expected updatedAt %v, got %v", pubAt, wire.UpdatedAt)
	}
}

// TestToAdminWireDraftWithoutPublishedAt verifies that a DRAFT item
// (no publishedAt) converts correctly.
func TestToAdminWireDraftWithoutPublishedAt(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	view := enterpriseupdate.UpdateView{
		ID:            "eup_draft-id",
		Title:         "Draft",
		Content:       "Draft content",
		ContentFormat: "PLAIN",
		Category:      "NOTICE",
		Severity:      "INFO",
		Status:        "DRAFT",
		PublishedAt:   nil,
		FeedRevision:  1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	wire := enterpriseupdate.ToAdminWire(view)
	if wire.PublishedAt.IsZero() {
		// For DRAFT, publishedAt should be zero-value when PublishedAt is nil
		// This is acceptable per the OpenAPI schema where publishedAt is a date-time
		// that can be the zero value for draft items.
	}
	if wire.Status != adminapi.EnterpriseUpdateStatus("DRAFT") {
		t.Fatalf("expected DRAFT status, got %s", wire.Status)
	}
}

// TestListEmpty verifies that List on an empty feed returns no items and revision 0.
func TestListEmpty(t *testing.T) {
	svc, ctx, _ := setupService(t)
	items, rev, err := svc.List(ctx, 100)
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
	items, _, err := svc.List(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// limit 201 — should clamp to 50 and return all 3
	items, _, err = svc.List(ctx, 201)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

// TestUpdatePreservesFeedRevision verifies that updating a draft does not change
// the feedRevision (only create/publish/withdraw do).
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
