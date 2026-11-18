package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetTemplateAudit records template and assignment commands independently
// from the per-capability BudgetAudit projection history.
type BudgetTemplateAudit struct{ ent.Schema }

func (BudgetTemplateAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("budget_template_id").Optional().Nillable(),
		field.String("user_id").Optional().Nillable(),
		field.Int64("template_revision").Default(0).NonNegative(),
		field.Int64("assignment_revision").Default(0).NonNegative(),
		field.String("actor_user_id"),
		field.Enum("action").Values("CREATE", "UPDATE", "DELETE", "ASSIGN", "REASSIGN", "UNASSIGN"),
		field.String("reason"),
		field.Bytes("before_json").Optional().Nillable(),
		field.Bytes("after_json").Optional().Nillable(),
		field.Time("created_at"),
	}
}

func (BudgetTemplateAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("budget_template_id", "created_at"),
		index.Fields("user_id", "created_at"),
	}
}
