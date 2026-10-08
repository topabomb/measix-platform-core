package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type WorkspaceOperation struct{ ent.Schema }

func (WorkspaceOperation) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(), field.String("workspace_service_id").Immutable(), field.String("user_id").Optional(),
		field.String("action").Immutable(), field.String("idempotency_key").Immutable(), field.String("request_hash").Immutable(),
		field.Int64("config_revision").Immutable(), field.Int64("binding_revision").Immutable(),
		// Only verified recovery may fill an unknown space ID or replace the
		// management secret reference. Service, address and account stay fixed.
		field.Bytes("target_json"),
		field.String("state"), field.String("step"), field.Bytes("result_json").Optional(),
		field.String("candidate_secret_id").Optional(), field.Int64("candidate_secret_version").Optional(),
		field.String("activation_id").Optional(), field.String("diagnostic_code").Optional(), field.String("evidence").Optional(),
		field.String("created_by_user_id").Immutable(), field.Time("created_at"), field.Time("updated_at"),
	}
}
func (WorkspaceOperation) Indexes() []ent.Index {
	return []ent.Index{index.Fields("created_by_user_id", "idempotency_key").Unique(), index.Fields("state", "updated_at")}
}
