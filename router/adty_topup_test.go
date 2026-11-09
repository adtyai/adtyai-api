package router

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type financeQueryTransport func(*http.Request) (*http.Response, error)

func TestPaymentScenesConstrainNativeEpayOrders(t *testing.T) {
	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
	setRouterBonusRates(t)
	shortEpayTimeouts(t)
	allowedGateway := newFakeEpay(t, epayQR)
	forbiddenGateway := newFakeEpay(t, func(url.Values) (int, string, time.Duration) {
		t.Error("the unapproved channel must never receive an order")
		return 200, `{}`, 0
	})
	allowed := seedRouterEpayChannel(t, "scene-allowed", allowedGateway.server.URL, model.AdtyEpayOrderModeAPI, 1)
	seedRouterEpayChannel(t, "scene-forbidden", forbiddenGateway.server.URL, model.AdtyEpayOrderModeAPI, 99)
	buyer, userID := bepusdtBuyer(t, f, "scene-epay-buyer")
	body := fmt.Sprintf(`{"expected_version":1,"scenes":[{"key":"limited","name":"Limited checkout","business":"wallet_topup","enabled":true,"providers":["adty_epay"],"methods":["alipay"],"channel_ids":[%d],"currencies":["CNY"],"site_scope":"main","site_ids":[],"user_types":["main_customer"],"min_usd":"50","max_usd":"100"}]}`, allowed.Id)
	saved := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/payment-scenes", body, f.root)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	for _, input := range []string{
		`{"amount":100,"method":"alipay"}`,
		`{"amount":49,"method":"alipay","payment_scene":"limited"}`,
		`{"amount":101,"method":"alipay","payment_scene":"limited"}`,
		`{"amount":100,"method":"wxpay","payment_scene":"limited"}`,
		`{"amount":100,"method":"alipay","payment_scene":"missing"}`,
	} {
		response := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/epay/pay", input, buyer)
		assert.Contains(t, response.Body.String(), `"success":false`, input)
	}
	var count int64
	require.NoError(t, f.db.Model(&model.TopUp{}).Where("user_id = ?", userID).Count(&count).Error)
	require.Zero(t, count, "denied native requests cannot create orders")
	valid := `{"amount":100,"method":"alipay","payment_scene":"limited"}`
	require.NoError(t, f.db.Model(&model.AdtyEpayChannelMethod{}).Where("channel_id = ? AND method = ?", allowed.Id, "alipay").Update("min_cny", 999999).Error)
	quoted := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", valid, buyer)
	quotedData := decodeAdtyData[struct {
		Available bool `json:"available"`
	}](t, quoted.Body)
	assert.False(t, quotedData.Available, "a channel outside the scene cannot make a quote available")
	require.NoError(t, f.db.Model(&model.AdtyEpayChannelMethod{}).Where("channel_id = ? AND method = ?", allowed.Id, "alipay").Update("min_cny", 0).Error)
	response := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/epay/pay", valid, buyer)
	view := decodeAdtyData[model.AdtyTopUpView](t, response.Body)
	require.NotNil(t, view.Ext)
	assert.Equal(t, allowed.Id, view.Ext.ChannelId)
	var frozen map[string]any
	raw, err := common.Marshal(view.Ext)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(raw, &frozen))
	assert.Equal(t, "limited", frozen["payment_scene"])
	assert.EqualValues(t, 2, frozen["payment_scene_version"])
	assert.NotContains(t, frozen, "payment_scene_snapshot", "policy audience details stay private")
}

func TestPaymentScenesRequireRootAndVersionedChanges(t *testing.T) {
	f := newAdtyGuardFixture(t)
	const path = "/api/adty/admin/payment-scenes"
	initial := adtyJSON(t, f.engine, http.MethodGet, path, "", f.root)
	require.Equal(t, http.StatusOK, initial.Code, initial.Body.String())
	config := decodeAdtyData[struct {
		Version int64 `json:"version"`
		Scenes  []struct {
			Key string `json:"key"`
		} `json:"scenes"`
	}](t, initial.Body)
	require.EqualValues(t, 1, config.Version)
	require.Len(t, config.Scenes, 1)
	assert.Equal(t, "wallet_topup", config.Scenes[0].Key)
	buyer := adtySessionToken(t, f.db, "scene-buyer", common.RoleCommonUser)
	for _, token := range []string{"", buyer, f.token} {
		response := adtyJSON(t, f.engine, http.MethodGet, path, "", token)
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, response.Code)
	}
	var root model.User
	require.NoError(t, f.db.Where("username = ?", "guard-root").First(&root).Error)
	readPAT := createAdtyScopedToken(t, root.Id, "option:read")
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, path, "", readPAT).Code)
	const disabled = `{"expected_version":1,"scenes":[{"key":"wallet_topup","name":"Wallet top-up","business":"wallet_topup","enabled":false,"providers":["bepusdt"],"methods":[],"channel_ids":[],"currencies":["USDT"],"site_scope":"all","site_ids":[],"user_types":[],"min_usd":"1","max_usd":"1000"}]}`
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPut, path, disabled, readPAT).Code)
	saved := adtyJSON(t, f.engine, http.MethodPut, path, disabled, f.root)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	assert.Contains(t, saved.Body.String(), `"version":2`)
	assert.Equal(t, http.StatusConflict, adtyJSON(t, f.engine, http.MethodPut, path, disabled, f.root).Code)
	info := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/topup/info", "", buyer)
	data := decodeAdtyData[struct {
		Scenes  []any `json:"scenes"`
		Methods []any `json:"methods"`
	}](t, info.Body)
	assert.Empty(t, data.Scenes)
	assert.Empty(t, data.Methods)
	var audits int64
	require.NoError(t, f.db.Model(&model.AuditLog{}).Where("action = ?", "adty.payment_scenes.update").Count(&audits).Error)
	assert.EqualValues(t, 1, audits, "only a committed configuration creates an audit")
}

func TestVerifiedNonPaidCallbacksRemainTraceable(t *testing.T) {
	f := newAdtyGuardFixture(t)
	var root model.User
	require.NoError(t, f.db.Where("username = ?", "guard-root").First(&root).Error)
	seedRouterOrder(t, f.db, root.Id, "callback-history", common.TopUpStatusPending, common.GetTimestamp()+600)
	cfg := setting.GetBEpusdtSetting()
	previousSecret := cfg.Secret
	cfg.Secret = "synthetic-callback-history-key"
	t.Cleanup(func() { cfg.Secret = previousSecret })
	for _, status := range []int{1, 3} {
		values := map[string]any{"order_id": "callback-history", "status": status}
		values["signature"] = service.BEpusdtSign(values, cfg.Secret)
		payload, err := common.Marshal(values)
		require.NoError(t, err)
		response := adtyJSON(t, f.engine, http.MethodPost, "/api/user/bepusdt/notify", string(payload), "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	var events []model.AdtyOrderPaymentEvent
	require.NoError(t, f.db.Where("trade_no = ?", "callback-history").Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, "received", events[0].Outcome)
	assert.Equal(t, "callback", events[0].Source)
	var buyer model.User
	require.NoError(t, f.db.First(&buyer, root.Id).Error)
	assert.Zero(t, buyer.Quota)
}

func (transport financeQueryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestFinanceQueryRequiresWritePermissionAndCreditsOnce(t *testing.T) {
	f := newAdtyGuardFixture(t)
	buyer := adtySessionToken(t, f.db, "query-buyer", common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "query-buyer").First(&user).Error)
	order := model.TopUp{UserId: user.Id, TradeNo: "creem-admin-query", Amount: 2, Money: 2.5,
		PaymentProvider: model.PaymentProviderCreem, PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusExpired}
	ext := model.AdtyTopupOrder{TradeNo: order.TradeNo, UserId: user.Id, Provider: order.PaymentProvider, Method: order.PaymentMethod,
		FaceUSD: decimal.NewFromInt(2), CreditQuota: 1000000, PayAmount: decimal.RequireFromString("2.5"), PayCurrency: "USD",
		ProviderProductId: "prod_query", ProviderEnvironment: "test", GatewayTradeId: "ch_query"}
	require.NoError(t, model.CreateAdtyTopUp(&order, &ext))
	oldKey, oldMode, oldTransport := setting.CreemApiKey, setting.CreemTestMode, http.DefaultTransport
	setting.CreemApiKey, setting.CreemTestMode = "synthetic-admin-query", true
	t.Cleanup(func() {
		setting.CreemApiKey, setting.CreemTestMode, http.DefaultTransport = oldKey, oldMode, oldTransport
	})
	queries := 0
	paid, wrongAmount, gatewayFails := false, false, false
	http.DefaultTransport = financeQueryTransport(func(req *http.Request) (*http.Response, error) {
		queries++
		require.Equal(t, "https://test-api.creem.io/v1/checkouts?checkout_id=ch_query", req.URL.String())
		require.Equal(t, "synthetic-admin-query", req.Header.Get("x-api-key"))
		if gatewayFails {
			return nil, errors.New("query failed with secret query-string-key")
		}
		status, amount := "pending", 250
		if paid {
			status = "completed"
		}
		if wrongAmount {
			amount = 251
		}
		payload, err := common.Marshal(map[string]any{"id": "ch_query", "request_id": order.TradeNo, "status": status, "mode": "test", "units": 1,
			"order": map[string]any{"id": "ord_query", "status": "paid", "type": "onetime", "mode": "test", "product": "prod_query", "currency": "USD", "amount": amount, "sub_total": amount, "amount_paid": amount}})
		require.NoError(t, err)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})
	path := "/api/adty/admin/topup/" + order.TradeNo + "/query"
	f.grant(t, authz.AdtyFinanceRead)
	readPAT := createAdtyScopedToken(t, f.admin.Id, "adty_finance:read")
	for _, credential := range []string{f.token, readPAT, buyer, ""} {
		response := adtyJSON(t, f.engine, http.MethodPost, path, `{}`, credential)
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, response.Code)
	}
	assert.Zero(t, queries)
	f.grant(t, authz.AdtyFinanceRead, authz.AdtyFinanceManualCredit)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, path, `{}`, readPAT).Code)
	writePAT := createAdtyScopedToken(t, f.admin.Id, "adty_finance:manual_credit")
	for _, scenario := range []struct {
		name                string
		paid, wrong, failed bool
		success             bool
	}{
		{"unconfirmed", false, false, false, true}, {"gateway failure", false, false, true, false},
		{"amount mismatch", true, true, false, false},
	} {
		paid, wrongAmount, gatewayFails = scenario.paid, scenario.wrong, scenario.failed
		response := adtyJSON(t, f.engine, http.MethodPost, path, `{}`, writePAT)
		var body struct {
			Success bool
			Data    struct{ Status string }
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, scenario.success, body.Success, scenario.name)
		assert.NotContains(t, response.Body.String(), "query-string-key")
		require.NoError(t, f.db.First(&user, user.Id).Error)
		assert.Zero(t, user.Quota, scenario.name)
		if scenario.success {
			assert.Equal(t, common.TopUpStatusExpired, body.Data.Status)
		}
	}
	paid, wrongAmount, gatewayFails = true, false, false
	for _, credential := range []string{writePAT, f.token, f.root} {
		response := adtyJSON(t, f.engine, http.MethodPost, path, `{}`, credential)
		require.Contains(t, response.Body.String(), `"success":true`, response.Body.String())
		assert.Contains(t, response.Body.String(), `"status":"success"`)
	}
	require.NoError(t, f.db.First(&user, user.Id).Error)
	assert.Equal(t, 1000000, user.Quota)
	var credits int64
	require.NoError(t, f.db.Model(&model.AdtyBalanceLedger{}).Where("ref_id = ?", order.TradeNo).Count(&credits).Error)
	assert.EqualValues(t, 1, credits)
	var audits []model.AuditLog
	require.NoError(t, f.db.Where("action = ?", "adty.topup.query").Find(&audits).Error)
	assert.GreaterOrEqual(t, len(audits), 2)
	for _, audit := range audits {
		assert.NotContains(t, fmt.Sprint(audit), "synthetic-admin-query")
		assert.NotContains(t, fmt.Sprint(audit), "query-string-key")
	}
	f.grant(t, authz.AdtyFinanceRead)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, path, `{}`, writePAT).Code)
	seedRouterOrder(t, f.db, user.Id, "bepusdt-no-query", common.TopUpStatusPending, common.GetTimestamp()+900)
	unsupported := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/bepusdt-no-query/query", `{}`, f.root)
	assert.Contains(t, unsupported.Body.String(), `"success":false`)
	require.NoError(t, f.db.Create(&model.TopUp{UserId: user.Id, TradeNo: "creem-missing-details", PaymentProvider: model.PaymentProviderCreem, PaymentMethod: model.PaymentMethodCreem, Status: common.TopUpStatusPending}).Error)
	require.NotPanics(t, func() {
		missing := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/topup/creem-missing-details/query", `{}`, f.root)
		assert.Equal(t, http.StatusOK, missing.Code)
		assert.Contains(t, missing.Body.String(), `"success":false`)
	})
}

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
