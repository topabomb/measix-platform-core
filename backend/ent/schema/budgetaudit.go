package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetAudit records human configuration and reconciliation commands. It is
// separate from settlement history because automated usage delivery is not an
// administrative mutation.
type BudgetAudit struct{ ent.Schema }

func (BudgetAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("user_id"),
		field.Enum("capability").Values("MODEL", "TTS", "ASR", "MCP"),
		field.Int("user_budget_id").Optional().Nillable(),
		field.Int64("budget_revision").Default(0).NonNegative(),
		field.String("request_id").Optional().Nillable(),
		field.String("actor_user_id"),
		field.Enum("action").Values("CREATE", "UPDATE", "RESOLVE_RECONCILIATION"),
		field.String("reason"),
		field.Bytes("before_json").Optional().Nillable(),
		field.Bytes("after_json"),
		field.Time("created_at"),
	}
}

func (BudgetAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "capability", "created_at"),
		index.Fields("request_id", "created_at"),
	}
}
