package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func collDay(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestComputeSinceDay(t *testing.T) {
	today := collDay(2026, 6, 10)
	cth := collDay(2026, 6, 10)
	require.Equal(t, collDay(2026, 6, 9), computeSinceDay(today, &cth, 2, 90))
	cth2 := collDay(2026, 6, 5)
	require.Equal(t, collDay(2026, 6, 6), computeSinceDay(today, &cth2, 2, 90))
	require.Equal(t, collDay(2026, 6, 9), computeSinceDay(today, nil, 2, 90))
	cth3 := collDay(2025, 1, 1)
	require.Equal(t, collDay(2026, 3, 12), computeSinceDay(today, &cth3, 2, 90))
}

func TestBuildIncrementWrites_ZonesAndZeroing(t *testing.T) {
	today := collDay(2026, 6, 10)
	covered := DayRange{From: collDay(2026, 6, 7), To: today}
	entries := []UpstreamUsageDailyEntry{
		{Day: collDay(2026, 6, 9), ScopeType: UsageScopeTotal, CostUSD: 1},
		{Day: collDay(2026, 6, 7), ScopeType: UsageScopeTotal, CostUSD: 2},
	}
	writes := buildIncrementWrites(covered, today, 2, entries)
	modeByDay := map[string]DayWriteMode{}
	for _, w := range writes {
		modeByDay[w.Day.Format("2006-01-02")] = w.Mode
	}
	require.Equal(t, DayWriteReplace, modeByDay["2026-06-10"])
	require.Equal(t, DayWriteReplace, modeByDay["2026-06-09"])
	require.Equal(t, DayWriteFillIfAbsent, modeByDay["2026-06-07"])
	_, has := modeByDay["2026-06-08"]
	require.False(t, has)
}

type collFakeUsageRepo struct {
	cursor   *UpstreamUsageCursor
	claimOK  bool
	written  []DayWrite
	released *CollectStateUpdate
}

func (f *collFakeUsageRepo) GetOrCreateCursor(_ context.Context, pid int64) (*UpstreamUsageCursor, error) {
	if f.cursor == nil {
		f.cursor = &UpstreamUsageCursor{ProviderID: pid}
	}
	return f.cursor, nil
}
func (f *collFakeUsageRepo) ListCursors(context.Context, []int64) (map[int64]*UpstreamUsageCursor, error) {
	return nil, nil
}
func (f *collFakeUsageRepo) ClaimCollect(context.Context, int64, time.Time, time.Time) (bool, error) {
	return f.claimOK, nil
}
func (f *collFakeUsageRepo) ReleaseCollect(_ context.Context, _ int64, upd CollectStateUpdate) error {
	f.released = &upd
	return nil
}
func (f *collFakeUsageRepo) WriteDays(_ context.Context, _ int64, w []DayWrite) error {
	f.written = append(f.written, w...)
	return nil
}
func (f *collFakeUsageRepo) SummaryBatch(context.Context, []int64, time.Time) (map[int64]TodayWeekMonthTotal, error) {
	return nil, nil
}
func (f *collFakeUsageRepo) Breakdown(context.Context, int64, string, time.Time, time.Time) ([]UsageScopeSum, error) {
	return nil, nil
}

type collFakeFetcher struct{ res UpstreamUsageResult }

func (f collFakeFetcher) FetchUsageDaily(context.Context, *UpstreamProvider, time.Time) (UpstreamUsageResult, error) {
	return f.res, nil
}

type collFakeProviderRepo struct{ p *UpstreamProvider }

func (f collFakeProviderRepo) GetByID(context.Context, int64) (*UpstreamProvider, error) { return f.p, nil }
func (collFakeProviderRepo) Create(context.Context, *UpstreamProvider) (*UpstreamProvider, error) {
	panic("n/a")
}
func (collFakeProviderRepo) List(context.Context) ([]*UpstreamProvider, error)  { panic("n/a") }
func (collFakeProviderRepo) Update(context.Context, *UpstreamProvider) (*UpstreamProvider, error) {
	panic("n/a")
}
func (collFakeProviderRepo) Delete(context.Context, int64) error                 { panic("n/a") }
func (collFakeProviderRepo) ListDue(context.Context, time.Time) ([]int64, error) { panic("n/a") }
func (collFakeProviderRepo) ClaimRefresh(context.Context, int64, time.Time, time.Time) (ClaimedRefresh, bool, error) {
	panic("n/a")
}
func (collFakeProviderRepo) CommitRefresh(context.Context, CommitRefreshInput) (bool, []UpstreamChangeEvent, error) {
	panic("n/a")
}
func (collFakeProviderRepo) UpdateCredentials(context.Context, int64, map[string]any) error {
	panic("n/a")
}
func (collFakeProviderRepo) MarkEventsNotified(context.Context, []int64) error { panic("n/a") }

func TestCollectProvider_LeaseHeldSkips(t *testing.T) {
	repo := &collFakeUsageRepo{claimOK: false}
	c := NewUpstreamUsageCollector(repo, collFakeProviderRepo{p: &UpstreamProvider{ID: 1, Type: "newapi"}},
		map[string]UpstreamUsageFetcher{"newapi": collFakeFetcher{}})
	require.NoError(t, c.CollectProvider(context.Background(), 1))
	require.Nil(t, repo.released, "租约被占应直接跳过,不释放")
	require.Empty(t, repo.written)
}
