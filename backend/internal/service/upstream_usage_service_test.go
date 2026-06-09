package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNYConversion(t *testing.T) {
	require.InDelta(t, 1.0, cny(10, 10), 1e-9)
	require.InDelta(t, 5.0, cny(10, 2), 1e-9)
	require.InDelta(t, 10.0, cny(10, 1), 1e-9)
	require.InDelta(t, 10.0, cny(10, 0), 1e-9)
	require.InDelta(t, 0.33, cny(1, 3), 1e-9)
}
