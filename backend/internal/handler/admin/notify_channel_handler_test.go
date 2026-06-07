package admin

import "testing"

func TestRedactNotifyChannelConfig_WebhookHeadersMasked(t *testing.T) {
	cfg := map[string]any{
		"url":     "https://oapi.dingtalk.com/robot/send?access_token=xxx",
		"headers": map[string]any{"Authorization": "Bearer secret", "X-Custom": "v"},
	}
	out := redactNotifyChannelConfig("webhook", cfg)

	headers, ok := out["headers"].(map[string]any)
	if !ok {
		t.Fatal("headers 应保留键名")
	}
	if headers["Authorization"] != "***" || headers["X-Custom"] != "***" {
		t.Fatalf("headers 值应替换为 ***,实际: %v", headers)
	}
	if out["url"] != cfg["url"] {
		t.Fatal("url 不应脱敏")
	}
	if cfg["headers"].(map[string]any)["Authorization"] != "Bearer secret" {
		t.Fatal("不应修改入参")
	}
}

func TestMergeNotifyChannelConfig_HeadersPreservedWhenOmitted(t *testing.T) {
	existing := map[string]any{
		"url":     "https://x.com/hook",
		"headers": map[string]any{"Authorization": "Bearer secret"},
	}
	incoming := map[string]any{
		"url":     "https://x.com/hook2",
		"headers": map[string]any{"Authorization": "***"},
	}
	out := mergeNotifyChannelConfig(existing, incoming)
	if out["url"] != "https://x.com/hook2" {
		t.Fatal("非敏感字段应被覆盖")
	}
	if out["headers"].(map[string]any)["Authorization"] != "Bearer secret" {
		t.Fatalf("*** 值应保留旧 header,实际: %v", out["headers"])
	}
}
