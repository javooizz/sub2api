//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"github.com/stretchr/testify/require"
)

// ===== fakes（窄接口 mock）=====

type fakePlazaChannels struct {
	channels []AvailableChannel
	err      error
}

func (f *fakePlazaChannels) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	return f.channels, f.err
}

type fakePlazaGroups struct {
	groups []Group
	err    error
}

func (f *fakePlazaGroups) GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error) {
	return f.groups, f.err
}

type fakePlazaConfig struct {
	rec *ExtensionConfigRecord
	err error
}

func (f *fakePlazaConfig) GetAdmin(ctx context.Context, agentID string) (*ExtensionConfigRecord, error) {
	return f.rec, f.err
}

func newPlazaService(ch []AvailableChannel, userGroups []Group, cfg *domain.ModelPlazaExtensionConfig) *ModelPlazaService {
	var rec *ExtensionConfigRecord
	if cfg != nil {
		rec = &ExtensionConfigRecord{
			AgentID: AgentIDModelPlaza,
			Payload: domain.ExtensionConfigPayload{ModelPlaza: cfg},
		}
	}
	return &ModelPlazaService{
		channels: &fakePlazaChannels{channels: ch},
		groups:   &fakePlazaGroups{groups: userGroups},
		config:   &fakePlazaConfig{rec: rec},
	}
}

func fp(v float64) *float64 { return &v }

// tokenPricing 返回一份带 input/output 价的 token 计费定价。
func tokenPricing(in, out float64) *ChannelModelPricing {
	return &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  fp(in),
		OutputPrice: fp(out),
	}
}

// ===== 可见性 =====

func TestGetPlazaForUser_PublicStandardGroupVisible(t *testing.T) {
	// 公开标准分组在 GetAvailableGroups 内（user 可绑定）→ 可见且 accessible=true。
	ch := []AvailableChannel{{
		ID: 1, Name: "cc_max", Status: StatusActive,
		Groups: []AvailableGroupRef{
			{ID: 10, Name: "cc_max", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1.8},
		},
		SupportedModels: []SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(9e-7, 4.5e-6)},
		},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	m := data.Models[0]
	require.Equal(t, "claude-sonnet-4-6", m.Name)
	require.Equal(t, "anthropic", m.Platform)
	require.Len(t, m.Groups, 1)
	require.True(t, m.Groups[0].Accessible)
	require.NotNil(t, m.Pricing)
	require.Equal(t, BillingModeToken, m.BillingMode)
}

func TestGetPlazaForUser_ExclusiveGroupOnlyForAuthorized(t *testing.T) {
	// 专属标准分组：不在 GetAvailableGroups → 整个分组（连带模型）不可见。
	ch := []AvailableChannel{{
		ID: 1, Name: "vip", Status: StatusActive,
		Groups: []AvailableGroupRef{
			{ID: 20, Name: "vip", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard, IsExclusive: true},
		},
		SupportedModels: []SupportedModel{
			{Name: "claude-opus-4-6", Platform: "anthropic", Pricing: tokenPricing(1.5e-6, 7.5e-6)},
		},
	}}
	// 未授权：空 user groups
	svc := newPlazaService(ch, nil, nil)
	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)

	// 已授权：GetAvailableGroups 含 20
	svc = newPlazaService(ch, []Group{{ID: 20}}, nil)
	data, err = svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.True(t, data.Models[0].Groups[0].Accessible)
}

func TestGetPlazaForUser_PublicSubscriptionGroupVisibleWithoutSub(t *testing.T) {
	// 公开订阅型分组：未订阅（不在 GetAvailableGroups）→ 仍可见（橱窗），accessible=false。
	ch := []AvailableChannel{{
		ID: 1, Name: "sub_ch", Status: StatusActive,
		Groups: []AvailableGroupRef{
			{ID: 30, Name: "pro_sub", Platform: "openai", SubscriptionType: SubscriptionTypeSubscription, IsExclusive: false, RateMultiplier: 0.5},
		},
		SupportedModels: []SupportedModel{
			{Name: "gpt-5.2", Platform: "openai", Pricing: tokenPricing(1.75e-7, 1.4e-6)},
		},
	}}
	svc := newPlazaService(ch, nil, nil)
	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.Len(t, data.Models[0].Groups, 1)
	require.False(t, data.Models[0].Groups[0].Accessible)

	// 已订阅 → accessible=true
	svc = newPlazaService(ch, []Group{{ID: 30}}, nil)
	data, err = svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, data.Models[0].Groups[0].Accessible)
}

func TestGetPlazaForUser_ExclusiveSubscriptionGroupHiddenWithoutAuth(t *testing.T) {
	// 专属订阅型分组：未订阅未授权 → 不可见（不走公开订阅型分支）。
	ch := []AvailableChannel{{
		ID: 1, Name: "vip_sub", Status: StatusActive,
		Groups: []AvailableGroupRef{
			{ID: 40, Name: "vip_sub", Platform: "openai", SubscriptionType: SubscriptionTypeSubscription, IsExclusive: true},
		},
		SupportedModels: []SupportedModel{
			{Name: "gpt-5.5", Platform: "openai", Pricing: tokenPricing(5e-7, 3e-6)},
		},
	}}
	svc := newPlazaService(ch, nil, nil)
	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)
}

// ===== 黑名单 =====

func TestGetPlazaForUser_ExcludeListsApply(t *testing.T) {
	groups := []AvailableGroupRef{
		{ID: 10, Name: "g10", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard},
		{ID: 11, Name: "g11", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard},
	}
	models := []SupportedModel{
		{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(9e-7, 4.5e-6)},
	}
	ch := []AvailableChannel{
		{ID: 1, Name: "ch1", Status: StatusActive, Groups: groups, SupportedModels: models},
		{ID: 2, Name: "ch2", Status: StatusActive, Groups: groups, SupportedModels: []SupportedModel{
			{Name: "claude-opus-4-6", Platform: "anthropic", Pricing: tokenPricing(1.5e-6, 7.5e-6)},
		}},
	}
	cfg := &domain.ModelPlazaExtensionConfig{
		ExcludedChannelIDs: []int64{2},  // 排除 ch2 → opus 不出现
		ExcludedGroupIDs:   []int64{11}, // 排除 g11 → 模型只挂 g10
	}
	svc := newPlazaService(ch, []Group{{ID: 10}, {ID: 11}}, cfg)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.Equal(t, "claude-sonnet-4-6", data.Models[0].Name)
	require.Len(t, data.Models[0].Groups, 1)
	require.Equal(t, int64(10), data.Models[0].Groups[0].ID)
}

func TestGetPlazaForUser_InactiveChannelSkipped(t *testing.T) {
	ch := []AvailableChannel{{
		ID: 1, Name: "disabled", Status: "disabled",
		Groups: []AvailableGroupRef{{ID: 10, Name: "g", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard}},
		SupportedModels: []SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(9e-7, 4.5e-6)},
		},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil)
	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, data.Models)
}

// ===== 聚合身份 =====

func TestGetPlazaForUser_MergesSameModelAcrossChannels(t *testing.T) {
	// 同 (platform, name) 跨渠道 → 一条记录，分组并集；大小写不敏感合并。
	g1 := AvailableGroupRef{ID: 10, Name: "g10", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard}
	g2 := AvailableGroupRef{ID: 11, Name: "g11", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard}
	ch := []AvailableChannel{
		{ID: 1, Name: "a_ch", Status: StatusActive, Groups: []AvailableGroupRef{g1}, SupportedModels: []SupportedModel{
			{Name: "Claude-Sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(9e-7, 4.5e-6)},
		}},
		{ID: 2, Name: "b_ch", Status: StatusActive, Groups: []AvailableGroupRef{g2}, SupportedModels: []SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(1e-6, 5e-6)},
		}},
	}
	svc := newPlazaService(ch, []Group{{ID: 10}, {ID: 11}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	m := data.Models[0]
	require.Len(t, m.Groups, 2)
	// 基准价取渠道名序第一个有可展示定价的（a_ch）
	require.Equal(t, 9e-7, *m.Pricing.InputPrice)
}

func TestGetPlazaForUser_CrossPlatformSameNameNotMerged(t *testing.T) {
	// 同名不同平台 → 两条独立记录。
	ch := []AvailableChannel{{
		ID: 1, Name: "multi", Status: StatusActive,
		Groups: []AvailableGroupRef{
			{ID: 10, Name: "g-ant", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard},
			{ID: 11, Name: "g-gem", Platform: "gemini", SubscriptionType: SubscriptionTypeStandard},
		},
		SupportedModels: []SupportedModel{
			{Name: "shared-model", Platform: "anthropic", Pricing: tokenPricing(1e-6, 2e-6)},
			{Name: "shared-model", Platform: "gemini", Pricing: tokenPricing(3e-7, 6e-7)},
		},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}, {ID: 11}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 2)
	// platform 排序：anthropic 在前
	require.Equal(t, "anthropic", data.Models[0].Platform)
	require.Equal(t, "gemini", data.Models[1].Platform)
	// 各自只挂本平台分组
	require.Equal(t, int64(10), data.Models[0].Groups[0].ID)
	require.Equal(t, int64(11), data.Models[1].Groups[0].ID)
}

func TestGetPlazaForUser_CrossPlatformLeakPrevention(t *testing.T) {
	// 渠道同时有 anthropic/openai 模型，但用户可见分组只覆盖 anthropic → openai 模型剔除。
	ch := []AvailableChannel{{
		ID: 1, Name: "mixed", Status: StatusActive,
		Groups: []AvailableGroupRef{
			{ID: 10, Name: "g-ant", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard},
			{ID: 11, Name: "g-oai", Platform: "openai", IsExclusive: true, SubscriptionType: SubscriptionTypeStandard},
		},
		SupportedModels: []SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(9e-7, 4.5e-6)},
			{Name: "gpt-5.2", Platform: "openai", Pricing: tokenPricing(1.75e-7, 1.4e-6)},
		},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil) // 只授权 anthropic 分组

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.Equal(t, "claude-sonnet-4-6", data.Models[0].Name)
}

// ===== 基准价选取 =====

func TestGetPlazaForUser_PricingSelection(t *testing.T) {
	g := AvailableGroupRef{ID: 10, Name: "g", Platform: "openai", SubscriptionType: SubscriptionTypeStandard}
	// a_ch 的定价全空（needsFallback）→ 跳过；b_ch 是 per_request 有价 → 选中。
	emptyPricing := &ChannelModelPricing{BillingMode: BillingModeToken}
	perReq := &ChannelModelPricing{BillingMode: BillingModePerRequest, PerRequestPrice: fp(0.08)}
	ch := []AvailableChannel{
		{ID: 1, Name: "a_ch", Status: StatusActive, Groups: []AvailableGroupRef{g}, SupportedModels: []SupportedModel{
			{Name: "gpt-image-2", Platform: "openai", Pricing: emptyPricing},
		}},
		{ID: 2, Name: "b_ch", Status: StatusActive, Groups: []AvailableGroupRef{g}, SupportedModels: []SupportedModel{
			{Name: "gpt-image-2", Platform: "openai", Pricing: perReq},
		}},
	}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	m := data.Models[0]
	require.NotNil(t, m.Pricing)
	require.Equal(t, 0.08, *m.Pricing.PerRequestPrice)
	require.Equal(t, BillingModePerRequest, m.BillingMode)
}

func TestGetPlazaForUser_ImageBillingPricingDisplayable(t *testing.T) {
	// image 计费形态：image_output_price / per_request_price 带价即可展示。
	g := AvailableGroupRef{ID: 10, Name: "g", Platform: "gemini", SubscriptionType: SubscriptionTypeStandard}
	imgPricing := &ChannelModelPricing{
		BillingMode:      BillingModeImage,
		ImageOutputPrice: fp(3e-5),
		PerRequestPrice:  fp(0.266),
	}
	ch := []AvailableChannel{{
		ID: 1, Name: "ch", Status: StatusActive, Groups: []AvailableGroupRef{g},
		SupportedModels: []SupportedModel{{Name: "gemini-3-pro-image", Platform: "gemini", Pricing: imgPricing}},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.NotNil(t, data.Models[0].Pricing)
	require.Equal(t, BillingModeImage, data.Models[0].BillingMode)
	require.Equal(t, 0.266, *data.Models[0].Pricing.PerRequestPrice)
}

func TestGetPlazaForUser_NoDisplayablePricingNil(t *testing.T) {
	// 全部候选无可展示价格 → Pricing=nil，BillingMode 回落 token。
	g := AvailableGroupRef{ID: 10, Name: "g", Platform: "openai", SubscriptionType: SubscriptionTypeStandard}
	ch := []AvailableChannel{{
		ID: 1, Name: "ch", Status: StatusActive, Groups: []AvailableGroupRef{g},
		SupportedModels: []SupportedModel{{Name: "mystery-model", Platform: "openai", Pricing: nil}},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, data.Models, 1)
	require.Nil(t, data.Models[0].Pricing)
	require.Equal(t, BillingModeToken, data.Models[0].BillingMode)
}

func TestGetPlazaForUser_IntervalOnlyPricingDisplayable(t *testing.T) {
	// flat 全空但 interval 带价 → 视为可展示（!pricingNeedsFallback）。
	g := AvailableGroupRef{ID: 10, Name: "g", Platform: "gemini", SubscriptionType: SubscriptionTypeStandard}
	maxTok := 200000
	interval := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: &maxTok, InputPrice: fp(2.5e-7), OutputPrice: fp(1.5e-6)},
		},
	}
	ch := []AvailableChannel{{
		ID: 1, Name: "ch", Status: StatusActive, Groups: []AvailableGroupRef{g},
		SupportedModels: []SupportedModel{{Name: "gemini-3-pro", Platform: "gemini", Pricing: interval}},
	}}
	svc := newPlazaService(ch, []Group{{ID: 10}}, nil)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, data.Models[0].Pricing)
	require.Len(t, data.Models[0].Pricing.Intervals, 1)
}

// ===== 描述与公告 =====

func TestGetPlazaForUser_DescriptionAndAnnouncement(t *testing.T) {
	g := AvailableGroupRef{ID: 10, Name: "g", Platform: "anthropic", SubscriptionType: SubscriptionTypeStandard}
	ch := []AvailableChannel{{
		ID: 1, Name: "ch", Status: StatusActive, Groups: []AvailableGroupRef{g},
		SupportedModels: []SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: tokenPricing(9e-7, 4.5e-6)},
		},
	}}
	cfg := &domain.ModelPlazaExtensionConfig{
		ModelDescriptions: map[string]string{"claude-sonnet-4-6": "旗舰对话模型"},
		Announcement:      "## 计费说明",
	}
	svc := newPlazaService(ch, []Group{{ID: 10}}, cfg)

	data, err := svc.GetPlazaForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "## 计费说明", data.Announcement)
	require.Equal(t, "旗舰对话模型", data.Models[0].Description)
}

// ===== admin 模型清单 =====

func TestListAllModelIdentities_DedupNoFilters(t *testing.T) {
	// 不带用户过滤、不带黑名单：active 渠道的全部模型 (platform, name) 去重排序。
	ch := []AvailableChannel{
		{ID: 1, Name: "a", Status: StatusActive,
			Groups: []AvailableGroupRef{{ID: 10, Name: "g", Platform: "anthropic", IsExclusive: true, SubscriptionType: SubscriptionTypeStandard}},
			SupportedModels: []SupportedModel{
				{Name: "claude-sonnet-4-6", Platform: "anthropic"},
				{Name: "Claude-Sonnet-4-6", Platform: "anthropic"}, // 大小写重复 → 去重
			}},
		{ID: 2, Name: "b", Status: StatusActive,
			Groups: []AvailableGroupRef{{ID: 11, Name: "g2", Platform: "openai", SubscriptionType: SubscriptionTypeStandard}},
			SupportedModels: []SupportedModel{
				{Name: "gpt-5.2", Platform: "openai"},
			}},
		{ID: 3, Name: "c", Status: "disabled", SupportedModels: []SupportedModel{
			{Name: "ghost", Platform: "openai"},
		}},
	}
	svc := newPlazaService(ch, nil, &domain.ModelPlazaExtensionConfig{ExcludedChannelIDs: []int64{2}})

	out, err := svc.ListAllModelIdentities(context.Background())
	require.NoError(t, err)
	// 黑名单不影响 admin 清单；disabled 渠道跳过
	require.Equal(t, []ModelIdentity{
		{Platform: "anthropic", Name: "claude-sonnet-4-6"},
		{Platform: "openai", Name: "gpt-5.2"},
	}, out)
}
