package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Secret struct{ ent.Schema }

func (Secret) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("name"),
		field.Int64("latest_secret_version"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}
