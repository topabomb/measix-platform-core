package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Session struct{ ent.Schema }

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id"),
		field.String("device_id").Optional().Nillable(),
		field.String("channel"),
		field.Bytes("refresh_digest").Optional().Nillable(),
		field.Time("expires_at"),
		field.String("status"),
		field.Time("created_at"),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}
