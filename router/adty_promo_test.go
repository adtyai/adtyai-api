package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredMarketingEndpointsReturnNotFound(t *testing.T) {
	f := newAdtyGuardFixture(t)
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/adty/marketing/consent-default"},
		{http.MethodGet, "/api/adty/marketing/self"},
		{http.MethodGet, "/api/adty/marketing/unsubscribe"},
		{http.MethodPost, "/api/adty/marketing/resend-webhook"},
		{http.MethodGet, "/api/adty/admin/marketing/settings"},
		{http.MethodGet, "/api/adty/admin/marketing/campaigns"},
		{http.MethodGet, "/api/adty/admin/marketing/triggers"},
		{http.MethodGet, "/api/adty/admin/marketing/sources"},
		{http.MethodGet, "/api/adty/admin/marketing/promo-codes"},
	} {
		response := adtyJSON(t, f.engine, request.method, request.path, `{}`, f.root)
		assert.Equal(t, http.StatusNotFound, response.Code, "%s %s", request.method, request.path)
	}
}

// 带优惠码的报价按用户限速；不带码的报价不计数（任务 12.4）。
func TestPromoQuoteRateLimited(t *testing.T) {
	oldEnable, oldNum := common.CriticalRateLimitEnable, common.CriticalRateLimitNum
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 3
	t.Cleanup(func() {
		common.CriticalRateLimitEnable = oldEnable
		common.CriticalRateLimitNum = oldNum
	})
	f := newAdtyGuardFixture(t)
	token := adtySessionToken(t, f.db, "promo-quote-limit", common.RoleCommonUser)
	plain := `{"amount":10,"method":"alipay"}`
	withCode := `{"amount":10,"method":"alipay","promo_code":"TRYCODE1"}`
	for i := 0; i < common.CriticalRateLimitNum+1; i++ {
		resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", plain, token)
		require.NotEqual(t, http.StatusTooManyRequests, resp.Code, "quote %d without a promo code", i+1)
	}
	for i := 0; i < common.CriticalRateLimitNum; i++ {
		resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", withCode, token)
		require.NotEqual(t, http.StatusTooManyRequests, resp.Code, "promo quote %d", i+1)
	}
	blocked := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", withCode, token)
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
}

// 优惠码已退役：报价、易支付下单和 USDT 下单只要带码就拒绝，空白码当没带。
func TestPromoCodesAreRejectedByQuoteAndPay(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})

	f := newAdtyGuardFixture(t)
	setRouterBonusRates(t)
	gw := useFakeBEpusdt(t)
	epayGW := newFakeEpay(t, epayQR)
	channel := seedRouterEpayChannel(t, "promo-off", epayGW.server.URL, model.AdtyEpayOrderModeAPI, 1)
	t.Cleanup(func() {
		_ = model.SetAdtyEpayChannelStatus(channel.Id, model.AdtyEpayStatusDisabled, "manual")
	})
	require.NoError(t, f.db.Create(&model.AdtyPromoCode{
		Code: "SAVE10PCT", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(10),
		Enabled: true, Funder: model.AdtyPromoFunderPlatform,
	}).Error)
	token := adtySessionToken(t, f.db, "promo-switch-user", common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "promo-switch-user").First(&user).Error)

	quoteWithCode := `{"amount":100,"method":"usdt.trc20","promo_code":"SAVE10PCT"}`
	rejected := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", quoteWithCode, token)
	require.Contains(t, rejected.Body.String(), `"success":false`)
	require.Contains(t, rejected.Body.String(), "Promo codes are not available")
	plain := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", `{"amount":100,"method":"usdt.trc20","promo_code":"   "}`, token)
	quoted := decodeAdtyData[struct {
		Quote struct {
			PromoBonusUSD decimal.Decimal `json:"promo_bonus_usd"`
			BonusUSD      decimal.Decimal `json:"bonus_usd"`
		} `json:"quote"`
	}](t, plain.Body)
	assert.Equal(t, "2.5", quoted.Quote.BonusUSD.String())
	assert.True(t, quoted.Quote.PromoBonusUSD.IsZero())

	epayRejected := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/epay/pay", `{"amount":100,"method":"alipay","device":"pc","promo_code":"SAVE10PCT"}`, token)
	require.Contains(t, epayRejected.Body.String(), "Promo codes are not available")
	usdtRejected := adtyJSON(t, f.engine, http.MethodPost, "/api/user/bepusdt/pay", `{"amount":100,"chain":"usdt.trc20","promo_code":"save10pct"}`, token)
	require.Contains(t, usdtRejected.Body.String(), "Promo codes are not available")
	assert.Zero(t, gw.snapshot().creates)
	var orders int64
	require.NoError(t, f.db.Model(&model.TopUp{}).Where("user_id = ?", user.Id).Count(&orders).Error)
	assert.Zero(t, orders)

}
