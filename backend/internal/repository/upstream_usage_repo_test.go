package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newUsageEntRepo(t *testing.T) (*upstreamUsageRepository, *dbent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:upstream_usage?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return &upstreamUsageRepository{client: client}, client
}

func dayUTC(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestUsageCursorClaimReleaseLease(t *testing.T) {
	repo, _ := newUsageEntRepo(t)
	ctx := context.Background()
	_, err := repo.GetOrCreateCursor(ctx, 1)
	require.NoError(t, err)

	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	stale := now.Add(-10 * time.Minute)

	ok, err := repo.ClaimCollect(ctx, 1, now, stale)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = repo.ClaimCollect(ctx, 1, now.Add(time.Minute), now.Add(time.Minute).Add(-10*time.Minute))
	require.NoError(t, err)
	require.False(t, ok)

	later := now.Add(20 * time.Minute)
	ok, err = repo.ClaimCollect(ctx, 1, later, later.Add(-10*time.Minute))
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, repo.ReleaseCollect(ctx, 1, service.CollectStateUpdate{LastError: ""}))
	ok, err = repo.ClaimCollect(ctx, 1, later.Add(time.Hour), later.Add(time.Hour).Add(-10*time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestWriteDaysReplaceAndFill(t *testing.T) {
	repo, client := newUsageEntRepo(t)
	ctx := context.Background()
	d1 := dayUTC(2026, 6, 8)

	require.NoError(t, repo.WriteDays(ctx, 1, []service.DayWrite{{
		Day: d1, Mode: service.DayWriteReplace,
		Entries: []service.UpstreamUsageDailyEntry{
			{Day: d1, ScopeType: service.UsageScopeTotal, CostUSD: 5, Requests: 10},
			{Day: d1, ScopeType: service.UsageScopeKey, ScopeKey: "k1", ScopeName: "key1", CostUSD: 5, Requests: 10},
		},
	}}))
	cnt, _ := client.UpstreamUsageDaily.Query().Count(ctx)
	require.Equal(t, 2, cnt)

	require.NoError(t, repo.WriteDays(ctx, 1, []service.DayWrite{{
		Day: d1, Mode: service.DayWriteReplace, Entries: nil,
	}}))
	cnt, _ = client.UpstreamUsageDaily.Query().Count(ctx)
	require.Equal(t, 0, cnt, "Replace 空结果应删空该日")

	require.NoError(t, repo.WriteDays(ctx, 1, []service.DayWrite{{
		Day: d1, Mode: service.DayWriteFillIfAbsent,
		Entries: []service.UpstreamUsageDailyEntry{{Day: d1, ScopeType: service.UsageScopeTotal, CostUSD: 3}},
	}}))
	cnt, _ = client.UpstreamUsageDaily.Query().Count(ctx)
	require.Equal(t, 1, cnt)

	require.NoError(t, repo.WriteDays(ctx, 1, []service.DayWrite{{
		Day: d1, Mode: service.DayWriteFillIfAbsent,
		Entries: []service.UpstreamUsageDailyEntry{{Day: d1, ScopeType: service.UsageScopeTotal, CostUSD: 99}},
	}}))
	cnt, _ = client.UpstreamUsageDaily.Query().Count(ctx)
	require.Equal(t, 1, cnt, "FillIfAbsent 已有数据应跳过")
}

func TestSummaryBatchWindows(t *testing.T) {
	repo, _ := newUsageEntRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	today := dayUTC(2026, 6, 10)
	old := dayUTC(2026, 1, 1)

	require.NoError(t, repo.WriteDays(ctx, 1, []service.DayWrite{
		{Day: today, Mode: service.DayWriteReplace, Entries: []service.UpstreamUsageDailyEntry{
			{Day: today, ScopeType: service.UsageScopeTotal, CostUSD: 2, Requests: 4},
		}},
		{Day: old, Mode: service.DayWriteFillIfAbsent, Entries: []service.UpstreamUsageDailyEntry{
			{Day: old, ScopeType: service.UsageScopeTotal, CostUSD: 100, Requests: 1},
		}},
	}))

	m, err := repo.SummaryBatch(ctx, []int64{1}, now)
	require.NoError(t, err)
	w := m[1]
	require.InDelta(t, 2, w.Today.CostUSD, 1e-9)
	require.InDelta(t, 102, w.Total.CostUSD, 1e-9)
	require.InDelta(t, 2, w.Month.CostUSD, 1e-9)
}

func TestBreakdownLatestName(t *testing.T) {
	repo, _ := newUsageEntRepo(t)
	ctx := context.Background()
	d1, d2 := dayUTC(2026, 6, 8), dayUTC(2026, 6, 9)
	require.NoError(t, repo.WriteDays(ctx, 1, []service.DayWrite{
		{Day: d1, Mode: service.DayWriteReplace, Entries: []service.UpstreamUsageDailyEntry{
			{Day: d1, ScopeType: service.UsageScopeKey, ScopeKey: "k1", ScopeName: "old", CostUSD: 1, Requests: 1},
		}},
		{Day: d2, Mode: service.DayWriteReplace, Entries: []service.UpstreamUsageDailyEntry{
			{Day: d2, ScopeType: service.UsageScopeKey, ScopeKey: "k1", ScopeName: "new", CostUSD: 3, Requests: 2},
		}},
	}))
	rows, err := repo.Breakdown(ctx, 1, service.UsageScopeKey, d1, d2.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "new", rows[0].LatestName)
	require.InDelta(t, 4, rows[0].CostUSD, 1e-9)
}
