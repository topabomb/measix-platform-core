package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UsageDetail stores provider-reported diagnostic quantities that are already
// included in a canonical budget meter or are otherwise non-deductible. It is
// deliberately separate from SemanticUsage so a detail can never be summed as
// a second budget or pricing fact.
type UsageDetail struct{ ent.Schema }

func (UsageDetail) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("request_id"),
		field.Int64("settlement_revision"),
		field.String("name"),
		field.Int64("quantity_units"),
		field.String("source"),
		field.String("completeness"),
		field.Time("occurred_at"),
	}
}

func (UsageDetail) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "settlement_revision", "name").Unique(),
		index.Fields("request_id", "settlement_revision"),
	}
}
