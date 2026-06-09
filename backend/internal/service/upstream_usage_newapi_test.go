package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeNewapiUsage_ConsumeAndRefund(t *testing.T) {
	day := time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC).Unix()
	logs := []newapiLogEntry{
		{ID: 1, CreatedAt: day, Type: 2, Quota: 500000, TokenID: 10, TokenName: "k", ModelName: "gpt", Group: "vip", PromptTokens: 3, CompletionTokens: 7},
		{ID: 2, CreatedAt: day, Type: 6, Quota: 100000, TokenID: 10, TokenName: "k", ModelName: "gpt", Group: "vip"},
		{ID: 3, CreatedAt: day, Type: 1, Quota: 999999},
	}
	entries := normalizeNewapiUsage(logs, 500000)

	byKey := map[string]UpstreamUsageDailyEntry{}
	for _, e := range entries {
		byKey[e.ScopeType+"/"+e.ScopeKey] = e
	}
	require.InDelta(t, 0.8, byKey["total/"].CostUSD, 1e-9)
	require.InDelta(t, 0.8, byKey["key/10"].CostUSD, 1e-9)
	require.Equal(t, "k", byKey["key/10"].ScopeName)
	require.InDelta(t, 0.8, byKey["group/vip"].CostUSD, 1e-9)
	require.InDelta(t, 0.8, byKey["model/gpt"].CostUSD, 1e-9)
	require.Equal(t, int64(10), byKey["total/"].Tokens)
}

func TestNormalizeNewapiUsage_RefundWithoutToken(t *testing.T) {
	day := time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC).Unix()
	logs := []newapiLogEntry{
		{ID: 1, CreatedAt: day, Type: 6, Quota: 500000, TokenID: 0, Group: ""},
	}
	entries := normalizeNewapiUsage(logs, 500000)
	require.Len(t, entries, 1)
	require.Equal(t, UsageScopeTotal, entries[0].ScopeType)
	require.InDelta(t, -1.0, entries[0].CostUSD, 1e-9)
}
