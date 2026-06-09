package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSub2apiDailyAndTotals(t *testing.T) {
	k1 := normalizeSub2apiDaily("100", "alpha", []sub2apiDailyEntry{
		{Date: "2026-06-08", Requests: 2, TotalTokens: 50, ActualCost: 1.5},
		{Date: "2026-06-09", Requests: 1, TotalTokens: 10, ActualCost: 0.5},
	})
	k2 := normalizeSub2apiDaily("200", "beta", []sub2apiDailyEntry{
		{Date: "2026-06-09", Requests: 3, TotalTokens: 30, ActualCost: 2.0},
	})
	require.Len(t, k1, 2)
	require.Equal(t, "alpha", k1[0].ScopeName)
	require.Equal(t, UsageScopeKey, k1[0].ScopeType)

	all := append(append([]UpstreamUsageDailyEntry{}, k1...), k2...)
	totals := mergeTotals(all)
	byDay := map[string]UpstreamUsageDailyEntry{}
	for _, e := range totals {
		byDay[e.Day.Format("2006-01-02")] = e
	}
	require.InDelta(t, 1.5, byDay["2026-06-08"].CostUSD, 1e-9)
	require.InDelta(t, 2.5, byDay["2026-06-09"].CostUSD, 1e-9)
	require.Equal(t, UsageScopeTotal, byDay["2026-06-09"].ScopeType)
}
