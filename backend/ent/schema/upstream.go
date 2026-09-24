package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Upstream struct{ ent.Schema }

func (Upstream) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("name"),
		field.Int64("config_revision"),
		field.Int64("active_config_revision").Optional().Nillable(),
		field.String("status"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}
