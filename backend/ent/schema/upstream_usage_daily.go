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

// UpstreamUsageDaily 上游消耗按日 × 维度 rollup。
// 删除由 upstream_provider_repo.Delete 在事务内级联,不依赖 DB FK。
type UpstreamUsageDaily struct {
	ent.Schema
}

func (UpstreamUsageDaily) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_usage_daily"}}
}

func (UpstreamUsageDaily) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (UpstreamUsageDaily) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id"),
		field.Time("day").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("scope_type").MaxLen(16),
		field.String("scope_key").MaxLen(128).Default(""),
		field.String("scope_name").MaxLen(200).Default(""),
		field.Int("requests").Default(0),
		field.Int64("tokens").Default(0),
		field.Float("cost_usd").Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
	}
}

func (UpstreamUsageDaily) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "day", "scope_type", "scope_key").Unique(),
		index.Fields("provider_id", "scope_type", "day"),
	}
}
