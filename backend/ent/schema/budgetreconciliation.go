package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetReconciliation makes unresolved forwarding/measurement state durable;
// it is never cleared merely because time passed.
type BudgetReconciliation struct{ ent.Schema }

func (BudgetReconciliation) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("request_id").Immutable(),
		field.Enum("state").Values("OPEN", "RESOLVED"),
		field.String("reason").Immutable(),
		field.Time("opened_at").Immutable(),
		field.Time("resolved_at").Optional().Nillable(),
		field.String("resolved_by_actor").Optional().Nillable(),
		field.Enum("resolution_action").Values("RELIABLE_SETTLEMENT", "CONFIRM_USAGE", "RELEASE_UNCERTAIN").Optional().Nillable(),
		field.String("resolution_reason").Optional().Nillable(),
	}
}

func (BudgetReconciliation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("request", BudgetRequest.Type).
			Ref("reconciliation").
			Field("request_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (BudgetReconciliation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "state"),
		index.Fields("state", "opened_at"),
	}
}
