package httpapi

import (
	"testing"
	"time"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/wire/clientapi"
)

func TestClientBudgetProjectionHidesTemplateIdentity(t *testing.T) {
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	view := clientBudgetView("usr_00000000-0000-4000-8000-000000000000", "UTC", []budget.EffectiveState{{
		Budget: budget.BudgetView{Capability: budget.CapabilityImageGeneration, Mode: budget.ModeLimited, Source: budget.SourceTemplate, Revision: 3},
		Status: budget.StatusAvailable, AsOf: now,
	}}, nil)
	if len(view.Items) != 1 || view.Items[0].Source != clientapi.EXPLICIT {
		t.Fatalf("client template source leaked or changed semantics: %+v", view)
	}
}
