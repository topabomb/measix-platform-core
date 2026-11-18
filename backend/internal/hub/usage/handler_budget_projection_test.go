package usage

import (
	"testing"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/wire/usageingestapi"
)

func TestWireBudgetDecisionHidesTemplateProvenance(t *testing.T) {
	view := toWireDecision(budget.AdmissionDecision{Source: budget.SourceTemplate})
	if view.Source != usageingestapi.EXPLICIT {
		t.Fatalf("wire source = %s", view.Source)
	}
}
