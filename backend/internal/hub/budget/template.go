package budget

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/budgetlimit"
	"measix/platform/ent/budgettemplate"
	"measix/platform/ent/budgettemplateassignment"
	"measix/platform/ent/budgettemplateaudit"
	"measix/platform/ent/user"
	"measix/platform/ent/userbudget"
	"measix/platform/pkg/platformid"
)

type TemplateRule struct {
	Capability Capability   `json:"capability"`
	Mode       Mode         `json:"mode"`
	Scopes     []ScopeInput `json:"scopes"`
}

type TemplateView struct {
	TemplateID        string         `json:"budgetTemplateId"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Revision          int64          `json:"revision"`
	Rules             []TemplateRule `json:"rules"`
	AssignedUserCount int            `json:"assignedUserCount"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type TemplatePage struct {
	Items      []TemplateView
	NextCursor string
}

type TemplateUserPage struct {
	UserIDs    []string
	NextCursor string
}

type TemplateAuditSnapshot struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Rules       []TemplateRule `json:"rules"`
}

type TemplateAuditRecord struct {
	ID                 int
	TemplateID         string
	UserID             *string
	Action             string
	TemplateRevision   int64
	AssignmentRevision int64
	ActorUserID        string
	Reason             string
	Before             *TemplateAuditSnapshot
	After              *TemplateAuditSnapshot
	CreatedAt          time.Time
}

type TemplateAuditPage struct {
	Items      []TemplateAuditRecord
	NextCursor string
}

type AssignmentView struct {
	UserID           string
	TemplateID       string
	TemplateName     string
	TemplateRevision int64
	Revision         int64
	AssignedAt       time.Time
}

type CreateTemplateInput struct {
	TemplateID, Name, Description string
	Rules                         []TemplateRule
	ActorUserID, Reason           string
}

type UpdateTemplateInput struct {
	TemplateID          string
	ExpectedRevision    int64
	Name, Description   string
	Rules               []TemplateRule
	ActorUserID, Reason string
}

type DeleteTemplateInput struct {
	TemplateID       string
	ExpectedRevision int64
	ActorUserID      string
	Reason           string
}

type AssignTemplateInput struct {
	UserID, TemplateID string
	ExpectedRevision   int64
	ActorUserID        string
	Reason             string
}

type UnassignTemplateInput struct {
	UserID           string
	ExpectedRevision int64
	ActorUserID      string
	Reason           string
}

type ClearOverrideInput struct {
	UserID           string
	Capability       Capability
	ExpectedRevision int64
	ActorUserID      string
	Reason           string
}

func (s *Service) CreateTemplate(ctx context.Context, input CreateTemplateInput) (TemplateView, error) {
	if input.TemplateID == "" {
		input.TemplateID = platformid.New(platformid.BudgetTemplate)
	}
	rules, payload, err := normalizeTemplateInput(input.TemplateID, input.Name, input.Description, input.Rules, input.ActorUserID, input.Reason)
	if err != nil {
		return TemplateView{}, err
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	defer tx.Rollback()
	row, err := tx.BudgetTemplate.Create().
		SetID(input.TemplateID).
		SetName(strings.TrimSpace(input.Name)).
		SetDescription(strings.TrimSpace(input.Description)).
		SetRulesJSON(payload).
		SetRevision(1).
		SetCreatedAt(now).
		SetCreatedByUserID(input.ActorUserID).
		SetUpdatedAt(now).
		SetUpdatedByUserID(input.ActorUserID).
		Save(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	view := templateView(row, rules, 0)
	after, _ := json.Marshal(view)
	if err := createTemplateAudit(ctx, tx, input.TemplateID, "", 1, 0, input.ActorUserID, "CREATE", input.Reason, nil, after, now); err != nil {
		return TemplateView{}, err
	}
	if err := tx.Commit(); err != nil {
		return TemplateView{}, err
	}
	return view, nil
}

func (s *Service) GetTemplate(ctx context.Context, templateID string) (TemplateView, error) {
	if platformid.Validate(platformid.BudgetTemplate, templateID) != nil {
		return TemplateView{}, ErrInvalidConfiguration
	}
	row, err := s.Client.BudgetTemplate.Get(ctx, templateID)
	if ent.IsNotFound(err) {
		return TemplateView{}, ErrTemplateNotFound
	}
	if err != nil {
		return TemplateView{}, err
	}
	rules, err := decodeTemplateRules(row.RulesJSON)
	if err != nil {
		return TemplateView{}, err
	}
	count, err := s.Client.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.BudgetTemplateIDEQ(templateID)).Count(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	return templateView(row, rules, count), nil
}

func (s *Service) ListTemplates(ctx context.Context, query string, pageSize int, cursor string) (TemplatePage, error) {
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 1 || pageSize > 100 {
		return TemplatePage{}, ErrInvalidConfiguration
	}
	offset, err := decodeTemplateCursor(cursor)
	if err != nil {
		return TemplatePage{}, err
	}
	builder := s.Client.BudgetTemplate.Query()
	if value := strings.TrimSpace(query); value != "" {
		builder.Where(budgettemplate.Or(budgettemplate.NameContainsFold(value), budgettemplate.DescriptionContainsFold(value)))
	}
	rows, err := builder.Order(ent.Asc(budgettemplate.FieldName), ent.Asc(budgettemplate.FieldID)).Offset(offset).Limit(pageSize + 1).All(ctx)
	if err != nil {
		return TemplatePage{}, err
	}
	page := TemplatePage{}
	for _, row := range rows[:min(len(rows), pageSize)] {
		rules, err := decodeTemplateRules(row.RulesJSON)
		if err != nil {
			return TemplatePage{}, err
		}
		count, err := s.Client.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.BudgetTemplateIDEQ(row.ID)).Count(ctx)
		if err != nil {
			return TemplatePage{}, err
		}
		page.Items = append(page.Items, templateView(row, rules, count))
	}
	if len(rows) > pageSize {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset + pageSize)))
	}
	return page, nil
}

func (s *Service) ListTemplateUsers(ctx context.Context, templateID, query string, pageSize int, cursor string) (TemplateUserPage, error) {
	if platformid.Validate(platformid.BudgetTemplate, templateID) != nil {
		return TemplateUserPage{}, ErrInvalidConfiguration
	}
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 1 || pageSize > 100 {
		return TemplateUserPage{}, ErrInvalidConfiguration
	}
	if exists, err := s.Client.BudgetTemplate.Query().Where(budgettemplate.IDEQ(templateID)).Exist(ctx); err != nil {
		return TemplateUserPage{}, err
	} else if !exists {
		return TemplateUserPage{}, ErrTemplateNotFound
	}
	offset, err := decodeTemplateCursor(cursor)
	if err != nil {
		return TemplateUserPage{}, err
	}
	userIDs, err := s.Client.BudgetTemplateAssignment.Query().
		Where(budgettemplateassignment.BudgetTemplateIDEQ(templateID)).
		Select(budgettemplateassignment.FieldUserID).Strings(ctx)
	if err != nil {
		return TemplateUserPage{}, err
	}
	if len(userIDs) == 0 {
		return TemplateUserPage{}, nil
	}
	builder := s.Client.User.Query().Where(user.IDIn(userIDs...))
	if value := strings.TrimSpace(query); value != "" {
		builder.Where(user.Or(user.UsernameContainsFold(value), user.DisplayNameContainsFold(value)))
	}
	rows, err := builder.Order(ent.Asc(user.FieldUsername), ent.Asc(user.FieldID)).Offset(offset).Limit(pageSize + 1).All(ctx)
	if err != nil {
		return TemplateUserPage{}, err
	}
	page := TemplateUserPage{}
	for _, row := range rows[:min(len(rows), pageSize)] {
		page.UserIDs = append(page.UserIDs, row.ID)
	}
	if len(rows) > pageSize {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset + pageSize)))
	}
	return page, nil
}

func (s *Service) ListTemplateAudit(ctx context.Context, templateID string, pageSize int, cursor string) (TemplateAuditPage, error) {
	if platformid.Validate(platformid.BudgetTemplate, templateID) != nil {
		return TemplateAuditPage{}, ErrInvalidConfiguration
	}
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 1 || pageSize > 100 {
		return TemplateAuditPage{}, ErrInvalidConfiguration
	}
	offset, err := decodeTemplateCursor(cursor)
	if err != nil {
		return TemplateAuditPage{}, err
	}
	rows, err := s.Client.BudgetTemplateAudit.Query().
		Where(budgettemplateaudit.BudgetTemplateIDEQ(templateID)).
		Order(ent.Desc(budgettemplateaudit.FieldCreatedAt), ent.Desc(budgettemplateaudit.FieldID)).
		Offset(offset).Limit(pageSize + 1).All(ctx)
	if err != nil {
		return TemplateAuditPage{}, err
	}
	if len(rows) == 0 && offset == 0 {
		exists, err := s.Client.BudgetTemplate.Query().Where(budgettemplate.IDEQ(templateID)).Exist(ctx)
		if err != nil {
			return TemplateAuditPage{}, err
		}
		if !exists {
			return TemplateAuditPage{}, ErrTemplateNotFound
		}
	}
	page := TemplateAuditPage{}
	for _, row := range rows[:min(len(rows), pageSize)] {
		record := TemplateAuditRecord{
			ID: row.ID, TemplateID: templateID, UserID: row.UserID, Action: string(row.Action),
			TemplateRevision: row.TemplateRevision, AssignmentRevision: row.AssignmentRevision,
			ActorUserID: row.ActorUserID, Reason: row.Reason, CreatedAt: row.CreatedAt,
		}
		if row.BeforeJSON != nil && len(*row.BeforeJSON) != 0 {
			var snapshot TemplateAuditSnapshot
			if err := json.Unmarshal(*row.BeforeJSON, &snapshot); err != nil {
				return TemplateAuditPage{}, err
			}
			record.Before = &snapshot
		}
		if row.AfterJSON != nil && len(*row.AfterJSON) != 0 {
			var snapshot TemplateAuditSnapshot
			if err := json.Unmarshal(*row.AfterJSON, &snapshot); err != nil {
				return TemplateAuditPage{}, err
			}
			record.After = &snapshot
		}
		page.Items = append(page.Items, record)
	}
	if len(rows) > pageSize {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset + pageSize)))
	}
	return page, nil
}

func (s *Service) UpdateTemplate(ctx context.Context, input UpdateTemplateInput) (TemplateView, error) {
	rules, payload, err := normalizeTemplateInput(input.TemplateID, input.Name, input.Description, input.Rules, input.ActorUserID, input.Reason)
	if err != nil || input.ExpectedRevision < 1 {
		return TemplateView{}, ErrInvalidConfiguration
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	defer tx.Rollback()
	row, err := tx.BudgetTemplate.Get(ctx, input.TemplateID)
	if ent.IsNotFound(err) {
		return TemplateView{}, ErrTemplateNotFound
	}
	if err != nil {
		return TemplateView{}, err
	}
	if row.Revision != input.ExpectedRevision {
		return TemplateView{}, ErrRevisionConflict
	}
	beforeRules, err := decodeTemplateRules(row.RulesJSON)
	if err != nil {
		return TemplateView{}, err
	}
	count, err := tx.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.BudgetTemplateIDEQ(row.ID)).Count(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	before := templateView(row, beforeRules, count)
	row, err = tx.BudgetTemplate.UpdateOneID(row.ID).
		SetName(strings.TrimSpace(input.Name)).
		SetDescription(strings.TrimSpace(input.Description)).
		SetRulesJSON(payload).
		SetRevision(row.Revision + 1).
		SetUpdatedAt(now).
		SetUpdatedByUserID(input.ActorUserID).
		Save(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	assignments, err := tx.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.BudgetTemplateIDEQ(row.ID)).All(ctx)
	if err != nil {
		return TemplateView{}, err
	}
	for _, assignment := range assignments {
		if err := s.projectRulesInTx(ctx, tx, assignment.UserID, rules, input.ActorUserID, input.Reason, "APPLY_TEMPLATE", now); err != nil {
			return TemplateView{}, err
		}
	}
	after := templateView(row, rules, len(assignments))
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err := createTemplateAudit(ctx, tx, row.ID, "", row.Revision, 0, input.ActorUserID, "UPDATE", input.Reason, beforeJSON, afterJSON, now); err != nil {
		return TemplateView{}, err
	}
	if err := tx.Commit(); err != nil {
		return TemplateView{}, err
	}
	return after, nil
}

func (s *Service) DeleteTemplate(ctx context.Context, input DeleteTemplateInput) error {
	if platformid.Validate(platformid.BudgetTemplate, input.TemplateID) != nil || input.ExpectedRevision < 1 || !validActorReason(input.ActorUserID, input.Reason) {
		return ErrInvalidConfiguration
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := tx.BudgetTemplate.Get(ctx, input.TemplateID)
	if ent.IsNotFound(err) {
		return ErrTemplateNotFound
	}
	if err != nil {
		return err
	}
	if row.Revision != input.ExpectedRevision {
		return ErrRevisionConflict
	}
	count, err := tx.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.BudgetTemplateIDEQ(row.ID)).Count(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return ErrTemplateAssigned
	}
	rules, err := decodeTemplateRules(row.RulesJSON)
	if err != nil {
		return err
	}
	before, _ := json.Marshal(templateView(row, rules, 0))
	if err := tx.BudgetTemplate.DeleteOneID(row.ID).Exec(ctx); err != nil {
		return err
	}
	if err := createTemplateAudit(ctx, tx, row.ID, "", row.Revision, 0, input.ActorUserID, "DELETE", input.Reason, before, nil, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) GetAssignment(ctx context.Context, userID string) (*AssignmentView, error) {
	if platformid.Validate(platformid.User, userID) != nil {
		return nil, ErrInvalidConfiguration
	}
	assignment, err := s.Client.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.UserIDEQ(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	template, err := s.Client.BudgetTemplate.Get(ctx, assignment.BudgetTemplateID)
	if err != nil {
		return nil, err
	}
	view := assignmentView(assignment, template)
	return &view, nil
}

func (s *Service) AssignTemplate(ctx context.Context, input AssignTemplateInput) (AssignmentView, error) {
	if platformid.Validate(platformid.User, input.UserID) != nil || platformid.Validate(platformid.BudgetTemplate, input.TemplateID) != nil || input.ExpectedRevision < 0 || !validActorReason(input.ActorUserID, input.Reason) {
		return AssignmentView{}, ErrInvalidConfiguration
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return AssignmentView{}, err
	}
	defer tx.Rollback()
	if _, err := tx.User.Get(ctx, input.UserID); err != nil {
		return AssignmentView{}, err
	}
	template, err := tx.BudgetTemplate.Get(ctx, input.TemplateID)
	if ent.IsNotFound(err) {
		return AssignmentView{}, ErrTemplateNotFound
	}
	if err != nil {
		return AssignmentView{}, err
	}
	rules, err := decodeTemplateRules(template.RulesJSON)
	if err != nil {
		return AssignmentView{}, err
	}
	current, err := tx.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.UserIDEQ(input.UserID)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return AssignmentView{}, err
	}
	if ent.IsNotFound(err) {
		current = nil
	}
	revision := int64(0)
	if current != nil {
		revision = current.Revision
	}
	if revision != input.ExpectedRevision {
		return AssignmentView{}, ErrRevisionConflict
	}
	var before []byte
	action := "ASSIGN"
	if current == nil {
		current, err = tx.BudgetTemplateAssignment.Create().
			SetUserID(input.UserID).SetBudgetTemplateID(template.ID).SetRevision(1).
			SetAssignedAt(now).SetUpdatedAt(now).SetUpdatedByUserID(input.ActorUserID).Save(ctx)
	} else {
		previousTemplate, previousErr := tx.BudgetTemplate.Get(ctx, current.BudgetTemplateID)
		if previousErr != nil {
			return AssignmentView{}, previousErr
		}
		previousRules, previousErr := decodeTemplateRules(previousTemplate.RulesJSON)
		if previousErr != nil {
			return AssignmentView{}, previousErr
		}
		before, _ = json.Marshal(templateAuditSnapshot(previousTemplate, previousRules))
		action = "REASSIGN"
		current, err = tx.BudgetTemplateAssignment.UpdateOneID(current.ID).
			SetBudgetTemplateID(template.ID).SetRevision(current.Revision + 1).
			SetAssignedAt(now).SetUpdatedAt(now).SetUpdatedByUserID(input.ActorUserID).Save(ctx)
	}
	if err != nil {
		return AssignmentView{}, err
	}
	if err := s.projectRulesInTx(ctx, tx, input.UserID, rules, input.ActorUserID, input.Reason, "APPLY_TEMPLATE", now); err != nil {
		return AssignmentView{}, err
	}
	view := assignmentView(current, template)
	after, _ := json.Marshal(templateAuditSnapshot(template, rules))
	if err := createTemplateAudit(ctx, tx, template.ID, input.UserID, template.Revision, current.Revision, input.ActorUserID, action, input.Reason, before, after, now); err != nil {
		return AssignmentView{}, err
	}
	if err := tx.Commit(); err != nil {
		return AssignmentView{}, err
	}
	return view, nil
}

func (s *Service) UnassignTemplate(ctx context.Context, input UnassignTemplateInput) (AssignmentView, error) {
	if platformid.Validate(platformid.User, input.UserID) != nil || input.ExpectedRevision < 1 || !validActorReason(input.ActorUserID, input.Reason) {
		return AssignmentView{}, ErrInvalidConfiguration
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return AssignmentView{}, err
	}
	defer tx.Rollback()
	assignment, err := tx.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.UserIDEQ(input.UserID)).Only(ctx)
	if ent.IsNotFound(err) {
		return AssignmentView{}, ErrAssignmentNotFound
	}
	if err != nil {
		return AssignmentView{}, err
	}
	if assignment.Revision != input.ExpectedRevision {
		return AssignmentView{}, ErrRevisionConflict
	}
	template, err := tx.BudgetTemplate.Get(ctx, assignment.BudgetTemplateID)
	if err != nil {
		return AssignmentView{}, err
	}
	beforeView := assignmentView(assignment, template)
	rules, err := decodeTemplateRules(template.RulesJSON)
	if err != nil {
		return AssignmentView{}, err
	}
	before, _ := json.Marshal(templateAuditSnapshot(template, rules))
	if err := s.projectRulesInTx(ctx, tx, input.UserID, nil, input.ActorUserID, input.Reason, "UNASSIGN_TEMPLATE", now); err != nil {
		return AssignmentView{}, err
	}
	if err := tx.BudgetTemplateAssignment.DeleteOneID(assignment.ID).Exec(ctx); err != nil {
		return AssignmentView{}, err
	}
	if err := createTemplateAudit(ctx, tx, template.ID, input.UserID, template.Revision, assignment.Revision, input.ActorUserID, "UNASSIGN", input.Reason, before, nil, now); err != nil {
		return AssignmentView{}, err
	}
	if err := tx.Commit(); err != nil {
		return AssignmentView{}, err
	}
	return beforeView, nil
}

func (s *Service) ClearOverride(ctx context.Context, input ClearOverrideInput) (BudgetView, error) {
	if validateSubject(input.UserID, input.Capability) != nil || input.ExpectedRevision < 1 || !validActorReason(input.ActorUserID, input.Reason) {
		return BudgetView{}, ErrInvalidConfiguration
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return BudgetView{}, err
	}
	defer tx.Rollback()
	row, err := tx.UserBudget.Query().Where(userbudget.UserIDEQ(input.UserID), userbudget.CapabilityEQ(userbudget.Capability(input.Capability))).Only(ctx)
	if ent.IsNotFound(err) || (err == nil && Source(row.Source) != SourceExplicit) {
		return BudgetView{}, ErrRevisionConflict
	}
	if err != nil {
		return BudgetView{}, err
	}
	if row.Revision != input.ExpectedRevision {
		return BudgetView{}, ErrRevisionConflict
	}
	desiredSource, desiredMode, desiredScopes := SourceDefault, ModeUnlimited, []ScopeInput(nil)
	assignment, err := tx.BudgetTemplateAssignment.Query().Where(budgettemplateassignment.UserIDEQ(input.UserID)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return BudgetView{}, err
	}
	if err == nil {
		template, err := tx.BudgetTemplate.Get(ctx, assignment.BudgetTemplateID)
		if err != nil {
			return BudgetView{}, err
		}
		rules, err := decodeTemplateRules(template.RulesJSON)
		if err != nil {
			return BudgetView{}, err
		}
		for _, rule := range rules {
			if rule.Capability == input.Capability {
				desiredSource, desiredMode, desiredScopes = SourceTemplate, rule.Mode, rule.Scopes
				break
			}
		}
	}
	view, err := s.applyBudgetInTx(ctx, tx, budgetMutation{
		UserID: input.UserID, Capability: input.Capability, ExpectedRevision: input.ExpectedRevision, CheckRevision: true,
		Mode: desiredMode, Scopes: desiredScopes, Source: desiredSource, ActorUserID: input.ActorUserID,
		Reason: input.Reason, Action: "CLEAR_OVERRIDE",
	}, now)
	if err != nil {
		return BudgetView{}, err
	}
	if err := tx.Commit(); err != nil {
		return BudgetView{}, err
	}
	return view, nil
}

func (s *Service) projectRulesInTx(ctx context.Context, tx *ent.Tx, userID string, rules []TemplateRule, actor, reason, action string, now time.Time) error {
	rulesByCapability := make(map[Capability]TemplateRule, len(rules))
	for _, rule := range rules {
		rulesByCapability[rule.Capability] = rule
	}
	capabilities := []Capability{CapabilityModel, CapabilityTTS, CapabilityASR, CapabilityMCP, CapabilityImageGeneration}
	for _, capability := range capabilities {
		row, err := tx.UserBudget.Query().Where(userbudget.UserIDEQ(userID), userbudget.CapabilityEQ(userbudget.Capability(capability))).Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return err
		}
		if ent.IsNotFound(err) {
			row = nil
		}
		if row != nil && Source(row.Source) == SourceExplicit {
			continue
		}
		rule, hasRule := rulesByCapability[capability]
		if row == nil && !hasRule {
			continue
		}
		source, mode, scopes := SourceDefault, ModeUnlimited, []ScopeInput(nil)
		if hasRule {
			source, mode, scopes = SourceTemplate, rule.Mode, rule.Scopes
		}
		if row != nil {
			limits, err := tx.BudgetLimit.Query().Where(budgetlimit.UserBudgetIDEQ(row.ID), budgetlimit.EffectiveToIsNil()).All(ctx)
			if err != nil {
				return err
			}
			if budgetMatchesDesired(viewFromRows(row, limits), source, mode, scopes) {
				continue
			}
		}
		if _, err := s.applyBudgetInTx(ctx, tx, budgetMutation{
			UserID: userID, Capability: capability, Mode: mode, Scopes: scopes, Source: source,
			ActorUserID: actor, Reason: reason, Action: action,
		}, now); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTemplateInput(templateID, name, description string, rules []TemplateRule, actor, reason string) ([]TemplateRule, []byte, error) {
	if platformid.Validate(platformid.BudgetTemplate, templateID) != nil || strings.TrimSpace(name) == "" || len(strings.TrimSpace(name)) > 100 || len(description) > 500 || len(rules) > 5 || !validActorReason(actor, reason) {
		return nil, nil, ErrInvalidConfiguration
	}
	normalized := append([]TemplateRule(nil), rules...)
	seen := make(map[Capability]struct{}, len(normalized))
	for i := range normalized {
		rule := &normalized[i]
		if !validCapability(rule.Capability) {
			return nil, nil, ErrInvalidConfiguration
		}
		if _, duplicate := seen[rule.Capability]; duplicate {
			return nil, nil, ErrInvalidConfiguration
		}
		seen[rule.Capability] = struct{}{}
		if rule.Mode != ModeLimited && rule.Mode != ModeUnlimited {
			return nil, nil, ErrInvalidConfiguration
		}
		if rule.Mode == ModeUnlimited && len(rule.Scopes) != 0 {
			return nil, nil, ErrInvalidConfiguration
		}
		if rule.Mode == ModeLimited && len(rule.Scopes) == 0 {
			return nil, nil, ErrInvalidConfiguration
		}
		seenPeriodMeter := make(map[string]struct{})
		for si := range rule.Scopes {
			scope := &rule.Scopes[si]
			if scope.ScopeKey != "" || !validPeriod(scope.Period) || len(scope.Limits) == 0 {
				return nil, nil, ErrInvalidConfiguration
			}
			for _, limit := range scope.Limits {
				if !validMeter(limit.Meter) || !meterAllowed(rule.Capability, limit.Meter) || limit.Limit < 0 {
					return nil, nil, ErrInvalidConfiguration
				}
				key := string(scope.Period) + "\x00" + string(limit.Meter)
				if _, duplicate := seenPeriodMeter[key]; duplicate {
					return nil, nil, ErrInvalidConfiguration
				}
				seenPeriodMeter[key] = struct{}{}
			}
			sort.Slice(scope.Limits, func(i, j int) bool { return scope.Limits[i].Meter < scope.Limits[j].Meter })
		}
		sort.Slice(rule.Scopes, func(i, j int) bool { return rule.Scopes[i].Period < rule.Scopes[j].Period })
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Capability < normalized[j].Capability })
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, err
	}
	return normalized, payload, nil
}

func decodeTemplateRules(payload []byte) ([]TemplateRule, error) {
	var rules []TemplateRule
	if err := json.Unmarshal(payload, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func budgetMatchesDesired(view BudgetView, source Source, mode Mode, scopes []ScopeInput) bool {
	if view.Source != source || view.Mode != mode {
		return false
	}
	if mode == ModeUnlimited {
		return true
	}
	current := make(map[string]int64)
	for _, scope := range view.Scopes {
		for _, limit := range scope.Limits {
			current[string(scope.Period)+"\x00"+string(limit.Meter)] = limit.Limit
		}
	}
	desired := make(map[string]int64)
	for _, scope := range scopes {
		for _, limit := range scope.Limits {
			desired[string(scope.Period)+"\x00"+string(limit.Meter)] = limit.Limit
		}
	}
	if len(current) != len(desired) {
		return false
	}
	for key, value := range current {
		if desired[key] != value {
			return false
		}
	}
	return true
}

func validActorReason(actor, reason string) bool {
	trimmed := strings.TrimSpace(reason)
	return platformid.Validate(platformid.User, actor) == nil && trimmed != "" && len(trimmed) <= 500
}

func templateView(row *ent.BudgetTemplate, rules []TemplateRule, count int) TemplateView {
	return TemplateView{TemplateID: row.ID, Name: row.Name, Description: row.Description, Revision: row.Revision, Rules: rules, AssignedUserCount: count, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func templateAuditSnapshot(row *ent.BudgetTemplate, rules []TemplateRule) TemplateAuditSnapshot {
	return TemplateAuditSnapshot{Name: row.Name, Description: row.Description, Rules: rules}
}

func assignmentView(row *ent.BudgetTemplateAssignment, template *ent.BudgetTemplate) AssignmentView {
	return AssignmentView{UserID: row.UserID, TemplateID: template.ID, TemplateName: template.Name, TemplateRevision: template.Revision, Revision: row.Revision, AssignedAt: row.AssignedAt}
}

func createTemplateAudit(ctx context.Context, tx *ent.Tx, templateID, userID string, templateRevision, assignmentRevision int64, actor, action, reason string, before, after []byte, now time.Time) error {
	builder := tx.BudgetTemplateAudit.Create().
		SetTemplateRevision(templateRevision).
		SetAssignmentRevision(assignmentRevision).
		SetActorUserID(actor).
		SetAction(budgettemplateaudit.Action(action)).
		SetReason(strings.TrimSpace(reason)).
		SetCreatedAt(now)
	if templateID != "" {
		builder.SetBudgetTemplateID(templateID)
	}
	if userID != "" {
		builder.SetUserID(userID)
	}
	if before != nil {
		builder.SetBeforeJSON(before)
	}
	if after != nil {
		builder.SetAfterJSON(after)
	}
	_, err := builder.Save(ctx)
	return err
}

func decodeTemplateCursor(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, ErrInvalidConfiguration
	}
	offset, err := strconv.Atoi(string(payload))
	if err != nil || offset < 0 {
		return 0, ErrInvalidConfiguration
	}
	return offset, nil
}
