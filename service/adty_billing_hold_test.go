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

// 有开口差额的用户**仍然可以付费请求**（`ISS-R4-0084`）。
//
// 老板的原话：「额度不足就不能调用，补充余额以后再调用呗」——所以「暂停」这套
// 机制整个是多余的。余额不足由请求前的余额检查挡，那一层已经存在，不需要
// 第二道**永久**封锁（它把「补了余额」也一起挡住了）。
//
// 差额记录本身保留（财务对账要用），只是不再有拦截副作用。
func TestShortfallHoldDoesNotBlockNextPaidRequest(t *testing.T) {
	db := prepareFloorDB(t)
	require.NoError(t, db.AutoMigrate(&model.AdtyBillingHold{}))
	site := floorSite(t, db, "hold")
	user, token := floorCustomer(t, db, site, "hold", 10_000)
	ctx, info, quota := floorRelay(t, user, token)
	info.RequestId = "hold-req-1"
	require.Nil(t, PreConsumeBilling(ctx, 4_000, info))
	require.NoError(t, SettleBilling(ctx, info, quota))
	require.Positive(t, info.BillingShortfall, "这一笔确实产生了差额")

	// 差额记录还在——这是财务对账的数据来源。
	var holds int64
	require.NoError(t, db.Model(&model.AdtyBillingHold{}).Where("user_id = ? AND status = ?",
		user.Id, model.AdtyBillingHoldOpen).Count(&holds).Error)
	require.EqualValues(t, 1, holds, "缺口必须被记下来")

	// **余额不足时该拒** —— 这是老板那句「额度不足就不能调用」的一半，
	// 而它就是请求前那层余额检查，和「暂停」无关。
	ctx2, info2, _ := floorRelay(t, user, token)
	info2.RequestId = "hold-req-2"
	require.NotNil(t, PreConsumeBilling(ctx2, 1_000, info2), "余额是 0，该拒")

	// **补充余额之后再调用就该通过** —— 这是另一半，也是修前做不到的：
	// 那时候第二道门禁是「有开口差额就永久 403」，补了余额也照样拒。
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).
		Update("quota", 50_000).Error)
	ctx3, info3, _ := floorRelay(t, user, token)
	info3.RequestId = "hold-req-3"
	require.Nil(t, PreConsumeBilling(ctx3, 1_000, info3),
		"补了余额就该能继续调用：修前这里仍然是 403「paused until reviewed」")
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
