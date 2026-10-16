package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func m1m2EnsureTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&TopupOrder{},
		&RiskReview{},
		&FirstTopupClaim{},
		&AffCommission{},
		&Option{},
	))
}

func m1m2Cleanup(t *testing.T) {
	t.Helper()
	m1m2EnsureTables(t)
	truncateTables(t)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM aff_commissions")
		DB.Exec("DELETE FROM risk_reviews")
		DB.Exec("DELETE FROM first_topup_claims")
		DB.Exec("DELETE FROM topup_orders")
		DB.Exec("DELETE FROM options")
	})
	DB.Exec("DELETE FROM aff_commissions")
	DB.Exec("DELETE FROM risk_reviews")
	DB.Exec("DELETE FROM first_topup_claims")
	DB.Exec("DELETE FROM topup_orders")
	resetBatchUpdateTestState(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })

	bpMu.Lock()
	oldRate := bpAffTopupCommissionRate
	oldFreeze := bpAffFreezeDays
	bpAffTopupCommissionRate = 0.10
	bpAffFreezeDays = 0
	bpMu.Unlock()
	t.Cleanup(func() {
		bpMu.Lock()
		bpAffTopupCommissionRate = oldRate
		bpAffFreezeDays = oldFreeze
		bpMu.Unlock()
	})
}

func m1m2User(t *testing.T, paid int, inviterId int) User {
	t.Helper()
	u := User{
		Username:    "m1m2-" + common.GetRandomString(8),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       paid,
		InviterId:   inviterId,
		AffCode:     "m1m2-" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&u).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnsureUserBonusWallet(tx, u.Id)
	}))
	return u
}

func m1m2Order(t *testing.T, userId int, orderNo string, creditUSD float64, bonusCredited int64, refundStatus string, refunded float64) TopupOrder {
	t.Helper()
	o := TopupOrder{
		OrderNo:            orderNo,
		UserId:             userId,
		AmountUSD:          int64(creditUSD),
		PaidAmountUSD:      creditUSD,
		CreditAmountUSD:    creditUSD,
		BonusAmountUSD:     0,
		PendingExtraUSD:    0,
		Status:             TopupOrderStatusPaid,
		PaidCreditedQuota:  int64(creditUSD * common.QuotaPerUnit),
		BonusCreditedQuota: bonusCredited,
		RefundStatus:       refundStatus,
		RefundedAmount:     refunded,
		CreateTime:         time.Now().Unix(),
		CompleteTime:       time.Now().Unix(),
	}
	if o.RefundStatus == "" {
		o.RefundStatus = RefundStatusNone
	}
	require.NoError(t, DB.Create(&o).Error)
	return o
}

func TestApproveRiskReviewWritesBonusSnapshot(t *testing.T) {
	m1m2Cleanup(t)
	user := m1m2User(t, 0, 0)
	order := m1m2Order(t, user.Id, "M1-SNAP-1", 100, int64(5*common.QuotaPerUnit), RefundStatusNone, 0)
	rr := RiskReview{
		Kind:      RiskReviewKindFirstTopupExtra,
		UserId:    user.Id,
		OrderNo:   order.OrderNo,
		AmountUSD: 7.5,
		Status:    RiskReviewStatusPending,
	}
	require.NoError(t, DB.Create(&rr).Error)

	out, err := ApproveRiskReview(rr.Id, 1)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusApproved, out.Status)

	var got TopupOrder
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&got).Error)
	wantAdd, err := DollarsToQuota(7.5)
	require.NoError(t, err)
	require.Equal(t, int64(5*common.QuotaPerUnit)+int64(wantAdd), got.BonusCreditedQuota)

	var gift int64
	require.NoError(t, DB.Model(&UserBonusWallet{}).Where("user_id = ?", user.Id).Select("balance").Scan(&gift).Error)
	require.Equal(t, int64(wantAdd), gift)
}

func TestApproveRiskReviewRejectsFullyRefundedOrder(t *testing.T) {
	m1m2Cleanup(t)
	user := m1m2User(t, 0, 0)
	order := m1m2Order(t, user.Id, "M1-FULL-1", 100, int64(5*common.QuotaPerUnit), RefundStatusFull, 100)
	rr := RiskReview{
		Kind:      RiskReviewKindFirstTopupExtra,
		UserId:    user.Id,
		OrderNo:   order.OrderNo,
		AmountUSD: 7.5,
		Status:    RiskReviewStatusPending,
	}
	require.NoError(t, DB.Create(&rr).Error)

	_, err := ApproveRiskReview(rr.Id, 1)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrRiskReviewOrderRefunded))
	var re *RiskReviewOrderRefundedError
	require.True(t, errors.As(err, &re))
	require.Equal(t, order.OrderNo, re.OrderNo)
	require.Equal(t, RefundStatusFull, re.RefundStatus)

	var still RiskReview
	require.NoError(t, DB.First(&still, rr.Id).Error)
	require.Equal(t, RiskReviewStatusPending, still.Status)

	var gift int64
	require.NoError(t, DB.Model(&UserBonusWallet{}).Where("user_id = ?", user.Id).Select("COALESCE(balance,0)").Scan(&gift).Error)
	require.Equal(t, int64(0), gift)
}

func TestApproveRiskReviewPartialRefundScalesBonus(t *testing.T) {
	m1m2Cleanup(t)
	user := m1m2User(t, 0, 0)
	// 已退 40/100 → 剩余 60%，多送 10 应按 6 入账
	order := m1m2Order(t, user.Id, "M1-PART-1", 100, int64(2*common.QuotaPerUnit), RefundStatusPartial, 40)
	rr := RiskReview{
		Kind:      RiskReviewKindFirstTopupExtra,
		UserId:    user.Id,
		OrderNo:   order.OrderNo,
		AmountUSD: 10,
		Status:    RiskReviewStatusPending,
	}
	require.NoError(t, DB.Create(&rr).Error)

	_, err := ApproveRiskReview(rr.Id, 1)
	require.NoError(t, err)

	wantAdd, err := DollarsToQuota(6)
	require.NoError(t, err)
	var got TopupOrder
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&got).Error)
	require.Equal(t, int64(2*common.QuotaPerUnit)+int64(wantAdd), got.BonusCreditedQuota)

	var gift int64
	require.NoError(t, DB.Model(&UserBonusWallet{}).Where("user_id = ?", user.Id).Select("balance").Scan(&gift).Error)
	require.Equal(t, int64(wantAdd), gift)
}

func TestProcessTopupAffCommissionUsesNetAfterRefund(t *testing.T) {
	m1m2Cleanup(t)
	inviter := m1m2User(t, 0, 0)
	invitee := m1m2User(t, 0, inviter.Id)
	// 实付 100，已退 40 → 计佣基数 60，费率 10% → 6 USD
	order := m1m2Order(t, invitee.Id, "M2-NET-1", 100, 0, RefundStatusPartial, 40)

	require.NoError(t, ProcessTopupAffCommission(order.OrderNo))

	var row AffCommission
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&row).Error)
	require.InDelta(t, 60.0, row.PaidAmountUSD, 1e-6)
	wantQuota, err := DollarsToQuota(6)
	require.NoError(t, err)
	require.Equal(t, int64(wantQuota), row.CommissionQuota)
	require.Equal(t, AffCommissionStatusAvailable, row.Status)
}

func TestProcessTopupAffCommissionSkipsFullyRefunded(t *testing.T) {
	m1m2Cleanup(t)
	inviter := m1m2User(t, 0, 0)
	invitee := m1m2User(t, 0, inviter.Id)
	order := m1m2Order(t, invitee.Id, "M2-FULL-1", 100, 0, RefundStatusFull, 100)

	require.NoError(t, ProcessTopupAffCommission(order.OrderNo))
	var n int64
	require.NoError(t, DB.Model(&AffCommission{}).Where("order_no = ?", order.OrderNo).Count(&n).Error)
	require.Equal(t, int64(0), n)
}

func TestCompensateMissingTopupAffCommissions(t *testing.T) {
	m1m2Cleanup(t)
	inviter := m1m2User(t, 0, 0)
	invitee := m1m2User(t, 0, inviter.Id)
	order := m1m2Order(t, invitee.Id, "M2-COMP-1", 50, 0, RefundStatusNone, 0)

	// 模拟重启前 goroutine 丢失：订单已付款、尚无佣金行
	var n int64
	require.NoError(t, DB.Model(&AffCommission{}).Where("order_no = ?", order.OrderNo).Count(&n).Error)
	require.Equal(t, int64(0), n)

	processed, err := CompensateMissingTopupAffCommissions(100)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var row AffCommission
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&row).Error)
	require.InDelta(t, 50.0, row.PaidAmountUSD, 1e-6)
	wantQuota, err := DollarsToQuota(5)
	require.NoError(t, err)
	require.Equal(t, int64(wantQuota), row.CommissionQuota)

	// 再跑一遍：已有佣金行，不应重复
	processed2, err := CompensateMissingTopupAffCommissions(100)
	require.NoError(t, err)
	require.Equal(t, 0, processed2)
}

func TestCompensateMissingTopupAffCommissionsSkipsNoInviterStarve(t *testing.T) {
	m1m2Cleanup(t)
	inviter := m1m2User(t, 0, 0)
	invitee := m1m2User(t, 0, inviter.Id)

	const limit = 5
	// 先插入 ≥limit 笔「已付、无邀请人、无佣金行」（以及若干全额退），按 id 排在前面；
	// 旧 SQL 会被这些单占满 LIMIT，永远补不到后面真正需要补算的订单。
	for i := 0; i < limit+2; i++ {
		lonely := m1m2User(t, 0, 0) // inviter_id=0
		m1m2Order(t, lonely.Id, "M2-STARVE-N-"+common.GetRandomString(6), 20, 0, RefundStatusNone, 0)
	}
	for i := 0; i < 3; i++ {
		u := m1m2User(t, 0, inviter.Id)
		m1m2Order(t, u.Id, "M2-STARVE-F-"+common.GetRandomString(6), 30, 0, RefundStatusFull, 30)
	}
	need := m1m2Order(t, invitee.Id, "M2-STARVE-NEED", 40, 0, RefundStatusNone, 0)

	processed, err := CompensateMissingTopupAffCommissions(limit)
	require.NoError(t, err)
	require.Equal(t, 1, processed, "must reach the invited order despite older no-inviter/full-refund noise")

	var row AffCommission
	require.NoError(t, DB.Where("order_no = ?", need.OrderNo).First(&row).Error)
	require.InDelta(t, 40.0, row.PaidAmountUSD, 1e-6)

	var lonelyComm int64
	require.NoError(t, DB.Model(&AffCommission{}).Where("order_no LIKE ?", "M2-STARVE-N-%").Count(&lonelyComm).Error)
	require.Equal(t, int64(0), lonelyComm)
	var fullComm int64
	require.NoError(t, DB.Model(&AffCommission{}).Where("order_no LIKE ?", "M2-STARVE-F-%").Count(&fullComm).Error)
	require.Equal(t, int64(0), fullComm)
}
