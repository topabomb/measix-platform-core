package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetTemplate owns one live, capability-level budget rule set. Rules are
// canonical typed JSON; user-specific effective rows remain owned by UserBudget.
type BudgetTemplate struct{ ent.Schema }

func (BudgetTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("name"),
		field.String("description"),
		field.Bytes("rules_json"),
		field.Int64("revision").Positive(),
		field.Time("created_at").Immutable(),
		// Creation attribution is stable during ordinary template updates, but
		// the typed user-deletion owner must be able to anonymize it.
		field.String("created_by_user_id"),
		field.Time("updated_at"),
		field.String("updated_by_user_id"),
	}
}

func (BudgetTemplate) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name", "id")}
}
