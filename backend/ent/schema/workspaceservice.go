package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type WorkspaceService struct{ ent.Schema }

func (WorkspaceService) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(), field.String("name"), field.Int64("config_revision"),
		field.Int64("active_config_revision").Optional().Nillable(), field.Bool("enabled"),
		field.String("state"), field.String("mcp_server_id").Immutable(), field.String("runtime_route_id").Immutable(),
		field.String("diagnostic_code").Optional(), field.Time("created_at"), field.Time("updated_at"),
	}
}
