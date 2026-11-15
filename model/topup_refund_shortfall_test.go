package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// 全额退款回调而余额已被消费掉一部分时，扣不回来的那部分必须挂账留痕，
// 订单也不能被标成「已退款完毕」——否则差额从余额、批次、流水、挂账四处一起消失。
func TestRefundWaffoFullRefundBooksUnrecoveredShortfall(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "waffo-shortfall", 0)
	seedHostedPendingTopUp(t, user.Id, "WAFFO-SHORTFALL-1", PaymentProviderWaffo, 2, 10)
	require.NoError(t, RechargeWaffo("WAFFO-SHORTFALL-1", "127.0.0.1"))
	credited := int64(2 * int(common.QuotaPerUnit))
	// 用户把 70% 花掉，余额只剩 30%
	left := credited * 30 / 100
	unrecovered := credited * 70 / 100
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", left).Error)

	require.NoError(t, RefundWaffo("WAFFO-SHORTFALL-1", "waffo-refund-short-1", true, ""))

	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	require.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "WAFFO-SHORTFALL-1"), "差额未收回时订单不能标成已退款完毕")

	var holds []AdtyBillingHold
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&holds).Error)
	require.Len(t, holds, 1)
	require.EqualValues(t, unrecovered, holds[0].ShortfallQuota)
	require.Equal(t, AdtyBillingHoldOpen, holds[0].Status)

	var ledger AdtyBalanceLedger
	require.NoError(t, db.Where("ref_id = ? AND type = ?", "WAFFO-SHORTFALL-1", AdtyLedgerTypeRefund).First(&ledger).Error)
	require.EqualValues(t, -left, ledger.DeltaQuota, "流水只记真正收回的部分")

	// 同一笔退款重放：不再扣、不再挂第二条账
	require.NoError(t, RefundWaffo("WAFFO-SHORTFALL-1", "waffo-refund-short-1", true, ""))
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&holds).Error)
	require.Len(t, holds, 1)
	require.EqualValues(t, 0, userQuota(t, db, user.Id))
}

// Pancake 退款把钱包扣走、批次销掉，却从不写余额流水；余额不足时还无条件把订单
// 标成 refunded。这是两条同一个函数上的漏记（ISS-R4-0093）。
func TestRefundWaffoPancakeWritesRefundLedger(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "pancake-ledger", 0)
	seedHostedPendingTopUp(t, user.Id, "PAN-LEDGER-1", PaymentProviderWaffoPancake, 2, 10)
	require.NoError(t, RechargeWaffoPancake("PAN-LEDGER-1"))
	credited := int64(2 * int(common.QuotaPerUnit))

	require.NoError(t, RefundWaffoPancake("PAN-LEDGER-1"))

	var ledger AdtyBalanceLedger
	require.NoError(t, db.Where("ref_id = ? AND type = ?", "PAN-LEDGER-1", AdtyLedgerTypeRefund).First(&ledger).Error)
	require.EqualValues(t, -credited, ledger.DeltaQuota)
	require.EqualValues(t, credited, ledger.BalanceBefore)
	require.EqualValues(t, 0, ledger.BalanceAfter)
	require.Equal(t, common.TopUpStatusRefunded, topUpStatus(t, db, "PAN-LEDGER-1"))
}

func TestRefundWaffoPancakeBooksShortfallOnInsufficientBalance(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "pancake-shortfall", 0)
	seedHostedPendingTopUp(t, user.Id, "PAN-SHORTFALL-1", PaymentProviderWaffoPancake, 2, 10)
	require.NoError(t, RechargeWaffoPancake("PAN-SHORTFALL-1"))
	credited := int64(2 * int(common.QuotaPerUnit))
	left := credited * 30 / 100
	unrecovered := credited - left
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", left).Error)

	require.NoError(t, RefundWaffoPancake("PAN-SHORTFALL-1"))

	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	var ledger AdtyBalanceLedger
	require.NoError(t, db.Where("ref_id = ? AND type = ?", "PAN-SHORTFALL-1", AdtyLedgerTypeRefund).First(&ledger).Error)
	require.EqualValues(t, -left, ledger.DeltaQuota, "收回多少就记多少")
	var holds []AdtyBillingHold
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&holds).Error)
	require.Len(t, holds, 1)
	require.EqualValues(t, unrecovered, holds[0].ShortfallQuota)
	require.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "PAN-SHORTFALL-1"), "一分钱没收齐时不能标成已退款")

	// 重放：不重复扣、不重复写流水、不重复挂账
	require.NoError(t, RefundWaffoPancake("PAN-SHORTFALL-1"))
	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	var ledgers int64
	require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("ref_id = ? AND type = ?", "PAN-SHORTFALL-1", AdtyLedgerTypeRefund).Count(&ledgers).Error)
	require.EqualValues(t, 1, ledgers)
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&holds).Error)
	require.Len(t, holds, 1)
}
