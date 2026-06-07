-- 上游站点管理:上游站点 / 变更事件 / 通知渠道
-- spec: docs/superpowers/specs/2026-06-07-upstream-provider-management-design.md

CREATE TABLE IF NOT EXISTS upstream_providers (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name                     VARCHAR(100) NOT NULL,
    type                     VARCHAR(20) NOT NULL,
    site_url                 VARCHAR(500) NOT NULL,
    api_base_url             VARCHAR(500) NOT NULL DEFAULT '',
    status                   VARCHAR(30) NOT NULL DEFAULT 'active',
    credentials              JSONB NOT NULL DEFAULT '{}'::jsonb,
    proxy_id                 BIGINT NULL,
    balance_threshold        DOUBLE PRECISION NULL,
    notify_on_price_change   BOOLEAN NOT NULL DEFAULT TRUE,
    refresh_interval_minutes INTEGER NOT NULL DEFAULT 60,
    latest_snapshot          JSONB NULL,
    last_refreshed_at        TIMESTAMPTZ NULL,
    last_error               TEXT NOT NULL DEFAULT '',
    consecutive_failures     INTEGER NOT NULL DEFAULT 0,
    balance_alerted          BOOLEAN NOT NULL DEFAULT FALSE,
    refresh_started_at       TIMESTAMPTZ NULL,
    remark                   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_upstream_providers_status ON upstream_providers (status);

CREATE TABLE IF NOT EXISTS upstream_change_events (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider_id BIGINT NOT NULL,
    type        VARCHAR(40) NOT NULL,
    summary     TEXT NOT NULL DEFAULT '',
    detail      JSONB NULL,
    notified    BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_upstream_change_events_provider_created
    ON upstream_change_events (provider_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notify_channels (
    id           BIGSERIAL PRIMARY KEY,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name         VARCHAR(100) NOT NULL,
    type         VARCHAR(20) NOT NULL,
    scope        VARCHAR(40) NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    events       JSONB NULL,
    config       JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_sent_at TIMESTAMPTZ NULL,
    last_error   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_notify_channels_scope ON notify_channels (scope);
