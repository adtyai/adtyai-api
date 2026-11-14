package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func dropBillingHoldTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(&model.AdtyBillingHold{}))
	require.False(t, db.Migrator().HasTable(&model.AdtyBillingHold{}))
}

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

func TestAdtyNoteBillingShortfallReturnsHoldWriteError(t *testing.T) {
	db := prepareFloorDB(t)
	dropBillingHoldTable(t, db)
	user, token := floorCustomer(t, db, nil, "hold-note-fail", 10)
	ctx, info, _ := floorRelay(t, user, token)
	info.RequestId = "hold-note-fail"
	info.BillingShortfall = 15

	err := adtyNoteBillingShortfall(ctx, info, 0)
	require.ErrorIs(t, err, model.ErrAdtyBillingHoldTableMissing)
	require.Equal(t, 15, ctx.GetInt(AdtyBillingShortfallContextKey))
}

func TestSettleBillingReturnsHoldWriteErrorAfterDebit(t *testing.T) {
	db := prepareFloorDB(t)
	user, token := floorCustomer(t, db, nil, "hold-settle-fail", 10)
	ctx, info, _ := floorRelay(t, user, token)
	info.RequestId = "hold-settle-fail"
	require.Nil(t, PreConsumeBilling(ctx, 10, info))
	dropBillingHoldTable(t, db)

	err := SettleBilling(ctx, info, 225)
	require.ErrorIs(t, err, model.ErrAdtyBillingHoldTableMissing)
	require.Equal(t, 0, floorBalance(t, user.Id), "wallet debit already succeeded and must not be rolled back")
	require.Equal(t, 215, info.BillingShortfall)
	require.False(t, unsettledBilling(info), "settlement stays applied after a hold-write failure")
}

func TestChargeViolationFeeIfNeededReturnsHoldWriteErrorAfterDebit(t *testing.T) {
	db := prepareFloorDB(t)
	savedGrok := *model_setting.GetGrokSettings()
	t.Cleanup(func() { *model_setting.GetGrokSettings() = savedGrok })
	model_setting.GetGrokSettings().ViolationDeductionEnabled = true
	model_setting.GetGrokSettings().ViolationDeductionAmount = 0.04

	user, token := floorCustomer(t, db, nil, "hold-violation-fail", 1_000)
	ctx, info, _ := floorRelay(t, user, token)
	info.RequestId = "hold-violation-fail"
	dropBillingHoldTable(t, db)

	charged, err := ChargeViolationFeeIfNeeded(ctx, info, types.NewError(fmt.Errorf("blocked"), types.ErrorCodeViolationFeeGrokCSAM))
	require.True(t, charged, "the fee debit already succeeded")
	require.ErrorIs(t, err, model.ErrAdtyBillingHoldTableMissing)
	require.Equal(t, 0, floorBalance(t, user.Id), "wallet debit already succeeded and must not be rolled back")
	require.Equal(t, 19_000, info.BillingShortfall)
}
