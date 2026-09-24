package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type ManagedState struct{ ent.Schema }

func (ManagedState) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("active_release_id").Optional().Nillable(),
		field.Int64("active_managed_generation"),
		field.Int64("desired_control_revision"),
		field.String("desired_bundle_hash").Optional().Nillable(),
		field.Int64("managed_state_revision"),
		field.String("runtime_status"),
		field.Time("updated_at"),
	}
}
