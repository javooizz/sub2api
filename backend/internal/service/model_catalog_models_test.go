//go:build unit

package service

import (
	"context"
	"testing"
	"time"

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

// ===== Task 3: groupUsableModels + TTL 缓存 测试 =====

type fakePlazaAccounts struct {
	byGroup map[int64][]Account
	errFor  map[int64]error
	calls   map[int64]int
}

func (f *fakePlazaAccounts) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	f.calls[groupID]++
	if err := f.errFor[groupID]; err != nil {
		return nil, err
	}
	return f.byGroup[groupID], nil
}

func newUsableFixture(accounts map[int64][]Account, errFor map[int64]error, now func() time.Time) (*ModelCatalogService, *fakePlazaAccounts) {
	fa := &fakePlazaAccounts{byGroup: accounts, errFor: errFor, calls: map[int64]int{}}
	return &ModelCatalogService{accounts: fa, now: now}, fa
}

func TestGroupUsableModels_UnionAndSampling(t *testing.T) {
	mapped := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"custom-claude": "x"})
	oauth := mkPlazaAccount(PlatformAnthropic, AccountTypeOAuth, nil)
	offPlatform := mkPlazaAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]string{"gpt-x": "y"})
	notSchedulable := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"hidden": "z"})
	notSchedulable.Schedulable = false
	inactive := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"dead": "z"})
	inactive.Status = StatusDisabled // 非 active

	svc, _ := newUsableFixture(map[int64][]Account{
		1: {mapped, oauth, offPlatform, notSchedulable, inactive},
	}, nil, nil)
	g := &Group{ID: 1, Platform: PlatformAnthropic}

	got, err := svc.groupUsableModels(context.Background(), g)
	require.NoError(t, err)
	// 混合分组：OAuth 默认模型不被吞（与网关 GetAvailableModels 的局限相反）
	require.Contains(t, got, "custom-claude")
	for _, def := range defaultModelsListCandidateIDs(PlatformAnthropic) {
		require.Contains(t, got, def)
	}
	// 平台不符 / 不可调度 / 非 active 账号全部忽略
	require.NotContains(t, got, "gpt-x")
	require.NotContains(t, got, "hidden")
	require.NotContains(t, got, "dead")
}

func TestGroupUsableModels_WildcardSkippedFromDisplay(t *testing.T) {
	acc := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"claude-*": "x", "exact-model": "y"})
	svc, _ := newUsableFixture(map[int64][]Account{1: {acc}}, nil, nil)

	got, err := svc.groupUsableModels(context.Background(), &Group{ID: 1, Platform: PlatformAnthropic})
	require.NoError(t, err)
	require.Equal(t, []string{"exact-model"}, got) // 通配键不进展示集
}

func TestGroupUsableModels_ModelAllowlistFilter(t *testing.T) {
	acc := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"claude-*": "x", "other": "y"})
	svc, _ := newUsableFixture(map[int64][]Account{1: {acc}}, nil, nil)
	g := &Group{ID: 1, Platform: PlatformAnthropic,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"claude-opus-4-6", "not-allowed"}}}

	got, err := svc.groupUsableModels(context.Background(), g)
	require.NoError(t, err)
	// 圈定项 claude-opus-4-6 被通配键 claude-* 放行；not-allowed 无模式匹配被剔除；
	// 未被圈定的 other 不出现（非朴素交集：结果以圈定清单为基）
	require.Equal(t, []string{"claude-opus-4-6"}, got)
}

func TestGroupUsableModels_ModelAllowlistModes(t *testing.T) {
	acc := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{
		"claude-opus-4-6": "x", "claude-sonnet-4-6": "y", "other": "z",
	})
	for _, tc := range []struct {
		name      string
		allowlist GroupModelAllowlist
		want      []string
	}{
		{
			name:      "disabled allowlist leaves available models visible",
			allowlist: GroupModelAllowlist{Models: []string{"other"}},
			want:      []string{"claude-opus-4-6", "claude-sonnet-4-6", "other"},
		},
		{
			name:      "wildcard expands case-insensitively in allowlist order without duplicates",
			allowlist: GroupModelAllowlist{Enabled: true, Models: []string{"other", "CLAUDE-*", "claude-opus-4-6"}},
			want:      []string{"other", "claude-opus-4-6", "claude-sonnet-4-6"},
		},
		{
			name:      "enabled empty allowlist hides all models",
			allowlist: GroupModelAllowlist{Enabled: true},
			want:      []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newUsableFixture(map[int64][]Account{1: {acc}}, nil, nil)
			got, err := svc.groupUsableModels(context.Background(), &Group{
				ID: 1, Platform: PlatformAnthropic, ModelAllowlist: tc.allowlist,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGroupUsableModels_EmptyAccounts(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		svc, _ := newUsableFixture(map[int64][]Account{}, nil, nil)
		got, err := svc.groupUsableModels(context.Background(), &Group{
			ID: 9, Platform: PlatformAnthropic,
			ModelAllowlist: GroupModelAllowlist{Enabled: enabled, Models: []string{"claude-opus-4-6"}},
		})
		require.NoError(t, err)
		require.Empty(t, got, "groups without accounts must not display default models, allowlist enabled=%t", enabled)
	}
}

func TestGroupUsableModels_TTLCache(t *testing.T) {
	acc := mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]string{"m1": "x"})
	current := time.Unix(1000, 0)
	svc, fa := newUsableFixture(map[int64][]Account{1: {acc}}, nil, func() time.Time { return current })
	g := &Group{ID: 1, Platform: PlatformAnthropic}
	ctx := context.Background()

	_, err := svc.groupUsableModels(ctx, g)
	require.NoError(t, err)
	_, err = svc.groupUsableModels(ctx, g)
	require.NoError(t, err)
	require.Equal(t, 1, fa.calls[1], "TTL 内不重查账号")

	current = current.Add(catalogUsableTTL + time.Second)
	_, err = svc.groupUsableModels(ctx, g)
	require.NoError(t, err)
	require.Equal(t, 2, fa.calls[1], "过期后重查")
}

func TestGroupUsableModels_ErrorNotCached(t *testing.T) {
	svc, fa := newUsableFixture(nil, map[int64]error{1: context.DeadlineExceeded}, nil)
	g := &Group{ID: 1, Platform: PlatformAnthropic}
	_, err := svc.groupUsableModels(context.Background(), g)
	require.Error(t, err)
	_, err = svc.groupUsableModels(context.Background(), g)
	require.Error(t, err)
	require.Equal(t, 2, fa.calls[1], "失败不缓存")
}
