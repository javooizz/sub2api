package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	upstreamusagecursor "github.com/Wei-Shaw/sub2api/ent/upstreamusagecursor"
	upstreamusagedaily "github.com/Wei-Shaw/sub2api/ent/upstreamusagedaily"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamUsageRepository struct {
	client *dbent.Client
}

// NewUpstreamUsageRepository 构造上游消耗 Repository。
func NewUpstreamUsageRepository(client *dbent.Client) service.UpstreamUsageRepository {
	return &upstreamUsageRepository{client: client}
}

func toServiceUsageCursor(e *dbent.UpstreamUsageCursor) *service.UpstreamUsageCursor {
	return &service.UpstreamUsageCursor{
		ProviderID:          e.ProviderID,
		CollectStartedAt:    e.CollectStartedAt,
		CollectedThroughDay: e.CollectedThroughDay,
		BackfillDone:        e.BackfillDone,
		BackfillOldestDay:   e.BackfillOldestDay,
		LastCollectedAt:     e.LastCollectedAt,
		LastError:           e.LastError,
		LastPartial:         e.LastPartial,
		PartialReason:       e.PartialReason,
	}
}

func (r *upstreamUsageRepository) GetOrCreateCursor(ctx context.Context, providerID int64) (*service.UpstreamUsageCursor, error) {
	e, err := r.client.UpstreamUsageCursor.Query().
		Where(upstreamusagecursor.ProviderIDEQ(providerID)).
		Only(ctx)
	if err == nil {
		return toServiceUsageCursor(e), nil
	}
	if !dbent.IsNotFound(err) {
		return nil, err
	}
	created, err := r.client.UpstreamUsageCursor.Create().
		SetProviderID(providerID).
		Save(ctx)
	if err != nil {
		if dbent.IsConstraintError(err) {
			e2, qerr := r.client.UpstreamUsageCursor.Query().
				Where(upstreamusagecursor.ProviderIDEQ(providerID)).Only(ctx)
			if qerr == nil {
				return toServiceUsageCursor(e2), nil
			}
			return nil, qerr
		}
		return nil, err
	}
	return toServiceUsageCursor(created), nil
}

func (r *upstreamUsageRepository) ListCursors(ctx context.Context, providerIDs []int64) (map[int64]*service.UpstreamUsageCursor, error) {
	out := make(map[int64]*service.UpstreamUsageCursor, len(providerIDs))
	if len(providerIDs) == 0 {
		return out, nil
	}
	rows, err := r.client.UpstreamUsageCursor.Query().
		Where(upstreamusagecursor.ProviderIDIn(providerIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range rows {
		out[e.ProviderID] = toServiceUsageCursor(e)
	}
	return out, nil
}

// ClaimCollect 条件 UPDATE 抢占采集租约(仿 ClaimRefresh)。
func (r *upstreamUsageRepository) ClaimCollect(ctx context.Context, providerID int64, now, staleBefore time.Time) (bool, error) {
	n, err := r.client.UpstreamUsageCursor.Update().
		Where(
			upstreamusagecursor.ProviderIDEQ(providerID),
			upstreamusagecursor.Or(
				upstreamusagecursor.CollectStartedAtIsNil(),
				upstreamusagecursor.CollectStartedAtLT(staleBefore),
			),
		).
		SetCollectStartedAt(now).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *upstreamUsageRepository) ReleaseCollect(ctx context.Context, providerID int64, upd service.CollectStateUpdate) error {
	u := r.client.UpstreamUsageCursor.Update().
		Where(upstreamusagecursor.ProviderIDEQ(providerID)).
		ClearCollectStartedAt().
		SetLastError(upd.LastError).
		SetLastPartial(upd.LastPartial).
		SetPartialReason(upd.PartialReason)
	if upd.CollectedThroughDay != nil {
		u = u.SetCollectedThroughDay(*upd.CollectedThroughDay)
	}
	if upd.LastCollectedAt != nil {
		u = u.SetLastCollectedAt(*upd.LastCollectedAt)
	}
	if upd.BackfillDone != nil {
		u = u.SetBackfillDone(*upd.BackfillDone)
	}
	if upd.BackfillOldestDay != nil {
		u = u.SetBackfillOldestDay(*upd.BackfillOldestDay)
	}
	_, err := u.Save(ctx)
	return err
}

// WriteDays 事务内按 Mode 写入多天 rollup(三区覆盖)。
func (r *upstreamUsageRepository) WriteDays(ctx context.Context, providerID int64, writes []service.DayWrite) error {
	if len(writes) == 0 {
		return nil
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, w := range writes {
		if w.Mode == service.DayWriteFillIfAbsent {
			n, err := tx.UpstreamUsageDaily.Query().
				Where(
					upstreamusagedaily.ProviderIDEQ(providerID),
					upstreamusagedaily.DayEQ(w.Day),
				).Count(ctx)
			if err != nil {
				return err
			}
			if n > 0 {
				continue
			}
		} else {
			if _, err := tx.UpstreamUsageDaily.Delete().
				Where(
					upstreamusagedaily.ProviderIDEQ(providerID),
					upstreamusagedaily.DayEQ(w.Day),
				).Exec(ctx); err != nil {
				return err
			}
		}
		for _, e := range w.Entries {
			if err := tx.UpstreamUsageDaily.Create().
				SetProviderID(providerID).
				SetDay(w.Day).
				SetScopeType(e.ScopeType).
				SetScopeKey(e.ScopeKey).
				SetScopeName(e.ScopeName).
				SetRequests(e.Requests).
				SetTokens(e.Tokens).
				SetCostUsd(e.CostUSD).
				Exec(ctx); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// SummaryBatch 加载各 provider 的 total 维度行,Go 侧按四时间窗求和。
func (r *upstreamUsageRepository) SummaryBatch(ctx context.Context, providerIDs []int64, now time.Time) (map[int64]service.TodayWeekMonthTotal, error) {
	out := make(map[int64]service.TodayWeekMonthTotal, len(providerIDs))
	if len(providerIDs) == 0 {
		return out, nil
	}
	rows, err := r.client.UpstreamUsageDaily.Query().
		Where(
			upstreamusagedaily.ProviderIDIn(providerIDs...),
			upstreamusagedaily.ScopeTypeEQ(service.UsageScopeTotal),
		).All(ctx)
	if err != nil {
		return nil, err
	}
	dayStart := timezone.StartOfDay(now)
	weekStart := timezone.StartOfWeek(now)
	monthStart := timezone.StartOfMonth(now)
	for _, e := range rows {
		w := out[e.ProviderID]
		cost, reqs := e.CostUsd, int64(e.Requests)
		add := func(s *service.UsageWindowSum) { s.CostUSD += cost; s.Requests += reqs }
		add(&w.Total)
		if !e.Day.Before(monthStart) {
			add(&w.Month)
		}
		if !e.Day.Before(weekStart) {
			add(&w.Week)
		}
		if !e.Day.Before(dayStart) {
			add(&w.Today)
		}
		out[e.ProviderID] = w
	}
	return out, nil
}

// Breakdown 加载某 scope 在 [from,to) 的行,Go 侧按 scope_key 聚合 + 取最新名。
func (r *upstreamUsageRepository) Breakdown(ctx context.Context, providerID int64, scopeType string, from, to time.Time) ([]service.UsageScopeSum, error) {
	rows, err := r.client.UpstreamUsageDaily.Query().
		Where(
			upstreamusagedaily.ProviderIDEQ(providerID),
			upstreamusagedaily.ScopeTypeEQ(scopeType),
			upstreamusagedaily.DayGTE(from),
			upstreamusagedaily.DayLT(to),
		).
		Order(dbent.Asc(upstreamusagedaily.FieldDay)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	agg := map[string]*service.UsageScopeSum{}
	for _, e := range rows {
		s, ok := agg[e.ScopeKey]
		if !ok {
			s = &service.UsageScopeSum{ScopeKey: e.ScopeKey}
			agg[e.ScopeKey] = s
		}
		s.CostUSD += e.CostUsd
		s.Requests += int64(e.Requests)
		s.Tokens += e.Tokens
		s.LatestName = e.ScopeName
	}
	out := make([]service.UsageScopeSum, 0, len(agg))
	for _, s := range agg {
		out = append(out, *s)
	}
	return out, nil
}
