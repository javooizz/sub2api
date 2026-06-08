# API 参考

所有端点前缀 `/api/v1/admin`,经 `adminAuth` 中间件(JWT + 管理员校验)。响应封装 `{code:0,message:"success",data}`,失败 `{code:<HTTP>,message,reason?}`。

> 鉴权:先 `POST /api/v1/auth/login {email,password}` 拿 `data.access_token`,后续请求带 `Authorization: Bearer <access_token>`。

## 一、上游站点

### `GET /upstream-providers` — 列表

返回数组。列表为**瘦身快照**:保留分组(含模型,供前端算模型数),去掉 `model_pricing` 与 `user_info`。凭证已脱敏。

### `GET /upstream-providers/:id` — 详情

返回**全量快照**(含 `model_pricing`)。

响应字段(脱敏后):
```jsonc
{
  "id": 1, "name": "...", "type": "sub2api|newapi",
  "site_url": "...", "api_base_url": "...", "effective_api_base_url": "...",
  "status": "active|credential_error|unreachable|disabled",
  "credentials": { /* 已剥离敏感键 */ },
  "credential_status": { "has_password": true, "has_access_token": true, "access_token_tail": "CdnI" },
  "balance_threshold": 5.0, "notify_on_price_change": true,
  "refresh_interval_minutes": 60,
  "latest_snapshot": { "balance": 9.99, "currency": "USD", "groups": [...], "partial": false },
  "last_refreshed_at": "...", "last_error": "", "consecutive_failures": 0,
  "remark": "", "created_at": "..."
}
```

### `POST /upstream-providers` — 创建
### `PUT /upstream-providers/:id` — 更新

请求体:
```jsonc
{
  "name": "本站", "type": "sub2api", "site_url": "http://localhost:8090",
  "api_base_url": "",                       // 可空,空=同 site_url
  "credentials": { "username": "a@x.com", "password": "..." },  // 或 { "access_token": "..." }
  "balance_threshold": 99999999,
  "notify_on_price_change": true, "refresh_interval_minutes": 60, "remark": ""
}
```
- 凭证:账密(username+password 成组)与 access_token **至少一组**。
- 更新时凭证敏感键缺省=不修改(保留旧值);`status`/运行时字段不被 PUT 覆盖。

### `DELETE /upstream-providers/:id` — 删除

事务内级联删事件 + 清理诊断截图。

### `POST /upstream-providers/test` — 测试连接(不落库)

请求体同创建 + 可选 `provider_id`(>0 时合并存量凭证)。仅账密时会自动 Login 续期一次再采集。返回采集到的 `UpstreamSnapshot`。

### `POST /upstream-providers/:id/refresh` — 手动刷新

同步执行(可能等待过盾,最多 ~70s)。
- 成功 → 200 + 最新 provider(含全量快照)
- 刷新中 → **409** `正在刷新中,请稍候`
- 采集/过盾/凭证失败 → **400** + 友好信息(失败状态与 `last_error` 已持久化,详情可见)
- provider 不存在 → 404

### `POST /upstream-providers/:id/relogin` — 重新登录

剥离 `access_token`/`cf_*`/`token_expires_at` 后强制走自动登录/过盾路径,再触发刷新。

### `GET /upstream-providers/:id/linked-accounts` — 关联帐号

按**有效 API 地址**与系统内 `type=upstream` 账号的 `base_url` 精确匹配(scheme 相等 + host 全等 + 路径段边界前缀,防 `example.com.evil` 后缀攻击与 `/api2` 误配)。

### `GET /upstream-providers/:id/tokens` — 上游 Token 列表
### `POST /upstream-providers/:id/tokens` — 创建 Token

请求 `{ "name": "...", "group": "" }`(group 为 newapi 分组,sub2api 忽略)。响应:
```jsonc
{ "token": { "id": ..., "name": "...", "key": "sk-..." /* 明文,仅本次 */ }, "api_base_url": "..." }
```

### `GET /upstream-providers/:id/events` — 变更历史(游标分页)

query:`limit`(默认 20,上限 100)/ `before_created_at`(RFC3339)/ `before_id`。
返回(snake_case):`[{ id, provider_id, type, summary, detail, notified, created_at }]`,按 `(created_at DESC, id DESC)`。

### `GET /upstream-providers/:id/diagnostics/:file` — 失败诊断截图

`:file` 严格匹配 `^[0-9]{14}\.png$`(防路径穿越)。需带 Authorization(前端用 blob 请求,非直链)。

## 二、专项设置

### `GET /settings/upstream-management`
### `PUT /settings/upstream-management`

```jsonc
{ "browser_cdp_url": "ws://127.0.0.1:9222", "proxy_url": "http://user:***@host:port", "allow_private_webhook": false }
```
- `browser_cdp_url`:CloakBrowser CDP 地址,**留空=禁用浏览器过盾**(遇 CF 盾时上游标记失败,需人工)。
- `proxy_url`:**全局采集代理**(HTTP 采集与 CloakBrowser 过盾共用,取代旧的 per-provider `proxy_id`)。GET 响应对密码段做 `***` 脱敏(`MaskUpstreamProxyURL`);PUT 时含 `***` 占位=保留旧值,空串=清除,其余=新值(`MergeUpstreamProxyURL`)。留空=直连。
- `allow_private_webhook`:是否允许 webhook 指向私网(内网部署/本地测试时开启)。

## 三、通知渠道

### `GET /notify-channels?scope=upstream` — 列表

webhook 的 `config.headers` 值已脱敏为 `***`。

### `POST /notify-channels` — 创建
### `PUT /notify-channels/:id` — 更新

```jsonc
// email
{ "name": "运维邮件", "type": "email", "scope": "upstream", "enabled": true,
  "events": ["balance_low","price_changed"],            // 空=订阅全部
  "config": { "recipients": ["ops@x.com","dev@x.com"] } }

// webhook
{ "name": "钉钉", "type": "webhook", "scope": "upstream", "enabled": true, "events": [],
  "config": {
    "url": "https://oapi.dingtalk.com/robot/send?access_token=...",
    "headers": { "Authorization": "Bearer ..." },        // PUT 时 *** 占位保留旧值
    "body_template": "{\"msgtype\":\"text\",\"text\":{\"content\":\"{{.Title}}: {{join .Items \"; \"}}\"}}"
  } }
```
> 可订阅事件:`balance_low`/`price_changed`/`model_added`/`model_removed`/`group_added`/`group_removed`/`refresh_failed`/`credential_error`。`balance_recovered` 仅记录不通知,故不在可订阅列表。

### `DELETE /notify-channels/:id` — 删除
### `POST /notify-channels/:id/test` — 测试发送

向该渠道发一条测试通知(不经 scope/events 过滤),记录 `last_error`。webhook 指向私网时,需先在专项设置开启 `allow_private_webhook`,否则被 SSRF 拒绝(400)。
