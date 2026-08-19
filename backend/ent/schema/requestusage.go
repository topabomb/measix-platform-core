package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RequestUsage struct{ ent.Schema }

func (RequestUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("request_id"),
		field.String("interaction_id").Optional().Nillable(),
		field.String("deployment_id"),
		field.String("user_id"),
		field.String("device_id").Optional().Nillable(),
		field.String("resource_id"),
		field.String("runtime_route_id"),
		field.String("upstream_id"),
		field.Int64("managed_generation"),
		field.Int64("control_revision"),
		field.Time("started_at"),
		field.Time("completed_at"),
		field.Bool("forwarded"),
		field.Int("http_status"),
		field.Int("upstream_http_status").Optional().Nillable(),
		field.Int64("request_bytes"),
		field.Int64("response_bytes"),
		field.Int64("duration_ms"),
		field.String("error_class").Optional().Nillable(),
		field.Time("ingested_at"),
	}
}
func (RequestUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id").Unique(),
		index.Fields("completed_at", "user_id"),
		index.Fields("completed_at", "resource_id"),
	}
}
