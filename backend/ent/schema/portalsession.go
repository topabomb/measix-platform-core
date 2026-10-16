package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// PortalSession is a short-lived grant and its one-time cookie transition.
// Parent identity is always revalidated; these rows never extend idle expiry.
type PortalSession struct{ ent.Schema }

func (PortalSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("session_id").Immutable(),
		field.String("origin").Immutable(),
		field.Bytes("ticket_digest").Unique().Immutable(),
		field.Bytes("cookie_digest").Unique().Optional().Nillable(),
		field.Time("grant_expires_at").Immutable(),
		field.Time("expires_at"),
		field.Bool("consumed").Default(false),
		field.Bool("revoked").Default(false),
	}
}
