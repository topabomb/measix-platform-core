package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Device struct{ ent.Schema }

func (Device) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("user_id"),
		field.String("installation_id").Optional().Nillable().Unique(),
		field.String("status"),
		field.String("app_version").Optional().Nillable(),
		field.Time("created_at"),
		field.Time("last_seen_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}
