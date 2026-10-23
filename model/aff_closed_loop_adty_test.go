package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestAffSelfInviteApproveHonorsFreezeFromCreatedAt(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, inviter.Id, 0, 0)
	order := l2l3l4Order(t, invitee.Id, "freeze-"+common.GetRandomString(8), 100)
	created := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	comm := &AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 100, RateSnapshot: 0.1,
		CommissionQuota: 1000, FreezeDaysSnap: 7, Status: AffCommissionStatusBlocked,
		UnfreezeAt: created,
	}
	require.NoError(t, DB.Create(comm).Error)
	require.NoError(t, DB.Model(comm).Updates(map[string]any{
		"created_at":  created,
		"unfreeze_at": created,
	}).Error)
	review := &RiskReview{
		Kind: RiskReviewKindAffSelfInvite, UserId: invitee.Id, OrderNo: order.OrderNo,
		AmountUSD: 100, Status: RiskReviewStatusPending, Reason: "self",
	}
	require.NoError(t, DB.Create(review).Error)

	approved, err := ApproveRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusApproved, approved.Status)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))

	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, AffCommissionStatusFrozen, saved.Status)
	require.True(t, saved.UnfreezeAt.After(time.Now().Add(6*24*time.Hour)), "unfreeze=%s", saved.UnfreezeAt)
}

func TestAffSelfInviteApproveCreditsAfterFreezeWindow(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, inviter.Id, 0, 0)
	order := l2l3l4Order(t, invitee.Id, "due-"+common.GetRandomString(8), 100)
	created := time.Now().Add(-8 * 24 * time.Hour).UTC().Truncate(time.Second)
	comm := &AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 100, RateSnapshot: 0.1,
		CommissionQuota: 1000, FreezeDaysSnap: 7, Status: AffCommissionStatusBlocked,
		UnfreezeAt: created,
	}
	require.NoError(t, DB.Create(comm).Error)
	require.NoError(t, DB.Model(comm).Updates(map[string]any{
		"created_at":  created,
		"unfreeze_at": created,
	}).Error)
	review := &RiskReview{
		Kind: RiskReviewKindAffSelfInvite, UserId: invitee.Id, OrderNo: order.OrderNo,
		AmountUSD: 100, Status: RiskReviewStatusPending, Reason: "self",
	}
	require.NoError(t, DB.Create(review).Error)

	_, err := ApproveRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, 1000, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, AffCommissionStatusAvailable, saved.Status)
}

func TestProcessTopupSelfInviteStoresFutureUnfreeze(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	oldDays := bpAffFreezeDays
	bpAffFreezeDays = 7
	t.Cleanup(func() { bpAffFreezeDays = oldDays })

	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	email := "same-" + common.GetRandomString(6) + "@example.com"
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.10",
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.10",
	}).Error)
	order := l2l3l4Order(t, invitee.Id, "write-"+common.GetRandomString(8), 100)

	require.NoError(t, ProcessTopupAffCommission(order.OrderNo))
	var comm AffCommission
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&comm).Error)
	require.Equal(t, AffCommissionStatusBlocked, comm.Status)
	require.Equal(t, 7, comm.FreezeDaysSnap)
	require.True(t, comm.UnfreezeAt.After(time.Now().Add(6*24*time.Hour)), "unfreeze=%s", comm.UnfreezeAt)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))
}

func TestAlipayWithdrawRequiresPayoutQr(t *testing.T) {
	l2l3l4Cleanup(t)
	user := l2l3l4User(t, 0, 1000, 0)
	_, err := CreateAffWithdrawal(user.Id, AffWithdrawCreateInput{
		AmountQuota: 1000,
		IdemKey:     "ali-no-qr-" + common.GetRandomString(6),
		PayMethod:   AffPayMethodAlipay,
		PayAccount:  "13800138000",
	})
	require.ErrorIs(t, err, ErrAffPayoutQrInvalid)
	require.Equal(t, 1000, reloadAffQuota(t, user.Id))
	var n int64
	require.NoError(t, DB.Model(&AffWithdrawal{}).Where("user_id = ?", user.Id).Count(&n).Error)
	require.Equal(t, int64(0), n)
}

func TestVoidBlockedCommissionDoesNotDebit(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, inviter.Id, 0, 0)
	order := l2l3l4Order(t, invitee.Id, "void-"+common.GetRandomString(8), 100)
	comm := &AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 100, RateSnapshot: 0.1,
		CommissionQuota: 1000, FreezeDaysSnap: 7, Status: AffCommissionStatusBlocked,
		UnfreezeAt: time.Now(),
	}
	require.NoError(t, DB.Create(comm).Error)
	review := &RiskReview{
		Kind: RiskReviewKindAffSelfInvite, UserId: invitee.Id, OrderNo: order.OrderNo,
		AmountUSD: 100, Status: RiskReviewStatusPending, Reason: "self",
	}
	require.NoError(t, DB.Create(review).Error)

	_, err := VoidAffCommission(comm.Id, 1)
	require.NoError(t, err)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, AffCommissionStatusVoided, saved.Status)
	var savedReview RiskReview
	require.NoError(t, DB.First(&savedReview, review.Id).Error)
	require.Equal(t, RiskReviewStatusRejected, savedReview.Status)
}

func TestReopenRejectedSelfInviteThenApprove(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "reopen-"+common.GetRandomString(8), 100)
	comm := &AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 100, RateSnapshot: 0.1,
		CommissionQuota: 1000, FreezeDaysSnap: 0, Status: AffCommissionStatusBlocked,
		UnfreezeAt: time.Now(),
	}
	require.NoError(t, DB.Create(comm).Error)
	review := &RiskReview{
		Kind: RiskReviewKindAffSelfInvite, UserId: invitee.Id, OrderNo: order.OrderNo,
		AmountUSD: 100, Status: RiskReviewStatusPending, Reason: "self",
	}
	require.NoError(t, DB.Create(review).Error)
	rejected, err := RejectRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusRejected, rejected.Status)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))

	reopened, err := ReopenAffSelfInviteReview(review.Id, 2)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusPending, reopened.Status)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))

	_, err = ApproveRiskReview(review.Id, 2)
	require.NoError(t, err)
	require.Equal(t, 1000, reloadAffQuota(t, inviter.Id))
	_, err = ReopenAffSelfInviteReview(review.Id, 2)
	require.ErrorIs(t, err, ErrRiskReviewStatusInvalid)
	require.Equal(t, 1000, reloadAffQuota(t, inviter.Id))
}

func TestAffSelfInviteApproveScalesFromCreditAmount(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "scale-"+common.GetRandomString(8), 80)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"paid_amount_usd": 100,
		"refund_status":   RefundStatusPartial,
		"refunded_amount": 40,
	}).Error)
	comm := &AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 80, RateSnapshot: 0.1,
		CommissionQuota: 800, FreezeDaysSnap: 0, Status: AffCommissionStatusBlocked,
		UnfreezeAt: time.Now(),
	}
	require.NoError(t, DB.Create(comm).Error)
	review := &RiskReview{
		Kind: RiskReviewKindAffSelfInvite, UserId: invitee.Id, OrderNo: order.OrderNo,
		AmountUSD: 80, Status: RiskReviewStatusPending, Reason: "self",
	}
	require.NoError(t, DB.Create(review).Error)

	_, err := ApproveRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, 400, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, int64(400), saved.CommissionQuota)
}

func TestAffSelfInviteApproveDoesNotRescaleNetWrittenAfterPartialRefund(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	oldRate := bpAffTopupCommissionRate
	oldDays := bpAffFreezeDays
	bpAffTopupCommissionRate = 0.10
	bpAffFreezeDays = 0
	t.Cleanup(func() {
		bpAffTopupCommissionRate = oldRate
		bpAffFreezeDays = oldDays
	})

	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	email := "net-" + common.GetRandomString(6) + "@example.com"
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.20",
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.20",
	}).Error)
	order := l2l3l4Order(t, invitee.Id, "net-"+common.GetRandomString(8), 100)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refund_status":   RefundStatusPartial,
		"refunded_amount": 40,
	}).Error)

	require.NoError(t, ProcessTopupAffCommission(order.OrderNo))
	var comm AffCommission
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&comm).Error)
	require.Equal(t, AffCommissionStatusBlocked, comm.Status)
	require.InDelta(t, 60.0, comm.PaidAmountUSD, 1e-9)
	require.Equal(t, int64(3000000), comm.CommissionQuota)

	var review RiskReview
	require.NoError(t, DB.Where("order_no = ? AND kind = ?", order.OrderNo, RiskReviewKindAffSelfInvite).First(&review).Error)
	_, err := ApproveRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, 3000000, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, int64(3000000), saved.CommissionQuota)
	require.Equal(t, AffCommissionStatusAvailable, saved.Status)
}

func TestAffSelfInviteApproveScalesOnlyRefundAfterNetWrite(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	oldRate := bpAffTopupCommissionRate
	oldDays := bpAffFreezeDays
	bpAffTopupCommissionRate = 0.10
	bpAffFreezeDays = 0
	t.Cleanup(func() {
		bpAffTopupCommissionRate = oldRate
		bpAffFreezeDays = oldDays
	})

	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	email := "later-" + common.GetRandomString(6) + "@example.com"
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.21",
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.21",
	}).Error)
	order := l2l3l4Order(t, invitee.Id, "later-"+common.GetRandomString(8), 100)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refund_status":   RefundStatusPartial,
		"refunded_amount": 40,
	}).Error)
	require.NoError(t, ProcessTopupAffCommission(order.OrderNo))
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refunded_amount": 60,
	}).Error)

	var review RiskReview
	require.NoError(t, DB.Where("order_no = ? AND kind = ?", order.OrderNo, RiskReviewKindAffSelfInvite).First(&review).Error)
	_, err := ApproveRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, 2000000, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&saved).Error)
	require.Equal(t, int64(2000000), saved.CommissionQuota)
	require.InDelta(t, 40.0, saved.PaidAmountUSD, 1e-9)
}

func TestRefundAfterNetWrittenCommissionClawsFullOrderProportion(t *testing.T) {
	l2l3l4Cleanup(t)
	inviter := l2l3l4User(t, 0, 3000000, 0)
	invitee := l2l3l4User(t, int(100*common.QuotaPerUnit), 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "net-refund-"+common.GetRandomString(6), 100)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refund_status":   RefundStatusPartial,
		"refunded_amount": 40,
	}).Error)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 60, RateSnapshot: 0.10,
		CommissionQuota: 3000000, Status: AffCommissionStatusAvailable,
		UnfreezeAt: time.Now().Add(-time.Hour),
	}).Error)

	preview, err := PreviewTopupRefund(order.OrderNo, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1000000), preview.AffClawQuota)

	res, err := RefundTopupOrder(order.OrderNo, 20, "partial after net write", "net-refund-rid", 1)
	require.NoError(t, err)
	require.Equal(t, int64(1000000), res.AffClawed)
	require.Equal(t, 2000000, reloadAffQuota(t, inviter.Id))

	res2, err := RefundTopupOrder(order.OrderNo, 20, "second partial after net write", "net-refund-rid-2", 1)
	require.NoError(t, err)
	require.Equal(t, int64(1000000), res2.AffClawed)
	require.Equal(t, 1000000, reloadAffQuota(t, inviter.Id))
}

func TestRefundAfterSelfInviteScaleClawsFullOrderProportion(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	oldRate := bpAffTopupCommissionRate
	oldDays := bpAffFreezeDays
	bpAffTopupCommissionRate = 0.10
	bpAffFreezeDays = 0
	t.Cleanup(func() {
		bpAffTopupCommissionRate = oldRate
		bpAffFreezeDays = oldDays
	})

	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, int(100*common.QuotaPerUnit), 0, inviter.Id)
	email := "scale-refund-" + common.GetRandomString(6) + "@example.com"
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.22",
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Updates(map[string]any{
		"email": email, "register_ip": "203.0.113.22",
	}).Error)
	order := l2l3l4Order(t, invitee.Id, "scale-refund-"+common.GetRandomString(6), 100)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refund_status":   RefundStatusPartial,
		"refunded_amount": 40,
	}).Error)
	require.NoError(t, ProcessTopupAffCommission(order.OrderNo))
	var review RiskReview
	require.NoError(t, DB.Where("order_no = ? AND kind = ?", order.OrderNo, RiskReviewKindAffSelfInvite).First(&review).Error)
	_, err := ApproveRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, 3000000, reloadAffQuota(t, inviter.Id))

	res, err := RefundTopupOrder(order.OrderNo, 20, "partial after scale", "scale-refund-rid", 1)
	require.NoError(t, err)
	require.Equal(t, int64(1000000), res.AffClawed)
	require.Equal(t, 2000000, reloadAffQuota(t, inviter.Id))
}

func TestTransferReleasesDueCommissionBeforeDebit(t *testing.T) {
	l2l3l4Cleanup(t)
	u := l2l3l4User(t, 0, 0, 0)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId: u.Id, InviteeId: u.Id + 1,
		OrderNo: "due-" + common.GetRandomString(6),
		Source:  AffCommissionSourceTopup, PaidAmountUSD: 1, RateSnapshot: 0.1,
		CommissionQuota: int64(common.QuotaPerUnit), Status: AffCommissionStatusFrozen,
		UnfreezeAt: time.Now().Add(-time.Minute),
	}).Error)
	err := u.TransferAffQuotaToQuota(int(common.QuotaPerUnit))
	require.NoError(t, err)
	require.Equal(t, 0, reloadAffQuota(t, u.Id))
	var paid int
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Select("quota").Scan(&paid).Error)
	require.Equal(t, int(common.QuotaPerUnit), paid)
}

func TestAffAdminUserDetailCommissionOffset(t *testing.T) {
	l2l3l4Cleanup(t)
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	var ids []int64
	for i := 0; i < 3; i++ {
		row := &AffCommission{
			InviterId: inviter.Id, InviteeId: invitee.Id,
			OrderNo: "page-" + common.GetRandomString(8),
			Source:  AffCommissionSourceTopup, PaidAmountUSD: 10, RateSnapshot: 0.1,
			CommissionQuota: int64(1000 + i), Status: AffCommissionStatusFrozen,
			UnfreezeAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, DB.Create(row).Error)
		ids = append(ids, row.Id)
	}
	detail, err := GetAffAdminUserDetailPage(inviter.Id, AffAdminUserDetailQuery{
		Limit: 1, CommissionOffset: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), detail.CommissionTotal)
	require.Len(t, detail.Commissions, 1)
	require.Equal(t, ids[1], detail.Commissions[0].Id)
}
