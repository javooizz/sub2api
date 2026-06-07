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

// NotifyChannel 通知渠道(spec §4.3)。scope 目前固定 "upstream",
// 实体设计为通用,后续其他模块复用时不动表结构。
type NotifyChannel struct {
	ent.Schema
}

func (NotifyChannel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "notify_channels"}}
}

func (NotifyChannel) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (NotifyChannel) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty(),
		// email | webhook
		field.String("type").MaxLen(20),
		field.String("scope").MaxLen(40),
		field.Bool("enabled").Default(true),
		// 订阅的事件类型;空数组 = 订阅全部
		field.JSON("events", []string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Optional(),
		// email: {recipients: [...]} / webhook: {url, headers?, body_template?}
		field.JSON("config", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("last_sent_at").Optional().Nillable(),
		field.String("last_error").Default("").SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (NotifyChannel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope"),
	}
}
