package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SemanticUsage struct{ ent.Schema }

func (SemanticUsage) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("request_id"),
		field.Int64("settlement_revision"),
		field.String("source_event_id"),
		field.String("meter"),
		field.Int64("quantity_units"),
		field.String("quantity_decimal"),
		field.String("completeness"),
		field.String("provider_cost").Optional().Nillable(),
		field.String("currency").Optional().Nillable(),
		field.String("source"),
		field.Time("occurred_at"),
	}
}
func (SemanticUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "settlement_revision", "meter").Unique(),
		index.Fields("request_id", "meter", "settlement_revision"),
	}
}
