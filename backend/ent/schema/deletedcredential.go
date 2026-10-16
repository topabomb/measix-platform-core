package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// DeletedCredential is a permanent, non-reversible refresh-token digest
// tombstone. User-owned sessions are purged, while this minimum deny fact lets
// a retired credential fail explicitly instead of becoming indistinguishable
// from malformed input.
type DeletedCredential struct{ ent.Schema }

func (DeletedCredential) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("digest").Immutable().Unique(),
		field.Time("deleted_at").Immutable(),
	}
}

func (DeletedCredential) Indexes() []ent.Index {
	return nil
}
