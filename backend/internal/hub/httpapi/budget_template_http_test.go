package httpapi_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
)

func TestBudgetTemplateHTTPCreateAssignOverrideClearAndDelete(t *testing.T) {
	_, identityService, _, ctx, _ := setupFullHandler(t)
	budgetService, err := budget.NewService(identityService.Client, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	budgetService.Now = identityService.Now
	usageService := usage.NewService(identityService.Client, budgetService)
	h := httpapi.NewFull(httpapi.Services{Identity: identityService, Budget: budgetService, Usage: usageService})
	adminCookie, csrf := loginAdmin(t, h)
	member, err := identityService.CreateUser(ctx, "template-member", "Template Member", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Cookie": adminCookie, "X-CSRF-Token": csrf}
	response := doJSON(t, h, http.MethodPost, "/api/admin/v1/budget-templates", headers, map[string]any{
		"name": "Standard", "description": "Standard allowance", "reason": "create standard",
		"rules": []any{
			map[string]any{"capability": "MODEL", "mode": "LIMITED", "limits": []any{map[string]any{"meter": "REQUESTS", "period": "DAY", "limit": "10"}}},
			map[string]any{"capability": "IMAGE_GENERATION", "mode": "LIMITED", "limits": []any{map[string]any{"meter": "REQUESTED_IMAGES", "period": "LIFETIME", "limit": "2"}}},
		},
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create template: %d %s", response.Code, response.Body)
	}
	var template adminapi.BudgetTemplate
	decodeJSON(t, response, &template)
	if !strings.HasPrefix(template.BudgetTemplateId, "bgt_") || template.Revision != 1 {
		t.Fatalf("server template identity = %+v", template)
	}

	response = doJSON(t, h, http.MethodPut, "/api/admin/v1/users/"+member.ID+"/budget-template", headers, map[string]any{
		"budgetTemplateId": template.BudgetTemplateId, "expectedAssignmentRevision": 0, "reason": "assign standard",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("assign template: %d %s", response.Code, response.Body)
	}
	var userBudget adminapi.UserBudgetView
	decodeJSON(t, response, &userBudget)
	if userBudget.TemplateAssignment == nil || userBudget.TemplateAssignment.BudgetTemplateId != template.BudgetTemplateId || len(userBudget.Items) != 5 {
		t.Fatalf("assigned budget = %+v", userBudget)
	}
	model := findAdminBudget(t, userBudget, "MODEL")
	if model.Source != adminapi.BudgetSource("TEMPLATE") || model.Limits[0].Limit != "10" {
		t.Fatalf("template model = %+v", model)
	}

	response = doJSON(t, h, http.MethodPut, "/api/admin/v1/users/"+member.ID+"/budgets/MODEL", headers, map[string]any{
		"expectedRevision": model.Revision, "mode": "LIMITED", "limits": []any{map[string]any{"meter": "REQUESTS", "period": "DAY", "limit": "4"}}, "reason": "member override",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("put override: %d %s", response.Code, response.Body)
	}
	decodeJSON(t, response, &model)
	if model.Source != adminapi.BudgetSource("EXPLICIT") {
		t.Fatalf("explicit override = %+v", model)
	}
	path := "/api/admin/v1/users/" + member.ID + "/budgets/MODEL?expectedRevision=" + strconv.Itoa(model.Revision) + "&reason=" + url.QueryEscape("restore template")
	response = doJSON(t, h, http.MethodDelete, path, headers, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("clear override: %d %s", response.Code, response.Body)
	}
	decodeJSON(t, response, &model)
	if model.Source != adminapi.BudgetSource("TEMPLATE") || model.Limits[0].Limit != "10" {
		t.Fatalf("restored template = %+v", model)
	}

	deletePath := "/api/admin/v1/budget-templates/" + template.BudgetTemplateId + "?expectedRevision=1&reason=" + url.QueryEscape("retire assigned")
	response = doJSON(t, h, http.MethodDelete, deletePath, headers, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete assigned template: %d %s", response.Code, response.Body)
	}
	unassignPath := "/api/admin/v1/users/" + member.ID + "/budget-template?expectedAssignmentRevision=1&reason=" + url.QueryEscape("remove assignment")
	response = doJSON(t, h, http.MethodDelete, unassignPath, headers, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("unassign template: %d %s", response.Code, response.Body)
	}
	response = doJSON(t, h, http.MethodDelete, strings.Replace(deletePath, "retire+assigned", "retire+template", 1), headers, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete template: %d %s", response.Code, response.Body)
	}
	response = doJSON(t, h, http.MethodGet, "/api/admin/v1/budget-templates/"+template.BudgetTemplateId+"/audit", map[string]string{"Cookie": adminCookie}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("deleted audit: %d %s", response.Code, response.Body)
	}
	var audit adminapi.BudgetTemplateAuditPage
	decodeJSON(t, response, &audit)
	if len(audit.Items) < 4 || audit.Items[0].Action != adminapi.BudgetTemplateAuditItemAction("DELETE") {
		t.Fatalf("audit = %+v", audit)
	}
}

func findAdminBudget(t *testing.T, view adminapi.UserBudgetView, capability string) adminapi.BudgetCapabilityView {
	t.Helper()
	for _, item := range view.Items {
		if string(item.Capability) == capability {
			return item
		}
	}
	t.Fatalf("missing %s budget", capability)
	return adminapi.BudgetCapabilityView{}
}
