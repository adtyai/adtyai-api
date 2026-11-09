package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestHostedProvidersSettleTheirFrozenAdtyQuota(t *testing.T) {
	for _, provider := range []string{PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			db := adtyTestDB(t)
			require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &AdtyTopupOrder{}, &AdtyOrderPaymentEvent{}, &AdtyBalanceLedger{}, &AdtyUserFingerprint{}, &PaymentOrphanEvent{}, &Log{}))
			user := User{Username: "hosted", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 100, Group: "default"}
			require.NoError(t, db.Create(&user).Error)
			topUp := TopUp{UserId: user.Id, TradeNo: "hosted-" + provider, Amount: 10, Money: 9, PaymentProvider: provider, PaymentMethod: provider, Status: common.TopUpStatusPending}
			ext := AdtyTopupOrder{TradeNo: topUp.TradeNo, UserId: user.Id, Provider: provider, Method: provider,
				FaceUSD: decimal.NewFromInt(10), BonusUSD: decimal.NewFromInt(2), CreditQuota: 6_000_000,
				PayAmount: decimal.NewFromInt(9), PayCurrency: "USD", ProviderProductId: "product", ProviderEnvironment: "test", GatewayTradeId: "checkout"}
			require.NoError(t, CreateAdtyTopUp(&topUp, &ext))
			facts := AdtyPaymentFacts{Provider: provider, Method: provider, PaidAmount: decimal.NewFromInt(9), PaidCurrency: "USD", Source: AdtyPaymentSourceCallback,
				ProductId: "product", Environment: "test", GatewayTradeId: "checkout", TxHash: "payment"}
			for _, field := range []string{"product", "environment", "checkout", "method"} {
				wrong := facts
				switch field {
				case "product":
					wrong.ProductId = "other"
				case "environment":
					wrong.Environment = "prod"
				case "checkout":
					wrong.GatewayTradeId = "other"
				case "method":
					wrong.Method = "other"
				}
				_, err := CompleteAdtyTopUp(topUp.TradeNo, wrong, "")
				require.ErrorIs(t, err, ErrPaymentAmountMismatch, field)
			}
			wrong := facts
			wrong.PaidCurrency = "EUR"
			_, err := CompleteAdtyTopUp(topUp.TradeNo, wrong, "")
			require.ErrorIs(t, err, ErrPaymentAmountMismatch)
			wrong = facts
			wrong.PaidAmount = decimal.NewFromInt(8)
			_, err = CompleteAdtyTopUp(topUp.TradeNo, wrong, "")
			require.ErrorIs(t, err, ErrPaymentAmountMismatch)
			already, err := CompleteAdtyTopUp(topUp.TradeNo, facts, "")
			require.NoError(t, err)
			require.False(t, already)
			already, err = CompleteAdtyTopUp(topUp.TradeNo, facts, "")
			require.NoError(t, err)
			require.True(t, already)
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, 6_000_100, stored.Quota+stored.QuotaPending)
			var entries []AdtyBalanceLedger
			require.NoError(t, db.Where("user_id = ? AND ref_type = ? AND ref_id = ?", user.Id, "topup", topUp.TradeNo).Find(&entries).Error)
			require.Len(t, entries, 1)
			require.EqualValues(t, 6_000_000, entries[0].DeltaQuota)
			require.EqualValues(t, 100, entries[0].BalanceBefore)
			require.EqualValues(t, 6_000_100, entries[0].BalanceAfter)
			orders, total, err := ListUserAdtyTopUps(user.Id, &common.PageInfo{Page: 1, PageSize: 10})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, orders, 1)
			require.NotNil(t, orders[0].Ext)
		})
	}
}

func TestHostedRefundAndDisputeEventsCannotAuthorizeACredit(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	require.NoError(t, db.Create(&TopUp{TradeNo: "refund-before-paid", PaymentProvider: PaymentProviderCreem, Status: common.TopUpStatusPending}).Error)
	for _, eventType := range []string{"refund.created", "refund.succeeded", "charge.refunded", "charge.dispute.created", "dispute.created"} {
		event := PaymentOrphanEvent{Provider: PaymentProviderCreem, ReferenceID: "refund-before-paid", EventType: eventType, Status: PaymentOrphanStatusPendingReview}
		require.False(t, CanCreditPaymentOrphanEvent(&event), eventType)
	}
}
