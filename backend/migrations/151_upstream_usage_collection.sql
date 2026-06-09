-- 上游消耗采集地基:充值比例 + 按日 rollup + 采集游标
-- spec: docs/superpowers/specs/2026-06-09-上游消耗采集地基-design.md

-- 充值比例 N(1:N,¥1 充得 $N 额度);真实成本¥ = cost_usd ÷ N
ALTER TABLE upstream_providers
    ADD COLUMN IF NOT EXISTS recharge_ratio DOUBLE PRECISION NOT NULL DEFAULT 1;

-- 按日 × 维度消耗 rollup
CREATE TABLE IF NOT EXISTS upstream_usage_daily (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider_id BIGINT NOT NULL,
    day         TIMESTAMPTZ NOT NULL,
    scope_type  VARCHAR(16) NOT NULL,
    scope_key   VARCHAR(128) NOT NULL DEFAULT '',
    scope_name  VARCHAR(200) NOT NULL DEFAULT '',
    requests    INTEGER NOT NULL DEFAULT 0,
    tokens      BIGINT NOT NULL DEFAULT 0,
    cost_usd    DECIMAL(20,10) NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_upstream_usage_daily_scope
    ON upstream_usage_daily (provider_id, day, scope_type, scope_key);
CREATE INDEX IF NOT EXISTS idx_upstream_usage_daily_query
    ON upstream_usage_daily (provider_id, scope_type, day);

-- 采集游标 + 乐观租约(每 provider 一行)
CREATE TABLE IF NOT EXISTS upstream_usage_cursor (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider_id           BIGINT NOT NULL,
    collect_started_at    TIMESTAMPTZ NULL,
    collected_through_day TIMESTAMPTZ NULL,
    backfill_done         BOOLEAN NOT NULL DEFAULT FALSE,
    backfill_oldest_day   TIMESTAMPTZ NULL,
    last_collected_at     TIMESTAMPTZ NULL,
    last_error            TEXT NOT NULL DEFAULT '',
    last_partial          BOOLEAN NOT NULL DEFAULT FALSE,
    partial_reason        VARCHAR(200) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_upstream_usage_cursor_provider
    ON upstream_usage_cursor (provider_id);
