-- 上游站点管理:移除 per-provider 网络代理存储
-- 代理统一改由「采集设置」(setting key: upstream_proxy_url) 全局配置,
-- 同时供 HTTP 采集与 CloakBrowser 过盾使用。详见 docs/上游管理。
-- 迁移 149 的 proxy_id 列无外键约束,直接删除即可(幂等)。

ALTER TABLE upstream_providers DROP COLUMN IF EXISTS proxy_id;
