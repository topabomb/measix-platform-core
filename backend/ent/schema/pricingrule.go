package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type PricingRule struct{ ent.Schema }

func (PricingRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("resource_id").Optional().Nillable(),
		field.String("upstream_id").Optional().Nillable(),
		field.String("meter"),
		field.String("unit_size"),
		field.String("unit_price_decimal"),
		field.String("currency"),
		field.Time("effective_from"),
		field.Time("effective_to").Optional().Nillable(),
	}
}
