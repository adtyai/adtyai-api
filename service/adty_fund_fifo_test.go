package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumeFundFIFOUsesPrincipalBeforeBonus(t *testing.T) {
	batches := []FundBatch{
		{ID: 2, Kind: "bonus", Remaining: 40, CreatedAt: 1},
		{ID: 1, Kind: "principal", Remaining: 30, CreatedAt: 2},
		{ID: 3, Kind: "principal", Remaining: 20, CreatedAt: 1},
	}
	got, left := ConsumeFundFIFO(batches, 45)
	require.EqualValues(t, 0, left)
	remaining := map[int64]int64{}
	for _, batch := range got {
		remaining[batch.ID] = batch.Remaining
	}
	require.EqualValues(t, 0, remaining[3])
	require.EqualValues(t, 5, remaining[1])
	require.EqualValues(t, 40, remaining[2])
}
