//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"github.com/stretchr/testify/require"
)

// ===== fakes（窄接口 mock；fakePlazaAccounts 等见 model_plaza_models_test.go）=====

type fakePlazaGroupLister struct {
	groups []Group
	err    error
}

func (f *fakePlazaGroupLister) ListActive(context.Context) ([]Group, error) { return f.groups, f.err }

type fakePlazaChannelsRaw struct {
	channels []Channel
	err      error
}

func (f *fakePlazaChannelsRaw) ListAll(context.Context) ([]Channel, error) {
	return f.channels, f.err
}

type fakePlazaPricing struct{ byModel map[string]*LiteLLMModelPricing }

func (f *fakePlazaPricing) GetModelPricing(model string) *LiteLLMModelPricing {
	return f.byModel[model]
}

type fakePlazaGroups struct {
	groups []Group
	err    error
}

func (f *fakePlazaGroups) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return f.groups, f.err
}

type fakePlazaConfig struct {
	rec *ExtensionConfigRecord
	err error
}

func (f *fakePlazaConfig) GetAdmin(context.Context, string) (*ExtensionConfigRecord, error) {
	return f.rec, f.err
}

// plazaFixture 聚合测试夹具。
type plazaFixture struct {
	allGroups  []Group             // ListActive 返回
	accounts   map[int64][]Account // 分组 ID → 账号
	accountErr map[int64]error
	channels   []Channel // 定价目录
	litellm    map[string]*LiteLLMModelPricing
	userGroups []Group // GetAvailableGroups 返回
	cfg        *domain.ModelPlazaExtensionConfig
	now        func() time.Time
}

func newPlazaService(f plazaFixture) *ModelPlazaService {
	var rec *ExtensionConfigRecord
	if f.cfg != nil {
		rec = &ExtensionConfigRecord{
			AgentID: AgentIDModelPlaza,
			Payload: domain.ExtensionConfigPayload{ModelPlaza: f.cfg},
		}
	}
	return &ModelPlazaService{
		groups:     &fakePlazaGroupLister{groups: f.allGroups},
		accounts:   &fakePlazaAccounts{byGroup: f.accounts, errFor: f.accountErr, calls: map[int64]int{}},
		channels:   &fakePlazaChannelsRaw{channels: f.channels},
		pricing:    &fakePlazaPricing{byModel: f.litellm},
		userGroups: &fakePlazaGroups{groups: f.userGroups},
		config:     &fakePlazaConfig{rec: rec},
		now:        f.now,
	}
}

func fp(v float64) *float64 { return &v }

// tokenPricing 返回一份带 input/output 价的 token 计费定价。
func tokenPricing(in, out float64) *ChannelModelPricing {
	return &ChannelModelPricing{BillingMode: BillingModeToken, InputPrice: fp(in), OutputPrice: fp(out)}
}

// pricedChannel 构造带显式 token 价的渠道。
func pricedChannel(id int64, name, platform string, models []string, in, out float64) Channel {
	return Channel{ID: id, Name: name, Status: StatusActive, ModelPricing: []ChannelModelPricing{{
		Platform: platform, Models: models, BillingMode: BillingModeToken,
		InputPrice: fp(in), OutputPrice: fp(out),
	}}}
}

// anthropicGroup 公开标准 anthropic 分组。
func anthropicGroup(id int64, name string) Group {
	return Group{ID: id, Name: name, Platform: PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1.0}
}

// mappedAccount anthropic apikey 账号，可用模型 = models。
func mappedAccount(models ...string) Account {
	mapping := make(map[string]string, len(models))
	for _, m := range models {
		mapping[m] = m
	}
	return mkPlazaAccount(PlatformAnthropic, AccountTypeAPIKey, mapping)
}

// ===== 可见性 =====

func TestGetPlazaForUser_PublicStandardGroupVisible(t *testing.T) {
	svc := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "cc_max")},
		accounts:   map[int64][]Account{10: {mappedAccount("claude-sonnet-4-6")}},
		channels:   []Channel{pricedChannel(1, "cc_max", PlatformAnthropic, []string{"claude-sonnet-4-6"}, 9e-7, 4.5e-6)},
		userGroups: []Group{{ID: 10}},
	})
	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	m := data.Models[0]
	require.Equal(t, "claude-sonnet-4-6", m.Name)
	require.Equal(t, PlatformAnthropic, m.Platform)
	require.Len(t, m.Groups, 1)
	require.True(t, m.Groups[0].Accessible)
	require.NotNil(t, m.Pricing)
	require.Equal(t, BillingModeToken, m.BillingMode)
}

func TestGetPlazaForUser_ExclusiveGroupOnlyForAuthorized(t *testing.T) {
	g := anthropicGroup(20, "vip")
	g.IsExclusive = true
	fix := plazaFixture{
		allGroups: []Group{g},
		accounts:  map[int64][]Account{20: {mappedAccount("claude-opus-4-6")}},
	}
	// 未授权：不可见
	data, err := newPlazaService(fix).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)
	// 已授权：可见且 accessible=true
	fix.userGroups = []Group{{ID: 20}}
	data, err = newPlazaService(fix).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.True(t, data.Models[0].Groups[0].Accessible)
}

func TestGetPlazaForUser_PublicSubscriptionShowcase(t *testing.T) {
	g := anthropicGroup(30, "pro_sub")
	g.SubscriptionType = SubscriptionTypeSubscription
	fix := plazaFixture{
		allGroups: []Group{g},
		accounts:  map[int64][]Account{30: {mappedAccount("claude-opus-4-6")}},
	}
	// 未订阅：可见（橱窗）但 accessible=false
	data, err := newPlazaService(fix).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.False(t, data.Models[0].Groups[0].Accessible)
	// 已订阅（∈ GetAvailableGroups）：accessible=true
	fix.userGroups = []Group{{ID: 30}}
	data, err = newPlazaService(fix).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, data.Models[0].Groups[0].Accessible)
}

func TestGetPlazaForUser_ExclusiveSubscriptionHiddenWithoutAuth(t *testing.T) {
	g := anthropicGroup(40, "vip_sub")
	g.SubscriptionType = SubscriptionTypeSubscription
	g.IsExclusive = true
	data, err := newPlazaService(plazaFixture{
		allGroups: []Group{g},
		accounts:  map[int64][]Account{40: {mappedAccount("claude-opus-4-6")}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)
}

func TestGetPlazaForUser_ExcludedGroup(t *testing.T) {
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("m1")}},
		userGroups: []Group{{ID: 10}},
		cfg:        &domain.ModelPlazaExtensionConfig{ExcludedGroupIDs: []int64{10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)
}

func TestGetPlazaForUser_EmptyAccountGroupDisappears(t *testing.T) {
	// 账号真相源核心语义：分组无账号 → 即使渠道定价里有模型，也不展示。
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{},
		channels:   []Channel{pricedChannel(1, "ch", PlatformAnthropic, []string{"claude-opus-4-6"}, 1e-6, 5e-6)},
		userGroups: []Group{{ID: 10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)
}

func TestGetPlazaForUser_GroupAccountErrorSkipped(t *testing.T) {
	// 单分组查账号失败：跳过该分组，其余正常（规格 §6）。
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "ok"), anthropicGroup(11, "bad")},
		accounts:   map[int64][]Account{10: {mappedAccount("m1")}},
		accountErr: map[int64]error{11: context.DeadlineExceeded},
		userGroups: []Group{{ID: 10}, {ID: 11}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.Equal(t, "m1", data.Models[0].Name)
}

// ===== 聚合 =====

func TestGetPlazaForUser_ModelGroupsFromAccountsOnly(t *testing.T) {
	// 两个分组各自账号能力不同：模型的分组列表只反映账号推导，
	// 渠道 group_ids 不参与（旧实现的"合并渠道传染"问题在此杜绝）。
	g1, g2 := anthropicGroup(10, "g1"), anthropicGroup(11, "g2")
	data, err := newPlazaService(plazaFixture{
		allGroups: []Group{g1, g2},
		accounts: map[int64][]Account{
			10: {mappedAccount("shared-model", "only-g1")},
			11: {mappedAccount("shared-model")},
		},
		userGroups: []Group{{ID: 10}, {ID: 11}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 2)
	byName := map[string]PlazaModel{}
	for _, m := range data.Models {
		byName[m.Name] = m
	}
	require.Len(t, byName["shared-model"].Groups, 2)
	require.Len(t, byName["only-g1"].Groups, 1)
	require.Equal(t, "g1", byName["only-g1"].Groups[0].Name)
}

func TestGetPlazaForUser_CrossPlatformSameNameNotMerged(t *testing.T) {
	ga := anthropicGroup(10, "ga")
	gb := Group{ID: 11, Name: "gb", Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}
	accA := mappedAccount("same-name")
	accB := mkPlazaAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]string{"same-name": "x"})
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{ga, gb},
		accounts:   map[int64][]Account{10: {accA}, 11: {accB}},
		userGroups: []Group{{ID: 10}, {ID: 11}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 2, "跨平台同名模型是两条独立记录")
	for _, m := range data.Models {
		require.Len(t, m.Groups, 1)
		require.Equal(t, m.Platform, m.Groups[0].Platform)
	}
}

// ===== 定价 join =====

func TestGetPlazaForUser_ExplicitPriceNotShadowedByEarlierChannelWithoutPrice(t *testing.T) {
	// 渠道 a（名序在前）有该模型条目但没填价；渠道 b 有显式价 → b 的价胜出（审查修复④）。
	noPriceCh := Channel{ID: 1, Name: "a-ch", Status: StatusActive, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-opus-4-6"}, BillingMode: BillingModeToken,
	}}}
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("claude-opus-4-6")}},
		channels:   []Channel{noPriceCh, pricedChannel(2, "b-ch", PlatformAnthropic, []string{"claude-opus-4-6"}, 5e-6, 2.5e-5)},
		userGroups: []Group{{ID: 10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.NotNil(t, data.Models[0].Pricing)
	require.Equal(t, 5e-6, *data.Models[0].Pricing.InputPrice)
}

func TestGetPlazaForUser_ExcludedChannelPricingIgnored(t *testing.T) {
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("claude-opus-4-6")}},
		channels:   []Channel{pricedChannel(7, "ch", PlatformAnthropic, []string{"claude-opus-4-6"}, 5e-6, 2.5e-5)},
		userGroups: []Group{{ID: 10}},
		cfg:        &domain.ModelPlazaExtensionConfig{ExcludedChannelIDs: []int64{7}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1, "排除渠道只影响定价，不影响模型可见性")
	require.Nil(t, data.Models[0].Pricing)
}

func TestGetPlazaForUser_LiteLLMFallback(t *testing.T) {
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("claude-opus-4-6")}},
		litellm:    map[string]*LiteLLMModelPricing{"claude-opus-4-6": {InputCostPerToken: 5e-6, OutputCostPerToken: 2.5e-5}},
		userGroups: []Group{{ID: 10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.NotNil(t, data.Models[0].Pricing, "渠道无价时回落 LiteLLM 合成展示价")
	require.Equal(t, BillingModeToken, data.Models[0].BillingMode)
}

func TestGetPlazaForUser_NoPricingAnywhere(t *testing.T) {
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("unknown-model")}},
		userGroups: []Group{{ID: 10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.Nil(t, data.Models[0].Pricing)
	require.Equal(t, BillingModeToken, data.Models[0].BillingMode)
}

func TestGetPlazaForUser_DisplayNameFromPricingCase(t *testing.T) {
	// 账号写法 CLAUDE-OPUS-4-6，定价目录存 claude-opus-4-6 → 显示名用定价原始大小写。
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("CLAUDE-OPUS-4-6")}},
		channels:   []Channel{pricedChannel(1, "ch", PlatformAnthropic, []string{"claude-opus-4-6"}, 1e-6, 5e-6)},
		userGroups: []Group{{ID: 10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "claude-opus-4-6", data.Models[0].Name)
}

// ===== 描述注入（复合键）=====

func TestGetPlazaForUser_DescriptionCompositeKey(t *testing.T) {
	data, err := newPlazaService(plazaFixture{
		allGroups:  []Group{anthropicGroup(10, "g")},
		accounts:   map[int64][]Account{10: {mappedAccount("claude-opus-4-6")}},
		userGroups: []Group{{ID: 10}},
		cfg: &domain.ModelPlazaExtensionConfig{ModelDescriptions: map[string]string{
			"anthropic/claude-opus-4-6": "旗舰模型",
			"openai/claude-opus-4-6":    "不应命中（平台不同）",
			"claude-opus-4-6":           "不应命中（旧裸键）",
		}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "旗舰模型", data.Models[0].Description)
}

// ===== admin 模型清单 =====

func TestListAllModelIdentities_AllGroupsNoUserFilterNoBlacklist(t *testing.T) {
	exclusive := anthropicGroup(20, "vip")
	exclusive.IsExclusive = true
	svc := newPlazaService(plazaFixture{
		allGroups: []Group{anthropicGroup(10, "pub"), exclusive},
		accounts: map[int64][]Account{
			10: {mappedAccount("model-a")},
			20: {mappedAccount("model-b")},
		},
		// 排除名单不影响管理清单
		cfg: &domain.ModelPlazaExtensionConfig{ExcludedGroupIDs: []int64{10}},
	})
	ids, err := svc.ListAllModelIdentities(context.Background())
	require.NoError(t, err)
	require.Len(t, ids, 2)
	require.Equal(t, "model-a", ids[0].Name)
	require.Equal(t, "model-b", ids[1].Name)
}

// ===== 展示计费模式归一 =====

func TestGetPlazaForUser_ImageModeFallbackWithoutPerImagePriceShowsToken(t *testing.T) {
	// gpt-image 系：LiteLLM mode=image_generation 但无每张价（只有图像 token 价）。
	// 回落合成物带 image 标签却无每张价，违反"image=按张计费"语义（渠道校验同款），
	// 展示层归一为 token，避免删渠道价后计费标签翻成"按图"。
	data, err := newPlazaService(plazaFixture{
		allGroups: []Group{{ID: 10, Name: "g", Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}},
		accounts: map[int64][]Account{10: {mkPlazaAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]string{"gpt-image-2": "gpt-image-2"})}},
		litellm: map[string]*LiteLLMModelPricing{"gpt-image-2": {
			Mode: "image_generation", InputCostPerToken: 5e-6, OutputCostPerToken: 1e-5, OutputCostPerImageToken: 3e-5,
		}},
		userGroups: []Group{{ID: 10}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	m := data.Models[0]
	require.Equal(t, BillingModeToken, m.BillingMode, "无每张价的 image 合成物应按 token 展示")
	require.NotNil(t, m.Pricing)
	require.NotNil(t, m.Pricing.ImageOutputPrice, "图像 token 价保留展示")
}

func TestGetPlazaForUser_ImageModeFallbackWithPerImagePriceKeepsImage(t *testing.T) {
	// gemini image 系：LiteLLM 有 output_cost_per_image（真按张）→ 保持 image 模式。
	data, err := newPlazaService(plazaFixture{
		allGroups: []Group{{ID: 11, Name: "gi", Platform: PlatformGemini, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}},
		accounts: map[int64][]Account{11: {mkPlazaAccount(PlatformGemini, AccountTypeAPIKey, map[string]string{"gemini-3-pro-image-preview": "x"})}},
		litellm: map[string]*LiteLLMModelPricing{"gemini-3-pro-image-preview": {
			Mode: "image_generation", OutputCostPerImage: 0.00012,
		}},
		userGroups: []Group{{ID: 11}},
	}).GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	m := data.Models[0]
	require.Equal(t, BillingModeImage, m.BillingMode)
	require.NotNil(t, m.Pricing.PerRequestPrice)
	require.Equal(t, 0.00012, *m.Pricing.PerRequestPrice)
}

// ===== 图像按次展示:纯函数 =====

func TestSynthesizeImageTierPricing(t *testing.T) {
	p := synthesizeImageTierPricing(0.6, 0.6, 0.8)
	require.Equal(t, BillingModeImage, p.BillingMode)
	require.NotNil(t, p.PerRequestPrice)
	require.InDelta(t, 0.6, *p.PerRequestPrice, 1e-10) // 模型级兜底 = 1K 档
	require.Len(t, p.Intervals, 3)
	require.Equal(t, "1K", p.Intervals[0].TierLabel)
	require.Equal(t, "2K", p.Intervals[1].TierLabel)
	require.Equal(t, "4K", p.Intervals[2].TierLabel)
	require.InDelta(t, 0.6, *p.Intervals[0].PerRequestPrice, 1e-10)
	require.InDelta(t, 0.6, *p.Intervals[1].PerRequestPrice, 1e-10)
	require.InDelta(t, 0.8, *p.Intervals[2].PerRequestPrice, 1e-10)
	// MinTokens/MaxTokens 零值(0/nil):按次档位不按 context 分层(规格 §3 ②)
	require.Equal(t, 0, p.Intervals[0].MinTokens)
	require.Nil(t, p.Intervals[0].MaxTokens)
	// 合成物必须被 plazaDisplayBillingMode 识别为 image(带价档位)
	require.Equal(t, BillingModeImage, plazaDisplayBillingMode(p))
}

func TestPricingHasPerRequest(t *testing.T) {
	require.False(t, pricingHasPerRequest(nil))
	require.False(t, pricingHasPerRequest(tokenPricing(1e-6, 2e-6)))
	// image 模式但无按次价(gpt-image LiteLLM 合成物形态)→ false
	require.False(t, pricingHasPerRequest(&ChannelModelPricing{
		BillingMode: BillingModeImage, InputPrice: fp(5e-6), ImageOutputPrice: fp(3e-5),
	}))
	// flat 按次价 → true
	require.True(t, pricingHasPerRequest(&ChannelModelPricing{
		BillingMode: BillingModeImage, PerRequestPrice: fp(0.6),
	}))
	// 仅带价档位 → true
	require.True(t, pricingHasPerRequest(&ChannelModelPricing{
		BillingMode: BillingModePerRequest,
		Intervals:   []PricingInterval{{TierLabel: "1K", PerRequestPrice: fp(0.6)}},
	}))
}
