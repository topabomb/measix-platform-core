package budget

import (
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
)

func TestBudgetTemplateLiveLinkAndExplicitOverridePrecedence(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	templateID := platformid.New(platformid.BudgetTemplate)

	created, err := f.service.CreateTemplate(ctx, CreateTemplateInput{
		TemplateID:  templateID,
		Name:        "Standard",
		Description: "Standard enterprise allowance",
		Rules: []TemplateRule{
			{Capability: CapabilityModel, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 10}}}}},
			{Capability: CapabilityImageGeneration, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodLifetime, Limits: []LimitSpec{{Meter: MeterRequestedImages, Limit: 2}}}}},
		},
		ActorUserID: f.adminID,
		Reason:      "create standard template",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 || created.AssignedUserCount != 0 || len(created.Rules) != 2 {
		t.Fatalf("created template = %+v", created)
	}

	assignment, err := f.service.AssignTemplate(ctx, AssignTemplateInput{
		UserID: f.userID, TemplateID: templateID, ExpectedRevision: 0,
		ActorUserID: f.adminID, Reason: "assign standard template",
	})
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Revision != 1 || assignment.TemplateRevision != 1 {
		t.Fatalf("assignment = %+v", assignment)
	}
	model, err := f.service.Get(ctx, f.userID, CapabilityModel)
	if err != nil || model.Source != SourceTemplate || model.Scopes[0].Limits[0].Limit != 10 {
		t.Fatalf("template model = %+v, %v", model, err)
	}
	image, err := f.service.Get(ctx, f.userID, CapabilityImageGeneration)
	if err != nil || image.Source != SourceTemplate || image.Scopes[0].Limits[0].Meter != MeterRequestedImages {
		t.Fatalf("template image = %+v, %v", image, err)
	}
	imageScope := image.Scopes[0].ScopeKey

	f.now = f.now.Add(time.Millisecond)
	explicit, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: model.Revision, Mode: ModeLimited,
		Scopes:      []ScopeInput{{ScopeKey: model.Scopes[0].ScopeKey, Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 7}}}},
		ActorUserID: f.adminID, Reason: "user-specific model allowance",
	})
	if err != nil || explicit.Source != SourceExplicit {
		t.Fatalf("explicit override = %+v, %v", explicit, err)
	}

	f.now = f.now.Add(time.Millisecond)
	updated, err := f.service.UpdateTemplate(ctx, UpdateTemplateInput{
		TemplateID: templateID, ExpectedRevision: 1, Name: "Standard", Description: "Revised",
		Rules: []TemplateRule{
			{Capability: CapabilityModel, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 20}}}}},
			{Capability: CapabilityImageGeneration, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodLifetime, Limits: []LimitSpec{{Meter: MeterRequestedImages, Limit: 3}}}}},
		},
		ActorUserID: f.adminID, Reason: "increase standard allowance",
	})
	if err != nil || updated.Revision != 2 || updated.AssignedUserCount != 1 {
		t.Fatalf("updated template = %+v, %v", updated, err)
	}
	model, _ = f.service.Get(ctx, f.userID, CapabilityModel)
	if model.Source != SourceExplicit || model.Scopes[0].Limits[0].Limit != 7 {
		t.Fatalf("template overwrote explicit model = %+v", model)
	}
	image, _ = f.service.Get(ctx, f.userID, CapabilityImageGeneration)
	if image.Source != SourceTemplate || image.Scopes[0].Limits[0].Limit != 3 || image.Scopes[0].ScopeKey != imageScope {
		t.Fatalf("live image update = %+v", image)
	}

	f.now = f.now.Add(time.Millisecond)
	restored, err := f.service.ClearOverride(ctx, ClearOverrideInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: model.Revision,
		ActorUserID: f.adminID, Reason: "restore template",
	})
	if err != nil || restored.Source != SourceTemplate || restored.Scopes[0].Limits[0].Limit != 20 {
		t.Fatalf("clear override = %+v, %v", restored, err)
	}
	modelScope := restored.Scopes[0].ScopeKey
	f.now = f.now.Add(time.Millisecond)
	updated, err = f.service.UpdateTemplate(ctx, UpdateTemplateInput{
		TemplateID: templateID, ExpectedRevision: 2, Name: "Standard", Description: "Monthly",
		Rules: []TemplateRule{
			{Capability: CapabilityModel, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodMonth, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 20}}}}},
			{Capability: CapabilityImageGeneration, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodLifetime, Limits: []LimitSpec{{Meter: MeterRequestedImages, Limit: 3}}}}},
		},
		ActorUserID: f.adminID, Reason: "move standard model allowance to month",
	})
	if err != nil || updated.Revision != 3 {
		t.Fatalf("period update = %+v, %v", updated, err)
	}
	model, _ = f.service.Get(ctx, f.userID, CapabilityModel)
	if model.Scopes[0].Period != PeriodMonth || model.Scopes[0].ScopeKey == modelScope {
		t.Fatalf("period change reused scope = %+v", model)
	}

	users, err := f.service.ListTemplateUsers(ctx, templateID, "budget-user", 10, "")
	if err != nil || len(users.UserIDs) != 1 || users.UserIDs[0] != f.userID {
		t.Fatalf("assigned users = %+v, %v", users, err)
	}
	audit, err := f.service.ListTemplateAudit(ctx, templateID, 20, "")
	if err != nil || len(audit.Items) < 4 || audit.Items[0].After == nil || audit.Items[0].After.Name != "Standard" {
		t.Fatalf("template audit = %+v, %v", audit, err)
	}
	if err := f.service.DeleteTemplate(ctx, DeleteTemplateInput{TemplateID: templateID, ExpectedRevision: 3, ActorUserID: f.adminID, Reason: "still assigned"}); !errors.Is(err, ErrTemplateAssigned) {
		t.Fatalf("assigned delete error = %v", err)
	}

	if _, err := f.service.UnassignTemplate(ctx, UnassignTemplateInput{
		UserID: f.userID, ExpectedRevision: assignment.Revision, ActorUserID: f.adminID, Reason: "remove template",
	}); err != nil {
		t.Fatal(err)
	}
	states, err := f.service.UserStates(ctx, f.userID)
	if err != nil || len(states) != 5 {
		t.Fatalf("five capability states = %+v, %v", states, err)
	}
	for _, state := range states {
		if state.Budget.Source != SourceDefault || state.Budget.Mode != ModeUnlimited {
			t.Fatalf("unassignment did not restore default: %+v", state.Budget)
		}
	}
	if err := f.service.DeleteTemplate(ctx, DeleteTemplateInput{TemplateID: templateID, ExpectedRevision: 3, ActorUserID: f.adminID, Reason: "retire template"}); err != nil {
		t.Fatal(err)
	}
	audit, err = f.service.ListTemplateAudit(ctx, templateID, 20, "")
	if err != nil || len(audit.Items) == 0 || audit.Items[0].Action != "DELETE" {
		t.Fatalf("deleted template audit = %+v, %v", audit, err)
	}
}

func TestBudgetTemplateRejectsDuplicateCapabilitiesAndWrongMeters(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	base := CreateTemplateInput{
		TemplateID: platformid.New(platformid.BudgetTemplate), Name: "Invalid", ActorUserID: f.adminID, Reason: "validation",
	}
	base.Rules = []TemplateRule{
		{Capability: CapabilityMCP, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 1}}}}},
		{Capability: CapabilityMCP, Mode: ModeUnlimited},
	}
	if _, err := f.service.CreateTemplate(ctx, base); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("duplicate capability error = %v", err)
	}
	base.TemplateID = platformid.New(platformid.BudgetTemplate)
	base.Rules = []TemplateRule{{Capability: CapabilityImageGeneration, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterInputTokens, Limit: 1}}}}}}
	if _, err := f.service.CreateTemplate(ctx, base); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("wrong image meter error = %v", err)
	}
}

func TestTemplateReassignmentAndUnassignmentPreserveExplicitOverrides(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	create := func(name string, rules []TemplateRule) string {
		t.Helper()
		view, err := f.service.CreateTemplate(ctx, CreateTemplateInput{
			Name: name, Rules: rules, ActorUserID: f.adminID, Reason: "create " + name,
		})
		if err != nil {
			t.Fatal(err)
		}
		return view.TemplateID
	}
	templateA := create("Template A", []TemplateRule{
		{Capability: CapabilityModel, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 10}}}}},
		{Capability: CapabilityImageGeneration, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodLifetime, Limits: []LimitSpec{{Meter: MeterRequestedImages, Limit: 2}}}}},
	})
	templateB := create("Template B", []TemplateRule{
		{Capability: CapabilityModel, Mode: ModeLimited, Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 20}}}}},
	})
	assignment, err := f.service.AssignTemplate(ctx, AssignTemplateInput{
		UserID: f.userID, TemplateID: templateA, ExpectedRevision: 0, ActorUserID: f.adminID, Reason: "assign A",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Millisecond)
	model, _ := f.service.Get(ctx, f.userID, CapabilityModel)
	model, err = f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: model.Revision, Mode: ModeLimited,
		Scopes:      []ScopeInput{{ScopeKey: model.Scopes[0].ScopeKey, Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 7}}}},
		ActorUserID: f.adminID, Reason: "explicit model",
	})
	if err != nil {
		t.Fatal(err)
	}
	assignment, err = f.service.AssignTemplate(ctx, AssignTemplateInput{
		UserID: f.userID, TemplateID: templateB, ExpectedRevision: assignment.Revision, ActorUserID: f.adminID, Reason: "replace A with B",
	})
	if err != nil || assignment.Revision != 2 || assignment.TemplateID != templateB {
		t.Fatalf("reassignment = %+v, %v", assignment, err)
	}
	model, _ = f.service.Get(ctx, f.userID, CapabilityModel)
	image, _ := f.service.Get(ctx, f.userID, CapabilityImageGeneration)
	if model.Source != SourceExplicit || model.Scopes[0].Limits[0].Limit != 7 {
		t.Fatalf("reassignment overwrote explicit model = %+v", model)
	}
	if image.Source != SourceDefault || image.Mode != ModeUnlimited {
		t.Fatalf("old template image rule survived reassignment = %+v", image)
	}
	if _, err := f.service.UnassignTemplate(ctx, UnassignTemplateInput{
		UserID: f.userID, ExpectedRevision: assignment.Revision, ActorUserID: f.adminID, Reason: "unlink B",
	}); err != nil {
		t.Fatal(err)
	}
	model, _ = f.service.Get(ctx, f.userID, CapabilityModel)
	if model.Source != SourceExplicit || model.Scopes[0].Limits[0].Limit != 7 {
		t.Fatalf("unassignment removed explicit model = %+v", model)
	}
	for _, capability := range []Capability{CapabilityTTS, CapabilityASR, CapabilityMCP, CapabilityImageGeneration} {
		view, err := f.service.Get(ctx, f.userID, capability)
		if err != nil || view.Source != SourceDefault || view.Mode != ModeUnlimited {
			t.Fatalf("%s after unassignment = %+v, %v", capability, view, err)
		}
	}
}
