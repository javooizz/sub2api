//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// mkPlazaAccount 构造推导测试用账号。mapping=nil 表示未配置 model_mapping。
func mkPlazaAccount(platform, typ string, mapping map[string]string) Account {
	creds := map[string]any{}
	if mapping != nil {
		mm := make(map[string]any, len(mapping))
		for k, v := range mapping {
			mm[k] = v
		}
		creds["model_mapping"] = mm
	}
	return Account{Platform: platform, Type: typ, Status: StatusActive, Schedulable: true, Credentials: creds}
}

func TestAccountUsableModelEntries(t *testing.T) {
	anthropicDefaults := defaultModelsListCandidateIDs(PlatformAnthropic)
	geminiDefaults := defaultModelsListCandidateIDs(PlatformGemini)
	openaiDefaults := defaultModelsListCandidateIDs(PlatformOpenAI)
	antigravityDefaults := defaultModelsListCandidateIDs(PlatformAntigravity)
	require.NotEmpty(t, anthropicDefaults)

	cases := []struct {
		name string
		acc  Account
		want []string
	}{
		// anthropic/gemini：OAuth 判定先于 mapping（即使配了 mapping 也返回默认集，
		// 与 admin/account_handler.go GetAvailableModels 一致——规格 §4.2 步骤 2）
		{"anthropic OAuth 无 mapping → 默认集", mkPlazaAccount(PlatformAnthropic, AccountTypeOAuth, nil), anthropicDefaults},
		{"anthropic OAuth 配了 mapping → 仍默认集", mkPlazaAccount(PlatformAnthropic, AccountTypeOAuth, map[string]string{"x": "y"}), anthropicDefaults},
		{"anthropic setup-token 视同 OAuth → 默认集", mkPlazaAccount(PlatformAnthropic, AccountTypeSetupToken, map[string]string{"x": "y"}), anthropicDefaults},
		{"anthropic apikey 有 mapping → mapping 键（排序）", mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"b-model": "u", "a-model": "v"}), []string{"a-model", "b-model"}},
		{"anthropic apikey 无 mapping → 默认集", mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, nil), anthropicDefaults},
		{"gemini OAuth 配了 mapping → 仍默认集", mkPlazaAccount(PlatformGemini, AccountTypeOAuth, map[string]string{"g": "h"}), geminiDefaults},
		{"gemini apikey 有 mapping → mapping 键", mkPlazaAccount(PlatformGemini, AccountTypeAPIKey, map[string]string{"gemini-x": "y"}), []string{"gemini-x"}},
		// openai：passthrough 先于 mapping
		{"openai apikey 有 mapping → mapping 键", mkPlazaAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]string{"gpt-z": "w"}), []string{"gpt-z"}},
		{"openai 无 mapping → 默认集", mkPlazaAccount(PlatformOpenAI, AccountTypeOAuth, nil), openaiDefaults},
		// antigravity：始终默认集（忽略 mapping）
		{"antigravity 配了 mapping → 仍默认集", mkPlazaAccount(PlatformAntigravity, AccountTypeAPIKey, map[string]string{"m": "n"}), antigravityDefaults},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, accountUsableModelEntries(&tc.acc))
		})
	}
}

func TestAccountUsableModelEntries_OpenAIPassthrough(t *testing.T) {
	// passthrough 绕过 mapping 改写 → 即使配 mapping 也返回默认集。
	// IsOpenAIPassthroughEnabled 读 a.Extra（不是 a.Credentials），需设置 Extra 字段。
	acc := mkPlazaAccount(PlatformOpenAI, AccountTypeOAuth, map[string]string{"gpt-z": "w"})
	acc.Extra = map[string]any{
		"openai_oauth_passthrough": true, // 兼容字段，等价于 openai_passthrough
	}
	require.True(t, acc.IsOpenAIPassthroughEnabled(), "fixture 必须真的开启 passthrough")
	require.Equal(t, defaultModelsListCandidateIDs(PlatformOpenAI), accountUsableModelEntries(&acc))
}

// TestPlazaFilterModelsByCustomList 钉住与 gateway_handler.go filterModelsByCustomList
// 的等价语义（规格 §4.2：结果=圈定清单中被 available 模式允许的项，保序去重去空；
// * 尾缀前缀通配；available 空时回落 fallback 作 source）。
func TestPlazaFilterModelsByCustomList(t *testing.T) {
	cases := []struct {
		name                          string
		available, fallback, selected []string
		want                          []string
	}{
		{"未配圈定 → 原样返回 available", []string{"a", "b"}, nil, nil, []string{"a", "b"}},
		{"结果取圈定清单且保序", []string{"m1", "m2", "m3"}, nil, []string{"m3", "m1"}, []string{"m3", "m1"}},
		{"圈定含 available 没有的 → 剔除", []string{"m1"}, nil, []string{"m1", "mx"}, []string{"m1"}},
		{"通配 allow：available 含 claude-* 放行前缀匹配", []string{"claude-*"}, nil, []string{"claude-opus-4-6", "gpt-5.2"}, []string{"claude-opus-4-6"}},
		{"圈定去重去空", []string{"m1"}, nil, []string{" m1 ", "m1", ""}, []string{"m1"}},
		{"available 空 → fallback 作 source", nil, []string{"d1"}, []string{"d1", "d2"}, []string{"d1"}},
		{"available 与 fallback 均空 → nil", nil, nil, []string{"x"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, plazaFilterModelsByCustomList(tc.available, tc.fallback, tc.selected))
		})
	}
}
