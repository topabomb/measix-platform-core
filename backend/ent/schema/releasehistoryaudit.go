package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// ReleaseHistoryAudit is bounded operational history, separate from releases.
type ReleaseHistoryAudit struct{ ent.Schema }

func (ReleaseHistoryAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("actor_user_id"),
		field.String("kind"),
		field.Bytes("details_json"),
		field.Time("created_at"),
	}
}
