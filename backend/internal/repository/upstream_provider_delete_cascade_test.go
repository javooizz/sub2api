package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	upstreamusagedaily "github.com/Wei-Shaw/sub2api/ent/upstreamusagedaily"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestDeleteCascadesUsageTables(t *testing.T) {
	db, err := sql.Open("sqlite", "file:del_cascade?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	provRepo := &upstreamProviderRepository{client: client}
	usageRepo := &upstreamUsageRepository{client: client}

	p, err := provRepo.Create(ctx, &service.UpstreamProvider{
		Name: "p", Type: domain.UpstreamTypeNewAPI, SiteURL: "http://x",
		Credentials: map[string]any{"access_token": "t"}, RefreshIntervalMinutes: 60,
	})
	require.NoError(t, err)
	_, err = usageRepo.GetOrCreateCursor(ctx, p.ID)
	require.NoError(t, err)
	require.NoError(t, usageRepo.WriteDays(ctx, p.ID, []service.DayWrite{{
		Day: dayUTC(2026, 6, 9), Mode: service.DayWriteReplace,
		Entries: []service.UpstreamUsageDailyEntry{{Day: dayUTC(2026, 6, 9), ScopeType: service.UsageScopeTotal, CostUSD: 1}},
	}}))

	require.NoError(t, provRepo.Delete(ctx, p.ID))

	dCnt, _ := client.UpstreamUsageDaily.Query().Where(upstreamusagedaily.ProviderIDEQ(p.ID)).Count(ctx)
	require.Equal(t, 0, dCnt, "rollup 应被级联删除")
	cCnt, _ := client.UpstreamUsageCursor.Query().Count(ctx)
	require.Equal(t, 0, cCnt, "cursor 应被级联删除")
}
