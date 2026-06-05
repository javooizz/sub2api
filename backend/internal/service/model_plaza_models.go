package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// 本文件是 fork 自有代码：模型广场"账号实际可用"推导（规格 §4.2 步骤 2）。
// 语义按平台逐分支复刻 admin/account_handler.go GetAvailableModels（OAuth 判定先于
// mapping；antigravity 始终默认集）。与网关 GetAvailableModels 不同——后者在混合
// 账号分组会吞掉 OAuth 默认模型（已知局限，规格 §2），广场不复用它。

// accountUsableModelEntries 推导单个账号可服务的模型条目。
// 返回原始写法、已排序；mapping 键可能含尾缀 * 通配模式（由调用方处理）。
func accountUsableModelEntries(acc *Account) []string {
	mapping := acc.GetModelMapping()
	switch acc.Platform {
	case PlatformOpenAI:
		// passthrough 绕过常规模型改写，回落到默认模型集（先于 mapping 判定）。
		if acc.IsOpenAIPassthroughEnabled() {
			return defaultModelsListCandidateIDs(PlatformOpenAI)
		}
		if len(mapping) > 0 {
			return sortedStringKeys(mapping)
		}
		return defaultModelsListCandidateIDs(PlatformOpenAI)
	case PlatformGemini:
		// OAuth 账号：OAuth 判定先于 mapping，始终返回默认集。
		if acc.IsOAuth() {
			return defaultModelsListCandidateIDs(PlatformGemini)
		}
		if len(mapping) > 0 {
			return sortedStringKeys(mapping)
		}
		return defaultModelsListCandidateIDs(PlatformGemini)
	case PlatformAntigravity:
		// 与 handler 一致：antigravity 忽略 mapping，始终返回默认模型集。
		return defaultModelsListCandidateIDs(PlatformAntigravity)
	default:
		// anthropic 及未知平台：与 handler 兜底分支一致。
		// OAuth/setup-token 账号：OAuth 判定先于 mapping，始终返回默认集。
		if acc.IsOAuth() {
			return defaultModelsListCandidateIDs(acc.Platform)
		}
		if len(mapping) > 0 {
			return sortedStringKeys(mapping)
		}
		return defaultModelsListCandidateIDs(acc.Platform)
	}
}

// sortedStringKeys 返回 map 的键列表，已按字母序排序。
func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// plazaFilterModelsByCustomList 复刻 gateway_handler.go filterModelsByCustomList。
// service 不能反向 import handler（会成环），等价性由 TestPlazaFilterModelsByCustomList 钉住。
// 注意"available 为空回落 fallback"分支在广场推导下实际不可达（无账号分组已提前消失），
// 保留只为函数级等价（规格 §4.2）。
func plazaFilterModelsByCustomList(availableModels, fallbackModels, selectedModels []string) []string {
	if len(selectedModels) == 0 {
		return availableModels
	}
	source := availableModels
	if len(source) == 0 {
		source = fallbackModels
	}
	if len(source) == 0 {
		return nil
	}

	allowed := make([]string, 0, len(source))
	for _, model := range source {
		model = strings.TrimSpace(model)
		if model != "" {
			allowed = append(allowed, model)
		}
	}

	seen := make(map[string]struct{}, len(selectedModels))
	filtered := make([]string, 0, len(selectedModels))
	for _, model := range selectedModels {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if !plazaCustomListAllowsModel(allowed, model) {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		filtered = append(filtered, model)
	}
	return filtered
}

// plazaCustomListAllowsModel 复刻 gateway_handler.go customModelsListAllowsModel。
func plazaCustomListAllowsModel(availablePatterns []string, model string) bool {
	for _, pattern := range availablePatterns {
		if pattern == model {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

// ===== Task 3: 分组可用模型集推导 + TTL 缓存 =====

// plazaUsableTTL 分组可用模型集缓存时长（规格 §4.2：~60s，配置/账号变更最迟一个 TTL 生效，
// 不做主动失效——纯展示页可接受）。
const plazaUsableTTL = time.Minute

type plazaUsableCacheEntry struct {
	models    []string
	expiresAt time.Time
}

// groupUsableModels 计算分组实际可用模型集（最终集：含 ModelsListConfig 过滤、剔除通配条目）。
// 取样口径"配置上可提供"（规格 §2）：ListByGroup（仓储已过滤 status=active、无时态谓词）
// + 内存过滤 Schedulable 标志与平台匹配——过载/限流窗口中的账号仍计入，目录不抖动。
// 缓存键 = 分组 ID，值 = 最终可用集；失败不写缓存（规格 §6）。
func (s *ModelPlazaService) groupUsableModels(ctx context.Context, g *Group) ([]string, error) {
	if cached, ok := s.usableFromCache(g.ID); ok {
		return cached, nil
	}
	accounts, err := s.accounts.ListByGroup(ctx, g.ID)
	if err != nil {
		return nil, fmt.Errorf("list accounts for group %d: %w", g.ID, err)
	}
	entrySet := make(map[string]struct{})
	for i := range accounts {
		acc := &accounts[i]
		// 仓储已按 status=active 过滤；此处显式复验，语义不依赖仓储实现细节（fake 也会经过同一过滤）。
		if acc.Status != StatusActive || !acc.Schedulable || acc.Platform != g.Platform {
			continue
		}
		for _, m := range accountUsableModelEntries(acc) {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			entrySet[m] = struct{}{}
		}
	}
	entries := make([]string, 0, len(entrySet))
	for m := range entrySet {
		entries = append(entries, m)
	}
	sort.Strings(entries)

	usable := entries
	if g.CustomModelsListEnabled() {
		usable = plazaFilterModelsByCustomList(entries, defaultModelsListCandidateIDs(g.Platform), g.ModelsListConfig.Models)
	}

	// 通配条目不进展示集：无法枚举为具体模型（规格 §4.2；作为 allow 模式已在过滤中生效）。
	out := make([]string, 0, len(usable))
	for _, m := range usable {
		if strings.Contains(m, "*") {
			slog.Debug("model plaza: skip wildcard model entry", "group_id", g.ID, "entry", m)
			continue
		}
		out = append(out, m)
	}
	s.storeUsableCache(g.ID, out)
	return out, nil
}

func (s *ModelPlazaService) timeNow() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *ModelPlazaService) usableFromCache(groupID int64) ([]string, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	e, ok := s.usableCache[groupID]
	if !ok || s.timeNow().After(e.expiresAt) {
		return nil, false
	}
	return append([]string(nil), e.models...), true
}

func (s *ModelPlazaService) storeUsableCache(groupID int64, models []string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.usableCache == nil {
		s.usableCache = make(map[int64]plazaUsableCacheEntry)
	}
	s.usableCache[groupID] = plazaUsableCacheEntry{
		models:    append([]string(nil), models...),
		expiresAt: s.timeNow().Add(plazaUsableTTL),
	}
}
