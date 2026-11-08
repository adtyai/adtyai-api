package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 营销设置：必填项没填时总开关打不开；保存后立即生效；webhook 密钥不回传（任务 12.1、约定 I）。
func TestMarketingSettingsRequireFieldsToEnable(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := adty_setting.GetMarketingSetting()
	saved := *cfg
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		*cfg = saved
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})
	cfg.Enabled, cfg.FromAddress, cfg.FooterCompany, cfg.FooterAddress, cfg.WebhookSecret = false, "", "", "", ""

	type view struct {
		Enabled                 bool     `json:"enabled"`
		FromAddress             string   `json:"from_address"`
		WebhookSecretConfigured bool     `json:"webhook_secret_configured"`
		WebhookSecret           *string  `json:"webhook_secret"`
		Missing                 []string `json:"missing"`
	}
	call := func(method, body, token string) (bool, string, view) {
		t.Helper()
		resp := adtyJSON(t, f.engine, method, "/api/adty/admin/marketing/settings", body, token)
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		var envelope struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    view   `json:"data"`
		}
		require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
		return envelope.Success, envelope.Message, envelope.Data
	}

	resp := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/marketing/settings", "", f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code, "admins need marketing permissions")
	f.grant(t, authz.AdtyMarketingRead)
	ok, _, got := call(http.MethodGet, "", f.token)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"from_address", "footer_company", "footer_address", "webhook_secret", "resend_api_key"}, got.Missing)
	resp = adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/settings", `{"enabled":true}`, f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code, "read permission cannot save")

	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingWrite)
	ok, message, _ := call(http.MethodPut, `{"enabled":true}`, f.token)
	assert.False(t, ok)
	assert.Equal(t, "Fill in the sender address, footer, webhook signing secret and Resend API key before turning marketing emails on", message)
	assert.False(t, cfg.Enabled)

	ok, message, _ = call(http.MethodPut, `{"webhook_secret":"not-a-secret"}`, f.token)
	assert.False(t, ok)
	assert.Equal(t, "Webhook signing secrets can only be changed on the marketing secrets page", message)
	secretByEditor := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/secrets", `{"webhook_secret":"whsec_plJ3nmyCDGBKInavdOK15jsl"}`, f.token)
	assert.Equal(t, http.StatusForbidden, secretByEditor.Code, "a marketing editor cannot change the signing secret")

	secret := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/secrets", `{"webhook_secret":"whsec_plJ3nmyCDGBKInavdOK15jsl","resend_api_key":"re_live"}`, f.root)
	require.Contains(t, secret.Body.String(), `"success":true`, secret.Body.String())
	assert.Equal(t, "whsec_plJ3nmyCDGBKInavdOK15jsl", cfg.WebhookSecret)
	ok, _, got = call(http.MethodPut, `{"enabled":true,"from_address":"ADTY AI <news@news.example.com>","footer_company":"ADTY Ltd","footer_address":"1 Example Road"}`, f.token)
	require.True(t, ok)
	assert.True(t, cfg.Enabled, "saved settings apply at once")
	assert.Equal(t, "ADTY AI <news@news.example.com>", cfg.FromAddress)
	assert.True(t, got.WebhookSecretConfigured)
	assert.Nil(t, got.WebhookSecret, "the secret is never sent back")
	assert.Empty(t, got.Missing)

	ok, _, _ = call(http.MethodPut, `{"footer_address":""}`, f.token)
	assert.False(t, ok, "a required field cannot be cleared while marketing is on")
	ok, message, _ = call(http.MethodPut, `{"enabled":false,"clear_webhook_secret":true}`, f.token)
	assert.False(t, ok)
	assert.Equal(t, "Webhook signing secrets can only be changed on the marketing secrets page", message)
	ok, _, _ = call(http.MethodPut, `{"enabled":false}`, f.token)
	require.True(t, ok)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "whsec_plJ3nmyCDGBKInavdOK15jsl", cfg.WebhookSecret, "settings cannot clear the signing secret")
}

func TestMarketingWebhookRejectsUnsigned(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := adty_setting.GetMarketingSetting()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.WebhookSecret = "whsec_plJ3nmyCDGBKInavdOK15jsl"
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/marketing/resend-webhook", `{"type":"email.complained","data":{"to":["a@b.c"]}}`, "")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

// 自动邮件规则：英文模板没写不能打开；退订链接 GET 不改状态，POST 才退订（任务 12.3）。
func TestMarketingTriggersAndUnsubscribeLink(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldSecret := common.SessionSecret
	if oldSecret == "" {
		common.SessionSecret = "router-marketing-secret"
	}
	t.Cleanup(func() { common.SessionSecret = oldSecret })
	f := newAdtyGuardFixture(t)
	cfg := adty_setting.GetMarketingSetting()
	saved := *cfg
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		*cfg = saved
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})
	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingWrite)
	resp := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/triggers", `{"unpaid":{"enabled":true,"days":3,"subjects":{},"bodies":{}}}`, f.token)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "Write the English subject and body before turning this email on")
	assert.False(t, cfg.TriggerUnpaidEnabled)

	resp = adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/triggers", `{"unpaid":{"enabled":true,"days":5,"subjects":{"en":"Hi"},"bodies":{"en":"<p>x</p>"}}}`, f.token)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"success":true`)
	assert.True(t, cfg.TriggerUnpaidEnabled, "saved settings apply at once")
	assert.Equal(t, 5, cfg.TriggerUnpaidDays)
	assert.Equal(t, "Hi", cfg.TriggerUnpaidSubjects["en"])

	_, err := model.SaveAdtyMarketingConsent(model.AdtyMarketingConsentInput{UserId: f.admin.Id, Email: "guard-admin@example.com", Consent: true})
	require.NoError(t, err)
	token := service.AdtyMarketingUnsubscribeToken(f.admin.Id)
	resp = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/marketing/unsubscribe?token="+token, "", "")
	require.Equal(t, http.StatusOK, resp.Code)
	contact, err := model.GetAdtyMarketingContact(f.admin.Id)
	require.NoError(t, err)
	assert.False(t, contact.Unsubscribed, "opening the link does not unsubscribe")
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/marketing/unsubscribe?token="+token, "List-Unsubscribe=One-Click", "")
	require.Equal(t, http.StatusOK, resp.Code)
	contact, err = model.GetAdtyMarketingContact(f.admin.Id)
	require.NoError(t, err)
	assert.True(t, contact.Unsubscribed)
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/marketing/unsubscribe?token=1.bad", "", "")
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

// 优惠码后台：创建和编辑分别授权；码统一大写、不能重复；百分比不能超过 100。
func TestPromoCodeAdmin(t *testing.T) {
	f := newAdtyGuardFixture(t)
	body := `{"code":"spring-10","kind":"percent","value":"10","min_face_usd":"30","max_uses":100,"per_user_limit":1,"enabled":true}`
	f.grant(t, authz.AdtyMarketingRead)
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/marketing/promo-codes", body, f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code)

	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingWrite)
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/marketing/promo-codes", body, f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code, "write cannot replace create")
	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingCreate)
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/marketing/promo-codes", body, f.token)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"code":"SPRING-10"`)
	assert.Contains(t, resp.Body.String(), `"funder":"platform"`)
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/marketing/promo-codes", body, f.token)
	assert.Contains(t, resp.Body.String(), "This promo code already exists")
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/marketing/promo-codes", `{"code":"BIG","kind":"percent","value":"101"}`, f.token)
	assert.Contains(t, resp.Body.String(), `"success":false`)
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/marketing/promo-codes", `{"code":"SHORT1","kind":"percent","value":"10","enabled":true}`, f.token)
	assert.Contains(t, resp.Body.String(), `"success":false`, "codes shorter than 8 characters are rejected")
	var shortCount int64
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("code = ?", "SHORT1").Count(&shortCount).Error)
	assert.Zero(t, shortCount)

	var code model.AdtyPromoCode
	require.NoError(t, f.db.Where("code = ?", "SPRING-10").First(&code).Error)
	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingWrite)
	resp = adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/promo-codes/"+common.Interface2String(code.Id),
		`{"code":"OTHER","kind":"fixed","value":"5","enabled":false}`, f.token)
	require.Equal(t, http.StatusOK, resp.Code)
	require.NoError(t, f.db.First(&code, code.Id).Error)
	assert.Equal(t, "SPRING-10", code.Code, "the code itself never changes")
	assert.Equal(t, model.AdtyPromoKindFixed, code.Kind)
	assert.False(t, code.Enabled)
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
	otherToken := adtySessionToken(t, f.db, "promo-other-owner", common.RoleCommonUser)
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
	assert.Equal(t, http.StatusForbidden, resp.Code, "only site owners")

	resp = adtyJSONWithProof(t, f.engine, http.MethodPost, "/api/adty/site/promo-codes", body, ownerToken, sessionProof(t, owner, service.AdtySiteOwnerActionPromo, 0))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
	var code model.AdtyPromoCode
	require.NoError(t, f.db.Where("code = ?", "SHOP-CODE1").First(&code).Error)
	assert.Equal(t, site.Id, code.SiteId, "the site comes from the caller, not the body")
	assert.Equal(t, model.AdtyPromoFunderSiteOwner, code.Funder)
	assert.Equal(t, owner.Id, code.CreatedBy)

	resp = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/site/promo-codes", "", ownerToken)
	assert.Contains(t, resp.Body.String(), `"SHOP-CODE1"`)
	resp = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/site/promo-codes", "", otherToken)
	assert.NotContains(t, resp.Body.String(), `"SHOP-CODE1"`, "owners only see their own codes")

	path := "/api/adty/site/promo-codes/" + common.Interface2String(code.Id)
	resp = adtyJSON(t, f.engine, http.MethodPut, path, `{"kind":"percent","value":"2","enabled":false}`, otherToken)
	assert.Contains(t, resp.Body.String(), `"success":false`, "another site's code")
	resp = adtyJSON(t, f.engine, http.MethodDelete, path, "", otherToken)
	assert.Contains(t, resp.Body.String(), `"success":false`)

	// 面值超过 max_site_bonus_extra 能给的上限，保存时就拒绝（报价时还会按站点档位再查一次）。
	resp = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/site/promo-codes", `{"code":"shop-big1","kind":"percent","value":"2.5","enabled":true}`, ownerToken)
	assert.Contains(t, resp.Body.String(), "This promo code adds more bonus than this site may give", "invalid input is rejected before asking for verification")

	resp = adtyJSONWithProof(t, f.engine, http.MethodPut, path, `{"kind":"percent","value":"2","enabled":false}`, ownerToken, sessionProof(t, owner, service.AdtySiteOwnerActionPromo, code.Id))
	require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
	require.NoError(t, f.db.First(&code, code.Id).Error)
	assert.False(t, code.Enabled)
	assert.Equal(t, site.Id, code.SiteId)

	// 站点暂停时站长仍能管理（只读和停用都要能做）。
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusPaused).Error)
	service.InvalidateAdtySiteCache()
	resp = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/site/promo-codes", "", ownerToken)
	assert.Contains(t, resp.Body.String(), `"SHOP-CODE1"`)

	resp = adtyJSONWithProof(t, f.engine, http.MethodDelete, path, "", ownerToken, sessionProof(t, owner, service.AdtySiteOwnerActionPromo, code.Id))
	require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
	var count int64
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("id = ?", code.Id).Count(&count).Error)
	assert.Zero(t, count)
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
	cfg := adty_setting.GetMarketingSetting()
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

	f.grant(t, authz.AdtyMarketingRead)
	settings := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/marketing/settings", "", f.token)
	require.Contains(t, settings.Body.String(), `"promo_enabled":false`)

	quoteWithCode := `{"amount":100,"method":"usdt.trc20","promo_code":"SAVE10PCT"}`
	rejected := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", quoteWithCode, token)
	require.Contains(t, rejected.Body.String(), `"success":false`)
	require.Contains(t, rejected.Body.String(), "Promo codes are turned off")
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
	require.Contains(t, epayRejected.Body.String(), "Promo codes are turned off")
	usdtRejected := adtyJSON(t, f.engine, http.MethodPost, "/api/user/bepusdt/pay", `{"amount":100,"chain":"usdt.trc20","promo_code":"save10pct"}`, token)
	require.Contains(t, usdtRejected.Body.String(), "Promo codes are turned off")
	assert.Zero(t, gw.snapshot().creates)
	var orders int64
	require.NoError(t, f.db.Model(&model.TopUp{}).Where("user_id = ?", user.Id).Count(&orders).Error)
	assert.Zero(t, orders)

	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingWrite)
	turnedOn := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/settings", `{"promo_enabled":true}`, f.token)
	require.Contains(t, turnedOn.Body.String(), `"promo_enabled":true`)
	require.True(t, cfg.PromoEnabled)

	accepted := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", quoteWithCode, token)
	acceptedQuote := decodeAdtyData[struct {
		Quote struct {
			PromoBonusUSD decimal.Decimal `json:"promo_bonus_usd"`
			PayAmount     decimal.Decimal `json:"pay_amount"`
		} `json:"quote"`
	}](t, accepted.Body)
	assert.Equal(t, "10", acceptedQuote.Quote.PromoBonusUSD.String())
	assert.Equal(t, "100", acceptedQuote.Quote.PayAmount.String())

	paid := adtyJSON(t, f.engine, http.MethodPost, "/api/user/bepusdt/pay", `{"amount":100,"chain":"usdt.trc20","promo_code":"SAVE10PCT"}`, token)
	usdtView := decodeAdtyData[model.AdtyTopUpView](t, paid.Body)
	require.NotNil(t, usdtView.Ext)
	assert.Equal(t, "10", usdtView.Ext.PromoBonusUSD.String())
	assert.Equal(t, 1, gw.snapshot().creates)

	epayPaid := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/epay/pay", `{"amount":100,"method":"alipay","device":"pc","promo_code":"SAVE10PCT"}`, token)
	epayView := decodeAdtyData[model.AdtyTopUpView](t, epayPaid.Body)
	require.NotNil(t, epayView.Ext)
	assert.Equal(t, "10", epayView.Ext.PromoBonusUSD.String())
	assert.Equal(t, channel.Id, epayView.Ext.ChannelId)

	f.grant(t, authz.AdtyMarketingRead, authz.AdtyMarketingWrite)
	turnedOff := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/marketing/settings", `{"promo_enabled":false}`, f.token)
	require.Contains(t, turnedOff.Body.String(), `"promo_enabled":false`)
	again := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/topup/quote", quoteWithCode, token)
	require.Contains(t, again.Body.String(), "Promo codes are turned off")
}
