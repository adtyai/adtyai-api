package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/adty_setting"
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

// 优惠码写入已退役。Root 也不能新建或修改；历史行仍可列出。
func TestPromoCodeAdmin(t *testing.T) {
	f := newAdtyGuardFixture(t)
	body := `{"code":"spring-10","kind":"percent","value":"10","min_face_usd":"30","max_uses":100,"per_user_limit":1,"enabled":true}`
	f.grant(t, authz.AdtyPaymentRead)
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/promo-codes", body, f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code)

	f.grant(t, authz.AdtyPaymentRead, authz.AdtyPaymentOperate)
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/promo-codes", body, f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code, "payment write cannot replace Root")

	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/promo-codes", body, f.root)
	assert.Contains(t, resp.Body.String(), "Promo codes are not available")
	resp = adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/promo-codes/1", `{"kind":"fixed","value":"5","enabled":false}`, f.root)
	assert.Contains(t, resp.Body.String(), "Promo codes are not available")
	var count int64
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("code = ?", "SPRING-10").Count(&count).Error)
	assert.Zero(t, count)

	require.NoError(t, f.db.Create(&model.AdtyPromoCode{
		Code: "HISTORY10", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(10), Enabled: true, Funder: model.AdtyPromoFunderPlatform,
	}).Error)
	listed := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/promo-codes", "", f.token)
	require.Equal(t, http.StatusOK, listed.Code)
	assert.Contains(t, listed.Body.String(), "HISTORY10")
}

// 站长中心的优惠码接口（12.4 站长出资）：只有 active 或 paused 站点的站长能管，码固定为本站、出资方是站长；
// 不能改、删别站的码；码不能和平台的码重名。
func TestSiteOwnerPromoCodes(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, f.db.AutoMigrate(&model.AuthFlow{}, &model.TwoFA{}, &model.PasskeyCredential{})) // 站长写操作的二次验证（11.6）
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(2.0), new(0.0), new(0.0)
	ownerToken := adtySessionToken(t, f.db, "promo-owner", common.RoleCommonUser)
	strangerToken := adtySessionToken(t, f.db, "promo-stranger", common.RoleCommonUser)
	_ = adtySessionToken(t, f.db, "promo-other-owner", common.RoleCommonUser)
	var owner, other model.User
	require.NoError(t, f.db.Where("username = ?", "promo-owner").First(&owner).Error)
	require.NoError(t, f.db.Where("username = ?", "promo-other-owner").First(&other).Error)
	site, err := service.OpenAdtySite(owner.Id, "promo-shop", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	otherSite, err := service.OpenAdtySite(other.Id, "promo-other", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id IN ?", []int{site.Id, otherSite.Id}).Update("agreement_ver", service.AdtyCurrentSiteAgreementVersion()).Error)
	service.InvalidateAdtySiteCache()

	body := `{"code":"shop-code1","kind":"percent","value":"1","site_id":999,"funder":"platform","enabled":true}`
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/site/promo-codes", body, strangerToken)
	assert.Contains(t, resp.Body.String(), "Promo codes are not available")
	resp = adtyJSONWithProof(t, f.engine, http.MethodPost, "/api/adty/site/promo-codes", body, ownerToken, sessionProof(t, owner, service.AdtySiteOwnerActionPromo, 0))
	assert.Contains(t, resp.Body.String(), "Promo codes are not available")
	var created int64
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("code = ?", "SHOP-CODE1").Count(&created).Error)
	assert.Zero(t, created)
	_ = site
	_ = otherSite
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

// 优惠码总开关默认关闭。关着时报价、易支付下单和 USDT 下单都不接受码；打开后同一条链路才计入优惠加赠。
func TestPromoDisabledBlocksQuoteAndPay(t *testing.T) {
	cfg := adty_setting.GetPromoSetting()
	savedPromo := cfg.PromoEnabled
	cfg.PromoEnabled = false
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		cfg.PromoEnabled = savedPromo
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})

	f := newAdtyGuardFixture(t)
	confirmRouterCompliance(t)
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

	f.grant(t, authz.AdtyPaymentRead)
	settings := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/promo-codes", "", f.token)
	require.Equal(t, http.StatusOK, settings.Code)

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

	f.grant(t, authz.AdtyPaymentRead, authz.AdtyPaymentOperate)
	turnedOn := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_marketing_setting.promo_enabled","value":"true"}`, f.token)
	require.Equal(t, http.StatusForbidden, turnedOn.Code)
	turnedOn = adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_marketing_setting.promo_enabled","value":"true"}`, f.root)
	require.Contains(t, turnedOn.Body.String(), `"success":true`)
	require.True(t, cfg.PromoEnabled)
	stillRefused := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", quoteWithCode, token)
	require.Contains(t, stillRefused.Body.String(), "Promo codes are not available")
	assert.Zero(t, gw.snapshot().creates)
}
