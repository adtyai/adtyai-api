package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRefundPreviewMatchesRejectsStaleAmount(t *testing.T) {
	preview := RefundClawPreview{
		AmountUSD:         10,
		PaidClawUSD:       8,
		BonusClawUSD:      2,
		BonusShortfallUSD: 0,
		AffClawUSD:        1,
		AffAction:         "void",
	}
	echo := RefundPreviewEcho{
		AmountUSD:         10,
		PaidClawUSD:       8,
		BonusClawUSD:      2,
		BonusShortfallUSD: 0,
		AffClawUSD:        1,
		AffAction:         "void",
	}
	require.True(t, RefundPreviewMatches(&preview, echo))
	echo.PaidClawUSD = 7
	require.False(t, RefundPreviewMatches(&preview, echo))
}

func TestPreviewTopupRefundMatchesWriteAndDoesNotWrite(t *testing.T) {
	l2l3l4Cleanup(t)
	invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, 0)
	order := l2l3l4Order(t, invitee.Id, "PREV-1", 10)
	require.NoError(t, DB.Model(&TopupOrder{}).Where("order_no = ?", order.OrderNo).Updates(map[string]any{
		"bonus_amount_usd":     2,
		"bonus_credited_quota": int64(2 * common.QuotaPerUnit),
	}).Error)
	require.NoError(t, DB.Model(&UserBonusWallet{}).Where("user_id = ?", invitee.Id).Update("balance", int64(2*common.QuotaPerUnit)).Error)
	inviter := l2l3l4User(t, 0, int(5*common.QuotaPerUnit), 0)
	require.NoError(t, DB.Create(&AffCommission{
		InviterId:       inviter.Id,
		InviteeId:       invitee.Id,
		OrderNo:         order.OrderNo,
		Source:          "topup",
		PaidAmountUSD:   10,
		RateSnapshot:    0.1,
		CommissionQuota: int64(common.QuotaPerUnit),
		Status:          AffCommissionStatusAvailable,
		UnfreezeAt:      time.Now().Add(-time.Hour),
	}).Error)

	var before int64
	require.NoError(t, DB.Model(&TopupRefund{}).Count(&before).Error)
	preview, err := PreviewTopupRefund(order.OrderNo, 5)
	require.NoError(t, err)
	var after int64
	require.NoError(t, DB.Model(&TopupRefund{}).Count(&after).Error)
	require.Equal(t, before, after)
	require.Equal(t, "claw", preview.AffAction)
	require.Greater(t, preview.BonusClawQuota, int64(0))
	require.Greater(t, preview.AffClawQuota, int64(0))

	res, err := RefundTopupOrder(order.OrderNo, 5, "preview-match", "prev-rid", 1)
	require.NoError(t, err)
	require.Equal(t, preview.AffClawQuota, res.AffClawed)
	require.Equal(t, preview.BonusClawQuota, res.BonusClawed)
	require.Equal(t, preview.BonusShortfallQuota, res.BonusShort)
}

func TestPreviewTopupRefundUsesCreditBeforePaid(t *testing.T) {
	l2l3l4Cleanup(t)
	invitee := l2l3l4User(t, int(100*common.QuotaPerUnit), 0, 0)
	order := l2l3l4Order(t, invitee.Id, "CRED-"+common.GetRandomString(6), 80)
	require.NoError(t, DB.Model(&TopupOrder{}).Where("id = ?", order.Id).Updates(map[string]any{
		"paid_amount_usd":   100,
		"credit_amount_usd": 80,
	}).Error)
	_, err := PreviewTopupRefund(order.OrderNo, 90)
	require.ErrorIs(t, err, ErrRefundExceedsAvailable)
}
