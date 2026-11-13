package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestShortfallHoldPausesNextPaidRequest(t *testing.T) {
	db := prepareFloorDB(t)
	require.NoError(t, db.AutoMigrate(&model.AdtyBillingHold{}))
	site := floorSite(t, db, "hold")
	user, token := floorCustomer(t, db, site, "hold", 10_000)
	ctx, info, quota := floorRelay(t, user, token)
	info.RequestId = "hold-req-1"
	require.Nil(t, PreConsumeBilling(ctx, 4_000, info))
	require.NoError(t, SettleBilling(ctx, info, quota))
	require.Positive(t, info.BillingShortfall)

	paused, err := model.AdtyUserPaidPaused(user.Id)
	require.NoError(t, err)
	require.True(t, paused)

	ctx2, info2, _ := floorRelay(t, user, token)
	info2.RequestId = "hold-req-2"
	require.NotNil(t, PreConsumeBilling(ctx2, 1_000, info2))
}
