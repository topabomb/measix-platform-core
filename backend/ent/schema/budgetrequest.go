package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetRequest captures the immutable admission identity and the lifecycle
// needed to recover an admit/forward/settle ambiguity after either process dies.
type BudgetRequest struct{ ent.Schema }

func (BudgetRequest) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("request_hash").Immutable(),
		field.String("deployment_id").Immutable(),
		field.String("user_id").Immutable(),
		field.String("interaction_id").Optional().Nillable().Immutable(),
		field.String("device_id").Optional().Nillable().Immutable(),
		field.Enum("capability").Values("MODEL", "IMAGE_GENERATION", "TTS", "ASR", "MCP").Immutable(),
		field.String("resource_id").Immutable(),
		field.String("client_protocol").Immutable(),
		field.String("upstream_id").Immutable(),
		field.Int64("managed_generation").NonNegative().Immutable(),
		field.Int64("control_revision").NonNegative().Immutable(),
		field.Int("user_budget_id").Optional().Nillable().Immutable(),
		field.Int64("budget_revision").Default(0).Immutable(),
		field.Enum("mode").Values("UNLIMITED", "LIMITED").Immutable(),
		field.Enum("source").Values("DEFAULT", "TEMPLATE", "EXPLICIT").Immutable(),
		field.Bytes("decision_json").Immutable(),
		field.Enum("state").Values("DENIED", "ADMITTED", "STARTED", "RECONCILIATION", "SETTLED", "RELEASED", "RESOLVED"),
		field.Time("admitted_at").Immutable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("last_settlement_revision").Default(0).NonNegative(),
		field.Int64("last_lifecycle_revision").Default(0).NonNegative(),
		field.String("last_lifecycle_hash").Optional().Nillable(),
		field.String("terminal_reason").Optional().Nillable(),
		field.Time("updated_at"),
	}
}

func (BudgetRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("reconciliation", BudgetReconciliation.Type).Unique(),
	}
}

func (BudgetRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "capability", "state"),
		index.Fields("state", "updated_at"),
	}
}
