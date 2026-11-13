package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderTraceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("database DSN not configured")
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			require.NoError(t, db.AutoMigrate(&model.TopUp{}))
			require.NoError(t, model.MigrateAdtyTables())
			buyer := model.User{Username: "trace-buyer", SiteId: 9}
			require.NoError(t, db.Create(&buyer).Error)
			order := model.TopUp{TradeNo: "frozen-trace", UserId: buyer.Id, Status: model.TopUpStatusRefunded, PaymentProvider: model.PaymentProviderStripe}
			require.NoError(t, db.Create(&order).Error)
			require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: order.TradeNo, UserId: buyer.Id, SiteId: 7, Provider: order.PaymentProvider,
				FaceUSD: decimal.NewFromInt(2), BonusUSD: decimal.RequireFromString("0.2"), PromoBonusUSD: decimal.RequireFromString("0.1"),
				RefundedAt: 101, RefundQuota: 123, RefundAmount: decimal.NewFromInt(1), PayCurrency: "USD", RefundReason: "authorized-refund-reason", ProviderPayload: "secret-provider-payload"}).Error)
			for i := range 21 {
				require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: order.TradeNo, UserId: buyer.Id, SiteId: 7, Provider: order.PaymentProvider,
					Source: "callback", Outcome: "replayed", PaidAmount: decimal.NewFromInt(2), PaidCurrency: "USD", CreatedAt: int64(i + 1)}).Error)
			}
			require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "unrelated-order", UserId: buyer.Id, SiteId: 7, Provider: order.PaymentProvider, Outcome: "failed"}).Error)
			for _, ref := range []string{order.TradeNo, "unrelated-order"} {
				require.NoError(t, db.Create(&model.AdtyBalanceLedger{UserId: buyer.Id, Type: "refund", RefType: "topup", RefId: ref, DeltaQuota: -123, BalanceBefore: 456, BalanceAfter: 333, Reason: "authorized-refund-reason", OperatorId: 41}).Error)
			}
			require.NoError(t, db.Create(&model.AdtyPromoRedemption{TradeNo: order.TradeNo, UserId: buyer.Id, BonusUSD: decimal.RequireFromString("0.1"), CreatedAt: 91}).Error)
			commission := model.ReferralCommission{SourceType: model.ReferralSourceTopup, SourceOrderId: order.Id, SourceTradeNo: order.TradeNo, InviteeUserId: buyer.Id, CommissionAmount: 0.5, Status: "clawed_back", SettlementCurrency: "USD"}
			require.NoError(t, db.Create(&commission).Error)
			require.NoError(t, db.Create(&model.ReferralCommissionLedger{CommissionId: commission.Id, Type: "commission_clawback", RefId: order.TradeNo, ExternalRefId: "trace-clawback", DeltaAvailable: -0.5, SettlementCurrency: "USD"}).Error)
			require.NoError(t, db.Create(&model.ReferralCommissionLedger{CommissionId: commission.Id + 99, Type: "commission_transfer", RefId: "other-private-commission", ExternalRefId: "unrelated-commission"}).Error)
			require.NoError(t, db.Create(&model.AdtySiteWalletLedger{SiteId: 7, WalletKind: "operating", Type: "adjustment", RefType: "topup", RefId: order.TradeNo, IdempotencyKey: "safe-order-adjustment", Delta: decimal.NewFromInt(-1)}).Error)
			require.NoError(t, db.Create(&model.AdtySiteWalletLedger{SiteId: 7, WalletKind: "margin", Type: "commission_clawback", RefType: "site_invite", RefId: order.TradeNo, IdempotencyKey: "trace-commission-clawback", Delta: decimal.RequireFromString("0.5")}).Error)
			require.NoError(t, db.Create(&model.AdtySiteInviteCommission{SiteId: 7, InviterId: 3, InviteeId: buyer.Id, TradeNo: order.TradeNo, FaceUSD: decimal.NewFromInt(2), Rate: decimal.RequireFromString("0.1"), AmountUSD: decimal.RequireFromString("0.2"), ClawedUSD: decimal.RequireFromString("0.2"), RecoveredUSD: decimal.RequireFromString("0.2"), Status: "clawed_back"}).Error)
			require.NoError(t, db.Create(&model.AdtySiteWalletLedger{SiteId: 9, WalletKind: "operating", Type: "adjustment", RefType: "topup", RefId: order.TradeNo, IdempotencyKey: "other-private-site-adjustment", Delta: decimal.NewFromInt(-91)}).Error)
			require.NoError(t, db.Create(&model.PaymentOrphanEvent{Provider: order.PaymentProvider, ReferenceID: order.TradeNo, EventID: "safe-reversal", EventType: "reversal", Status: "refunded", Payload: "secret-orphan-payload", Error: "secret-provider-error"}).Error)
			require.NoError(t, db.Create(&model.PaymentOrphanReview{Provider: order.PaymentProvider, TradeNo: order.TradeNo, Status: "refunded", ExternalAmount: decimal.NewFromInt(1), ExternalCurrency: "USD", Note: "authorized-review", Request: "secret-review-request", Proof: "private-review-proof"}).Error)
			engine := gin.New()
			engine.GET("/orders/:trade_no", GetRechargeAuditOrder)
			read := func(path string) (*httptest.ResponseRecorder, struct {
				Success bool
				Data    struct {
					Order  rechargeAuditOrder
					Total  int
					Items  []adtyOrderTraceEntry
					Refund struct {
						Reason                 string
						RefundAmount           string `json:"refund_amount"`
						CommissionClawedUSD    string `json:"commission_clawed_usd"`
						CommissionRecoveredUSD string `json:"commission_recovered_usd"`
					}
					PromoBonusUSD string `json:"promo_bonus_usd"`
				}
			}) {
				t.Helper()
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
				var result struct {
					Success bool
					Data    struct {
						Order  rechargeAuditOrder
						Total  int
						Items  []adtyOrderTraceEntry
						Refund struct {
							Reason                 string
							RefundAmount           string `json:"refund_amount"`
							CommissionClawedUSD    string `json:"commission_clawed_usd"`
							CommissionRecoveredUSD string `json:"commission_recovered_usd"`
						}
						PromoBonusUSD string `json:"promo_bonus_usd"`
					}
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result), recorder.Body.String())
				for _, secret := range []string{"secret-provider-payload", "secret-orphan-payload", "secret-provider-error", "secret-review-request", "private-review-proof", "other-private-commission", "other-private-site-adjustment", "unrelated-order"} {
					assert.NotContains(t, recorder.Body.String(), secret)
				}
				return recorder, result
			}
			recorder, result := read("/orders/frozen-trace?section=payments&page_size=20&p=1")
			require.True(t, result.Success)
			assert.Contains(t, recorder.Body.String(), `"site_wallet"`)
			assert.Contains(t, recorder.Body.String(), `"commissions"`)
			assert.Equal(t, 21, result.Data.Total)
			require.Len(t, result.Data.Items, 20)
			assert.Equal(t, int64(21), result.Data.Items[0].Id)
			assert.Equal(t, 7, *result.Data.Order.SiteId)
			assert.Equal(t, "authorized-refund-reason", result.Data.Refund.Reason)
			assert.Equal(t, "1", result.Data.Refund.RefundAmount)
			assert.Equal(t, "0.2", result.Data.Refund.CommissionClawedUSD)
			assert.Equal(t, "0.2", result.Data.Refund.CommissionRecoveredUSD)
			assert.Equal(t, "0.1", result.Data.PromoBonusUSD)
			_, result = read("/orders/frozen-trace?section=payments&page_size=20&p=2")
			require.Len(t, result.Data.Items, 1)
			assert.Equal(t, int64(1), result.Data.Items[0].Id)
			for _, section := range []string{"wallet", "bonuses", "commissions", "commission_movements", "site_wallet", "exceptions", "reviews"} {
				_, result = read(fmt.Sprintf("/orders/frozen-trace?section=%s", section))
				require.True(t, result.Success, section)
				wantTotal := 1
				if section == "site_wallet" {
					wantTotal = 2
				}
				assert.Equal(t, wantTotal, result.Data.Total, section)
				require.Len(t, result.Data.Items, wantTotal, section)
				if section == "commission_movements" {
					assert.Equal(t, "-0.5", result.Data.Items[0].AvailableDelta.String())
				}
			}
			_, result = read("/orders/frozen-trace?section=unknown")
			assert.False(t, result.Success)
			recorder, result = read("/orders/missing-order")
			assert.Equal(t, http.StatusNotFound, recorder.Code)
			assert.False(t, result.Success)
		})
	}
}
