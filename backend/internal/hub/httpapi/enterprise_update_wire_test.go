package httpapi

import (
	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/wire/adminapi"
	"testing"
	"time"
)

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
	wire := enterpriseUpdateWire(view)
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
	if wire.PublishedAt == nil || *wire.PublishedAt != pubAt {
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
	wire := enterpriseUpdateWire(view)
	if wire.PublishedAt != nil {
		t.Fatalf("expected nil publishedAt for DRAFT, got %v", *wire.PublishedAt)
	}
	if wire.Status != adminapi.EnterpriseUpdateStatus("DRAFT") {
		t.Fatalf("expected DRAFT status, got %s", wire.Status)
	}
}
