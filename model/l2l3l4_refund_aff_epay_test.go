package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func l2l3l4Cleanup(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&TopupOrder{}, &TopupRefund{}, &AffCommission{}, &AffWithdrawal{}, &LedgerEntry{}, &LedgerIdempotency{}))
	truncateTables(t)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM topup_refunds")
		DB.Exec("DELETE FROM aff_commissions")
		DB.Exec("DELETE FROM topup_orders")
		DB.Exec("DELETE FROM ledger_idempotency")
		DB.Exec("DELETE FROM ledger_entries")
	})
	DB.Exec("DELETE FROM topup_refunds")
	DB.Exec("DELETE FROM aff_commissions")
	DB.Exec("DELETE FROM topup_orders")
	DB.Exec("DELETE FROM ledger_idempotency")
	DB.Exec("DELETE FROM ledger_entries")
	resetBatchUpdateTestState(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
}

func l2l3l4User(t *testing.T, paid, affQuota, inviterId int) User {
	t.Helper()
	u := User{
		Username:    "l234-" + common.GetRandomString(8),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       paid,
		AffQuota:    affQuota,
		InviterId:   inviterId,
		AffCode:     "l234-" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&u).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnsureUserBonusWallet(tx, u.Id)
	}))
	return u
}

func l2l3l4Order(t *testing.T, userId int, orderNo string, creditUSD float64) TopupOrder {
	t.Helper()
	o := TopupOrder{
		OrderNo:            orderNo,
		UserId:             userId,
		AmountUSD:          int64(creditUSD),
		PaidAmountUSD:      creditUSD,
		CreditAmountUSD:    creditUSD,
		Status:             TopupOrderStatusPaid,
		PaidCreditedQuota:  int64(creditUSD * common.QuotaPerUnit),
		BonusCreditedQuota: 0,
		RefundStatus:       RefundStatusNone,
		CreateTime:         time.Now().Unix(),
		CompleteTime:       time.Now().Unix(),
	}
	require.NoError(t, DB.Create(&o).Error)
	return o
}

// L2：sum 查询失败必须向上返回，不得当 0。
func TestSumPriorRefundClawbacksPropagatesError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// 故意不建 topup_refunds
	_, _, err = sumPriorRefundClawbacks(db, "NO-TABLE")
	require.Error(t, err)
}

func TestSumPriorRefundClawbacksSums(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.Create(&TopupRefund{
		OrderNo: "L2-SUM-1", RequestId: "l2-sum-a", Amount: 1, Reason: "a",
		BonusClawed: 100, AffClawed: 20,
	}).Error)
	require.NoError(t, DB.Create(&TopupRefund{
		OrderNo: "L2-SUM-1", RequestId: "l2-sum-b", Amount: 1, Reason: "b",
		BonusClawed: 50, AffClawed: 7,
	}).Error)
	require.NoError(t, DB.Create(&TopupRefund{
		OrderNo: "L2-OTHER", RequestId: "l2-sum-c", Amount: 1, Reason: "c",
		BonusClawed: 999, AffClawed: 999,
	}).Error)

	b, a, err := sumPriorRefundClawbacks(DB, "L2-SUM-1")
	require.NoError(t, err)
	require.Equal(t, int64(150), b)
	require.Equal(t, int64(27), a)

	b0, a0, err := sumPriorRefundClawbacks(DB, "L2-NONE")
	require.NoError(t, err)
	require.Equal(t, int64(0), b0)
	require.Equal(t, int64(0), a0)
}

// L2 集成：历史 claw 查询路径出错时整笔退款失败（通过临时改表名模拟）。
func TestRefundTopupOrderFailsWhenPriorClawbackSumErrors(t *testing.T) {
	l2l3l4Cleanup(t)
	invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, 0)
	order := l2l3l4Order(t, invitee.Id, "L2-FAIL-1", 10)

	// 存在性检查会查 topup_refunds；先把表改名，使 sum（以及该查）失败。
	require.NoError(t, DB.Exec("ALTER TABLE topup_refunds RENAME TO topup_refunds_l2bak").Error)
	t.Cleanup(func() {
		_ = DB.Exec("ALTER TABLE topup_refunds_l2bak RENAME TO topup_refunds")
	})

	_, err := RefundTopupOrder(order.OrderNo, 1, "l2-sum-fail", "l2-fail-rid", 1)
	require.Error(t, err)

	// 订单未动
	var got TopupOrder
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&got).Error)
	require.Equal(t, RefundStatusNone, got.RefundStatus)
	require.InDelta(t, 0, got.RefundedAmount, 1e-9)
}

// L3：可用佣金扣回写到佣金行 InviterId，即使用户当前邀请人已变。
func TestRefundTopupOrderClawsAffFromCommissionInviterAvailable(t *testing.T) {
	l2l3l4Cleanup(t)
	histInv := l2l3l4User(t, 0, int(5*common.QuotaPerUnit), 0)  // 佣金行上的邀请人，已入账 5
	currInv := l2l3l4User(t, 0, int(99*common.QuotaPerUnit), 0) // 用户当前邀请人，不应被动
	invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, currInv.Id)
	order := l2l3l4Order(t, invitee.Id, "L3-AVAIL-1", 10)

	commQuota := int64(5 * common.QuotaPerUnit)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId:       histInv.Id,
		InviteeId:       invitee.Id,
		OrderNo:         order.OrderNo,
		Source:          "topup",
		PaidAmountUSD:   10,
		RateSnapshot:    0.5,
		CommissionQuota: commQuota,
		Status:          AffCommissionStatusAvailable,
		UnfreezeAt:      time.Now().Add(-time.Hour),
	}).Error)

	res, err := RefundTopupOrder(order.OrderNo, 10, "l3-avail", "l3-avail-rid", 1)
	require.NoError(t, err)
	require.Equal(t, commQuota, res.AffClawed)

	var histAfter, currAfter User
	require.NoError(t, DB.Select("id", "aff_quota").First(&histAfter, histInv.Id).Error)
	require.NoError(t, DB.Select("id", "aff_quota").First(&currAfter, currInv.Id).Error)
	require.Equal(t, 0, histAfter.AffQuota)
	require.Equal(t, int(99*common.QuotaPerUnit), currAfter.AffQuota)

	var clawN int64
	require.NoError(t, DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND account = ? AND kind = ?", histInv.Id, "aff", "aff_refund_claw").
		Count(&clawN).Error)
	require.Equal(t, int64(1), clawN)
	var wrongN int64
	require.NoError(t, DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND account = ?", currInv.Id, "aff").
		Count(&wrongN).Error)
	require.Equal(t, int64(0), wrongN)
}

// L3：冻结佣金作废流水也写到佣金行 InviterId。
func TestRefundTopupOrderVoidsAffOnCommissionInviterFrozen(t *testing.T) {
	l2l3l4Cleanup(t)
	histInv := l2l3l4User(t, 0, 0, 0)
	currInv := l2l3l4User(t, 0, int(50*common.QuotaPerUnit), 0)
	invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, currInv.Id)
	order := l2l3l4Order(t, invitee.Id, "L3-FRZ-1", 10)

	commQuota := int64(3 * common.QuotaPerUnit)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId:       histInv.Id,
		InviteeId:       invitee.Id,
		OrderNo:         order.OrderNo,
		Source:          "topup",
		PaidAmountUSD:   10,
		RateSnapshot:    0.3,
		CommissionQuota: commQuota,
		Status:          AffCommissionStatusFrozen,
		UnfreezeAt:      time.Now().Add(24 * time.Hour),
	}).Error)

	res, err := RefundTopupOrder(order.OrderNo, 10, "l3-frz", "l3-frz-rid", 1)
	require.NoError(t, err)
	require.Equal(t, commQuota, res.AffClawed)

	var histAfter, currAfter User
	require.NoError(t, DB.Select("id", "aff_quota").First(&histAfter, histInv.Id).Error)
	require.NoError(t, DB.Select("id", "aff_quota").First(&currAfter, currInv.Id).Error)
	// 冻结未入账，余额都不变
	require.Equal(t, 0, histAfter.AffQuota)
	require.Equal(t, int(50*common.QuotaPerUnit), currAfter.AffQuota)

	var voidN int64
	require.NoError(t, DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND account = ? AND kind = ?", histInv.Id, "aff", "aff_refund_void").
		Count(&voidN).Error)
	require.Equal(t, int64(1), voidN)
	var wrongN int64
	require.NoError(t, DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND account = ?", currInv.Id, "aff").
		Count(&wrongN).Error)
	require.Equal(t, int64(0), wrongN)

	var comm AffCommission
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&comm).Error)
	require.Equal(t, AffCommissionStatusVoided, comm.Status)
	require.Equal(t, int64(0), comm.CommissionQuota)
}
