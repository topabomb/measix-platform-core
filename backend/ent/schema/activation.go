package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Activation struct{ ent.Schema }

func (Activation) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("kind"),
		field.String("state"),
		field.String("idempotency_key"),
		field.String("request_hash"),
		field.Int64("control_revision"),
		field.String("bundle_hash"),
		field.Int64("target_generation").Optional().Nillable(),
		field.Bytes("target_descriptor_json"),
		field.String("subject_id").Optional().Nillable(),
		field.Bytes("pending_operation_json").Optional().Nillable(),
		field.String("error_code").Optional().Nillable(),
		field.String("created_by_user_id"),
		field.Time("created_at"),
		field.Time("completed_at").Optional().Nillable(),
	}
}
