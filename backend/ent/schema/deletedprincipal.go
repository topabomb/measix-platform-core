package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// DeletedPrincipal is a permanent security tombstone, not a user profile. User
// identifiers are never reused, and retaining only the identifier plus deletion
// time lets every retired credential fail with the same explicit protocol code.
type DeletedPrincipal struct{ ent.Schema }

func (DeletedPrincipal) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.Time("deleted_at").Immutable(),
	}
}

func (DeletedPrincipal) Indexes() []ent.Index {
	return nil
}
