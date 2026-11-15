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
