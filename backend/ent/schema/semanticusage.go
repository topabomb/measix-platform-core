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
		field.String("request_id").Optional().Nillable(),
		field.String("upstream_id"),
		field.String("resource_id").Optional().Nillable(),
		field.String("source_event_id").Optional().Nillable(),
		field.String("meter"),
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
		index.Fields("upstream_id", "source_event_id").Unique(),
	}
}
