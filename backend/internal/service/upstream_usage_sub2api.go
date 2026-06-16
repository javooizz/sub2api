package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// sub2apiDailyEntry 对应 /api/v1/user/api-keys/:id/usage/daily 的每日条目。
type sub2apiDailyEntry struct {
	Date        string  `json:"date"`
	Requests    int     `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	ActualCost  float64 `json:"actual_cost"`
}

// normalizeSub2apiDaily 把单密钥的按日条目转成 key 维度 Entry(纯函数)。
func normalizeSub2apiDaily(keyID, keyName string, daily []sub2apiDailyEntry) []UpstreamUsageDailyEntry {
	out := make([]UpstreamUsageDailyEntry, 0, len(daily))
	for _, d := range daily {
		day, err := time.ParseInLocation("2006-01-02", d.Date, timezone.Today().Location())
		if err != nil {
			continue
		}
		out = append(out, UpstreamUsageDailyEntry{
			Day: day, ScopeType: UsageScopeKey, ScopeKey: keyID, ScopeName: keyName,
			Requests: d.Requests, Tokens: d.TotalTokens, CostUSD: d.ActualCost,
		})
	}
	return out
}

// mergeTotals 把全部 key 维度条目按日汇总成 total 维度。
func mergeTotals(keyEntries []UpstreamUsageDailyEntry) []UpstreamUsageDailyEntry {
	type acc struct {
		cost     float64
		requests int
		tokens   int64
	}
	m := map[string]*acc{}
	dayOf := map[string]time.Time{}
	for _, e := range keyEntries {
		k := e.Day.Format("2006-01-02")
		dayOf[k] = e.Day
		a := m[k]
		if a == nil {
			a = &acc{}
			m[k] = a
		}
		a.cost += e.CostUSD
		a.requests += e.Requests
		a.tokens += e.Tokens
	}
	out := make([]UpstreamUsageDailyEntry, 0, len(m))
	for k, a := range m {
		out = append(out, UpstreamUsageDailyEntry{
			Day: dayOf[k], ScopeType: UsageScopeTotal, ScopeKey: "",
			Requests: a.requests, Tokens: a.tokens, CostUSD: a.cost,
		})
	}
	return out
}

// FetchUsageDaily 列密钥 → 每密钥按日聚合 → key + total。sub2api 无 group 维度。
func (a *Sub2APIAdapter) FetchUsageDaily(ctx context.Context, p *UpstreamProvider, sinceDay time.Time) (UpstreamUsageResult, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamSnapshotTimeout)
	defer cancel()
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return UpstreamUsageResult{}, err
	}
	// 单请求超时交由整体 ctx 控制(同 newapi 采集:Client.Timeout 15s 会抢先掐断慢接口)。
	client.client.Timeout = 0
	today := timezone.Today()
	res := UpstreamUsageResult{CoveredDays: DayRange{From: sinceDay, To: today}}

	keys, err := a.ListTokens(ctx, p)
	if err != nil {
		return UpstreamUsageResult{}, err
	}
	days := int(today.Sub(sinceDay).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}
	var keyEntries []UpstreamUsageDailyEntry
	for _, k := range keys {
		idStr := upstreamTokenIDString(k.ID)
		if idStr == "" {
			continue
		}
		var env sub2apiEnvelope
		path := "/api/v1/user/api-keys/" + idStr + "/usage/daily?days=" + strconv.Itoa(days)
		if _, err := client.doJSON(ctx, http.MethodGet, path, nil, a.authHeaders(p), &env); err != nil {
			res.Partial, res.Reason = true, "部分密钥日用量拉取失败"
			continue
		}
		if env.Code != 0 {
			res.Partial, res.Reason = true, "部分密钥日用量返回失败"
			continue
		}
		var daily []sub2apiDailyEntry
		var wrap struct {
			DailyUsage []sub2apiDailyEntry `json:"daily_usage"`
		}
		if json.Unmarshal(env.Data, &wrap) == nil && wrap.DailyUsage != nil {
			daily = wrap.DailyUsage
		} else {
			_ = json.Unmarshal(env.Data, &daily)
		}
		keyEntries = append(keyEntries, normalizeSub2apiDaily(idStr, k.Name, daily)...)
	}
	res.Entries = append(keyEntries, mergeTotals(keyEntries)...)
	return res, nil
}

// upstreamTokenIDString 把 UpstreamToken.ID(any)转字符串。
func upstreamTokenIDString(id any) string {
	switch v := id.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}
