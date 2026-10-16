package contract_test

import (
	"testing"
)

// Product requirements (§12) require the Release read model to carry the full
// publish provenance: source draft revision, publishedAt/by, diff summary and
// activation history — not just generation/hash/status.
func TestAdminReleaseReadModelCarriesPublishProvenance(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "Release")
	for _, field := range []string{
		"releaseId",
		"managedGeneration",
		"status",
		"snapshotHash",
		"createdAt",
		"sourceDraftRevision",
		"publishedAt",
		"publishedBy",
		"diffSummary",
		"activationHistory",
	} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("Release read model is missing field %q", field)
		}
	}
}

// The diff summary must distinguish Added / Changed / Removed counts by resource
// kind so the Releases page can show a real change summary (not a count dialog).
func TestAdminReleaseDiffSummaryIsStructured(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "Release")
	diff := getProp(t, schema, "diffSummary")
	for _, field := range []string{"added", "changed", "removed"} {
		if _, ok := diff.Properties[field]; !ok {
			t.Fatalf("Release.diffSummary is missing field %q", field)
		}
	}
	// Per-kind breakdown is required for the relationship/change view.
	if _, ok := diff.Properties["details"]; !ok {
		t.Fatal("Release.diffSummary must expose a per-kind 'details' breakdown")
	}
	details := getItems(t, diff.Properties["details"].Value)
	kind := details.Properties["kind"]
	if kind == nil || kind.Value == nil || len(kind.Value.Enum) == 0 {
		t.Fatal("Release.diffSummary.details[].kind must be a frozen enum")
	}
}

// Activation history must expose the per-attempt state transitions so the
// timeline can be rendered without the browser guessing.
func TestAdminReleaseActivationHistoryIsStructured(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "Release")
	hist := getProp(t, schema, "activationHistory")
	item := getItems(t, hist)
	for _, field := range []string{"activationId", "state", "createdAt"} {
		if _, ok := item.Properties[field]; !ok {
			t.Fatalf("Release.activationHistory[] is missing field %q", field)
		}
	}
	state := item.Properties["state"]
	if state == nil || state.Value == nil || len(state.Value.Enum) == 0 {
		t.Fatal("Release.activationHistory[].state must be a frozen enum")
	}
}
