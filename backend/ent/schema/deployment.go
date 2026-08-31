package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Deployment struct{ ent.Schema }

func (Deployment) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("name"),
		field.String("status"),
		field.String("timezone").Default("UTC"),
		field.Int64("feed_revision").Default(0),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}
