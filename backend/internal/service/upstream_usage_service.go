package service

import (
	"context"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// UsageWindowStat 单时间窗(含 ¥)。
type UsageWindowStat struct {
	CostUSD  float64 `json:"cost_usd"`
	CostCNY  float64 `json:"cost_cny"`
	Requests int64   `json:"requests"`
}

// UsageSummary provider 消耗摘要。
type UsageSummary struct {
	Today          UsageWindowStat `json:"today"`
	Week           UsageWindowStat `json:"week"`
	Month          UsageWindowStat `json:"month"`
	Total          UsageWindowStat `json:"total"`
	BackfilledFrom *time.Time      `json:"backfilled_from,omitempty"`
	Partial        bool            `json:"partial"`
	PartialReason  string          `json:"partial_reason,omitempty"`
}

// UsageBreakdownRow breakdown 单行。
type UsageBreakdownRow struct {
	ScopeKey  string  `json:"scope_key"`
	ScopeName string  `json:"scope_name"`
	Deleted   bool    `json:"deleted"`
	CostUSD   float64 `json:"cost_usd"`
	CostCNY   float64 `json:"cost_cny"`
	Requests  int64   `json:"requests"`
	Tokens    int64   `json:"tokens"`
}

// UsageBreakdown breakdown 响应。
type UsageBreakdown struct {
	Scope     string              `json:"scope"`
	Window    string              `json:"window"`
	Supported bool                `json:"supported"`
	Items     []UsageBreakdownRow `json:"items"`
}

// UpstreamUsageService 消耗读取(summary/breakdown + ¥ 换算)。
type UpstreamUsageService struct {
	usage     UpstreamUsageRepository
	providers UpstreamProviderRepository
	adapters  map[string]UpstreamAdapter
}

func NewUpstreamUsageService(usage UpstreamUsageRepository, providers UpstreamProviderRepository, adapters map[string]UpstreamAdapter) *UpstreamUsageService {
	return &UpstreamUsageService{usage: usage, providers: providers, adapters: adapters}
}

func ProvideUpstreamUsageService(usage UpstreamUsageRepository, providers UpstreamProviderRepository, adapters map[string]UpstreamAdapter) *UpstreamUsageService {
	return NewUpstreamUsageService(usage, providers, adapters)
}

func cny(usd, ratio float64) float64 {
	if ratio <= 0 {
		ratio = 1
	}
	return math.Round(usd/ratio*100) / 100
}

func toWindowStat(s UsageWindowSum, ratio float64) UsageWindowStat {
	return UsageWindowStat{CostUSD: s.CostUSD, CostCNY: cny(s.CostUSD, ratio), Requests: s.Requests}
}

// SummariesBatch 列表用:批量算各 provider 摘要。
func (s *UpstreamUsageService) SummariesBatch(ctx context.Context, providers []*UpstreamProvider) (map[int64]UsageSummary, error) {
	ids := make([]int64, 0, len(providers))
	ratio := map[int64]float64{}
	for _, p := range providers {
		ids = append(ids, p.ID)
		ratio[p.ID] = p.RechargeRatio
	}
	sums, err := s.usage.SummaryBatch(ctx, ids, timezone.Now())
	if err != nil {
		return nil, err
	}
	cursors, err := s.usage.ListCursors(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]UsageSummary, len(ids))
	for _, id := range ids {
		w := sums[id]
		r := ratio[id]
		us := UsageSummary{
			Today: toWindowStat(w.Today, r),
			Week:  toWindowStat(w.Week, r),
			Month: toWindowStat(w.Month, r),
			Total: toWindowStat(w.Total, r),
		}
		if cur := cursors[id]; cur != nil {
			us.Partial = cur.LastPartial
			us.PartialReason = cur.PartialReason
			us.BackfilledFrom = cur.BackfillOldestDay
		}
		out[id] = us
	}
	return out, nil
}

// Breakdown 详情用:某 scope 在 window 的明细。
func (s *UpstreamUsageService) Breakdown(ctx context.Context, providerID int64, scope, window string) (UsageBreakdown, error) {
	p, err := s.providers.GetByID(ctx, providerID)
	if err != nil {
		return UsageBreakdown{}, err
	}
	bd := UsageBreakdown{Scope: scope, Window: window, Supported: true, Items: []UsageBreakdownRow{}}

	if scope == UsageScopeGroup && p.Type == domain.UpstreamTypeSub2API {
		bd.Supported = false
		return bd, nil
	}
	from, to := windowRange(window, timezone.Now())
	rows, err := s.usage.Breakdown(ctx, providerID, scope, from, to)
	if err != nil {
		return UsageBreakdown{}, err
	}
	live := s.currentScopeKeys(ctx, p, scope)
	for _, r := range rows {
		row := UsageBreakdownRow{
			ScopeKey: r.ScopeKey, ScopeName: r.LatestName,
			CostUSD: r.CostUSD, CostCNY: cny(r.CostUSD, p.RechargeRatio),
			Requests: r.Requests, Tokens: r.Tokens,
		}
		if live != nil && scope != UsageScopeModel {
			_, exists := live[r.ScopeKey]
			row.Deleted = !exists
		}
		bd.Items = append(bd.Items, row)
	}
	return bd, nil
}

// windowRange 返回 [from,to) 半开区间。total → 极早到极晚。
func windowRange(window string, now time.Time) (time.Time, time.Time) {
	end := timezone.StartOfDay(now).AddDate(0, 0, 1)
	switch window {
	case "today":
		return timezone.StartOfDay(now), end
	case "week":
		return timezone.StartOfWeek(now), end
	case "month":
		return timezone.StartOfMonth(now), end
	default:
		return time.Unix(0, 0), end
	}
}

// currentScopeKeys 取当前存在的 scope_key 集合(key→ListTokens;group→snapshot.groups)。
// 返回 nil 表示无法判定(如 ListTokens 失败),调用方据此不标 deleted。
func (s *UpstreamUsageService) currentScopeKeys(ctx context.Context, p *UpstreamProvider, scope string) map[string]struct{} {
	switch scope {
	case UsageScopeKey:
		adapter, ok := s.adapters[p.Type]
		if !ok {
			return nil
		}
		tokens, err := adapter.ListTokens(ctx, p)
		if err != nil {
			return nil
		}
		set := map[string]struct{}{}
		for _, t := range tokens {
			set[upstreamTokenIDString(t.ID)] = struct{}{}
		}
		return set
	case UsageScopeGroup:
		set := map[string]struct{}{}
		if p.LatestSnapshot != nil {
			for _, g := range p.LatestSnapshot.Groups {
				set[g.Name] = struct{}{}
			}
		}
		return set
	default:
		return nil
	}
}
