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
		field.Bytes("refresh_digest").Optional().Nillable().Unique(),
		field.Bytes("previous_refresh_digest").Optional().Nillable().Unique(),
		field.String("refresh_request_key").Optional().Nillable(),
		field.Time("refresh_replay_until").Optional().Nillable(),
		field.Bytes("refresh_response_ciphertext").Optional().Nillable(),
		field.Time("expires_at"),
		field.String("status"),
		field.Time("created_at"),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
		field.Int64("applied_managed_generation").Optional().Nillable(),
		field.String("applied_snapshot_hash").Optional().Nillable(),
		field.Time("applied_reported_at").Optional().Nillable(),
	}
}
