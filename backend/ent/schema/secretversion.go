package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SecretVersion struct{ ent.Schema }

func (SecretVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("secret_id"),
		field.Int64("secret_version"),
		field.Bytes("encrypted_payload"),
		field.Int("key_version"),
		field.String("created_by_user_id"),
		field.Time("created_at"),
	}
}
func (SecretVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("secret_id", "secret_version").Unique(),
	}
}
