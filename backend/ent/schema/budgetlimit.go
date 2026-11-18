package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetLimit is an immutable-in-meaning limit version. Editing an amount ends
// the active row and appends a new row with the same scope key. Changing period
// creates a new scope key so that existing buckets can never be reset by an edit.
type BudgetLimit struct{ ent.Schema }

func (BudgetLimit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int("user_budget_id"),
		field.String("scope_key"),
		field.Enum("period").Values("DAY", "WEEK", "MONTH", "LIFETIME"),
		field.Enum("meter").Values("REQUESTS", "REQUESTED_IMAGES", "INPUT_TOKENS", "OUTPUT_TOKENS", "CACHED_TOKENS", "TOTAL_TOKENS", "CHARACTERS", "AUDIO_MILLISECONDS"),
		field.Int64("limit_quantity").NonNegative(),
		field.Time("scope_started_at").Immutable(),
		field.Time("effective_from"),
		field.Time("effective_to").Optional().Nillable(),
		field.Time("created_at"),
		field.String("created_by_user_id"),
	}
}

func (BudgetLimit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_budget_id", "scope_key", "meter", "effective_from").Unique(),
		index.Fields("user_budget_id", "effective_from", "effective_to"),
		index.Fields("scope_key", "meter", "effective_from"),
	}
}
