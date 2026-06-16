package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	newapiUsagePageSize       = 100
	newapiUsageMaxPages       = 200
	newapiUsagePageTimeout    = 30 * time.Second // 单次日志/状态请求超时(每页独立,替代整次共用)
	newapiUsageOverallTimeout = 5 * time.Minute  // 整次采集安全上限(防翻页失控)
	newapiUsagePageRetries    = 2                // 单页失败重试次数(慢页/抖动给机会)
	newapiUsagePageInterval   = 300 * time.Millisecond // 页间间隔(降低上游压力与限流概率)
	newapiLogTypeConsume      = 2 // 消费日志
	newapiLogTypeRefund       = 6 // 退款日志
)

// newapiLogEntry 对应 new-api model.Log 的采集所需字段。
type newapiLogEntry struct {
	ID               int64   `json:"id"`
	CreatedAt        int64   `json:"created_at"`
	Type             int     `json:"type"`
	Quota            float64 `json:"quota"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TokenID          int64   `json:"token_id"`
	TokenName        string  `json:"token_name"`
	ModelName        string  `json:"model_name"`
	Group            string  `json:"group"`
}

// normalizeNewapiUsage 把消费/退款日志聚合成按日×维度 Entry(纯函数)。
// type=2 计正、type=6 计负;退款缺 token_id/group 只入 total。
func normalizeNewapiUsage(logs []newapiLogEntry, quotaPerUnit float64) []UpstreamUsageDailyEntry {
	if quotaPerUnit <= 0 {
		quotaPerUnit = newapiDefaultQuotaPerUnit
	}
	type acc struct {
		day       time.Time
		scopeType string
		scopeKey  string
		name      string
		cost      float64
		requests  int
		tokens    int64
	}
	buckets := map[string]*acc{}
	add := func(day time.Time, st, sk, name string, cost float64, toks int64) {
		k := day.Format("2006-01-02") + "|" + st + "|" + sk
		a, ok := buckets[k]
		if !ok {
			a = &acc{day: day, scopeType: st, scopeKey: sk, name: name}
			buckets[k] = a
		}
		if name != "" {
			a.name = name
		}
		a.cost += cost
		a.requests++
		a.tokens += toks
	}
	for _, l := range logs {
		if l.Type != newapiLogTypeConsume && l.Type != newapiLogTypeRefund {
			continue
		}
		sign := 1.0
		if l.Type == newapiLogTypeRefund {
			sign = -1.0
		}
		cost := sign * l.Quota / quotaPerUnit
		day := timezone.StartOfDay(time.Unix(l.CreatedAt, 0))
		toks := int64(l.PromptTokens + l.CompletionTokens)
		add(day, UsageScopeTotal, "", "", cost, toks)
		if l.TokenID != 0 {
			add(day, UsageScopeKey, strconv.FormatInt(l.TokenID, 10), l.TokenName, cost, toks)
		}
		if l.Group != "" {
			add(day, UsageScopeGroup, l.Group, l.Group, cost, toks)
		}
		if l.ModelName != "" {
			add(day, UsageScopeModel, l.ModelName, l.ModelName, cost, toks)
		}
	}
	out := make([]UpstreamUsageDailyEntry, 0, len(buckets))
	for _, a := range buckets {
		out = append(out, UpstreamUsageDailyEntry{
			Day: a.day, ScopeType: a.scopeType, ScopeKey: a.scopeKey, ScopeName: a.name,
			Requests: a.requests, Tokens: a.tokens, CostUSD: a.cost,
		})
	}
	return out
}

// FetchUsageDaily 翻 /api/log/self(仅消费 type=2 与退款 type=6)→ 归一化。
// 每个 HTTP 请求独立超时(newapiUsagePageTimeout),不再整次共用一个超时:
// 日志量大的上游单查很慢,整次 60s 易在 p=1 就 awaiting headers 超时致一条都采不到;
// 改为每页独立超时 + 整次安全上限,并按 type 精确拉取(只消费/退款),缩小上游扫描面。
func (a *NewAPIAdapter) FetchUsageDaily(ctx context.Context, p *UpstreamProvider, sinceDay time.Time) (UpstreamUsageResult, error) {
	ctx, cancel := context.WithTimeout(ctx, newapiUsageOverallTimeout)
	defer cancel()
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return UpstreamUsageResult{}, err
	}
	// 翻页超时由每页 ctx(newapiUsagePageTimeout)控制;清掉 http.Client 自带的 15s
	// 单接口超时,否则它先于 30s 页超时触发,慢分页站点永远采不到数据。
	client.client.Timeout = 0
	today := timezone.Today()
	res := UpstreamUsageResult{CoveredDays: DayRange{From: sinceDay, To: today}}

	// quota_per_unit 探测(独立超时,不与翻页互相挤占)
	quotaPerUnit := newapiDefaultQuotaPerUnit
	sctx, scancel := context.WithTimeout(ctx, newapiUsagePageTimeout)
	var statusEnv newapiEnvelope
	_, statusErr := client.doJSON(sctx, http.MethodGet, "/api/status", nil, nil, &statusEnv)
	scancel()
	if statusErr == nil && statusEnv.Success {
		var st struct {
			QuotaPerUnit float64 `json:"quota_per_unit"`
		}
		if json.Unmarshal(statusEnv.Data, &st) == nil && st.QuotaPerUnit > 0 {
			quotaPerUnit = st.QuotaPerUnit
		} else {
			res.Partial, res.Reason = true, "quota_per_unit 探测失败"
		}
	} else {
		res.Partial, res.Reason = true, "quota_per_unit 探测失败"
	}

	startTs := sinceDay.Unix()
	var all []newapiLogEntry
	// 仅采消费(2)与退款(6):缩小上游扫描面、降低单查超时概率(消耗口径取 type∈{2,6})。
	// 单页失败不再整次失败:保留已拉到的页并标记 Partial(由 collector 决定可写范围),
	// 否则慢分页站点一次超时就丢掉全部已采数据、永远出不了数。
	pageErr := false
	for _, logType := range []int{newapiLogTypeConsume, newapiLogTypeRefund} {
		if pageErr {
			break
		}
		for page := 1; page <= newapiUsageMaxPages; page++ {
			if page > 1 {
				time.Sleep(newapiUsagePageInterval)
			}
			path := "/api/log/self?type=" + strconv.Itoa(logType) +
				"&start_timestamp=" + strconv.FormatInt(startTs, 10) +
				"&p=" + strconv.Itoa(page) + "&page_size=" + strconv.Itoa(newapiUsagePageSize)
			var env newapiEnvelope
			var perr error
			for attempt := 0; attempt <= newapiUsagePageRetries; attempt++ {
				pctx, pcancel := context.WithTimeout(ctx, newapiUsagePageTimeout)
				env = newapiEnvelope{}
				t0 := time.Now()
				_, perr = client.doJSON(pctx, http.MethodGet, path, nil, a.authHeaders(p), &env)
				pcancel()
				if perr != nil || time.Since(t0) > 10*time.Second {
					slog.Debug("upstream usage: 日志页采集慢/失败",
						"provider_id", p.ID, "type", logType, "page", page,
						"attempt", attempt, "elapsed_ms", time.Since(t0).Milliseconds(), "err", perr)
				}
				if perr == nil || ctx.Err() != nil { // 整次超时则不再重试
					break
				}
			}
			if perr != nil {
				if len(all) == 0 {
					return UpstreamUsageResult{}, perr
				}
				res.Partial, res.Reason = true, "日志分页中断: "+perr.Error()
				pageErr = true
				break
			}
			if !env.Success {
				res.Partial, res.Reason = true, "日志分页失败"
				break
			}
			var wrap struct {
				Items []newapiLogEntry `json:"items"`
			}
			var batch []newapiLogEntry
			if json.Unmarshal(env.Data, &wrap) == nil && wrap.Items != nil {
				batch = wrap.Items
			} else {
				_ = json.Unmarshal(env.Data, &batch)
			}
			all = append(all, batch...)
			if len(batch) < newapiUsagePageSize {
				break
			}
		}
	}
	res.Entries = normalizeNewapiUsage(all, quotaPerUnit)
	return res, nil
}
