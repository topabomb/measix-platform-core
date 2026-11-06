package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type WorkspaceAudit struct{ ent.Schema }

func (WorkspaceAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"), field.String("request_id"), field.String("actor_id"), field.String("user_id"), field.String("agent_space_id"),
		field.String("action"), field.String("path"), field.String("outcome"), field.Int64("bytes"), field.Time("created_at"), field.Time("completed_at").Optional().Nillable(),
	}
}
