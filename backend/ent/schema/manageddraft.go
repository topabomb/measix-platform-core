package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type ManagedDraft struct{ ent.Schema }

func (ManagedDraft) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.Int64("draft_revision"),
		field.Bytes("content_json"),
		field.String("updated_by_user_id"),
		field.Time("updated_at"),
	}
}
