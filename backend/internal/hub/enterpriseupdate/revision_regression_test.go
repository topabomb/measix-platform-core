package enterpriseupdate_test

import "testing"

// ERX-UPD-007: invisible draft authoring must not invalidate the published Feed.
func TestDraftCreateDoesNotAdvancePublicFeedRevision(t *testing.T) {
	s, ctx, admin := setupService(t)
	before, err := s.LatestFeedRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, admin, "Draft", "Body", "PLAIN", "NOTICE", "INFO"); err != nil {
		t.Fatal(err)
	}
	after, err := s.LatestFeedRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("draft changed public revision: %d -> %d", before, after)
	}
}
