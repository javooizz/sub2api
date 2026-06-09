package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UpstreamUsageCursor 每 provider 一行的采集游标 + 乐观租约。
type UpstreamUsageCursor struct {
	ent.Schema
}

func (UpstreamUsageCursor) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_usage_cursor"}}
}

func (UpstreamUsageCursor) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (UpstreamUsageCursor) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id").Unique(),
		field.Time("collect_started_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("collected_through_day").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Bool("backfill_done").Default(false),
		field.Time("backfill_oldest_day").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_collected_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error").Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Bool("last_partial").Default(false),
		field.String("partial_reason").MaxLen(200).Default(""),
	}
}

func (UpstreamUsageCursor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id").Unique(),
	}
}
