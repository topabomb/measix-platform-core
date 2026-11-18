package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("username"),
		field.String("password_hash").Optional().Nillable(),
		field.String("display_name"),
		field.String("role"),
		field.String("status"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("username").Unique(),
	}
}
