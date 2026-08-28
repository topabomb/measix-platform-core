package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type EnterpriseUpdate struct{ ent.Schema }

func (EnterpriseUpdate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("title"),
		field.Text("content"),
		field.String("status"),
		field.Time("published_at").Optional().Nillable(),
		field.Int64("feed_revision"),
		field.String("created_by_user_id"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (EnterpriseUpdate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("feed_revision"),
		index.Fields("published_at"),
	}
}
