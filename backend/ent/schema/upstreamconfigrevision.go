package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UpstreamConfigRevision struct{ ent.Schema }

func (UpstreamConfigRevision) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("upstream_id"),
		field.Int64("revision"),
		field.Bytes("config_json"),
		field.String("created_by_user_id"),
		field.Time("created_at"),
	}
}
func (UpstreamConfigRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upstream_id", "revision").Unique(),
	}
}
