package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ===== 依赖接口（窄化为本服务所需，便于单测 mock）=====

// plazaGroupLister 提供全部 active 分组。GroupRepository 天然满足。
type plazaGroupLister interface {
	ListActive(ctx context.Context) ([]Group, error)
}

// plazaAccountLister 提供分组下账号（仓储已过滤 status=active，无时态谓词——
// "配置上可提供"取样口径，规格 §2）。AccountRepository 天然满足。
type plazaAccountLister interface {
	ListByGroup(ctx context.Context, groupID int64) ([]Account, error)
}

// plazaChannelLister 提供渠道原始数据（纯定价目录）。不用 ChannelService.ListAvailable——
// 它返回前会执行 fillGlobalPricingFallback，回落价会遮蔽其他渠道显式价（规格 §4.2 步骤 4）。
// ChannelRepository 天然满足。
type plazaChannelLister interface {
	ListAll(ctx context.Context) ([]Channel, error)
}

// plazaPricingCatalog 提供 LiteLLM 全局定价回落。*PricingService 天然满足。
type plazaPricingCatalog interface {
	GetModelPricing(model string) *LiteLLMModelPricing
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
	// ImagePricing 该分组的出图计费展示信息；仅图像生成模型注入（规格 2026-06-07 §4.3：
	// (模型×分组) 维度——fallback 档价依赖模型，在 aggregate 模型循环内 per-model 写入，
	// 依赖 entry.Groups append 的值拷贝语义，不会串扰其他模型）。
	ImagePricing *PlazaGroupImagePricing
}

// PlazaGroupImagePricing 分组出图计费展示信息（规格 2026-06-07 §4.2）。
// 展示优先级与真实计费 calculateOpenAIImageCost 一致：渠道按次价 > 分组图片价 > 平台默认价。
type PlazaGroupImagePricing struct {
	Allowed            bool     // Group.AllowImageGeneration；false → 前端显示"该分组不支持出图"
	Price1K            *float64 // 解析后的分组基准档价（逐档 分组配价 ?? defaultImageTierPrices）；
	Price2K            *float64 // 渠道按次显式价优先时为 nil（展示用模型级 Pricing）
	Price4K            *float64
	MultiplierOverride *float64 // image_rate_independent 时非 nil：固定倍率，不吃用户专属倍率
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

// ModelPlazaService 聚合"模型广场"视图（2026-06-05 修订，规格 §4.2/§9）：
// 模型↔分组关联以账号实际可用为准（运行时推导），渠道仅作纯定价目录。
// 可见性只作用于广场展示，不影响绑定/计费权限（后者由 GetAvailableGroups 把关）。
type ModelPlazaService struct {
	groups     plazaGroupLister
	accounts   plazaAccountLister
	channels   plazaChannelLister
	pricing    plazaPricingCatalog
	userGroups plazaGroupProvider
	config     plazaConfigProvider

	cacheMu     sync.RWMutex
	usableCache map[int64]plazaUsableCacheEntry
	now         func() time.Time // 测试注入；nil 时用 time.Now
}

// NewModelPlazaService 构造（wire 注入具体实现，接口在构造时收窄）。
func NewModelPlazaService(
	groupRepo GroupRepository,
	accountRepo AccountRepository,
	channelRepo ChannelRepository,
	pricingService *PricingService,
	apiKeyService *APIKeyService,
	extensionConfigService *ExtensionConfigService,
) *ModelPlazaService {
	return &ModelPlazaService{
		groups:     groupRepo,
		accounts:   accountRepo,
		channels:   channelRepo,
		pricing:    pricingService,
		userGroups: apiKeyService,
		config:     extensionConfigService,
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

// plazaAggKey 是模型聚合身份：(Platform, lowercase(Name))，跨平台同名模型互不串台。
type plazaAggKey struct {
	platform  string
	nameLower string
}

// plazaGroupModels 一个可见分组及其账号推导出的可用模型集。
// img* 为图像计费原始配置(从 Group 拷贝):ImagePricing 是 (模型×分组) 维度——
// fallback 档价依赖模型,故原始配置带到 aggregate 阶段 per-model 解析(规格 2026-06-07 §4.3)。
type plazaGroupModels struct {
	ref    PlazaGroupRef
	models []string

	imgAllow       bool
	imgIndependent bool
	imgMultiplier  float64
	imgPrice1K     *float64
	imgPrice2K     *float64
	imgPrice4K     *float64
}

// plazaPriceEntry 显式价索引条目（displayName = 定价的原始大小写，模型身份的事实来源）。
type plazaPriceEntry struct {
	displayName string
	pricing     *ChannelModelPricing
}

// GetPlazaForUser 聚合当前用户可见的广场模型列表（规格 §4.2）。
//
// 可见分组（仅作用于广场展示）：
//
//	visible(g) = g ∈ GetAvailableGroups(userID)            // 公开标准 + 授权专属 + 已订阅
//	           || (g.IsSubscriptionType() && !g.IsExclusive) // 公开订阅型：未订阅也展示（橱窗）
//
// 再减排除分组名单；账号推导可用集为空的分组自然消失。
func (s *ModelPlazaService) GetPlazaForUser(ctx context.Context, userID int64) (*ModelPlazaData, error) {
	allGroups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	userGroups, err := s.userGroups.GetAvailableGroups(ctx, userID)
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
	excludedGrp := int64Set(cfg.ExcludedGroupIDs)

	visible := make([]plazaGroupModels, 0, len(allGroups))
	for i := range allGroups {
		g := &allGroups[i]
		if _, ex := excludedGrp[g.ID]; ex {
			continue
		}
		_, acc := accessible[g.ID]
		publicSubscription := g.IsSubscriptionType() && !g.IsExclusive
		if !acc && !publicSubscription {
			continue
		}
		models, derr := s.groupUsableModels(ctx, g)
		if derr != nil {
			// 单分组查账号失败：跳过不中断整体（规格 §6），失败不写缓存。
			slog.Warn("model plaza: derive group usable models failed", "group_id", g.ID, "error", derr)
			continue
		}
		if len(models) == 0 {
			continue // 空账号分组自然消失（规格 §2）
		}
		visible = append(visible, plazaGroupModels{
			ref: PlazaGroupRef{
				AvailableGroupRef: AvailableGroupRef{
					ID:               g.ID,
					Name:             g.Name,
					Platform:         g.Platform,
					SubscriptionType: g.SubscriptionType,
					RateMultiplier:   g.RateMultiplier,
					IsExclusive:      g.IsExclusive,
				},
				Accessible: acc,
			},
			models:         models,
			imgAllow:       g.AllowImageGeneration,
			imgIndependent: g.ImageRateIndependent,
			imgMultiplier:  g.ImageRateMultiplier,
			imgPrice1K:     g.ImagePrice1K,
			imgPrice2K:     g.ImagePrice2K,
			imgPrice4K:     g.ImagePrice4K,
		})
	}

	priceIdx, err := s.buildPricingIndex(ctx, int64Set(cfg.ExcludedChannelIDs))
	if err != nil {
		return nil, err
	}

	return &ModelPlazaData{Announcement: cfg.Announcement, Models: s.aggregate(visible, priceIdx, &cfg)}, nil
}

// aggregate 倒排聚合 模型×分组，并完成定价 join、描述注入与排序（规格 §4.2 步骤 3–6）。
func (s *ModelPlazaService) aggregate(visible []plazaGroupModels, priceIdx map[plazaAggKey]plazaPriceEntry, cfg *domain.ModelPlazaExtensionConfig) []PlazaModel {
	agg := make(map[plazaAggKey]*PlazaModel)
	groupSeen := make(map[plazaAggKey]map[int64]struct{})
	for _, gm := range visible {
		for _, name := range gm.models {
			k := plazaAggKey{platform: gm.ref.Platform, nameLower: strings.ToLower(name)}
			entry, ok := agg[k]
			if !ok {
				entry = &PlazaModel{Name: name, Platform: gm.ref.Platform}
				agg[k] = entry
				groupSeen[k] = make(map[int64]struct{})
			} else if name < entry.Name {
				// 显示名确定性：未被定价索引覆盖时取字典序最小写法（规格 §4.2 步骤 4）。
				entry.Name = name
			}
			if _, dup := groupSeen[k][gm.ref.ID]; dup {
				continue
			}
			groupSeen[k][gm.ref.ID] = struct{}{}
			entry.Groups = append(entry.Groups, gm.ref)
		}
	}

	// 分组图像原始配置索引(分组 ID → 配置);ImagePricing 在模型循环内 per-model 解析。
	groupConf := make(map[int64]*plazaGroupModels, len(visible))
	for i := range visible {
		groupConf[visible[i].ref.ID] = &visible[i]
	}

	out := make([]PlazaModel, 0, len(agg))
	for k, m := range agg {
		// channelHit 区分 Pricing 来源:仅"渠道显式按次价"可遮蔽分组图片价(规格 §3 ①
		// "priceIdx 命中且带价");LiteLLM 回落合成物即使自带 PerRequestPrice(gemini image
		// 系)也不遮蔽——真实计费 getImageUnitPrice 是 分组价 > LiteLLM 价。
		channelHit := false
		if pe, ok := priceIdx[k]; ok {
			m.Pricing = pe.pricing // 只读别名，本服务不经此指针写入
			m.Name = pe.displayName
			channelHit = true
		}
		// LiteLLM 条目一次查取:渠道未命中时回落合成,图像生成模型判定与 fallback 解析共用。
		// 注:依赖 GetModelPricing 内部 case-insensitive 查找(PricingService 实现保证)。
		lp := s.pricing.GetModelPricing(m.Name)
		if m.Pricing == nil && lp != nil {
			// LiteLLM 全局目录回落：复用可用渠道页同款合成（image/token 模式自动判定）。
			m.Pricing = synthesizePricingFromLiteLLM(lp, nil)
		}
		// 分组排序提前:图像注入"首个配价分组"按名序取(规格 §3 ②)。
		sort.SliceStable(m.Groups, func(i, j int) bool { return m.Groups[i].Name < m.Groups[j].Name })
		if lp != nil && lp.Mode == "image_generation" {
			s.applyGroupImagePricing(m, lp, groupConf, channelHit)
		}
		// 描述按复合键 "platform/name" 注入（规格 §4.2 步骤 5；admin 清单与本聚合
		// 同源同显示名口径，键可字节精确命中）。
		// 已知边界：若排除渠道恰好改变了某模型显式价的"首选渠道大小写"，本侧显示名与
		// admin 清单（不带排除名单，见 ListAllModelIdentities）可能出现大小写漂移导致
		// 描述不命中——触发需同模型跨渠道大小写不一致且较前名序渠道被排除，现实中罕见。
		if desc, ok := cfg.ModelDescriptions[m.Platform+"/"+m.Name]; ok {
			m.Description = desc
		}
		m.BillingMode = plazaDisplayBillingMode(m.Pricing)
		out = append(out, *m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// applyGroupImagePricing 对图像生成模型(LiteLLM mode=image_generation)注入
// "分组图片价"真实计费链的展示(规格 2026-06-07 §3/§4.3)。展示优先级与
// calculateOpenAIImageCost 一致:
//
//	渠道显式按次价 > 分组图片价(逐档 配价??默认) > 平台默认价(defaultImageTierPrices)
//
// channelHit = m.Pricing 来自 priceIdx(渠道显式价):仅此来源的按次价可遮蔽分组档价
// (注入 Allowed/MultiplierOverride,档价 nil,用模型级 Pricing);其余情形——含渠道仅
// token 价(真实出图计费会忽略它)与 LiteLLM 回落合成物自带按张价(真实计费分组价更优)
// ——一律以分组价/默认价合成按次形态覆盖;模型级基准取首个(按名序,Groups 已排序)
// 显式配价的可出图分组,全未配则用平台默认三档。
func (s *ModelPlazaService) applyGroupImagePricing(m *PlazaModel, lp *LiteLLMModelPricing, groupConf map[int64]*plazaGroupModels, channelHit bool) {
	// "命中且带价"是有意比真实计费的 mode-only 判定更严:渠道 Image 模式但只填了
	// 图像 token 价(无按次价)的退化配置,真实计费会按 0 元扣(本身是配置错误),
	// 展示侧回落分组价/默认价更有意义——不要"修"成 mode-only。
	channelPerRequest := channelHit && pricingHasPerRequest(m.Pricing)
	d1, d2, d4 := defaultImageTierPrices(lp)

	var base *PlazaGroupImagePricing // 首个配价分组的解析档价(模型级基准)
	for i := range m.Groups {
		gc := groupConf[m.Groups[i].ID]
		if gc == nil {
			continue // 防御:Groups 均源自 visible,不可达
		}
		ip := &PlazaGroupImagePricing{Allowed: gc.imgAllow}
		if gc.imgIndependent {
			ip.MultiplierOverride = ptrF(gc.imgMultiplier)
		}
		if !channelPerRequest {
			ip.Price1K = ptrF(orDefaultF(gc.imgPrice1K, d1))
			ip.Price2K = ptrF(orDefaultF(gc.imgPrice2K, d2))
			ip.Price4K = ptrF(orDefaultF(gc.imgPrice4K, d4))
			explicit := gc.imgPrice1K != nil || gc.imgPrice2K != nil || gc.imgPrice4K != nil
			if base == nil && gc.imgAllow && explicit {
				base = ip
			}
		}
		m.Groups[i].ImagePricing = ip
	}
	if channelPerRequest {
		return // 模型级 Pricing 已是渠道按次价
	}
	if base == nil {
		base = &PlazaGroupImagePricing{Price1K: &d1, Price2K: &d2, Price4K: &d4}
	}
	m.Pricing = synthesizeImageTierPricing(*base.Price1K, *base.Price2K, *base.Price4K)
}

// buildPricingIndex 用渠道原始数据构建显式价索引（规格 §4.2 步骤 4）：
// active 渠道、排除名单生效、渠道名字母序首个 !pricingNeedsFallback 的定价胜出；
// 渠道 group_ids 不参与（渠道已退化为纯定价目录）。
func (s *ModelPlazaService) buildPricingIndex(ctx context.Context, excludedCh map[int64]struct{}) (map[plazaAggKey]plazaPriceEntry, error) {
	channels, err := s.channels.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	sort.SliceStable(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})
	idx := make(map[plazaAggKey]plazaPriceEntry)
	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		if _, ex := excludedCh[ch.ID]; ex {
			continue
		}
		for _, m := range ch.SupportedModels() {
			if pricingNeedsFallback(m.Pricing) {
				continue
			}
			k := plazaAggKey{platform: m.Platform, nameLower: strings.ToLower(m.Name)}
			if _, dup := idx[k]; dup {
				continue
			}
			idx[k] = plazaPriceEntry{displayName: m.Name, pricing: m.Pricing}
		}
	}
	return idx, nil
}

// ListAllModelIdentities 返回全部 active 分组聚合出的模型身份清单
// （不带用户过滤、不带排除名单），供管理页"模型描述"编辑器使用。
// 显示名与 GetPlazaForUser 同口径（定价原始大小写优先，否则字典序最小写法），
// 保证管理员配置的复合键能在用户端字节精确命中。
func (s *ModelPlazaService) ListAllModelIdentities(ctx context.Context) ([]ModelIdentity, error) {
	allGroups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	priceIdx, err := s.buildPricingIndex(ctx, nil)
	if err != nil {
		return nil, err
	}
	display := make(map[plazaAggKey]string)
	for i := range allGroups {
		g := &allGroups[i]
		models, derr := s.groupUsableModels(ctx, g)
		if derr != nil {
			slog.Warn("model plaza: derive group usable models failed", "group_id", g.ID, "error", derr)
			continue
		}
		for _, name := range models {
			k := plazaAggKey{platform: g.Platform, nameLower: strings.ToLower(name)}
			if cur, ok := display[k]; !ok || name < cur {
				display[k] = name
			}
		}
	}
	out := make([]ModelIdentity, 0, len(display))
	for k, name := range display {
		if pe, ok := priceIdx[k]; ok {
			name = pe.displayName
		}
		out = append(out, ModelIdentity{Platform: k.platform, Name: name})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// plazaDisplayBillingMode 归一展示计费模式：image/per_request 必须有每张价或带价档位
// （与渠道校验 BILLING_MODE_MISSING_PRICE 同语义），否则按 token 展示。
// 典型场景：gpt-image 系在 LiteLLM 标 image_generation 但只有图像 token 价
// （output_cost_per_image_token），回落合成物若直接透出 image 标签，会与真实计费
// 形态（按图像 token）不符——渠道显式价被删时计费标签会凭空翻成"按图"。
// 注：图像生成模型经 applyGroupImagePricing 合成按次价后自然透出 image（规格 2026-06-07）。
func plazaDisplayBillingMode(p *ChannelModelPricing) BillingMode {
	if p == nil || p.BillingMode == "" {
		return BillingModeToken
	}
	if p.BillingMode != BillingModeImage && p.BillingMode != BillingModePerRequest {
		return p.BillingMode
	}
	if pricingHasPerRequest(p) {
		return p.BillingMode
	}
	return BillingModeToken
}

// pricingHasPerRequest 判定定价是否为"带价"的按次形态（flat 按次价或任一带价档位）。
// 与渠道校验 BILLING_MODE_MISSING_PRICE 同语义。
func pricingHasPerRequest(p *ChannelModelPricing) bool {
	if p == nil {
		return false
	}
	if p.BillingMode != BillingModeImage && p.BillingMode != BillingModePerRequest {
		return false
	}
	if p.PerRequestPrice != nil {
		return true
	}
	for _, iv := range p.Intervals {
		if iv.PerRequestPrice != nil {
			return true
		}
	}
	return false
}

// synthesizeImageTierPricing 把三档按次基准价合成展示用 ChannelModelPricing（规格 §3 ②）。
// PerRequestPrice = 1K 档（表格摘要/兜底）；MinTokens/MaxTokens 取零值——按次档位以
// TierLabel 标识，不按 context 分层。
func synthesizeImageTierPricing(p1k, p2k, p4k float64) *ChannelModelPricing {
	return &ChannelModelPricing{
		BillingMode:     BillingModeImage,
		PerRequestPrice: ptrF(p1k),
		Intervals: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: ptrF(p1k)},
			{TierLabel: "2K", PerRequestPrice: ptrF(p2k)},
			{TierLabel: "4K", PerRequestPrice: ptrF(p4k)},
		},
	}
}

// orDefaultF 解引用可空价格，nil 时用默认值。
func orDefaultF(p *float64, def float64) float64 {
	if p != nil {
		return *p
	}
	return def
}

// ptrF 返回 float64 指针（生产代码侧 helper；测试侧已有 fp）。
func ptrF(v float64) *float64 { return &v }

// int64Set 把 slice 转为查询集合。
func int64Set(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}
