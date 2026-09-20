package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetBucket is the enforcement counter. Quantities are integers; audio is
// represented as milliseconds so budget arithmetic never depends on floats.
type BudgetBucket struct{ ent.Schema }

func (BudgetBucket) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("scope_key"),
		field.Enum("period").Values("DAY", "WEEK", "MONTH", "LIFETIME"),
		field.Time("period_start"),
		field.Time("period_end").Optional().Nillable(),
		field.Enum("meter").Values("REQUESTS", "INPUT_TOKENS", "OUTPUT_TOKENS", "CACHED_TOKENS", "TOTAL_TOKENS", "CHARACTERS", "AUDIO_MILLISECONDS"),
		field.Int64("settled_quantity").Default(0).NonNegative(),
		field.Int64("reserved_quantity").Default(0).NonNegative(),
		field.Time("updated_at"),
	}
}

func (BudgetBucket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_key", "period_start", "meter").Unique(),
		index.Fields("period_end"),
	}
}
