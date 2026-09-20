package usage

import (
	"testing"

	"measix/platform/internal/hub/budget"
)

func TestUserBudgetFilterUsesCrossCapabilityPrecedenceAndExactNearLimitThreshold(t *testing.T) {
	limited := func(used int64) budget.EffectiveState {
		return budget.EffectiveState{
			Budget: budget.BudgetView{Mode: budget.ModeLimited},
			Status: budget.StatusAvailable,
			Limits: []budget.EffectiveLimitState{{Limit: 100, Used: used}},
		}
	}
	tests := []struct {
		name   string
		states []budget.EffectiveState
		filter UserBudgetFilter
		want   bool
	}{
		{"below threshold", []budget.EffectiveState{limited(79)}, UserBudgetNearLimit, false},
		{"at threshold", []budget.EffectiveState{limited(80)}, UserBudgetNearLimit, true},
		{"reserved reaches threshold", []budget.EffectiveState{{Budget: budget.BudgetView{Mode: budget.ModeLimited}, Status: budget.StatusAvailable, Limits: []budget.EffectiveLimitState{{Limit: 100, Used: 70, Reserved: 10}}}}, UserBudgetNearLimit, true},
		{"exhausted wins over near", []budget.EffectiveState{limited(80), {Status: budget.StatusExhausted}}, UserBudgetNearLimit, false},
		{"exhausted selected", []budget.EffectiveState{limited(80), {Status: budget.StatusExhausted}}, UserBudgetExhausted, true},
		{"reconciliation wins over exhausted", []budget.EffectiveState{{Status: budget.StatusExhausted}, {Status: budget.StatusReconciliation}}, UserBudgetReconciliation, true},
		{"reconciliation excludes exhausted", []budget.EffectiveState{{Status: budget.StatusExhausted}, {Status: budget.StatusReconciliation}}, UserBudgetExhausted, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := userBudgetMatches(test.states, test.filter); got != test.want {
				t.Fatalf("userBudgetMatches(%s) = %v, want %v", test.filter, got, test.want)
			}
		})
	}
}
