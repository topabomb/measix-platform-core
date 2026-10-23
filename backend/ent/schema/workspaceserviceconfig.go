package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type WorkspaceServiceConfig struct{ ent.Schema }

func (WorkspaceServiceConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"), field.String("workspace_service_id").Immutable(), field.Int64("revision").Immutable(),
		field.Bytes("config_json").Immutable(), field.String("created_by_user_id").Immutable(), field.Time("created_at").Immutable(),
	}
}
func (WorkspaceServiceConfig) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workspace_service_id", "revision").Unique()}
}
