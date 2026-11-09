package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UsageEvent is the immutable request-level settlement envelope. It owns
// idempotency for the complete fact+meter payload; SemanticUsage owns the
// queryable meter projection and BudgetSettlement owns deductible deltas.
type UsageEvent struct{ ent.Schema }

func (UsageEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("request_id").Immutable(),
		field.Int64("revision").Positive().Immutable(),
		field.String("event_hash").Immutable(),
		field.String("source_event_id").Immutable(),
		field.Bytes("payload_json").Immutable(),
		field.Time("created_at").Immutable(),
	}
}

func (UsageEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "revision").Unique(),
		index.Fields("source_event_id").Unique(),
	}
}
