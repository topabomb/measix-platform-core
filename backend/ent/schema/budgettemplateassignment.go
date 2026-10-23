package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetTemplateAssignment is the single live template link for one user.
type BudgetTemplateAssignment struct{ ent.Schema }

func (BudgetTemplateAssignment) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("user_id"),
		field.String("budget_template_id"),
		field.Int64("revision").Positive(),
		field.Time("assigned_at"),
		field.Time("updated_at"),
		field.String("updated_by_user_id"),
	}
}

func (BudgetTemplateAssignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
		index.Fields("budget_template_id", "user_id"),
	}
}
