-- OneBoolFlow agents 扩展配置（管理员维护）
-- 协议参考：onebool-flow/docs/integration-protocol.md §9

CREATE TABLE IF NOT EXISTS extension_configs (
    id          BIGSERIAL PRIMARY KEY,
    agent_id    VARCHAR(64)  NOT NULL UNIQUE,
    payload     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    updated_by  BIGINT       NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE  extension_configs              IS 'OneBoolFlow agents 扩展配置（管理员维护，单一真相源）';
COMMENT ON COLUMN extension_configs.agent_id     IS '智能体 slug，对应 onebool-flow 的 agent id（如 image-gen）';
COMMENT ON COLUMN extension_configs.payload      IS 'jsonb 强类型，结构见 backend/internal/domain/extension_config.go (ExtensionConfigPayload)';
COMMENT ON COLUMN extension_configs.updated_by   IS '最后修改人 user_id，允许 NULL';
