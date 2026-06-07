package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UpstreamProvider 上游中转站点(spec §4.1)。
// 注意:删除时由 repository 在事务内级联删除事件与诊断截图,不依赖 DB 级联。
type UpstreamProvider struct {
	ent.Schema
}

func (UpstreamProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_providers"}}
}

func (UpstreamProvider) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (UpstreamProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty(),
		// type: sub2api | newapi
		field.String("type").MaxLen(20),
		// 官网地址:登录与数据采集;允许含路径前缀(反代场景)
		field.String("site_url").MaxLen(500).NotEmpty(),
		// 模型网关地址,空 = 同 site_url;仅用于关联帐号匹配与 token 展示
		field.String("api_base_url").MaxLen(500).Optional().Default(""),
		field.String("status").MaxLen(30).Default(domain.UpstreamStatusActive),
		// 凭证:username/password/access_token/user_id/cf_clearance/... 见 spec §4.1.1
		field.JSON("credentials", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("proxy_id").Optional().Nillable(),
		field.Float("balance_threshold").Optional().Nillable(),
		field.Bool("notify_on_price_change").Default(true),
		field.Int("refresh_interval_minutes").Default(60),
		field.JSON("latest_snapshot", &domain.UpstreamSnapshot{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Optional(),
		field.Time("last_refreshed_at").Optional().Nillable(),
		field.String("last_error").Default("").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int("consecutive_failures").Default(0),
		field.Bool("balance_alerted").Default(false),
		// 刷新抢占锁(spec §7.1):非空=刷新进行中,5 分钟 stale 可重抢
		field.Time("refresh_started_at").Optional().Nillable(),
		field.String("remark").Default("").SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (UpstreamProvider) Edges() []ent.Edge {
	return []ent.Edge{
		// 同 account 惯例:可选代理
		edge.To("proxy", Proxy.Type).Unique().Field("proxy_id"),
	}
}

func (UpstreamProvider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
