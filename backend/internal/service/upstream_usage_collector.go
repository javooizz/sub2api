package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// ProvideUpstreamUsageFetchers 两种上游的消耗采集器(复用无状态适配器实例)。
// map 键必须与 provider.Type / ProvideUpstreamAdapters 一致。
func ProvideUpstreamUsageFetchers() map[string]UpstreamUsageFetcher {
	return map[string]UpstreamUsageFetcher{
		domain.UpstreamTypeNewAPI:  NewNewAPIAdapter(),
		domain.UpstreamTypeSub2API: NewSub2APIAdapter(),
	}
}

const (
	usageMutableWindowDays = 2 // K:可变窗天数(today, today-1)
	// 回填窗口:首次只回填最近 N 天。曾为 90,但日志量大的上游 /api/log/self 单查 90 天
	// 易整体超时致一条都采不到;收窄到 7 天让首采快速成功(更早历史以 backfill_oldest_day 标注)。
	usageBackfillDays      = 7
	usageCollectStaleAfter = 10 * time.Minute
)

// computeSinceDay 算增量重算起点:
// min(today-(K-1), collectedThroughDay+1),下限 today-backfill。nil → today-(K-1)。
func computeSinceDay(today time.Time, collectedThroughDay *time.Time, k, backfillDays int) time.Time {
	mutableStart := today.AddDate(0, 0, -(k - 1))
	since := mutableStart
	if collectedThroughDay != nil {
		cand := collectedThroughDay.AddDate(0, 0, 1)
		if cand.Before(since) {
			since = cand
		}
	}
	low := today.AddDate(0, 0, -backfillDays)
	if since.Before(low) {
		since = low
	}
	return since
}

// dayWriteMode 可变窗内 = Replace,否则 FillIfAbsent。
func dayWriteMode(day, today time.Time, k int) DayWriteMode {
	mutableStart := today.AddDate(0, 0, -(k - 1))
	if !day.Before(mutableStart) {
		return DayWriteReplace
	}
	return DayWriteFillIfAbsent
}

// groupEntriesByDay 把 Entry 按 day 分桶。
func groupEntriesByDay(entries []UpstreamUsageDailyEntry) map[string][]UpstreamUsageDailyEntry {
	m := map[string][]UpstreamUsageDailyEntry{}
	for _, e := range entries {
		k := e.Day.Format("2006-01-02")
		m[k] = append(m[k], e)
	}
	return m
}

// buildIncrementWrites 遍历 covered 范围每一天,产出 DayWrite(空日也产出以便 Replace 删空)。
// partial=true 表示本次采集数据不完整:只写可变窗内(Replace)的天——这些天下一轮会被
// 重新 Replace 修正;窗外 FillIfAbsent 一旦写入不完整数据就不会再被覆盖(永久偏小),跳过。
func buildIncrementWrites(covered DayRange, today time.Time, k int, entries []UpstreamUsageDailyEntry, partial bool) []DayWrite {
	byDay := groupEntriesByDay(entries)
	var writes []DayWrite
	for d := covered.From; !d.After(today); d = d.AddDate(0, 0, 1) {
		mode := dayWriteMode(d, today, k)
		dayKey := d.Format("2006-01-02")
		es := byDay[dayKey]
		if mode == DayWriteFillIfAbsent && (partial || len(es) == 0) {
			continue
		}
		writes = append(writes, DayWrite{Day: d, Mode: mode, Entries: es})
	}
	return writes
}

// UpstreamUsageCollector 采集编排(租约抢占 + 三区覆盖 + 回填)。
type UpstreamUsageCollector struct {
	repo      UpstreamUsageRepository
	providers UpstreamProviderRepository
	fetchers  map[string]UpstreamUsageFetcher
}

func NewUpstreamUsageCollector(repo UpstreamUsageRepository, providers UpstreamProviderRepository, fetchers map[string]UpstreamUsageFetcher) *UpstreamUsageCollector {
	return &UpstreamUsageCollector{repo: repo, providers: providers, fetchers: fetchers}
}

// CollectProvider 对单 provider 采集一轮(增量 + 必要时回填)。
func (c *UpstreamUsageCollector) CollectProvider(ctx context.Context, providerID int64) error {
	p, err := c.providers.GetByID(ctx, providerID)
	if err != nil {
		return err
	}
	if p.Status == domain.UpstreamStatusDisabled {
		return nil
	}
	fetcher, ok := c.fetchers[p.Type]
	if !ok {
		return nil
	}
	cursor, err := c.repo.GetOrCreateCursor(ctx, providerID)
	if err != nil {
		return err
	}
	now := timezone.Now()
	ok, err = c.repo.ClaimCollect(ctx, providerID, now, now.Add(-usageCollectStaleAfter))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	today := timezone.Today()
	upd := CollectStateUpdate{}

	since := computeSinceDay(today, cursor.CollectedThroughDay, usageMutableWindowDays, usageBackfillDays)
	res, ferr := fetcher.FetchUsageDaily(ctx, p, since)
	if ferr != nil {
		upd.LastError = ferr.Error()
		_ = c.releaseCollect(providerID, upd)
		return ferr
	}
	writes := buildIncrementWrites(res.CoveredDays, today, usageMutableWindowDays, res.Entries, res.Partial)
	if err := c.repo.WriteDays(ctx, providerID, writes); err != nil {
		upd.LastError = err.Error()
		_ = c.releaseCollect(providerID, upd)
		return err
	}
	// partial 时不推进游标:窗外天被跳过未写,推进会导致缺口永远不补。
	if !res.Partial {
		ct := today
		upd.CollectedThroughDay = &ct
	}
	upd.LastCollectedAt = &now
	upd.LastPartial = res.Partial
	upd.PartialReason = res.Reason

	if !cursor.BackfillDone {
		if oldest, berr := c.backfill(ctx, p, fetcher, today); berr != nil {
			slog.Warn("upstream usage: 回填失败", "provider_id", providerID, "error", berr)
			upd.LastPartial = true
			if upd.PartialReason == "" {
				upd.PartialReason = "回填未完成"
			}
		} else {
			done := true
			upd.BackfillDone = &done
			upd.BackfillOldestDay = oldest
		}
	}

	return c.releaseCollect(providerID, upd)
}

// releaseCollect 用独立 ctx 释放租约并落错误状态:调用方 ctx 可能已超时/取消
// (手动刷新的异步采集到达上限时),复用它会导致租约清不掉、错误状态丢失,
// 后续采集在 stale 窗口(10min)内被全部跳过。
func (c *UpstreamUsageCollector) releaseCollect(providerID int64, upd CollectStateUpdate) error {
	rctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.repo.ReleaseCollect(rctx, providerID, upd)
}

// backfill 回填 [today-90, today-K] 区间(FillIfAbsent),返回实际最早日。
func (c *UpstreamUsageCollector) backfill(ctx context.Context, p *UpstreamProvider, fetcher UpstreamUsageFetcher, today time.Time) (*time.Time, error) {
	since := today.AddDate(0, 0, -usageBackfillDays)
	res, err := fetcher.FetchUsageDaily(ctx, p, since)
	if err != nil {
		return nil, err
	}
	// 回填全部走 FillIfAbsent,不完整数据写入即永久偏小 → partial 视为本轮回填失败,下轮重试。
	if res.Partial {
		return nil, fmt.Errorf("采集不完整: %s", res.Reason)
	}
	cutoff := today.AddDate(0, 0, -(usageMutableWindowDays - 1))
	byDay := groupEntriesByDay(res.Entries)
	var writes []DayWrite
	var oldest *time.Time
	for d := since; d.Before(cutoff); d = d.AddDate(0, 0, 1) {
		es := byDay[d.Format("2006-01-02")]
		if len(es) == 0 {
			continue
		}
		writes = append(writes, DayWrite{Day: d, Mode: DayWriteFillIfAbsent, Entries: es})
		if oldest == nil {
			dd := d
			oldest = &dd
		}
	}
	if err := c.repo.WriteDays(ctx, p.ID, writes); err != nil {
		return nil, err
	}
	if oldest == nil {
		oldest = &since
	}
	return oldest, nil
}
