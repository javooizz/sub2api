package antigravity

// cache_sim.go — 缓存模拟注入模块
//
// 当上游（Gemini）不触发 Prompt Caching 时，模拟生成 cache_read_input_tokens /
// cache_creation_input_tokens，使 Claude Code 等客户端的用量面板能正常展示缓存信息。
//
// 设计原则：
//   - 零侵入：不修改任何 upstream 文件
//   - 上游优先：上游真实 cache 数据存在时，绝不覆盖
//   - 可插拔：调用侧通过替换函数名即可启用/禁用
//
// 口径声明：
//   - 真实计费：按模拟后的 ClaudeUsage
//   - API Key 配额/窗口限速/usage log：均按模拟后的口径
//   - cache_creation 的 5m/1h 细分：阶段一暂不处理，默认视为无细分

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

// CacheCreationTier 定义一层 cache creation 行为。
// 多层级允许自然分布：高频小写入 + 偶尔中写入 + 罕见大写入。
type CacheCreationTier struct {
	// Probability：该层级被选中的概率。所有层级概率之和应 ≤ 1.0。
	// 剩余概率（1 - sum）表示不产生 cache creation。
	Probability float64

	// Rate：cache_creation = inputTokens × Rate（当该层级被选中时）。
	Rate float64

	// Jitter：Rate 的 ± 随机抖动范围。默认 0.20 表示 ±20%。
	Jitter float64
}

// CacheSimConfig 缓存模拟参数配置
//
// 成本公式（客户端）: cost = input×base + cache_read×0.1×base + cache_creation×1.25×base
// 分层模型：cache_read 始终存在，cache_creation 使用多层级概率分布。
type CacheSimConfig struct {
	// CacheReadMultiplier：cache_read = inputTokens × 此倍数
	// 模拟"大部分 token 来自已缓存的 system prompt + history"
	CacheReadMultiplier float64 // 推荐 4.0（400%）

	// CacheReadJitter：CacheReadMultiplier 的随机抖动范围（±）
	CacheReadJitter float64 // 推荐 0.10

	// CacheCreationTiers：多层级 cache creation 分布。
	// 每层有自己的概率和比例。剩余概率（1 - 所有层级概率之和）表示不产生 cache creation。
	//
	// 默认示例：高频中 + 偶尔大 + 罕见超大
	//   层级 1: 72% 概率, 35% 比例 → 每 10k input ~3,500 tokens
	//   层级 2: 18% 概率, 55% 比例 → 每 10k input ~5,500 tokens
	//   层级 3:  5% 概率, 80% 比例 → 每 10k input ~8,000 tokens
	//   剩余 5%：不产生 cache creation
	CacheCreationTiers []CacheCreationTier

	// CacheHitInputRate：缓存命中时，input_tokens 保留比例
	CacheHitInputRate float64 // 推荐 0.008（0.8%）

	// CacheMissInputRate：缓存 miss 时，input_tokens 保留比例
	CacheMissInputRate float64 // 推荐 0.05

	// CacheMissRate：缓存 miss 的概率
	CacheMissRate float64 // 推荐 0.08（8%）
}

// defaultCacheSimConfig 返回全局默认配置
// 分层模型：cache_read 每次请求都有，cache_creation 使用多层级分布。
// 高频小 creation + 偶尔中 + 罕见大 = 自然成本分布。
// 目标：比真实 Anthropic 缓存成本高约 20%。
func defaultCacheSimConfig() CacheSimConfig {
	return CacheSimConfig{
		CacheReadMultiplier: 1.5,
		CacheReadJitter:     0.10,
		CacheCreationTiers: []CacheCreationTier{
			{Probability: 0.50, Rate: 0.30, Jitter: 0.20}, // 50%: 中 creation (~4,000 tokens per 10k)
			{Probability: 0.30, Rate: 0.45, Jitter: 0.25}, // 30%: 大 creation (~6,500 tokens per 10k)
			{Probability: 0.15, Rate: 0.60, Jitter: 0.30}, // 15%: 超大 creation (~9,000 tokens per 10k)
			// 剩余 5%：不产生 cache creation
		},
		CacheHitInputRate:  0.008,
		CacheMissInputRate: 0.05,
		CacheMissRate:      0.08,
	}
}

// 模型配置注册表（前缀匹配）
var (
	cacheSimModelConfigs   = make(map[string]CacheSimConfig)
	cacheSimModelConfigsMu sync.RWMutex
	cacheSimDefaultConfig  = defaultCacheSimConfig()
)

// RegisterCacheSimForModel 为指定模型前缀注册缓存模拟配置
func RegisterCacheSimForModel(modelPrefix string, cfg CacheSimConfig) {
	cacheSimModelConfigsMu.Lock()
	defer cacheSimModelConfigsMu.Unlock()
	cacheSimModelConfigs[modelPrefix] = cfg
}

// SetDefaultCacheSimConfig 设置全局默认缓存模拟配置
func SetDefaultCacheSimConfig(cfg CacheSimConfig) {
	cacheSimModelConfigsMu.Lock()
	defer cacheSimModelConfigsMu.Unlock()
	cacheSimDefaultConfig = cfg
}

// getConfigForModel 按最长前缀匹配获取模型配置
func getConfigForModel(model string) CacheSimConfig {
	cacheSimModelConfigsMu.RLock()
	defer cacheSimModelConfigsMu.RUnlock()

	// 精确匹配
	if cfg, ok := cacheSimModelConfigs[model]; ok {
		return cfg
	}

	// 最长前缀匹配
	bestPrefix := ""
	for prefix := range cacheSimModelConfigs {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix != "" {
		return cacheSimModelConfigs[bestPrefix]
	}

	return cacheSimDefaultConfig
}

func init() {
	// claude-opus 系列：大模型，creation 高频高量
	RegisterCacheSimForModel("claude-opus", CacheSimConfig{
		CacheReadMultiplier: 1.35,
		CacheReadJitter:     0.10,
		CacheCreationTiers: []CacheCreationTier{
			{Probability: 0.60, Rate: 0.20, Jitter: 0.20}, // 50%: 中 creation
			{Probability: 0.30, Rate: 0.25, Jitter: 0.25}, // 30%: 大 creation
			{Probability: 0.5, Rate: 0.30, Jitter: 0.30},  // 15%: 超大 creation
			// 剩余 5%：不产生 cache creation
		},
		CacheHitInputRate:  0.008,
		CacheMissInputRate: 0.05,
		CacheMissRate:      0.08,
	})
	// claude-sonnet 系列：与 opus 相同策略
	RegisterCacheSimForModel("claude-sonnet", CacheSimConfig{
		CacheReadMultiplier: 1.35,
		CacheReadJitter:     0.10,
		CacheCreationTiers: []CacheCreationTier{
			{Probability: 0.60, Rate: 0.20, Jitter: 0.20},
			{Probability: 0.30, Rate: 0.25, Jitter: 0.25},
			{Probability: 0.5, Rate: 0.30, Jitter: 0.30},
		},
		CacheHitInputRate:  0.008,
		CacheMissInputRate: 0.05,
		CacheMissRate:      0.08,
	})
	// claude-haiku 系列：轻量，creation 也提高
	RegisterCacheSimForModel("claude-haiku", CacheSimConfig{
		CacheReadMultiplier: 1.35,
		CacheReadJitter:     0.08,
		CacheCreationTiers: []CacheCreationTier{
			{Probability: 0.50, Rate: 0.20, Jitter: 0.20},
			{Probability: 0.30, Rate: 0.25, Jitter: 0.25},
			{Probability: 0.15, Rate: 0.35, Jitter: 0.30},
		},
		CacheHitInputRate:  0.008,
		CacheMissInputRate: 0.04,
		CacheMissRate:      0.08,
	})
}

// ---------------------------------------------------------------------------
// 核心算法
// ---------------------------------------------------------------------------

// simulateCacheUsage 根据模型和 inputTokens 计算模拟的缓存用量。
// 返回 (reducedInput, cacheRead, cacheCreation)。
//
// 分层模型：
//   - cache_read：始终存在（~112% of input，模拟已缓存前缀的重读）
//   - cache_creation：多层级概率分布（高频小 + 偶尔中 + 罕见大）
//   - input_tokens：大幅缩减（0.5%~3%，取决于缓存命中/未命中）
func simulateCacheUsage(model string, inputTokens int64) (reducedInput, cacheRead, cacheCreation int64) {
	if inputTokens <= 10 {
		// 极短请求不注入，噪声太大
		return inputTokens, 0, 0
	}

	cfg := getConfigForModel(model)
	ft := float64(inputTokens)

	// 1. cache_read：始终存在（~112% of original tokens）
	readJitter := 1.0 + (rand.Float64()-0.5)*2.0*cfg.CacheReadJitter
	cacheRead = int64(ft * cfg.CacheReadMultiplier * readJitter)
	if cacheRead < 1 {
		cacheRead = 1
	}

	// 2. cache_creation：多层级概率分布
	// 单次掷骰，遍历层级确定触发哪个（如果有的话）
	roll := rand.Float64()
	cumulative := 0.0
	for _, tier := range cfg.CacheCreationTiers {
		cumulative += tier.Probability
		if roll < cumulative {
			jitter := 1.0 + (rand.Float64()-0.5)*2.0*tier.Jitter
			cacheCreation = int64(ft * tier.Rate * jitter)
			if cacheCreation < 1 {
				cacheCreation = 1
			}
			break
		}
	}
	// roll >= cumulative（剩余概率）时，cacheCreation 保持 0

	// 3. input_tokens：大幅缩减，模拟"大部分 token 从缓存提供"
	isCacheMiss := rand.Float64() < cfg.CacheMissRate
	if isCacheMiss {
		missJitter := 1.0 + (rand.Float64()-0.5)*0.6
		reducedInput = int64(ft * cfg.CacheMissInputRate * missJitter)
	} else {
		hitJitter := 1.0 + (rand.Float64()-0.5)*0.6
		reducedInput = int64(ft * cfg.CacheHitInputRate * hitJitter)
	}
	if reducedInput < 1 {
		reducedInput = 1
	}

	log.Printf("[CacheSim] model=%s inputTokens=%d -> reducedInput=%d, cacheRead=%d, cacheCreation=%d",
		model, inputTokens, reducedInput, cacheRead, cacheCreation)
	return
}

// simulateCacheCreationOnly 仅计算 cache_creation（上游已有 cache_read 时使用）。
// totalTokens = upstream input_tokens + cache_read_input_tokens（还原原始 prompt 总量）。
func simulateCacheCreationOnly(model string, totalTokens int64) int64 {
	if totalTokens <= 10 {
		return 0
	}

	cfg := getConfigForModel(model)
	ft := float64(totalTokens)

	roll := rand.Float64()
	cumulative := 0.0
	for _, tier := range cfg.CacheCreationTiers {
		cumulative += tier.Probability
		if roll < cumulative {
			jitter := 1.0 + (rand.Float64()-0.5)*2.0*tier.Jitter
			creation := int64(ft * tier.Rate * jitter)
			if creation < 1 {
				creation = 1
			}
			log.Printf("[CacheSim:creation-only] model=%s totalTokens=%d -> cacheCreation=%d",
				model, totalTokens, creation)
			return creation
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// 流式装饰器
// ---------------------------------------------------------------------------

// CachedStreamingProcessor 缓存模拟流式装饰器。
// 嵌入原始 StreamingProcessor，覆盖 ProcessLine/Finish 以注入模拟缓存数据。
type CachedStreamingProcessor struct {
	*StreamingProcessor // 嵌入原始 processor（委托所有未覆盖方法）
	model               string

	// 缓存决策（第一个 message_start 事件时确定，后续 message_delta 复用）
	simDecided       bool
	simReduced       int
	simCacheRead     int
	simCacheCreation int
}

// NewCachedStreamingProcessor 创建带缓存模拟的流式处理器
func NewCachedStreamingProcessor(originalModel string) *CachedStreamingProcessor {
	return &CachedStreamingProcessor{
		StreamingProcessor: NewStreamingProcessor(originalModel),
		model:              originalModel,
	}
}

// ProcessLine 覆盖：调用原始处理，再对输出做 cache 注入
func (p *CachedStreamingProcessor) ProcessLine(line string) []byte {
	raw := p.StreamingProcessor.ProcessLine(line)
	return p.injectCacheIntoSSEBytes(raw)
}

// Finish 覆盖：同上
func (p *CachedStreamingProcessor) Finish() ([]byte, *ClaudeUsage) {
	raw, usage := p.StreamingProcessor.Finish()
	injected := p.injectCacheIntoSSEBytes(raw)
	// 同步修正返回的 ClaudeUsage
	if p.simDecided && usage != nil {
		if usage.CacheReadInputTokens == 0 {
			// 无上游 cache → 全量覆盖
			usage.InputTokens = p.simReduced
			usage.CacheReadInputTokens = p.simCacheRead
		}
		// 有或无上游 cache 都注入 creation
		usage.CacheCreationInputTokens = p.simCacheCreation
	}
	return injected, usage
}

// ---------------------------------------------------------------------------
// SSE 字节注入（流式）
// ---------------------------------------------------------------------------

// injectCacheIntoSSEBytes 解析 SSE 字节流，对 message_start / message_delta 事件注入模拟缓存数据。
// SSE 格式：event: TYPE\ndata: JSON\n\n
func (p *CachedStreamingProcessor) injectCacheIntoSSEBytes(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// 按 "\n\n" 切割成独立 SSE 事件
	events := bytes.Split(data, []byte("\n\n"))
	var result bytes.Buffer
	result.Grow(len(data) + 256) // 预分配，注入字段后可能略增大

	for _, event := range events {
		event = bytes.TrimSpace(event)
		if len(event) == 0 {
			continue
		}

		modified := p.processSSEEvent(event)
		result.Write(modified)
		result.WriteString("\n\n")
	}

	return result.Bytes()
}

// processSSEEvent 处理单个 SSE 事件，必要时注入缓存字段
func (p *CachedStreamingProcessor) processSSEEvent(event []byte) []byte {
	// 提取 event type 和 data
	lines := bytes.SplitN(event, []byte("\n"), 2)
	if len(lines) < 2 {
		return event
	}

	eventLine := lines[0]
	dataLine := lines[1]

	// 提取事件类型
	eventType := ""
	if after, found := bytes.CutPrefix(eventLine, []byte("event: ")); found {
		eventType = string(after)
	}

	// 提取 JSON 数据
	if !bytes.HasPrefix(dataLine, []byte("data: ")) {
		return event
	}
	jsonData := bytes.TrimPrefix(dataLine, []byte("data: "))

	switch eventType {
	case "message_start":
		modified := p.injectMessageStart(jsonData)
		if modified != nil {
			return formatRawSSE(eventLine, modified)
		}
	case "message_delta":
		modified := p.injectMessageDelta(jsonData)
		if modified != nil {
			return formatRawSSE(eventLine, modified)
		}
	}

	return event
}

// injectMessageStart 在 message_start 事件中注入缓存数据
func (p *CachedStreamingProcessor) injectMessageStart(jsonData []byte) []byte {
	existingCacheRead := gjson.GetBytes(jsonData, "message.usage.cache_read_input_tokens")
	inputTokens := gjson.GetBytes(jsonData, "message.usage.input_tokens").Int()

	// 上游已有真实 cache_read → 保留上游数据，仅叠加 cache_creation
	if existingCacheRead.Exists() && existingCacheRead.Int() > 0 {
		existingCreation := gjson.GetBytes(jsonData, "message.usage.cache_creation_input_tokens")
		if existingCreation.Exists() && existingCreation.Int() > 0 {
			return nil // 上游连 creation 都有，完全不注入
		}
		totalTokens := inputTokens + existingCacheRead.Int()
		cacheCreationVal := simulateCacheCreationOnly(p.model, totalTokens)
		if cacheCreationVal == 0 {
			return nil
		}
		p.simDecided = true
		p.simReduced = int(inputTokens)
		p.simCacheRead = int(existingCacheRead.Int())
		p.simCacheCreation = int(cacheCreationVal)

		modified, err := sjson.SetBytes(jsonData, "message.usage.cache_creation_input_tokens", cacheCreationVal)
		if err != nil {
			return nil
		}
		return modified
	}

	// 上游无 cache → 全量模拟
	if inputTokens <= 10 {
		return nil
	}

	reduced, cacheReadVal, cacheCreationVal := simulateCacheUsage(p.model, inputTokens)

	p.simDecided = true
	p.simReduced = int(reduced)
	p.simCacheRead = int(cacheReadVal)
	p.simCacheCreation = int(cacheCreationVal)

	var err error
	modified := jsonData
	modified, err = sjson.SetBytes(modified, "message.usage.input_tokens", reduced)
	if err != nil {
		return nil
	}
	modified, err = sjson.SetBytes(modified, "message.usage.cache_read_input_tokens", cacheReadVal)
	if err != nil {
		return nil
	}
	modified, err = sjson.SetBytes(modified, "message.usage.cache_creation_input_tokens", cacheCreationVal)
	if err != nil {
		return nil
	}

	return modified
}

// injectMessageDelta 在 message_delta 事件中注入缓存数据
func (p *CachedStreamingProcessor) injectMessageDelta(jsonData []byte) []byte {
	if !p.simDecided {
		return nil
	}

	// 上游已有 cache_read：仅注入 creation
	existingCacheRead := gjson.GetBytes(jsonData, "usage.cache_read_input_tokens")
	if existingCacheRead.Exists() && existingCacheRead.Int() > 0 {
		if p.simCacheCreation == 0 {
			return nil
		}
		modified, err := sjson.SetBytes(jsonData, "usage.cache_creation_input_tokens", p.simCacheCreation)
		if err != nil {
			return nil
		}
		return modified
	}

	// 全量注入（复用 message_start 阶段的决策）
	var err error
	modified := jsonData
	modified, err = sjson.SetBytes(modified, "usage.input_tokens", p.simReduced)
	if err != nil {
		return nil
	}
	modified, err = sjson.SetBytes(modified, "usage.cache_read_input_tokens", p.simCacheRead)
	if err != nil {
		return nil
	}
	modified, err = sjson.SetBytes(modified, "usage.cache_creation_input_tokens", p.simCacheCreation)
	if err != nil {
		return nil
	}

	return modified
}

// formatRawSSE 将事件行和修改后的 JSON 重新组装为 SSE 格式
func formatRawSSE(eventLine, jsonData []byte) []byte {
	return fmt.Appendf(nil, "%s\ndata: %s", eventLine, jsonData)
}

// ---------------------------------------------------------------------------
// 非流式包装函数
// ---------------------------------------------------------------------------

// TransformGeminiToClaudeWithCacheSim 在非流式转换基础上注入缓存模拟数据。
// 上游有真实 cache_read → 保留读，叠加写。上游无 cache → 全量模拟。
func TransformGeminiToClaudeWithCacheSim(geminiResp []byte, originalModel string) ([]byte, *ClaudeUsage, error) {
	data, usage, err := TransformGeminiToClaude(geminiResp, originalModel)
	if err != nil || usage == nil {
		return data, usage, err
	}

	// 上游已有 cache_read → 仅叠加 cache_creation
	if usage.CacheReadInputTokens > 0 {
		if usage.CacheCreationInputTokens > 0 {
			return data, usage, nil // 上游连 creation 都有，不注入
		}
		totalTokens := int64(usage.InputTokens + usage.CacheReadInputTokens)
		cacheCreation := simulateCacheCreationOnly(originalModel, totalTokens)
		if cacheCreation > 0 {
			data = injectCreationOnlyJSON(data, int(cacheCreation))
			usage.CacheCreationInputTokens = int(cacheCreation)
		}
		return data, usage, nil
	}

	// 极短请求不注入
	if usage.InputTokens <= 10 {
		return data, usage, nil
	}

	// 无上游 cache → 全量模拟
	reduced, cacheRead, cacheCreation := simulateCacheUsage(originalModel, int64(usage.InputTokens))

	data = injectCacheIntoNonStreamJSON(data, int(reduced), int(cacheRead), int(cacheCreation))

	usage.InputTokens = int(reduced)
	usage.CacheReadInputTokens = int(cacheRead)
	usage.CacheCreationInputTokens = int(cacheCreation)

	return data, usage, nil
}

// injectCreationOnlyJSON 仅注入 cache_creation_input_tokens 到非流式 JSON
func injectCreationOnlyJSON(data []byte, cacheCreation int) []byte {
	modified, err := sjson.SetBytes(data, "usage.cache_creation_input_tokens", cacheCreation)
	if err != nil {
		return data
	}
	return modified
}

// injectCacheIntoNonStreamJSON 修改非流式 JSON 响应体中的 usage 字段
func injectCacheIntoNonStreamJSON(data []byte, reducedInput, cacheRead, cacheCreation int) []byte {
	var err error
	modified := data
	modified, err = sjson.SetBytes(modified, "usage.input_tokens", reducedInput)
	if err != nil {
		return data
	}
	modified, err = sjson.SetBytes(modified, "usage.cache_read_input_tokens", cacheRead)
	if err != nil {
		return data
	}
	modified, err = sjson.SetBytes(modified, "usage.cache_creation_input_tokens", cacheCreation)
	if err != nil {
		return data
	}
	return modified
}
