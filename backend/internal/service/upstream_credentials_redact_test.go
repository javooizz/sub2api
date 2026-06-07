package service

import "testing"

func TestRedactUpstreamCredentials(t *testing.T) {
	creds := map[string]any{
		"username":         "alice",
		"password":         "p@ss",
		"access_token":     "sk-abcdef123456",
		"user_id":          float64(42),
		"cf_clearance":     "cfc-secret",
		"cf_user_agent":    "Mozilla/5.0 ...",
		"cf_expires_at":    "2026-07-01T00:00:00Z",
		"token_expires_at": "2026-06-30T00:00:00Z",
	}
	out, status := RedactUpstreamCredentials(creds)

	for _, k := range []string{"password", "access_token", "cf_clearance", "cf_user_agent"} {
		if _, exists := out[k]; exists {
			t.Fatalf("敏感键 %s 不应出现在脱敏结果", k)
		}
	}
	if out["username"] != "alice" || out["user_id"] != float64(42) || out["cf_expires_at"] != "2026-07-01T00:00:00Z" {
		t.Fatalf("非敏感键应保留: %v", out)
	}
	if !status.HasPassword || !status.HasAccessToken {
		t.Fatalf("状态布尔不符: %+v", status)
	}
	if status.AccessTokenTail != "3456" {
		t.Fatalf("token 尾 4 位不符: %s", status.AccessTokenTail)
	}
	if creds["password"] != "p@ss" {
		t.Fatal("不应修改入参")
	}
}

func TestRedactUpstreamCredentials_ShortToken(t *testing.T) {
	out, status := RedactUpstreamCredentials(map[string]any{"access_token": "ab"})
	if _, exists := out["access_token"]; exists {
		t.Fatal("敏感键应剥离")
	}
	if status.AccessTokenTail != "" {
		t.Fatalf("过短 token 不应给尾码: %q", status.AccessTokenTail)
	}
}

func TestMergeUpstreamCredentials_PreservesSensitiveWhenOmitted(t *testing.T) {
	existing := map[string]any{
		"username": "alice", "password": "old-pass",
		"access_token": "old-token", "cf_clearance": "old-cf",
	}
	incoming := map[string]any{
		"username":     "bob",
		"access_token": "new-token",
	}
	out := MergeUpstreamCredentials(existing, incoming)
	if out["username"] != "bob" {
		t.Fatal("非敏感键应由 incoming 决定")
	}
	if out["password"] != "old-pass" {
		t.Fatal("未提供的敏感键应保留")
	}
	if out["access_token"] != "new-token" {
		t.Fatal("显式提供的敏感键应覆盖")
	}
	if out["cf_clearance"] != "old-cf" {
		t.Fatal("未提供的 cf_clearance 应保留")
	}
}
