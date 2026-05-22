package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// ExtensionConfig 存储某个 onebool-flow agent 的扩展配置（管理员维护）。
// 一行 = 一个 agent_id，未来可加 ppt-gen / chat 等。
//
// 删除策略：硬删除（管理员清空配置等同删除整行；无 SoftDeleteMixin）。
type ExtensionConfig struct {
	ent.Schema
}

func (ExtensionConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "extension_configs"},
	}
}

func (ExtensionConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ExtensionConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("agent_id").
			MaxLen(64).
			NotEmpty().
			Unique().
			Comment("智能体标识，对应 onebool-flow 的 agent slug（如 image-gen / ppt-gen）"),

		field.JSON("payload", domain.ExtensionConfigPayload{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Default(domain.ExtensionConfigPayload{}).
			Comment("扩展配置 JSON，强类型见 domain.ExtensionConfigPayload"),

		field.Int64("updated_by").
			Optional().
			Nillable().
			Comment("最后修改人 user_id（admin），允许为空（系统自动写入时）"),
	}
}

// Indexes：agent_id 已有 Unique 约束，无需额外索引。
