package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type IdempotencyRecord struct{ ent.Schema }

func (IdempotencyRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("admin_user_id"),
		field.String("method"),
		field.String("normalized_path"),
		field.String("idempotency_key"),
		field.String("request_hash"),
		field.String("activation_id").Optional().Nillable(),
		field.Int("status_code").Optional().Nillable(),
		field.Bytes("response_json").Optional().Nillable(),
		field.Time("created_at"),
	}
}
func (IdempotencyRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("admin_user_id", "method", "normalized_path", "idempotency_key").Unique(),
	}
}
