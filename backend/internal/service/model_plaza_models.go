package service

import (
	"sort"
	"strings"
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
