package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Enrollment struct{ ent.Schema }

func (Enrollment) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id"),
		field.Bytes("token_digest").Unique(),
		field.Time("expires_at"),
		field.Time("consumed_at").Optional().Nillable(),
		field.String("created_by_user_id"),
		field.Time("created_at"),
	}
}
