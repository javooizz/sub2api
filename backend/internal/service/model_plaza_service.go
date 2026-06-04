package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ===== 依赖接口（窄化为本服务所需，便于单测 mock）=====

// plazaChannelLister 提供渠道可用视图。*ChannelService 天然满足。
type plazaChannelLister interface {
	ListAvailable(ctx context.Context) ([]AvailableChannel, error)
}

// plazaGroupProvider 提供用户实际可绑定的分组（公开标准 + 授权专属 + 已订阅订阅型）。
// *APIKeyService 天然满足。
type plazaGroupProvider interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error)
}

// plazaConfigProvider 提供 model-plaza 的扩展配置。*ExtensionConfigService 天然满足。
type plazaConfigProvider interface {
	GetAdmin(ctx context.Context, agentID string) (*ExtensionConfigRecord, error)
}

// ===== 输出类型 =====

// PlazaGroupRef 广场视图中模型可用分组的引用。
// Accessible = 分组 ∈ GetAvailableGroups（用户当前实际可绑定）；
// 公开订阅型未订阅时 Accessible=false，前端据此显示"需订阅"标签。
type PlazaGroupRef struct {
	AvailableGroupRef
	Accessible bool
}

// PlazaModel 广场视图中的一个模型条目。唯一身份 = (Platform, lower(Name))。
type PlazaModel struct {
	Name        string
	Platform    string
	Description string
	BillingMode BillingMode
	Pricing     *ChannelModelPricing // nil = 无可展示定价
	Groups      []PlazaGroupRef
}

// ModelIdentity 是 admin 模型清单条目（管理页"模型描述"编辑器的数据源）。
type ModelIdentity struct {
	Platform string
	Name     string
}

// ModelPlazaData 是 GetPlazaForUser 的返回值。
type ModelPlazaData struct {
	Announcement string
	Models       []PlazaModel
}

// ModelPlazaService 聚合"模型广场"视图：以模型为中心，合并渠道定价
//（标准基准价）与用户可见分组（倍率），供用户端目录页展示。
//
// 可见性只作用于广场展示，不影响绑定/计费权限（后者由 GetAvailableGroups 把关）。
type ModelPlazaService struct {
	channels plazaChannelLister
	groups   plazaGroupProvider
	config   plazaConfigProvider
}

// NewModelPlazaService 构造（wire 注入具体 service，接口在构造时收窄）。
func NewModelPlazaService(
	channelService *ChannelService,
	apiKeyService *APIKeyService,
	extensionConfigService *ExtensionConfigService,
) *ModelPlazaService {
	return &ModelPlazaService{
		channels: channelService,
		groups:   apiKeyService,
		config:   extensionConfigService,
	}
}

// loadConfig 读取 model-plaza 扩展配置；未配置时返回零值配置（全部放行、无公告）。
func (s *ModelPlazaService) loadConfig(ctx context.Context) (domain.ModelPlazaExtensionConfig, error) {
	rec, err := s.config.GetAdmin(ctx, AgentIDModelPlaza)
	if err != nil {
		return domain.ModelPlazaExtensionConfig{}, err
	}
	if rec == nil || rec.Payload.ModelPlaza == nil {
		return domain.ModelPlazaExtensionConfig{}, nil
	}
	return *rec.Payload.ModelPlaza, nil
}

// plazaAggKey 是模型聚合身份：与 Channel.SupportedModels 的
// (Platform, lowercase(Name)) 去重口径一致，避免跨平台同名模型串台。
type plazaAggKey struct {
	platform  string
	nameLower string
}

// GetPlazaForUser 聚合当前用户可见的广场模型列表。
//
// 可见分组（仅作用于广场展示）：
//
//	visible(g) = g ∈ GetAvailableGroups(userID)                         // 公开标准 + 授权专属 + 已订阅
//	           || (g.SubscriptionType == subscription && !g.IsExclusive) // 公开订阅型：未订阅也展示（橱窗）
//
// 基准价取首个有可展示定价（!pricingNeedsFallback）的渠道；渠道顺序按
// ListAvailable 的渠道名序，保证输出稳定。
func (s *ModelPlazaService) GetPlazaForUser(ctx context.Context, userID int64) (*ModelPlazaData, error) {
	channels, err := s.channels.ListAvailable(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available channels: %w", err)
	}
	userGroups, err := s.groups.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user available groups: %w", err)
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load model plaza config: %w", err)
	}

	accessible := make(map[int64]struct{}, len(userGroups))
	for i := range userGroups {
		accessible[userGroups[i].ID] = struct{}{}
	}
	excludedCh := int64Set(cfg.ExcludedChannelIDs)
	excludedGrp := int64Set(cfg.ExcludedGroupIDs)

	agg := make(map[plazaAggKey]*PlazaModel)
	groupSeen := make(map[plazaAggKey]map[int64]struct{})

	for ci := range channels {
		ch := &channels[ci]
		if ch.Status != StatusActive {
			continue
		}
		if _, ex := excludedCh[ch.ID]; ex {
			continue
		}

		// 该渠道下用户可见的分组（含 accessible 标记）。
		visGroups := make([]PlazaGroupRef, 0, len(ch.Groups))
		for _, g := range ch.Groups {
			if _, ex := excludedGrp[g.ID]; ex {
				continue
			}
			_, acc := accessible[g.ID]
			publicSubscription := g.SubscriptionType == SubscriptionTypeSubscription && !g.IsExclusive
			if !acc && !publicSubscription {
				continue
			}
			visGroups = append(visGroups, PlazaGroupRef{AvailableGroupRef: g, Accessible: acc})
		}
		if len(visGroups) == 0 {
			continue
		}
		platformSet := make(map[string]struct{}, 4)
		for _, g := range visGroups {
			if g.Platform != "" {
				platformSet[g.Platform] = struct{}{}
			}
		}

		for mi := range ch.SupportedModels {
			m := &ch.SupportedModels[mi]
			// 跨平台信息防泄漏：模型平台必须有用户可见分组。
			if _, ok := platformSet[m.Platform]; !ok {
				continue
			}
			k := plazaAggKey{platform: m.Platform, nameLower: strings.ToLower(m.Name)}
			entry, ok := agg[k]
			if !ok {
				entry = &PlazaModel{Name: m.Name, Platform: m.Platform}
				agg[k] = entry
				groupSeen[k] = make(map[int64]struct{})
			}
			// 基准价：首个可展示定价胜出（渠道名序稳定）。
			if entry.Pricing == nil && !pricingNeedsFallback(m.Pricing) {
				entry.Pricing = m.Pricing // 只读别名：ListAvailable 每次返回 Clone，本服务不经此指针写入
			}
			for _, g := range visGroups {
				if g.Platform != m.Platform {
					continue
				}
				if _, dup := groupSeen[k][g.ID]; dup {
					continue
				}
				groupSeen[k][g.ID] = struct{}{}
				entry.Groups = append(entry.Groups, g)
			}
		}
	}

	models := make([]PlazaModel, 0, len(agg))
	for _, m := range agg {
		// 描述按【模型名】键控（与 admin 编辑器契约一致，规格 §4.2 步骤 5）：
		// 字节精确匹配原始大小写；跨平台同名模型共享同一条描述，属设计限制，勿改成 platform 键控。
		if desc, ok := cfg.ModelDescriptions[m.Name]; ok {
			m.Description = desc
		}
		if m.Pricing != nil && m.Pricing.BillingMode != "" {
			m.BillingMode = m.Pricing.BillingMode
		} else {
			m.BillingMode = BillingModeToken
		}
		sort.SliceStable(m.Groups, func(i, j int) bool { return m.Groups[i].Name < m.Groups[j].Name })
		models = append(models, *m)
	}
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Platform != models[j].Platform {
			return models[i].Platform < models[j].Platform
		}
		return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
	})

	return &ModelPlazaData{Announcement: cfg.Announcement, Models: models}, nil
}

// ListAllModelIdentities 返回全部 active 渠道聚合出的模型身份清单
//（不带用户过滤、不带黑名单），供管理页"模型描述"编辑器使用。
func (s *ModelPlazaService) ListAllModelIdentities(ctx context.Context) ([]ModelIdentity, error) {
	channels, err := s.channels.ListAvailable(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available channels: %w", err)
	}
	seen := make(map[plazaAggKey]struct{})
	out := make([]ModelIdentity, 0, 64)
	for ci := range channels {
		ch := &channels[ci]
		if ch.Status != StatusActive {
			continue
		}
		for mi := range ch.SupportedModels {
			m := &ch.SupportedModels[mi]
			k := plazaAggKey{platform: m.Platform, nameLower: strings.ToLower(m.Name)}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, ModelIdentity{Platform: m.Platform, Name: m.Name})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// int64Set 把 slice 转为查询集合。
func int64Set(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}
