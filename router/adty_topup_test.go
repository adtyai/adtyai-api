package router

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeAdtyProvider struct {
	name      string
	cancelErr error
	canceled  []string
}

func (p *fakeAdtyProvider) Name() string { return p.name }
func (p *fakeAdtyProvider) Methods() []service.AdtyMethodInfo {
	return []service.AdtyMethodInfo{{Id: "usdt.trc20", Kind: "usdt", Available: true}}
}
func (p *fakeAdtyProvider) AvailableFor(string, *service.AdtyQuote) bool { return true }
func (p *fakeAdtyProvider) Cancel(ext *model.AdtyTopupOrder) error {
	p.canceled = append(p.canceled, ext.TradeNo)
	return p.cancelErr
}

func confirmRouterCompliance(t *testing.T) {
	t.Helper()
	payment := operation_setting.GetPaymentSetting()
	oldConfirmed, oldVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() { payment.ComplianceConfirmed, payment.ComplianceTermsVersion = oldConfirmed, oldVersion })
}

// setRouterBonusRates 设一个 $100 加赠 2.5% 的档位，结束时还原。加赠没有内置档位（任务 3.6）。
func setRouterBonusRates(t *testing.T) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	old := setting.BonusRates
	setting.BonusRates = map[int]float64{10: 1, 100: 2.5}
	t.Cleanup(func() { setting.BonusRates = old })
}

func seedRouterOrder(t *testing.T, db *gorm.DB, userID int, tradeNo, status string, expireAt int64) {
	t.Helper()
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: userID, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, CreateTime: 1, Status: status},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: userID, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
			FaceUSD: decimal.NewFromInt(100), BonusUSD: decimal.RequireFromString("2.5"), CreditQuota: 51_250_000,
			PayAmount: decimal.NewFromInt(100), PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1), ExpireAt: expireAt},
	))
}

func TestAdtyWalletTopUpAPIs(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	provider := &fakeAdtyProvider{name: model.PaymentProviderAdtyBEpusdt}
	real := service.AdtyPaymentProviderByName(model.PaymentProviderAdtyBEpusdt)
	service.RegisterAdtyPaymentProvider(provider)
	t.Cleanup(func() { service.RegisterAdtyPaymentProvider(real) })
	owner := adtySessionToken(t, f.db, "wallet-owner", common.RoleCommonUser)
	other := adtySessionToken(t, f.db, "wallet-other", common.RoleCommonUser)
	var ownerUser model.User
	require.NoError(t, f.db.Where("username = ?", "wallet-owner").First(&ownerUser).Error)
	now := common.GetTimestamp()
	seedRouterOrder(t, f.db, ownerUser.Id, "EPU-wallet-old", common.TopUpStatusPending, now+600)
	seedRouterOrder(t, f.db, ownerUser.Id, "EPU-wallet-new", common.TopUpStatusPending, now+900)
	seedRouterOrder(t, f.db, ownerUser.Id, "EPU-wallet-gone", common.TopUpStatusPending, now-10)
	seedRouterOrder(t, f.db, ownerUser.Id, "EPU-wallet-paid", common.TopUpStatusSuccess, now+900)

	info := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/topup/info", "", owner)
	require.Equal(t, http.StatusOK, info.Code)
	body := decodeAdtyData[struct {
		Tiers   []struct{ Amount int } `json:"tiers"`
		Pending *struct {
			TradeNo string `json:"trade_no"`
		} `json:"pending"`
	}](t, info.Body)
	assert.Len(t, body.Tiers, len(operation_setting.GetPaymentSetting().AmountOptions), "tiers are the amount options set in the admin")
	require.NotNil(t, body.Pending)
	assert.Equal(t, "EPU-wallet-new", body.Pending.TradeNo, "the newest unexpired pending order")

	quote := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", `{"amount":150,"method":"usdt.trc20"}`, owner)
	quoted := decodeAdtyData[struct {
		Quote struct {
			BonusUSD decimal.Decimal `json:"bonus_usd"`
		} `json:"quote"`
		Available bool `json:"available"`
	}](t, quote.Body)
	assert.Equal(t, "3.75", quoted.Quote.BonusUSD.String())
	assert.True(t, quoted.Available)

	notMine := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/topup/orders/EPU-wallet-new", "", other)
	assert.Equal(t, http.StatusNotFound, notMine.Code)

	cancelPaid := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/orders/EPU-wallet-paid/cancel", "", owner)
	assert.Contains(t, cancelPaid.Body.String(), `"success":false`)

	provider.cancelErr = service.ErrAdtyPaymentDetected
	failed := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/orders/EPU-wallet-new/cancel", "", owner)
	assert.Contains(t, failed.Body.String(), `"success":false`)
	assert.Contains(t, failed.Body.String(), "A transfer has been detected")
	var stored model.TopUp
	require.NoError(t, f.db.Where("trade_no = ?", "EPU-wallet-new").First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)

	provider.cancelErr = errors.New("dial tcp: connection refused")
	canceled := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/orders/EPU-wallet-new/cancel", "", owner)
	assert.Contains(t, canceled.Body.String(), `"success":true`, canceled.Body.String())
	require.NoError(t, f.db.Where("trade_no = ?", "EPU-wallet-new").First(&stored).Error)
	assert.Equal(t, model.TopUpStatusCanceled, stored.Status)

	// 2026-11-01 取消了支付合规确认：没确认也能报价和下单。
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false
	open := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", `{"amount":150,"method":"usdt.trc20"}`, owner)
	assert.Contains(t, open.Body.String(), `"success":true`, open.Body.String())
}

func TestAdtyTopUpMaintenanceExpiresOrders(t *testing.T) {
	f := newAdtyGuardFixture(t)
	adtySessionToken(t, f.db, "expire-owner", common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "expire-owner").First(&user).Error)
	seedRouterOrder(t, f.db, user.Id, "EPU-expire", common.TopUpStatusPending, 100)
	service.RunAdtyTopUpMaintenanceOnce(common.GetTimestamp())
	var stored model.TopUp
	require.NoError(t, f.db.Where("trade_no = ?", "EPU-expire").First(&stored).Error)
	assert.Equal(t, common.TopUpStatusExpired, stored.Status)
}

func TestFinanceActionsRequirePermissions(t *testing.T) {
	f := newAdtyGuardFixture(t)
	var user model.User
	adtySessionToken(t, f.db, "finance-buyer", common.RoleCommonUser)
	require.NoError(t, f.db.Where("username = ?", "finance-buyer").First(&user).Error)
	seedRouterOrder(t, f.db, user.Id, "EPU-finance", common.TopUpStatusPending, common.GetTimestamp()+900)

	f.grant(t, authz.AdtyFinanceRead)
	audit := adtyJSON(t, f.engine, http.MethodGet, "/api/user/admin/finance/recharge-audit", "", f.token)
	assert.Contains(t, audit.Body.String(), `"EPU-finance"`)
	complete := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/EPU-finance/complete", `{"proof":"0xabc","note":"checked"}`, f.token)
	assert.Equal(t, http.StatusForbidden, complete.Code)
	refund := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/EPU-finance/refund", `{"reason":"x","idempotency_key":"k1","refund_quota":1}`, f.token)
	assert.Equal(t, http.StatusForbidden, refund.Code)

	f.grant(t, authz.AdtyFinanceRead, authz.AdtyFinanceManualCredit, authz.AdtyFinanceRefund)
	missingProof := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/EPU-finance/complete", `{"note":"checked"}`, f.token)
	assert.Contains(t, missingProof.Body.String(), `"success":false`)
	complete = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/EPU-finance/complete", `{"proof":"0xabc","note":"checked"}`, f.token)
	require.Contains(t, complete.Body.String(), `"success":true`, complete.Body.String())
	var stored model.User
	require.NoError(t, f.db.First(&stored, user.Id).Error)
	assert.Equal(t, 51_250_000, stored.Quota, "manual completion credits face plus bonus")
	ext, err := model.GetAdtyTopupOrder("EPU-finance")
	require.NoError(t, err)
	assert.Equal(t, "0xabc", ext.PaidTxHash)
	assert.Equal(t, model.AdtyPaymentSourceManual, ext.PaidSource)
	var audits int64
	require.NoError(t, f.db.Model(&model.AuditLog{}).Where("action = ?", "adty.topup.manual_complete").Count(&audits).Error)
	assert.EqualValues(t, 1, audits)

	preview := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/topup/EPU-finance/refund-preview", "", f.token)
	require.Contains(t, preview.Body.String(), `"success":true`, preview.Body.String())
	require.Contains(t, preview.Body.String(), `"refund_quota":50000000`)
	refund = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/EPU-finance/refund", `{"reason":"duplicate","idempotency_key":"refund-1","refund_quota":50000000}`, f.token)
	require.Contains(t, refund.Body.String(), `"success":true`, refund.Body.String())
	assert.Contains(t, refund.Body.String(), `"deducted_quota":51250000`)
	assert.Contains(t, refund.Body.String(), `"refund_quota":50000000`, "the bonus is not refunded")
	assert.Contains(t, refund.Body.String(), `"refund_amount":"100"`)
	assert.Contains(t, refund.Body.String(), `"currency":"USDT"`)
	require.NoError(t, f.db.First(&stored, user.Id).Error)
	assert.Zero(t, stored.Quota)

	summary := adtyJSON(t, f.engine, http.MethodGet, "/api/user/admin/finance/recharge-audit/summary", "", f.token)
	assert.Contains(t, summary.Body.String(), `"currency":"USD"`)
}

func TestPaymentOrphanReviewRequiresWritePermissionAndEvidence(t *testing.T) {
	f := newAdtyGuardFixture(t)
	event := &model.PaymentOrphanEvent{Provider: model.PaymentProviderStripe, ReferenceID: "review-router-order", EventID: "reversal:router-review", EventType: "dispute.open"}
	require.NoError(t, model.RecordPaymentOrphanEvent(event))
	path := fmt.Sprintf("/api/user/admin/finance/payment-orphans/%d/resolve", event.ID)
	body := `{"status":"pending_review","trade_no":"review-router-order","external_amount":"0","external_currency":"USD","proof":"provider-dispute-open","note":"not an actual completed reversal"}`
	f.grant(t, authz.AdtyFinanceRead)
	denied := adtyJSON(t, f.engine, http.MethodPost, path, body, f.token)
	require.Equal(t, http.StatusForbidden, denied.Code)
	f.grant(t, authz.AdtyFinanceRead, authz.AdtyFinanceManualCredit)
	missingProof := adtyJSON(t, f.engine, http.MethodPost, path, `{"status":"refunded","note":"clicked refunded"}`, f.token)
	require.Contains(t, missingProof.Body.String(), `"success":false`)
	result := adtyJSON(t, f.engine, http.MethodPost, path, body, f.token)
	require.Contains(t, result.Body.String(), `"success":true`, result.Body.String())
	require.Contains(t, result.Body.String(), `"external_amount":"0"`)
	listed := adtyJSON(t, f.engine, http.MethodGet, "/api/user/admin/finance/payment-orphans?status=pending_review", "", f.token)
	require.Contains(t, listed.Body.String(), `"proof":"provider-dispute-open"`)
	var reviews int64
	require.NoError(t, f.db.Model(&model.PaymentOrphanReview{}).Count(&reviews).Error)
	require.EqualValues(t, 1, reviews)
}

func TestOrphanCreditRunsBonusAndHooks(t *testing.T) {
	f := newAdtyGuardFixture(t)
	f.grant(t, authz.AdtyFinanceRead, authz.AdtyFinanceManualCredit)
	var user model.User
	adtySessionToken(t, f.db, "orphan-buyer", common.RoleCommonUser)
	require.NoError(t, f.db.Where("username = ?", "orphan-buyer").First(&user).Error)

	hookCalls := 0
	old := model.AdtyAfterTopUpCreditedTx
	model.AdtyAfterTopUpCreditedTx = func(*gorm.DB, *model.TopUp, *model.AdtyTopupOrder) error { hookCalls++; return nil }
	t.Cleanup(func() { model.AdtyAfterTopUpCreditedTx = old })

	seedRouterOrder(t, f.db, user.Id, "EPU-orphan", common.TopUpStatusExpired, 100)
	require.NoError(t, model.UpdateAdtyTopupOrderFields("EPU-orphan", map[string]any{"pay_address": "TAddr", "gateway_trade_id": "gw-1"}))
	usdt := &model.PaymentOrphanEvent{Provider: model.PaymentProviderAdtyBEpusdt, EventType: "callback", ReferenceID: "EPU-orphan",
		Reason: "temporary failure", Payload: `{"actual_amount":100,"amount":100,"token":"TAddr","trade_id":"gw-1","block_transaction_id":"0xtx"}`}
	require.NoError(t, model.RecordPaymentOrphanEvent(usdt))

	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: user.Id, Amount: 100, TradeNo: "EPY-orphan", PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderAdtyEpay, CreateTime: 1, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: "EPY-orphan", UserId: user.Id, Provider: model.PaymentProviderAdtyEpay, Method: "alipay",
			FaceUSD: decimal.NewFromInt(100), BonusUSD: decimal.RequireFromString("2.5"), CreditQuota: 51_250_000,
			PayAmount: decimal.RequireFromString("730"), PayCurrency: "CNY", TopupGroupRatio: decimal.NewFromInt(1), ExpireAt: 100},
	))
	epay := &model.PaymentOrphanEvent{Provider: model.PaymentProviderAdtyEpay, EventType: "callback", ReferenceID: "EPY-orphan",
		Reason: "temporary failure", Payload: `{"money":"730.00","trade_no":"2026103100001"}`}
	require.NoError(t, model.RecordPaymentOrphanEvent(epay))

	for _, event := range []*model.PaymentOrphanEvent{usdt, epay} {
		resp := adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/user/admin/finance/payment-orphans/%d/credit", event.ID), `{"note":"verified"}`, f.token)
		require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
	}
	var stored model.User
	require.NoError(t, f.db.First(&stored, user.Id).Error)
	assert.Equal(t, 2*51_250_000, stored.Quota, "both orders credited with their bonus")
	assert.Equal(t, 2, hookCalls)

	var orphan model.PaymentOrphanEvent
	require.NoError(t, f.db.First(&orphan, usdt.ID).Error)
	assert.Equal(t, model.PaymentOrphanStatusCredited, orphan.Status)
	again := adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/user/admin/finance/payment-orphans/%d/credit", usdt.ID), `{}`, f.token)
	assert.Contains(t, again.Body.String(), `"success":false`)
}
