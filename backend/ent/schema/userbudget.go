package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserBudget is the current configuration head for one user and capability.
// Absence is meaningful and projects as DEFAULT UNLIMITED; no sentinel row is
// created for that state.
type UserBudget struct{ ent.Schema }

func (UserBudget) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("user_id"),
		field.Enum("capability").Values("MODEL", "IMAGE_GENERATION", "TTS", "ASR", "MCP"),
		field.Enum("mode").Values("UNLIMITED", "LIMITED"),
		field.Enum("source").Values("DEFAULT", "TEMPLATE", "EXPLICIT"),
		field.Int64("revision").Positive(),
		field.Time("activated_at"),
		field.Time("updated_at"),
		field.String("updated_by_user_id"),
	}
}

func (UserBudget) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "capability").Unique(),
	}
}
