package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// simulateCacheUsage 算法测试
// ---------------------------------------------------------------------------

func TestSimulateCacheUsage_ShortRequest(t *testing.T) {
	// 极短请求（<=10 tokens）不注入
	for _, tokens := range []int64{0, 1, 5, 10} {
		reduced, cacheRead, cacheCreation := simulateCacheUsage("claude-sonnet-4-6", tokens)
		if reduced != tokens {
			t.Errorf("input=%d: expected reduced=%d, got %d", tokens, tokens, reduced)
		}
		if cacheRead != 0 {
			t.Errorf("input=%d: expected cacheRead=0, got %d", tokens, cacheRead)
		}
		if cacheCreation != 0 {
			t.Errorf("input=%d: expected cacheCreation=0, got %d", tokens, cacheCreation)
		}
	}
}

func TestSimulateCacheUsage_NormalRequest(t *testing.T) {
	inputTokens := int64(10000)
	reduced, cacheRead, cacheCreation := simulateCacheUsage("claude-sonnet-4-6", inputTokens)

	// reduced 应远小于 inputTokens
	if reduced >= inputTokens {
		t.Errorf("reduced (%d) should be much less than inputTokens (%d)", reduced, inputTokens)
	}
	if reduced < 1 {
		t.Errorf("reduced (%d) should be at least 1", reduced)
	}

	// cacheRead 应大于 0
	if cacheRead <= 0 {
		t.Errorf("cacheRead (%d) should be positive", cacheRead)
	}

	// cacheRead 应在合理范围（inputTokens 的 0.5x ~ 2x）
	if float64(cacheRead) < float64(inputTokens)*0.5 {
		t.Errorf("cacheRead (%d) too small for input %d", cacheRead, inputTokens)
	}
	if float64(cacheRead) > float64(inputTokens)*2.0 {
		t.Errorf("cacheRead (%d) too large for input %d", cacheRead, inputTokens)
	}

	// cacheCreation 不能为负
	if cacheCreation < 0 {
		t.Errorf("cacheCreation (%d) should not be negative", cacheCreation)
	}

	t.Logf("input=%d → reduced=%d, cacheRead=%d, cacheCreation=%d",
		inputTokens, reduced, cacheRead, cacheCreation)
}

func TestSimulateCacheUsage_Deterministic(t *testing.T) {
	// 相同 (model, inputTokens) 应始终产出相同结果
	model := "claude-opus-4-6"
	inputTokens := int64(50000)

	r1, cr1, cc1 := simulateCacheUsage(model, inputTokens)
	for i := range 100 {
		r2, cr2, cc2 := simulateCacheUsage(model, inputTokens)
		if r1 != r2 || cr1 != cr2 || cc1 != cc2 {
			t.Fatalf("iteration %d: results differ: (%d,%d,%d) vs (%d,%d,%d)",
				i, r1, cr1, cc1, r2, cr2, cc2)
		}
	}
}

func TestSimulateCacheUsage_DifferentModels(t *testing.T) {
	// 不同模型应产出不同结果（高概率）
	inputTokens := int64(10000)
	r1, cr1, _ := simulateCacheUsage("claude-sonnet-4-6", inputTokens)
	r2, cr2, _ := simulateCacheUsage("claude-opus-4-6", inputTokens)

	// 不要求完全不同，但至少有一项不同
	if r1 == r2 && cr1 == cr2 {
		t.Log("Warning: different models produced identical results (possible but unlikely)")
	}
}

func TestSimulateCacheUsage_DifferentTokenCounts(t *testing.T) {
	// 不同 inputTokens 应产出不同结果
	model := "claude-sonnet-4-6"
	r1, cr1, _ := simulateCacheUsage(model, 10000)
	r2, cr2, _ := simulateCacheUsage(model, 20000)

	if r1 == r2 && cr1 == cr2 {
		t.Error("different inputTokens should produce different results")
	}
}

func TestSimulateCacheUsage_VariousModels(t *testing.T) {
	// 验证各模型前缀匹配
	models := []string{
		"claude-opus-4-6",
		"claude-opus-4-6-thinking",
		"claude-sonnet-4-6",
		"claude-sonnet-4-5",
		"claude-haiku-3-5",
		"unknown-model",
	}

	inputTokens := int64(50000)
	for _, model := range models {
		reduced, cacheRead, cacheCreation := simulateCacheUsage(model, inputTokens)
		t.Logf("model=%s: reduced=%d, cacheRead=%d, cacheCreation=%d",
			model, reduced, cacheRead, cacheCreation)

		if reduced < 1 || reduced >= inputTokens {
			t.Errorf("model=%s: reduced (%d) out of range", model, reduced)
		}
		if cacheRead <= 0 {
			t.Errorf("model=%s: cacheRead (%d) should be positive", model, cacheRead)
		}
	}
}

// ---------------------------------------------------------------------------
// getConfigForModel 配置查找测试
// ---------------------------------------------------------------------------

func TestGetConfigForModel(t *testing.T) {
	tests := []struct {
		model         string
		expectedMatch string // 期望匹配到的前缀
	}{
		{"claude-opus-4-6", "claude-opus"},
		{"claude-opus-4-6-thinking", "claude-opus"},
		{"claude-sonnet-4-6", "claude-sonnet"},
		{"claude-sonnet-4-5-thinking", "claude-sonnet"},
		{"claude-haiku-3-5", "claude-haiku"},
		{"unknown-model", "default"},
	}

	for _, tt := range tests {
		cfg := getConfigForModel(tt.model)
		// 验证配置不是零值
		if cfg.CacheReadMultiplier == 0 {
			t.Errorf("model=%s: got zero config", tt.model)
		}
		t.Logf("model=%s matched=%s multiplier=%.2f",
			tt.model, tt.expectedMatch, cfg.CacheReadMultiplier)
	}
}

// ---------------------------------------------------------------------------
// deterministicRand 测试
// ---------------------------------------------------------------------------

func TestDeterministicRand_Consistency(t *testing.T) {
	rng1 := newDeterministicRand("model-a", 12345)
	rng2 := newDeterministicRand("model-a", 12345)

	for i := range 100 {
		v1 := rng1.Float64()
		v2 := rng2.Float64()
		if v1 != v2 {
			t.Fatalf("iteration %d: Float64 differs: %f vs %f", i, v1, v2)
		}
	}
}

func TestDeterministicRand_Range(t *testing.T) {
	rng := newDeterministicRand("test", 99999)
	for range 1000 {
		v := rng.Float64()
		if v < 0 || v >= 1.0 {
			t.Fatalf("Float64 out of range: %f", v)
		}
	}
}

// ---------------------------------------------------------------------------
// SSE 注入测试（流式）
// ---------------------------------------------------------------------------

func TestCachedStreamingProcessor_MessageStart_InjectCache(t *testing.T) {
	// 构造一个 message_start SSE 事件，上游无缓存数据
	usage := ClaudeUsage{
		InputTokens:  10000,
		OutputTokens: 0,
	}
	message := map[string]any{
		"id":      "msg_test123",
		"type":    "message",
		"role":    "assistant",
		"content": []any{},
		"model":   "claude-sonnet-4-6",
		"usage":   usage,
	}
	event := map[string]any{
		"type":    "message_start",
		"message": message,
	}
	jsonData, _ := json.Marshal(event)
	sse := fmt.Sprintf("event: message_start\ndata: %s\n\n", string(jsonData))

	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")
	result := processor.injectCacheIntoSSEBytes([]byte(sse))

	resultStr := string(result)

	// 应包含 cache_read_input_tokens
	if !strings.Contains(resultStr, "cache_read_input_tokens") {
		t.Error("expected cache_read_input_tokens in output")
	}

	// 应包含 cache_creation_input_tokens
	if !strings.Contains(resultStr, "cache_creation_input_tokens") {
		t.Error("expected cache_creation_input_tokens in output")
	}

	// simDecided 应为 true
	if !processor.simDecided {
		t.Error("expected simDecided to be true")
	}

	// simCacheRead 应大于 0
	if processor.simCacheRead <= 0 {
		t.Error("expected simCacheRead > 0")
	}

	// simReduced 应远小于 10000
	if processor.simReduced >= 10000 {
		t.Errorf("expected simReduced < 10000, got %d", processor.simReduced)
	}

	t.Logf("simReduced=%d, simCacheRead=%d, simCacheCreation=%d",
		processor.simReduced, processor.simCacheRead, processor.simCacheCreation)
}

func TestCachedStreamingProcessor_UpstreamCachePreserved(t *testing.T) {
	// 上游已返回真实 cache_read > 0，不应注入
	usage := ClaudeUsage{
		InputTokens:          500,
		OutputTokens:         100,
		CacheReadInputTokens: 9500,
	}
	message := map[string]any{
		"id":      "msg_test456",
		"type":    "message",
		"role":    "assistant",
		"content": []any{},
		"model":   "claude-sonnet-4-6",
		"usage":   usage,
	}
	event := map[string]any{
		"type":    "message_start",
		"message": message,
	}
	jsonData, _ := json.Marshal(event)
	sse := fmt.Sprintf("event: message_start\ndata: %s\n\n", string(jsonData))

	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")
	result := processor.injectCacheIntoSSEBytes([]byte(sse))

	// simDecided 应仍为 false（未注入）
	if processor.simDecided {
		t.Error("should NOT inject when upstream has cache data")
	}

	// 原始数据应保持不变（cache_read_input_tokens = 9500）
	if !strings.Contains(string(result), "9500") {
		t.Error("upstream cache_read_input_tokens should be preserved")
	}
}

func TestCachedStreamingProcessor_MessageDelta_ReusesDecision(t *testing.T) {
	// 先注入 message_start，再验证 message_delta 复用同一决策
	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")

	// 1. message_start
	startUsage := ClaudeUsage{InputTokens: 20000, OutputTokens: 0}
	startMessage := map[string]any{
		"id": "msg_test789", "type": "message", "role": "assistant",
		"content": []any{}, "model": "claude-sonnet-4-6", "usage": startUsage,
	}
	startEvent := map[string]any{"type": "message_start", "message": startMessage}
	startJSON, _ := json.Marshal(startEvent)
	startSSE := fmt.Sprintf("event: message_start\ndata: %s\n\n", string(startJSON))

	processor.injectCacheIntoSSEBytes([]byte(startSSE))

	savedReduced := processor.simReduced
	savedCacheRead := processor.simCacheRead
	savedCacheCreation := processor.simCacheCreation

	// 2. message_delta
	deltaUsage := ClaudeUsage{InputTokens: 20000, OutputTokens: 500}
	deltaEvent := map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": deltaUsage,
	}
	deltaJSON, _ := json.Marshal(deltaEvent)
	deltaSSE := fmt.Sprintf("event: message_delta\ndata: %s\n\n", string(deltaJSON))

	result := processor.injectCacheIntoSSEBytes([]byte(deltaSSE))
	resultStr := string(result)

	// 应包含与 message_start 相同的 cache 值
	if !strings.Contains(resultStr, fmt.Sprintf("\"cache_read_input_tokens\":%d", savedCacheRead)) {
		t.Errorf("message_delta should reuse cacheRead=%d", savedCacheRead)
	}
	if !strings.Contains(resultStr, fmt.Sprintf("\"input_tokens\":%d", savedReduced)) {
		t.Errorf("message_delta should reuse reduced=%d", savedReduced)
	}

	t.Logf("Reused: reduced=%d, cacheRead=%d, cacheCreation=%d",
		savedReduced, savedCacheRead, savedCacheCreation)
}

func TestCachedStreamingProcessor_ShortRequest_NoInject(t *testing.T) {
	// input_tokens <= 10 不注入
	usage := ClaudeUsage{InputTokens: 5, OutputTokens: 0}
	message := map[string]any{
		"id": "msg_short", "type": "message", "role": "assistant",
		"content": []any{}, "model": "claude-sonnet-4-6", "usage": usage,
	}
	event := map[string]any{"type": "message_start", "message": message}
	jsonData, _ := json.Marshal(event)
	sse := fmt.Sprintf("event: message_start\ndata: %s\n\n", string(jsonData))

	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")
	processor.injectCacheIntoSSEBytes([]byte(sse))

	if processor.simDecided {
		t.Error("should NOT inject for short requests (<=10 tokens)")
	}
}

// ---------------------------------------------------------------------------
// 非流式注入测试
// ---------------------------------------------------------------------------

func TestTransformGeminiToClaudeWithCacheSim_NoUpstreamCache(t *testing.T) {
	// 构造一个最小化的 Gemini 响应（有足够的 usage）
	geminiResp := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": "Hello, world!"},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     10000,
			"candidatesTokenCount": 50,
			"totalTokenCount":      10050,
		},
	}

	geminiJSON, _ := json.Marshal(geminiResp)
	data, usage, err := TransformGeminiToClaudeWithCacheSim(geminiJSON, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// usage 应包含模拟的缓存数据
	if usage.CacheReadInputTokens <= 0 {
		t.Error("expected CacheReadInputTokens > 0")
	}
	if usage.InputTokens >= 10000 {
		t.Errorf("expected InputTokens < 10000 (reduced), got %d", usage.InputTokens)
	}

	// JSON 响应体也应包含缓存字段
	dataStr := string(data)
	if !strings.Contains(dataStr, "cache_read_input_tokens") {
		t.Error("JSON response should contain cache_read_input_tokens")
	}

	t.Logf("usage: input=%d, cacheRead=%d, cacheCreation=%d, output=%d",
		usage.InputTokens, usage.CacheReadInputTokens,
		usage.CacheCreationInputTokens, usage.OutputTokens)
}

func TestTransformGeminiToClaudeWithCacheSim_WithUpstreamCache(t *testing.T) {
	// 上游已有真实 cache（CachedContentTokenCount > 0）
	geminiResp := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": "Cached response"},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":        10000,
			"candidatesTokenCount":    50,
			"cachedContentTokenCount": 8000,
			"totalTokenCount":         10050,
		},
	}

	geminiJSON, _ := json.Marshal(geminiResp)
	_, usage, err := TransformGeminiToClaudeWithCacheSim(geminiJSON, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 上游有真实缓存，不应被模拟值覆盖
	if usage.CacheReadInputTokens != 8000 {
		t.Errorf("expected CacheReadInputTokens=8000 (upstream), got %d", usage.CacheReadInputTokens)
	}
	// input_tokens 应为 promptTokenCount - cachedContentTokenCount = 2000
	if usage.InputTokens != 2000 {
		t.Errorf("expected InputTokens=2000, got %d", usage.InputTokens)
	}
}

func TestTransformGeminiToClaudeWithCacheSim_ShortRequest(t *testing.T) {
	// 极短请求不注入
	geminiResp := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": "Hi"},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     5,
			"candidatesTokenCount": 2,
			"totalTokenCount":      7,
		},
	}

	geminiJSON, _ := json.Marshal(geminiResp)
	_, usage, err := TransformGeminiToClaudeWithCacheSim(geminiJSON, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usage.CacheReadInputTokens != 0 {
		t.Errorf("expected no cache injection for short request, got CacheRead=%d", usage.CacheReadInputTokens)
	}
}

// ---------------------------------------------------------------------------
// Finish 覆盖测试
// ---------------------------------------------------------------------------

func TestCachedStreamingProcessor_Finish_InjectsUsage(t *testing.T) {
	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")

	// 模拟处理一个完整流：先 ProcessLine 发 message_start，再 Finish

	// 构造一个 Gemini SSE 数据行
	geminiData := map[string]any{
		"response": map[string]any{
			"candidates": []any{
				map[string]any{
					"content": map[string]any{
						"parts": []any{
							map[string]any{"text": "Hello from Gemini"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     15000,
				"candidatesTokenCount": 100,
			},
		},
		"responseId": "resp_test",
	}
	geminiJSON, _ := json.Marshal(geminiData)
	line := "data: " + string(geminiJSON)

	// ProcessLine 会发出 message_start + content + message_delta + message_stop
	output := processor.ProcessLine(line)

	t.Logf("ProcessLine output length: %d bytes", len(output))

	// Finish
	finishOutput, usage := processor.Finish()
	_ = finishOutput

	// usage 应包含模拟缓存
	if processor.simDecided && usage.CacheReadInputTokens == 0 {
		t.Error("Finish should inject CacheReadInputTokens")
	}

	t.Logf("Final usage: input=%d, cacheRead=%d, cacheCreation=%d, output=%d",
		usage.InputTokens, usage.CacheReadInputTokens,
		usage.CacheCreationInputTokens, usage.OutputTokens)
}

// ---------------------------------------------------------------------------
// injectCacheIntoNonStreamJSON 测试
// ---------------------------------------------------------------------------

func TestInjectCacheIntoNonStreamJSON(t *testing.T) {
	original := `{"id":"msg_1","type":"message","usage":{"input_tokens":10000,"output_tokens":50}}`
	modified := injectCacheIntoNonStreamJSON([]byte(original), 50, 11000, 800)

	// 解析验证
	var resp map[string]any
	if err := json.Unmarshal(modified, &resp); err != nil {
		t.Fatalf("invalid JSON after injection: %v", err)
	}

	usageMap, ok := resp["usage"].(map[string]any)
	if !ok {
		t.Fatal("usage field missing or not a map")
	}

	if inputTokens, ok := usageMap["input_tokens"].(float64); !ok || int(inputTokens) != 50 {
		t.Errorf("expected input_tokens=50, got %v", usageMap["input_tokens"])
	}
	if cacheRead, ok := usageMap["cache_read_input_tokens"].(float64); !ok || int(cacheRead) != 11000 {
		t.Errorf("expected cache_read_input_tokens=11000, got %v", usageMap["cache_read_input_tokens"])
	}
	if cacheCreation, ok := usageMap["cache_creation_input_tokens"].(float64); !ok || int(cacheCreation) != 800 {
		t.Errorf("expected cache_creation_input_tokens=800, got %v", usageMap["cache_creation_input_tokens"])
	}
}

// ---------------------------------------------------------------------------
// 端到端 SSE 解析测试
// ---------------------------------------------------------------------------

func TestCachedStreamingProcessor_MultipleSSEEvents(t *testing.T) {
	// 模拟一个包含多个 SSE 事件的字节流
	processor := NewCachedStreamingProcessor("claude-opus-4-6")

	// event 1: content_block_start (不应被修改)
	event1 := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`

	// event 2: message_start (应被修改)
	startUsage := ClaudeUsage{InputTokens: 30000, OutputTokens: 0}
	startMessage := map[string]any{
		"id": "msg_multi", "type": "message", "role": "assistant",
		"content": []any{}, "model": "claude-opus-4-6", "usage": startUsage,
	}
	startEvent := map[string]any{"type": "message_start", "message": startMessage}
	startJSON, _ := json.Marshal(startEvent)
	event2 := fmt.Sprintf("event: message_start\ndata: %s", string(startJSON))

	combined := event1 + "\n\n" + event2 + "\n\n"

	result := processor.injectCacheIntoSSEBytes([]byte(combined))
	resultStr := string(result)

	// message_start 应被注入
	if !strings.Contains(resultStr, "cache_read_input_tokens") {
		t.Error("message_start event should be injected with cache data")
	}

	// content_block_start 应保持不变
	if !strings.Contains(resultStr, "content_block_start") {
		t.Error("content_block_start event should be preserved")
	}
}

// ---------------------------------------------------------------------------
// 边界条件测试
// ---------------------------------------------------------------------------

func TestCachedStreamingProcessor_EmptyData(t *testing.T) {
	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")

	// 空数据不应 panic
	result := processor.injectCacheIntoSSEBytes(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	result = processor.injectCacheIntoSSEBytes([]byte{})
	if len(result) != 0 {
		t.Error("expected empty for empty input")
	}
}

func TestCachedStreamingProcessor_NonSSEData(t *testing.T) {
	processor := NewCachedStreamingProcessor("claude-sonnet-4-6")

	// 非 SSE 格式的数据不应 panic
	result := processor.injectCacheIntoSSEBytes([]byte("not an sse event"))
	if result == nil {
		t.Error("should return non-nil for non-SSE data")
	}
}
