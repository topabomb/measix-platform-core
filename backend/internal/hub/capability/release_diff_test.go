package capability

import (
	"testing"

	"measix/platform/internal/wire/adminapi"
)

func TestReleaseContentDiffCountsAddedChangedRemovedByKind(t *testing.T) {
	prev := &adminapi.ManagedDraftContent{
		Providers: []adminapi.ProviderDefinition{{ProviderId: "prv_a", DisplayName: "A"}},
		Models:    []adminapi.ModelDefinition{{ModelId: "mdl_a", DisplayName: "A"}, {ModelId: "mdl_b", DisplayName: "B"}},
	}
	cur := &adminapi.ManagedDraftContent{
		Providers: []adminapi.ProviderDefinition{{ProviderId: "prv_a", DisplayName: "A"}, {ProviderId: "prv_b", DisplayName: "B"}},
		Models: []adminapi.ModelDefinition{
			{ModelId: "mdl_a", DisplayName: "A changed"},
			{ModelId: "mdl_c", DisplayName: "C"},
		},
	}
	diff := releaseContentDiff(cur, prev)
	if diff.Added != 2 { // prv_b + mdl_c
		t.Fatalf("added=%d want 2", diff.Added)
	}
	if diff.Changed != 1 { // mdl_a
		t.Fatalf("changed=%d want 1", diff.Changed)
	}
	if diff.Removed != 1 { // mdl_b
		t.Fatalf("removed=%d want 1", diff.Removed)
	}
	if diff.Details == nil {
		t.Fatal("expected per-kind details")
	}
	var providerDiff, modelDiff *adminapi.ResourceDiff
	for _, d := range *diff.Details {
		switch d.Kind {
		case adminapi.ReleaseDiffKindPROVIDER:
			providerDiff = &d
		case adminapi.ReleaseDiffKindMODEL:
			modelDiff = &d
		}
	}
	if providerDiff == nil || providerDiff.Added != 1 {
		t.Fatalf("provider detail=%+v want added=1", providerDiff)
	}
	if modelDiff == nil || modelDiff.Changed != 1 || modelDiff.Added != 1 || modelDiff.Removed != 1 {
		t.Fatalf("model detail=%+v want changed=1 added=1 removed=1", modelDiff)
	}
}

func TestReleaseContentDiffIdenticalContentIsNoop(t *testing.T) {
	content := &adminapi.ManagedDraftContent{
		Providers: []adminapi.ProviderDefinition{{ProviderId: "prv_a", DisplayName: "A"}},
	}
	diff := releaseContentDiff(content, content)
	if diff.Added != 0 || diff.Changed != 0 || diff.Removed != 0 {
		t.Fatalf("identical content produced diff=%+v", diff)
	}
	if diff.Details != nil {
		t.Fatalf("identical content should not emit details, got %+v", *diff.Details)
	}
}
