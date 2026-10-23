package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetSettlement is the append-only settlement/correction history. The
// payload hash makes same-revision retries idempotent and conflicting payloads
// detectable without reapplying quantities.
type BudgetSettlement struct{ ent.Schema }

func (BudgetSettlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("request_id").Immutable(),
		field.Int64("revision").Positive().Immutable(),
		field.String("payload_hash").Immutable(),
		field.Bytes("meters_json").Immutable(),
		field.Bool("complete").Immutable(),
		field.Enum("source").Values("RELAY", "ADMIN").Immutable(),
		field.Enum("outcome").Values("SETTLED", "CORRECTED", "RECONCILIATION").Immutable(),
		field.String("reported_by").Immutable(),
		field.Time("created_at").Immutable(),
	}
}

func (BudgetSettlement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "revision").Unique(),
		index.Fields("request_id", "created_at"),
	}
}
