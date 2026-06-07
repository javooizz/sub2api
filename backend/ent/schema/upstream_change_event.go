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

// UpstreamChangeEvent 上游变更事件,只追加(spec §4.2)。
type UpstreamChangeEvent struct {
	ent.Schema
}

func (UpstreamChangeEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_change_events"}}
}

func (UpstreamChangeEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (UpstreamChangeEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id"),
		field.String("type").MaxLen(40),
		// 人读摘要,如「分组 vip 倍率 1.0 → 1.5」
		field.String("summary").SchemaType(map[string]string{dialect.Postgres: "text"}),
		// 结构化 diff(前后值);凭证类事件只存错误类别+状态码,严禁凭证值
		field.JSON("detail", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Optional(),
		field.Bool("notified").Default(false),
	}
}

func (UpstreamChangeEvent) Indexes() []ent.Index {
	return []ent.Index{
		// 游标分页:provider 维度按 (created_at DESC, id DESC)
		index.Fields("provider_id", "created_at"),
	}
}
