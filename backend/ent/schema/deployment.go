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
		field.String("public_origin").Default(""),
		field.Int64("feed_revision").Default(0),
		field.Bytes("release_retention_json").Optional(),
		field.Int64("release_retention_revision").Default(1),
		field.Time("release_cleanup_at").Optional().Nillable(),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}
