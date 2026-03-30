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
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

// CacheSimConfig 缓存模拟参数配置
type CacheSimConfig struct {
	// CacheReadMultiplier：cache_read = inputTokens × 此倍数
	// 模拟"大部分 token 来自已缓存的 system prompt + history"
	CacheReadMultiplier float64 // 推荐 1.10（110%）

	// CacheReadJitter：CacheReadMultiplier 的随机抖动范围（±）
	CacheReadJitter float64 // 推荐 0.08

	// CacheCreationProbability：触发 cache_creation 的概率
	CacheCreationProbability float64 // 推荐 0.30（30%）

	// CacheCreationRate：cache_creation = inputTokens × 此比例（触发时）
	CacheCreationRate float64 // 推荐 0.08

	// CacheHitInputRate：缓存命中时，input_tokens 保留比例
	CacheHitInputRate float64 // 推荐 0.005（0.5%）

	// CacheMissInputRate：缓存 miss 时，input_tokens 保留比例
	CacheMissInputRate float64 // 推荐 0.03

	// CacheMissRate：缓存 miss 的概率
	CacheMissRate float64 // 推荐 0.05（5%）
}

// defaultCacheSimConfig 返回全局默认配置
func defaultCacheSimConfig() CacheSimConfig {
	return CacheSimConfig{
		CacheReadMultiplier:      1.10,
		CacheReadJitter:          0.08,
		CacheCreationProbability: 0.40,
		CacheCreationRate:        0.12,
		CacheHitInputRate:        0.005,
		CacheMissInputRate:       0.03,
		CacheMissRate:            0.05,
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
	var bestCfg CacheSimConfig
	found := false
	for prefix, cfg := range cacheSimModelConfigs {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestCfg = cfg
			found = true
		}
	}
	if found {
		return bestCfg
	}

	return cacheSimDefaultConfig
}

func init() {
	// claude-opus 系列：贵，命中率适中
	RegisterCacheSimForModel("claude-opus", CacheSimConfig{
		CacheReadMultiplier:      1.12,
		CacheReadJitter:          0.08,
		CacheCreationProbability: 0.45,
		CacheCreationRate:        0.10,
		CacheHitInputRate:        0.004,
		CacheMissInputRate:       0.03,
		CacheMissRate:            0.06,
	})
	// claude-sonnet 系列：均衡
	RegisterCacheSimForModel("claude-sonnet", CacheSimConfig{
		CacheReadMultiplier:      1.12,
		CacheReadJitter:          0.08,
		CacheCreationProbability: 0.45,
		CacheCreationRate:        0.10,
		CacheHitInputRate:        0.004,
		CacheMissInputRate:       0.03,
		CacheMissRate:            0.06,
	})
	// claude-haiku 系列：轻量，命中率高
	RegisterCacheSimForModel("claude-haiku", CacheSimConfig{
		CacheReadMultiplier:      1.15,
		CacheReadJitter:          0.06,
		CacheCreationProbability: 0.65,
		CacheCreationRate:        0.10,
		CacheHitInputRate:        0.004,
		CacheMissInputRate:       0.02,
		CacheMissRate:            0.06,
	})
}

// ---------------------------------------------------------------------------
// 确定性伪随机工具
// ---------------------------------------------------------------------------

// deterministicRand 基于稳定种子的确定性伪随机数生成器。
// 同一 (model, inputTokens) 组合始终产生相同结果，满足请求级确定性。
type deterministicRand struct {
	state uint64
}

// newDeterministicRand 创建基于 model + inputTokens 的确定性 PRNG
func newDeterministicRand(model string, inputTokens int64) *deterministicRand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(model))
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(inputTokens))
	_, _ = h.Write(b)
	return &deterministicRand{state: h.Sum64()}
}

// Float64 返回 [0.0, 1.0) 范围内的确定性伪随机浮点数
func (r *deterministicRand) Float64() float64 {
	// SplitMix64 算法
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z = z ^ (z >> 31)
	return float64(z>>11) / float64(1<<53)
}

// ---------------------------------------------------------------------------
// 核心算法
// ---------------------------------------------------------------------------

// simulateCacheUsage 根据模型和 inputTokens 计算模拟的缓存用量。
// 返回 (reducedInput, cacheRead, cacheCreation)。
// 使用确定性 PRNG，相同 (model, inputTokens) 始终产出相同结果。
func simulateCacheUsage(model string, inputTokens int64) (reducedInput, cacheRead, cacheCreation int64) {
	if inputTokens <= 10 {
		// 极短请求不注入，噪声太大
		return inputTokens, 0, 0
	}

	cfg := getConfigForModel(model)
	rng := newDeterministicRand(model, inputTokens)

	// 1. cache_read（每次都有）
	jitter := (rng.Float64()*2 - 1) * cfg.CacheReadJitter // [-jitter, +jitter]
	multiplier := cfg.CacheReadMultiplier + jitter
	cacheRead = int64(float64(inputTokens) * multiplier)

	// 2. cache_creation（概率触发）
	if rng.Float64() < cfg.CacheCreationProbability {
		creationJitter := 0.8 + rng.Float64()*0.4 // [0.8, 1.2]
		cacheCreation = int64(float64(inputTokens) * cfg.CacheCreationRate * creationJitter)
	}

	// 3. input_tokens（大幅缩减）
	if rng.Float64() < cfg.CacheMissRate {
		// 缓存 miss（冷启动）
		missJitter := 0.8 + rng.Float64()*0.4
		reducedInput = int64(float64(inputTokens) * cfg.CacheMissInputRate * missJitter)
	} else {
		// 缓存 hit
		hitJitter := 0.8 + rng.Float64()*0.4
		reducedInput = int64(float64(inputTokens) * cfg.CacheHitInputRate * hitJitter)
	}

	// 确保最小值为 1
	if reducedInput < 1 {
		reducedInput = 1
	}

	return reducedInput, cacheRead, cacheCreation
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
	if p.simDecided && usage != nil && usage.CacheReadInputTokens == 0 {
		usage.InputTokens = p.simReduced
		usage.CacheReadInputTokens = p.simCacheRead
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
	// 上游优先：若已有真实 cache 数据，跳过
	existingCacheRead := gjson.GetBytes(jsonData, "message.usage.cache_read_input_tokens")
	if existingCacheRead.Exists() && existingCacheRead.Int() > 0 {
		return nil
	}

	inputTokens := gjson.GetBytes(jsonData, "message.usage.input_tokens").Int()
	if inputTokens <= 10 {
		return nil
	}

	// 计算模拟值（确定性）
	reduced, cacheReadVal, cacheCreationVal := simulateCacheUsage(p.model, inputTokens)

	// 记录决策供后续 message_delta 复用
	p.simDecided = true
	p.simReduced = int(reduced)
	p.simCacheRead = int(cacheReadVal)
	p.simCacheCreation = int(cacheCreationVal)

	// 注入字段
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

	// 上游优先：若已有真实 cache 数据，跳过
	existingCacheRead := gjson.GetBytes(jsonData, "usage.cache_read_input_tokens")
	if existingCacheRead.Exists() && existingCacheRead.Int() > 0 {
		return nil
	}

	// 复用 message_start 阶段的决策
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
// 上游有真实 cache → 不注入。
func TransformGeminiToClaudeWithCacheSim(geminiResp []byte, originalModel string) ([]byte, *ClaudeUsage, error) {
	data, usage, err := TransformGeminiToClaude(geminiResp, originalModel)
	if err != nil || usage == nil {
		return data, usage, err
	}

	// 上游有真实 cache → 不注入
	if usage.CacheReadInputTokens > 0 {
		return data, usage, nil
	}

	// 极短请求不注入
	if usage.InputTokens <= 10 {
		return data, usage, nil
	}

	// 注入模拟 cache
	reduced, cacheRead, cacheCreation := simulateCacheUsage(originalModel, int64(usage.InputTokens))

	// 修改 JSON 响应体
	data = injectCacheIntoNonStreamJSON(data, int(reduced), int(cacheRead), int(cacheCreation))

	// 修正 usage（供计费）
	usage.InputTokens = int(reduced)
	usage.CacheReadInputTokens = int(cacheRead)
	usage.CacheCreationInputTokens = int(cacheCreation)

	return data, usage, nil
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
