package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ID is the owning Core user ID, not a second space ID. No user FK: cleanup
// authority must survive removal of the enterprise principal.
type AgentSpace struct{ ent.Schema }

func (AgentSpace) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(), field.String("workspace_service_id").Immutable(), field.String("remote_username").Immutable(),
		field.String("agent_space_id").Optional(), field.Int64("binding_revision"), field.String("intent"), field.String("state"),
		field.String("mcp_secret_id").Optional(), field.Int64("mcp_secret_version").Optional(),
		field.String("dav_secret_id").Optional(), field.Int64("dav_secret_version").Optional(),
		field.Bool("dav_confirmed"), field.Bool("remote_active"), field.Bool("stop_pending"),
		field.Int64("applied_control_revision").Optional(), field.String("diagnostic_code").Optional(),
		field.Time("observed_at").Optional().Nillable(), field.Time("created_at"), field.Time("updated_at"),
	}
}
func (AgentSpace) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workspace_service_id", "remote_username").Unique(), index.Fields("workspace_service_id", "agent_space_id").Unique()}
}
