package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetAllocation pins a request to the exact scope and natural-period bucket
// chosen at admission. Late settlement therefore cannot drift into a new rule
// revision or period.
type BudgetAllocation struct{ ent.Schema }

func (BudgetAllocation) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("request_id").Immutable(),
		field.Int("budget_limit_id").Immutable(),
		field.Int("budget_bucket_id").Immutable(),
		field.String("scope_key").Immutable(),
		field.Enum("period").Values("DAY", "WEEK", "MONTH", "LIFETIME").Immutable(),
		field.Enum("meter").Values("REQUESTS", "REQUESTED_IMAGES", "INPUT_TOKENS", "OUTPUT_TOKENS", "CACHED_TOKENS", "TOTAL_TOKENS", "CHARACTERS", "AUDIO_MILLISECONDS").Immutable(),
		field.Int64("reserved_quantity").Default(0).NonNegative().Immutable(),
		field.Bool("reservation_released").Default(false),
		field.Int64("settled_quantity").Default(0).NonNegative(),
		field.Bool("resolved").Default(false),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (BudgetAllocation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "scope_key", "meter").Unique(),
		index.Fields("budget_bucket_id"),
	}
}
