package service

import (
	"context"
	"time"
)

// scope_type 取值。
const (
	UsageScopeTotal = "total"
	UsageScopeKey   = "key"
	UsageScopeGroup = "group"
	UsageScopeModel = "model"
)

// UpstreamUsageDailyEntry 适配器归一化后的"按日×维度"条目。
type UpstreamUsageDailyEntry struct {
	Day       time.Time // 配置时区当天 00:00
	ScopeType string
	ScopeKey  string
	ScopeName string
	Requests  int
	Tokens    int64
	CostUSD   float64
}

// DayRange 闭区间(均为配置时区当天 00:00)。
type DayRange struct {
	From time.Time
	To   time.Time
}

// UpstreamUsageResult 一次 FetchUsageDaily 的结果。CoveredDays 与 Entries 解耦:
// "成功查询到但当天为空"(应归零)≠"查询失败/未覆盖"(应保留)。
type UpstreamUsageResult struct {
	CoveredDays DayRange
	Entries     []UpstreamUsageDailyEntry
	Partial     bool
	Reason      string
}

// UpstreamUsageFetcher 适配器消耗采集能力(newapi/sub2api 各实现)。
type UpstreamUsageFetcher interface {
	FetchUsageDaily(ctx context.Context, p *UpstreamProvider, sinceDay time.Time) (UpstreamUsageResult, error)
}

// UpstreamUsageCursor 采集游标(对应 ent upstream_usage_cursor)。
type UpstreamUsageCursor struct {
	ProviderID          int64
	CollectStartedAt    *time.Time
	CollectedThroughDay *time.Time
	BackfillDone        bool
	BackfillOldestDay   *time.Time
	LastCollectedAt     *time.Time
	LastError           string
	LastPartial         bool
	PartialReason       string
}

// DayWriteMode 决定一天 rollup 的写入方式。
type DayWriteMode int

const (
	// DayWriteReplace 先删该日全部 scope 再插(空=删空,可把某日归零)。
	DayWriteReplace DayWriteMode = iota
	// DayWriteFillIfAbsent 仅当该日无任何行时才插(回填/补采,绝不删历史)。
	DayWriteFillIfAbsent
)

// DayWrite 一天的写入意图。
type DayWrite struct {
	Day     time.Time
	Mode    DayWriteMode
	Entries []UpstreamUsageDailyEntry
}

// CollectStateUpdate 释放租约时一并写回的游标状态(nil 字段=不更新)。
type CollectStateUpdate struct {
	CollectedThroughDay *time.Time
	LastCollectedAt     *time.Time
	LastError           string
	LastPartial         bool
	PartialReason       string
	BackfillDone        *bool
	BackfillOldestDay   *time.Time
}

// UsageWindowSum 单时间窗的 USD 聚合(¥ 由读取服务按 recharge_ratio 换算)。
type UsageWindowSum struct {
	CostUSD  float64
	Requests int64
}

// TodayWeekMonthTotal 四窗聚合容器。
type TodayWeekMonthTotal struct {
	Today UsageWindowSum
	Week  UsageWindowSum
	Month UsageWindowSum
	Total UsageWindowSum
}

// UsageScopeSum breakdown 单 scope_key 的聚合。
type UsageScopeSum struct {
	ScopeKey   string
	LatestName string
	CostUSD    float64
	Requests   int64
	Tokens     int64
}

// UpstreamUsageRepository usage 存储接口。
type UpstreamUsageRepository interface {
	GetOrCreateCursor(ctx context.Context, providerID int64) (*UpstreamUsageCursor, error)
	ListCursors(ctx context.Context, providerIDs []int64) (map[int64]*UpstreamUsageCursor, error)
	ClaimCollect(ctx context.Context, providerID int64, now, staleBefore time.Time) (ok bool, err error)
	ReleaseCollect(ctx context.Context, providerID int64, upd CollectStateUpdate) error
	WriteDays(ctx context.Context, providerID int64, writes []DayWrite) error
	SummaryBatch(ctx context.Context, providerIDs []int64, now time.Time) (map[int64]TodayWeekMonthTotal, error)
	Breakdown(ctx context.Context, providerID int64, scopeType string, from, to time.Time) ([]UsageScopeSum, error)
}
