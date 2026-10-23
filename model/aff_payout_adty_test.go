package model

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func usdtWithdrawInput(amount int64, idem, account, chain string) AffWithdrawCreateInput {
	return AffWithdrawCreateInput{
		AmountQuota: amount,
		IdemKey:     idem,
		PayMethod:   AffPayMethodUSDT,
		PayAccount:  account,
		UsdtChain:   chain,
	}
}

func alipayWithdrawInput(amount int64, idem, account string, qrId int64) AffWithdrawCreateInput {
	return AffWithdrawCreateInput{
		AmountQuota: amount,
		IdemKey:     idem,
		PayMethod:   AffPayMethodAlipay,
		PayAccount:  account,
		PayoutQrId:  qrId,
	}
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestAffPayoutValidationAndIdempotency(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&AffPayoutQr{}))
	user := l2l3l4User(t, 0, 20000, 0)
	other := l2l3l4User(t, 0, 20000, 0)

	_, err := CreateAffWithdrawal(user.Id, AffWithdrawCreateInput{AmountQuota: 1000, IdemKey: "empty-method"})
	require.ErrorIs(t, err, ErrAffWithdrawMethodInvalid)
	require.Equal(t, 20000, reloadAffQuota(t, user.Id))

	_, err = CreateAffWithdrawal(user.Id, alipayWithdrawInput(1000, "bad-ali", "not an account", 0))
	require.ErrorIs(t, err, ErrAffAlipayAccountInvalid)
	_, err = CreateAffWithdrawal(user.Id, alipayWithdrawInput(1000, "bad-url", "http://pay.example", 0))
	require.ErrorIs(t, err, ErrAffAlipayAccountInvalid)
	require.Equal(t, 20000, reloadAffQuota(t, user.Id))

	_, err = CreateAffWithdrawal(user.Id, alipayWithdrawInput(1000, "ali-ok", "13800138000", 0))
	require.ErrorIs(t, err, ErrAffPayoutQrInvalid)
	require.Equal(t, 20000, reloadAffQuota(t, user.Id))

	pngBytes := testPNG(t, 8, 8)
	qr, err := SaveAffPayoutQr(user.Id, pngBytes)
	require.NoError(t, err)
	require.NotEmpty(t, qr.Sha256)
	require.Nil(t, qr.Data)

	withQR, err := CreateAffWithdrawal(user.Id, alipayWithdrawInput(1000, "ali-qr", "pay@example.com", qr.Id))
	require.NoError(t, err)
	require.Equal(t, qr.Id, withQR.PayoutQrId)
	require.Equal(t, 19000, reloadAffQuota(t, user.Id))
	replay, err := CreateAffWithdrawal(user.Id, alipayWithdrawInput(1000, "ali-qr", "pay@example.com", qr.Id))
	require.NoError(t, err)
	require.Equal(t, withQR.Id, replay.Id)
	require.Equal(t, 19000, reloadAffQuota(t, user.Id))
	mismatch, err := CreateAffWithdrawal(user.Id, alipayWithdrawInput(1000, "ali-qr", "13800138000", qr.Id))
	require.ErrorIs(t, err, ErrAffWithdrawIdempotentMismatch)
	require.Nil(t, mismatch)
	require.NotContains(t, err.Error(), "pay@example.com")
	require.NotContains(t, err.Error(), "13800138000")
	stored, err := GetAffPayoutQrForUser(qr.Id, user.Id)
	require.NoError(t, err)
	require.Equal(t, withQR.Id, stored.WithdrawalId)
	_, err = GetAffPayoutQrForUser(qr.Id, other.Id)
	require.ErrorIs(t, err, ErrAffPayoutQrNotFound)

	_, err = CreateAffWithdrawal(other.Id, alipayWithdrawInput(1000, "steal-qr", "13900139000", qr.Id))
	require.ErrorIs(t, err, ErrAffPayoutQrInvalid)
	require.Equal(t, 20000, reloadAffQuota(t, other.Id))

	_, err = ValidatePayoutQrImage([]byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"))
	require.ErrorIs(t, err, ErrAffPayoutQrInvalid)
	_, err = ValidatePayoutQrImage(bytes.Repeat([]byte("a"), 512*1024+1))
	require.ErrorIs(t, err, ErrAffPayoutQrInvalid)
	_, err = ValidatePayoutQrImage(testPNG(t, 2049, 1))
	require.ErrorIs(t, err, ErrAffPayoutQrInvalid)
	var jpegBuf bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegBuf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil))
	_, err = ValidatePayoutQrImage(jpegBuf.Bytes())
	require.NoError(t, err)

	legacy := AffWithdrawal{PayAccount: "old-account"}
	require.Equal(t, AffPayMethodLegacy, AffWithdrawalDisplayMethod(&legacy))
	require.Equal(t, AffPayMethodUSDT, AffWithdrawalDisplayMethod(&AffWithdrawal{UsdtChain: "trc20"}))

	mover := l2l3l4User(t, 0, int(common.QuotaPerUnit)*2, 0)
	require.NoError(t, mover.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
	var moverWds int64
	require.NoError(t, DB.Model(&AffWithdrawal{}).Where("user_id = ?", mover.Id).Count(&moverWds).Error)
	require.Equal(t, int64(0), moverWds)
}

func TestAffSelfInviteReviewDoesNotTouchFirstTopup(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}, &FirstTopupClaim{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "SELF-"+common.GetRandomString(6), 100)
	comm := AffCommission{
		InviterId:       inviter.Id,
		InviteeId:       invitee.Id,
		OrderNo:         order.OrderNo,
		Source:          AffCommissionSourceTopup,
		PaidAmountUSD:   100,
		RateSnapshot:    0.1,
		CommissionQuota: 1000,
		FreezeDaysSnap:  0,
		Status:          AffCommissionStatusBlocked,
		UnfreezeAt:      time.Now().Add(-time.Minute),
		BlockReason:     "self_invite",
	}
	require.NoError(t, DB.Create(&comm).Error)
	review := RiskReview{
		Kind:    RiskReviewKindAffSelfInvite,
		UserId:  inviter.Id,
		OrderNo: order.OrderNo,
		Status:  RiskReviewStatusPending,
	}
	require.NoError(t, DB.Create(&review).Error)
	otherReview := RiskReview{
		Kind:    RiskReviewKindFirstTopupExtra,
		UserId:  inviter.Id,
		OrderNo: "OTHER-" + common.GetRandomString(6),
		Status:  RiskReviewStatusPending,
	}
	require.NoError(t, DB.Create(&otherReview).Error)
	claim := FirstTopupClaim{UserId: inviter.Id, OrderNo: otherReview.OrderNo, Status: FirstTopupStatusPendingReview}
	require.NoError(t, DB.Create(&claim).Error)

	rejected, err := RejectRiskReview(review.Id, 1)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusRejected, rejected.Status)
	var still AffCommission
	require.NoError(t, DB.First(&still, comm.Id).Error)
	require.Equal(t, AffCommissionStatusBlocked, still.Status)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))
	var other RiskReview
	require.NoError(t, DB.First(&other, otherReview.Id).Error)
	require.Equal(t, RiskReviewStatusPending, other.Status)
	var claimAfter FirstTopupClaim
	require.NoError(t, DB.First(&claimAfter, claim.Id).Error)
	require.Equal(t, FirstTopupStatusPendingReview, claimAfter.Status)
}

func TestAffSelfInviteApproveCreditsOnceAndScalesPartialRefund(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "SELF2-"+common.GetRandomString(6), 100)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refund_status":   RefundStatusPartial,
		"refunded_amount": 40,
	}).Error)
	comm := AffCommission{
		InviterId:       inviter.Id,
		InviteeId:       invitee.Id,
		OrderNo:         order.OrderNo,
		Source:          AffCommissionSourceTopup,
		PaidAmountUSD:   100,
		CommissionQuota: 1000,
		Status:          AffCommissionStatusBlocked,
		UnfreezeAt:      time.Now().Add(-time.Minute),
	}
	require.NoError(t, DB.Create(&comm).Error)
	review := RiskReview{Kind: RiskReviewKindAffSelfInvite, UserId: inviter.Id, OrderNo: order.OrderNo, Status: RiskReviewStatusPending}
	require.NoError(t, DB.Create(&review).Error)

	approved, err := ApproveRiskReview(review.Id, 7)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusApproved, approved.Status)
	require.Equal(t, 600, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, AffCommissionStatusAvailable, saved.Status)
	require.Equal(t, int64(600), saved.CommissionQuota)

	again, err := ApproveRiskReview(review.Id, 7)
	require.NoError(t, err)
	require.Equal(t, RiskReviewStatusApproved, again.Status)
	require.Equal(t, 600, reloadAffQuota(t, inviter.Id))
}

func TestAffSelfInviteApproveRefusesFullRefund(t *testing.T) {
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&RiskReview{}))
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "SELF3-"+common.GetRandomString(6), 100)
	require.NoError(t, DB.Model(&order).Updates(map[string]any{
		"refund_status":   RefundStatusFull,
		"refunded_amount": 100,
	}).Error)
	comm := AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 100, CommissionQuota: 1000,
		Status: AffCommissionStatusBlocked, UnfreezeAt: time.Now().Add(-time.Minute),
	}
	require.NoError(t, DB.Create(&comm).Error)
	review := RiskReview{Kind: RiskReviewKindAffSelfInvite, UserId: inviter.Id, OrderNo: order.OrderNo, Status: RiskReviewStatusPending}
	require.NoError(t, DB.Create(&review).Error)
	_, err := ApproveRiskReview(review.Id, 1)
	require.ErrorIs(t, err, ErrRiskReviewOrderRefunded)
	require.Equal(t, 0, reloadAffQuota(t, inviter.Id))
	var saved AffCommission
	require.NoError(t, DB.First(&saved, comm.Id).Error)
	require.Equal(t, AffCommissionStatusBlocked, saved.Status)
	require.False(t, strings.Contains(err.Error(), "138"))
}

func TestAffAdminUserDetailTotalsAndStatusFilter(t *testing.T) {
	l2l3l4Cleanup(t)
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	stamp := common.GetRandomString(8)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: "FROZEN-" + stamp,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 10, RateSnapshot: 0.1,
		CommissionQuota: 1000, Status: AffCommissionStatusFrozen, UnfreezeAt: time.Now().Add(time.Hour),
	}).Error)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: "AVAIL-" + stamp,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 20, RateSnapshot: 0.1,
		CommissionQuota: 2000, Status: AffCommissionStatusAvailable, UnfreezeAt: time.Now().Add(-time.Hour),
	}).Error)
	rows, total, err := ListAffCommissionsAdminFiltered(AffAdminListFilter{
		InviterId: inviter.Id, Status: AffCommissionStatusFrozen, Limit: 20,
	}, true)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, AffCommissionStatusFrozen, rows[0].Status)
	require.NotEqual(t, invitee.Username, rows[0].InviteeName)

	require.NoError(t, DB.Create(&AffWithdrawal{
		UserId: inviter.Id, AmountQuota: 1000, NetQuota: 1000, IdemKey: "idem-" + stamp,
		Status: AffWithdrawStatusPending, PayMethod: AffPayMethodUSDT, PayAccount: "T" + stamp, UsdtChain: "trc20",
	}).Error)
	require.NoError(t, DB.Create(&LedgerEntry{
		UserId: inviter.Id, Account: "aff", Kind: "aff_transfer", Delta: -500, BalanceAfter: 0,
		CreatedAt: time.Now(),
	}).Error)

	detail, err := GetAffAdminUserDetail(inviter.Id, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), detail.CommissionTotal)
	require.Len(t, detail.Commissions, 2)
	require.Equal(t, int64(1), detail.WithdrawalTotal)
	require.Equal(t, int64(1), detail.TransferTotal)
	require.Len(t, detail.Transfers, 1)
	require.Equal(t, invitee.Username, detail.Commissions[0].InviteeName)
}

func reloadAffQuota(t *testing.T, userId int) int {
	t.Helper()
	var u User
	require.NoError(t, DB.Select("aff_quota").First(&u, userId).Error)
	return u.AffQuota
}
