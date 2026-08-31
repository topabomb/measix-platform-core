package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ManagedRelease struct{ ent.Schema }

func (ManagedRelease) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.Int64("managed_generation"),
		field.String("status"),
		field.Bytes("release_content_json"),
		field.Int("snapshot_schema_version"),
		field.Bytes("snapshot_json"),
		field.String("snapshot_hash"),
		field.Int64("source_draft_revision"),
		field.String("created_by_user_id"),
		field.Time("created_at"),
	}
}
func (ManagedRelease) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("managed_generation").Unique(),
	}
}
