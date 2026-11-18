package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DeploymentSettingAudit preserves the operator and before/after value for
// every mutable deployment-wide setting change.
type DeploymentSettingAudit struct{ ent.Schema }

func (DeploymentSettingAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("deployment_id"),
		field.String("actor_user_id"),
		field.String("old_name"),
		field.String("new_name"),
		field.String("old_public_origin").Default(""),
		field.String("new_public_origin").Default(""),
		field.Time("created_at"),
	}
}

func (DeploymentSettingAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deployment_id", "created_at"),
	}
}
