package router

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 分站管理接口（任务 11.1、11.9 后端）。

func withResellerSetting(t *testing.T) *adty_setting.ResellerSetting {
	t.Helper()
	cfg := adty_setting.GetResellerSetting()
	saved := *cfg
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		*cfg = saved
		common.OptionMap = oldMap
		service.InvalidateAdtySiteCache()
	})
	*cfg = adty_setting.DefaultResellerSetting()
	return cfg
}

type apiEnvelope struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

func TestResellerSettingsSaveRules(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	put := func(body string) apiEnvelope {
		t.Helper()
		resp := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/reseller/settings", body, f.root)
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		var envelope apiEnvelope
		require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
		return envelope
	}

	assert.False(t, put(`{"enabled":true,"root_domains":["sites.example.com"]}`).Success, "money settings are unset")
	saved := put(`{"enabled":true,"root_domains":["Sites.Example.com"],"cost_factor":0.8,"min_cost_factor":0.5,"max_markup":3,
		"max_site_bonus_extra":0,"trial_grant_max_usd":5,"trial_grant_daily_usd":20}`)
	require.True(t, saved.Success, saved.Message)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, []string{"sites.example.com"}, cfg.RootDomains)
	require.NotNil(t, cfg.CostFactor)
	assert.EqualValues(t, 0.8, *cfg.CostFactor)
	assert.Equal(t, true, saved.Data["money_configured"])

	assert.False(t, put(`{"cost_factor":null}`).Success, "cannot clear d while the feature is on")
	assert.False(t, put(`{"cost_factor":"abc"}`).Success, "unparseable values are rejected, not silently dropped")
	assert.False(t, put(`{"unknown_key":1}`).Success)

	off := put(`{"custom_domain_enabled":true}`)
	assert.False(t, off.Success)
	assert.Equal(t, "Fill in the Cloudflare API token, zone ID and CNAME target before turning custom domains on", off.Message)
	on := put(`{"custom_domain_enabled":true,"cloudflare_apiToken":"cf-secret","cloudflare_zone_id":"zone","cname_target":"customers.sites.example.com"}`)
	require.True(t, on.Success, on.Message)
	assert.Equal(t, "cf-secret", cfg.CloudflareApiToken)
	assert.Equal(t, "", on.Data["cloudflare_apiToken"], "the token is never returned")
	assert.Equal(t, true, on.Data["cloudflare_token_configured"])
	assert.True(t, put(`{"cloudflare_apiToken":""}`).Success)
	assert.Equal(t, "cf-secret", cfg.CloudflareApiToken, "an empty secret means unchanged")
	assert.True(t, put(`{"turnstile_widgets":[{"id":"w1","site_key":"site-key-1"},{"id":"w2","site_key":"site-key-2"}],"turnstile_widget_secret":{"w1":"widget-secret-1","w2":"widget-secret-2"}}`).Success)
	assert.True(t, put(`{"turnstile_widget_secret":{"w1":"replacement-secret"}}`).Success)
	assert.Equal(t, map[string]string{"w1": "replacement-secret", "w2": "widget-secret-2"}, cfg.TurnstileWidgetSecret, "Root can replace one widget secret without changing another")
	cleared := put(`{"custom_domain_enabled":false,"clear_secrets":["cloudflare_apiToken","turnstile_widget_secret"]}`)
	assert.True(t, cleared.Success, cleared.Message)
	assert.Empty(t, cfg.CloudflareApiToken)
	assert.Empty(t, cfg.TurnstileWidgetSecret)

	assert.False(t, put(`{"sender_domain_enabled":true}`).Success, "Resend API key is not set")
	common.OptionMap["ResendApiKey"] = "re_test"
	assert.True(t, put(`{"sender_domain_enabled":true}`).Success)

	stored, ok, err := model.ReadOptionFromDB("adty_reseller_setting.cost_factor")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "0.8", stored)

	var pendingFactors []float64
	previousNotify := service.NotifySiteOwnersGlobalCostFactorPending
	service.NotifySiteOwnersGlobalCostFactorPending = func(factor decimal.Decimal, _ int64) {
		value, _ := factor.Float64()
		pendingFactors = append(pendingFactors, value)
	}
	t.Cleanup(func() { service.NotifySiteOwnersGlobalCostFactorPending = previousNotify })
	raised := put(`{"cost_factor":0.9}`)
	require.True(t, raised.Success, raised.Message)
	require.NotEmpty(t, pendingFactors, "a higher global d is delayed and site owners are notified")
	assert.InDelta(t, 0.9, pendingFactors[0], 0.000001)
	assert.EqualValues(t, 0.8, *cfg.CostFactor)

	generic := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_reseller_setting.enabled","value":"false"}`, f.root)
	assert.Contains(t, generic.Body.String(), `"success":false`, "the generic option API cannot bypass the checks")
	assert.True(t, cfg.Enabled)
	money := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_referral_setting.default_rate","value":"99"}`, f.root)
	assert.Contains(t, money.Body.String(), "Referral settings can only be changed on the referral settings page")
	secret := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_marketing_setting.webhook_secret","value":"whsec_bypass"}`, f.root)
	assert.Contains(t, secret.Body.String(), "Marketing secrets can only be changed on the marketing secrets page")
	enabled := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_marketing_setting.enabled","value":"true"}`, f.root)
	assert.Contains(t, enabled.Body.String(), `"success":false`)
	assert.False(t, adty_setting.GetMarketingSetting().Enabled)
	referral := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", `{"key":"adty_referral_setting.enabled","value":"true"}`, f.root)
	assert.Contains(t, referral.Body.String(), `"success":false`)
	assert.False(t, adty_setting.GetReferralSetting().Enabled, "the generic option API cannot turn referral on without the default rate")
}

func TestAdminSiteCustomersMaskEmailWithoutUserRead(t *testing.T) {
	f := newAdtyGuardFixture(t)
	customer := model.User{Username: "mask-customer", Email: "alice@example.com", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, SiteId: 7, Group: "default"}
	require.NoError(t, f.db.Create(&customer).Error)
	path := "/api/adty/admin/sites/7/customers"
	f.grant(t, authz.AdtySiteRead)
	masked := adtyJSON(t, f.engine, http.MethodGet, path, "", f.token)
	assert.Contains(t, masked.Body.String(), "a***@example.com")
	assert.NotContains(t, masked.Body.String(), "alice@example.com")
	f.grant(t, authz.AdtySiteRead, authz.AdtyUserRead)
	full := adtyJSON(t, f.engine, http.MethodGet, path, "", f.token)
	assert.Contains(t, full.Body.String(), "alice@example.com")
}

func TestSiteAdminPermissions(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)
	adtySessionToken(t, f.db, "future-owner", common.RoleCommonUser)
	var owner model.User
	require.NoError(t, f.db.Where("username = ?", "future-owner").First(&owner).Error)
	create := fmt.Sprintf(`{"user_id":%d,"subdomain":"perm-site"}`, owner.Id)

	f.grant(t, authz.AdtySiteRead)
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites", "", f.token).Code)
	for _, call := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/adty/admin/sites", create},
		{http.MethodPut, "/api/adty/admin/sites/1", `{"cost_factor":"0.7"}`},
		{http.MethodPut, "/api/adty/admin/sites/1/status", `{"status":"paused"}`},
		{http.MethodGet, "/api/adty/admin/reseller/settings", ``},
		{http.MethodPut, "/api/adty/admin/reseller/settings", `{}`},
		{http.MethodPost, "/api/adty/admin/reseller/invite-codes", `{}`},
	} {
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, call.method, call.path, call.body, f.token).Code, call.method+" "+call.path)
	}

	f.grant(t, authz.AdtySiteRead, authz.AdtySiteReview)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/sites", create, f.token).Code)

	f.grant(t, authz.AdtySiteRead, authz.AdtySiteAdjust)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/sites", create, f.token).Code, "adjust cannot replace create")
	f.grant(t, authz.AdtySiteRead, authz.AdtySiteCreate)
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/sites", create, f.token)
	require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
	var envelope apiEnvelope
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
	siteID := int(envelope.Data["id"].(float64))
	adjust := fmt.Sprintf("/api/adty/admin/sites/%d", siteID)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPut, adjust, `{"cost_factor":"0.7"}`, f.token).Code, "create cannot replace adjust")
	f.grant(t, authz.AdtySiteRead, authz.AdtySiteAdjust)
	low := adtyJSON(t, f.engine, http.MethodPut, adjust, `{"cost_factor":"0.4"}`, f.token)
	assert.Contains(t, low.Body.String(), "The cost factor cannot be below the minimum cost factor")
	ok := adtyJSON(t, f.engine, http.MethodPut, adjust, `{"cost_factor":"0.7","reason":"volume"}`, f.token)
	assert.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
	closeNoTarget := adtyJSON(t, f.engine, http.MethodPut, adjust+"/status", `{"status":"closed"}`, f.token)
	assert.Contains(t, closeNoTarget.Body.String(), "Choose where the customers go before closing the site")

	detail := adtyJSON(t, f.engine, http.MethodGet, adjust, "", f.token)
	assert.Contains(t, detail.Body.String(), `"action":"adjust"`)
	assert.True(t, strings.Contains(detail.Body.String(), `"owner_username":"future-owner"`), detail.Body.String())

	f.grant(t, authz.AdtySiteSettings)
	settings := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/reseller/settings", `{"enabled":false}`, f.token)
	assert.Equal(t, http.StatusOK, settings.Code, settings.Body.String())
	assert.Contains(t, settings.Body.String(), `"success":true`)
	code := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/reseller/invite-codes", `{"max_uses":2,"cost_factor":"0.6"}`, f.token)
	assert.Equal(t, http.StatusForbidden, code.Code, "settings write cannot replace invite creation")
	f.grant(t, authz.AdtySiteSettingsCreate)
	code = adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/reseller/invite-codes", `{"max_uses":2,"cost_factor":"0.6"}`, f.token)
	assert.Contains(t, code.Body.String(), `"used_count":0`, code.Body.String())
}

func TestResellerSettingsDelegation(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)
	cfg.CloudflareApiToken = "fixture-cf-token"
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "w1", SiteKey: "fixture-site-key"}}
	cfg.TurnstileWidgetSecret = map[string]string{"w1": "fixture-widget-secret"}
	path := "/api/adty/admin/reseller/settings"
	regular := adtySessionToken(t, f.db, "settings-user", common.RoleCommonUser)
	ownerToken := adtySessionToken(t, f.db, "settings-owner", common.RoleCommonUser)
	var owner model.User
	require.NoError(t, f.db.Where("username = ?", "settings-owner").First(&owner).Error)
	_, err := service.OpenAdtySite(owner.Id, "settings-owner", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)

	t.Run("six_identities", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			token       string
			permissions []authz.Permission
			status      int
		}{
			{name: "anonymous", status: http.StatusUnauthorized},
			{name: "regular_user", token: regular, status: http.StatusForbidden},
			{name: "site_owner", token: ownerToken, status: http.StatusForbidden},
			{name: "read_only_admin", token: f.token, permissions: []authz.Permission{authz.AdtySiteRead}, status: http.StatusForbidden},
			{name: "admin_without_module_grant", token: f.token, status: http.StatusForbidden},
			{name: "root", token: f.root, status: http.StatusOK},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f.grant(t, tc.permissions...)
				for _, method := range []string{http.MethodGet, http.MethodPut} {
					resp := adtyJSON(t, f.engine, method, path, `{"freeze_days":8}`, tc.token)
					assert.Equal(t, tc.status, resp.Code, method+" "+resp.Body.String())
					if tc.status == http.StatusOK {
						assert.Contains(t, resp.Body.String(), `"success":true`)
						assert.NotContains(t, resp.Body.String(), "fixture-cf-token")
						assert.NotContains(t, resp.Body.String(), "fixture-widget-secret")
					}
				}
			})
		}
	})

	t.Run("granted_admin_and_revocation", func(t *testing.T) {
		f.grant(t, authz.AdtySiteSettingsRead, authz.AdtySiteSettings)
		view := adtyJSON(t, f.engine, http.MethodGet, path, "", f.token)
		assert.Equal(t, http.StatusOK, view.Code, view.Body.String())
		assert.Contains(t, view.Body.String(), `"cloudflare_token_configured":true`)
		assert.NotContains(t, view.Body.String(), "fixture-cf-token")
		assert.NotContains(t, view.Body.String(), "fixture-widget-secret")
		save := adtyJSON(t, f.engine, http.MethodPut, path, `{"freeze_days":9}`, f.token)
		assert.Equal(t, http.StatusOK, save.Code, save.Body.String())
		assert.Contains(t, save.Body.String(), `"success":true`)
		assert.Equal(t, 9, cfg.FreezeDays)
		stored, ok, err := model.ReadOptionFromDB("adty_reseller_setting.freeze_days")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "9", stored)
		assert.Equal(t, "fixture-cf-token", cfg.CloudflareApiToken)
		assert.Equal(t, map[string]string{"w1": "fixture-widget-secret"}, cfg.TurnstileWidgetSecret)
		f.grant(t)
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			resp := adtyJSON(t, f.engine, method, path, `{"freeze_days":10}`, f.token)
			assert.Equal(t, http.StatusForbidden, resp.Code, method+" "+resp.Body.String())
		}
		assert.Equal(t, 9, cfg.FreezeDays)
	})

	t.Run("secret_keys_reject_the_whole_request", func(t *testing.T) {
		f.grant(t, authz.AdtySiteSettings)
		before, err := common.Marshal(cfg)
		require.NoError(t, err)
		var optionsBefore []model.Option
		require.NoError(t, f.db.Order("key").Find(&optionsBefore).Error)
		originalSecrets := cfg.TurnstileWidgetSecret
		notifications := 0
		previousNotify := service.NotifySiteOwnersGlobalCostFactorPending
		service.NotifySiteOwnersGlobalCostFactorPending = func(decimal.Decimal, int64) { notifications++ }
		t.Cleanup(func() { service.NotifySiteOwnersGlobalCostFactorPending = previousNotify })
		for _, tc := range []struct{ name, secret string }{
			{"token", `"cloudflare_apiToken":"replacement"`},
			{"token_empty", `"cloudflare_apiToken":""`},
			{"token_null", `"cloudflare_apiToken":null`},
			{"widget", `"turnstile_widget_secret":{"w1":"replacement","w2":"new"}`},
			{"widget_empty", `"turnstile_widget_secret":{}`},
			{"widget_null", `"turnstile_widget_secret":null`},
			{"clear_token", `"clear_secrets":["cloudflare_apiToken"]`},
			{"clear_widgets", `"clear_secrets":["turnstile_widget_secret"]`},
			{"clear_empty", `"clear_secrets":[]`},
			{"clear_null", `"clear_secrets":null`},
			{"mixed_secret_and_clear", `"cloudflare_apiToken":"replacement","clear_secrets":["turnstile_widget_secret"]`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := `{"freeze_days":99,"cost_factor":0.9,"min_cost_factor":0.6,` + tc.secret + `}`
				resp := adtyJSON(t, f.engine, http.MethodPut, path, body, f.token)
				assert.Equal(t, http.StatusForbidden, resp.Code, resp.Body.String())
				after, err := common.Marshal(cfg)
				require.NoError(t, err)
				assert.JSONEq(t, string(before), string(after), "rejected requests leave all current settings unchanged")
				assert.Equal(t, map[string]string{"w1": "fixture-widget-secret"}, originalSecrets, "the original secret map must not be changed through a shallow copy")
				var optionsAfter []model.Option
				require.NoError(t, f.db.Order("key").Find(&optionsAfter).Error)
				assert.Equal(t, optionsBefore, optionsAfter, "ordinary and secret fields must not be saved from a rejected mixed request")
				assert.Zero(t, notifications, "secret authorization must happen before price-change notifications")
			})
		}
	})

	t.Run("pat_scope_and_live_grant", func(t *testing.T) {
		require.NoError(t, model.EnsureLegacyAccessTokenRetireAt(time.Now().Unix()))
		f.grant(t, authz.AdtySiteSettingsRead, authz.AdtySiteSettings)
		settingsPAT := createAdtyScopedToken(t, f.admin.Id, service.AccessTokenScopeOf(authz.AdtySiteSettingsRead), service.AccessTokenScopeOf(authz.AdtySiteSettings))
		wrongScope := createAdtyScopedToken(t, f.admin.Id, service.AccessTokenScopeOf(authz.AdtySiteRead))
		adminOptionPAT := createAdtyScopedToken(t, f.admin.Id, "option:read", "option:write")
		var root model.User
		require.NoError(t, f.db.Where("username = ?", "guard-root").First(&root).Error)
		rootReadPAT := createAdtyScopedToken(t, root.Id, "option:read")
		rootWritePAT := createAdtyScopedToken(t, root.Id, "option:write")
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", rootReadPAT).Code, "new tokens cannot borrow the historical option alias")
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPut, path, `{"freeze_days":11}`, rootWritePAT).Code)
		require.NoError(t, f.db.Model(&model.UserAccessToken{}).Where("token_hash IN ?", []string{model.AccessTokenFingerprint(rootReadPAT), model.AccessTokenFingerprint(rootWritePAT)}).Update("scope_version", 0).Error)
		for _, tc := range []struct {
			name, token, method string
			status              int
		}{
			{"admin_get", settingsPAT, http.MethodGet, http.StatusOK},
			{"admin_put", settingsPAT, http.MethodPut, http.StatusOK},
			{"wrong_scope_get", wrongScope, http.MethodGet, http.StatusForbidden},
			{"wrong_scope_put", wrongScope, http.MethodPut, http.StatusForbidden},
			{"admin_option_get", adminOptionPAT, http.MethodGet, http.StatusForbidden},
			{"admin_option_put", adminOptionPAT, http.MethodPut, http.StatusForbidden},
			{"root_old_read", rootReadPAT, http.MethodGet, http.StatusOK},
			{"root_old_write", rootWritePAT, http.MethodPut, http.StatusOK},
			{"root_old_read_cannot_write", rootReadPAT, http.MethodPut, http.StatusForbidden},
			{"root_old_write_cannot_read", rootWritePAT, http.MethodGet, http.StatusForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resp := adtyJSON(t, f.engine, tc.method, path, `{"freeze_days":11}`, tc.token)
				assert.Equal(t, tc.status, resp.Code, resp.Body.String())
				if tc.status == http.StatusOK {
					assert.Contains(t, resp.Body.String(), `"success":true`)
				}
			})
		}
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/reseller/invite-codes", "", rootReadPAT).Code)
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/reseller/invite-codes", `{"max_uses":1}`, rootWritePAT).Code)
		assert.Equal(t, http.StatusNotFound, adtyJSON(t, f.engine, http.MethodPost, path, `{}`, settingsPAT).Code)
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/option/", "", settingsPAT).Code)
		f.grant(t)
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, method, path, `{"freeze_days":12}`, settingsPAT).Code)
		}
	})
}

// 分站管理的经营数据、排序、风控复核和举报（任务 11.9 后端）。
func TestSiteAdminStatsSortAndReports(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)
	open := func(name string) *model.AdtySite {
		owner := model.User{Username: name + "-owner", Password: "x", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
		require.NoError(t, f.db.Create(&owner).Error)
		site, err := service.OpenAdtySite(owner.Id, name, model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
		require.NoError(t, err)
		return site
	}
	small, big := open("small"), open("big")
	customer := model.User{Username: "flagged", Password: "x", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, SiteId: small.Id}
	require.NoError(t, f.db.Create(&customer).Error)
	require.NoError(t, f.db.Create(&model.AdtySiteCustomer{UserId: customer.Id, SiteId: small.Id, RiskFlag: model.AdtySiteRiskSelfUse, RiskReason: "same IP", JoinedAt: time.Now().Unix()}).Error)
	today := time.Now().Format("2006-01-02")
	row := func(site *model.AdtySite, kind string, revenue, platform, earning int64) model.AdtySiteEarningDaily {
		return model.AdtySiteEarningDaily{SiteId: site.Id, UserId: customer.Id, Day: today, Kind: kind, Status: model.AdtySiteEarningPending,
			Revenue: decimal.NewFromInt(revenue), Platform: decimal.NewFromInt(platform), Earning: decimal.NewFromInt(earning)}
	}
	require.NoError(t, f.db.Create(&[]model.AdtySiteEarningDaily{
		row(small, model.AdtySiteEarningUsage, 10, 8, 2),
		row(small, model.AdtySiteEarningSkipped, 5, 4, 1),
		row(big, model.AdtySiteEarningUsage, 100, 80, 20),
	}).Error)
	now := time.Now().Unix()
	require.NoError(t, f.db.Create(&[]model.AdtySiteDomain{
		{SiteId: small.Id, Hostname: "a.small.example", Status: model.AdtySiteDomainActive, CreatedAt: now},
		{SiteId: small.Id, Hostname: "b.small.example", Status: model.AdtySiteDomainPending, CreatedAt: now},
		{SiteId: small.Id, Hostname: "gone.small.example", Status: model.AdtySiteDomainRemoved, CreatedAt: now},
		{SiteId: small.Id, Hostname: "bad.small.example", Status: model.AdtySiteDomainFailed, CreatedAt: now},
	}).Error)
	require.NoError(t, f.db.Model(small).Update("earning_hook_failures", 3).Error)
	require.NoError(t, f.db.Model(big).Update("settle_hold_reason", "usage mismatch").Error)
	require.NoError(t, f.db.Model(&model.ReferralCommissionAccount{}).Where("user_id = ?", small.OwnerUserId).Update("withdrawn_amount", 12.5).Error)

	f.grant(t, authz.AdtySiteRead)
	list := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites?sort=revenue", "", f.token)
	require.Equal(t, http.StatusOK, list.Code)
	body := list.Body.String()
	assert.Less(t, strings.Index(body, `"subdomain":"big"`), strings.Index(body, `"subdomain":"small"`), "highest revenue first")
	assert.Contains(t, body, `"month_revenue_usd":"100"`)
	assert.Contains(t, body, `"effective_cost_factor":"0.8"`)
	asc := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites?sort=revenue&order=asc", "", f.token).Body.String()
	assert.Less(t, strings.Index(asc, `"subdomain":"small"`), strings.Index(asc, `"subdomain":"big"`))
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites?sort=bogus", "", f.token).Body.String(), `"success":false`)

	// overview 是日汇总的直接合计：消费 115，平台 92，站长 22（skipped 的 1 单列）。
	var overview struct {
		Data service.AdtySitesOverview `json:"data"`
	}
	require.NoError(t, common.Unmarshal(adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites/overview", "", f.token).Body.Bytes(), &overview))
	assert.True(t, overview.Data.SiteRevenueUSD.Equal(decimal.NewFromInt(115)), "got %s", overview.Data.SiteRevenueUSD)
	assert.True(t, overview.Data.SitePlatformUSD.Equal(decimal.NewFromInt(92)))
	assert.True(t, overview.Data.SiteEarningUSD.Equal(decimal.NewFromInt(22)))
	assert.True(t, overview.Data.SiteSkippedUSD.Equal(decimal.NewFromInt(1)))
	assert.EqualValues(t, 1, overview.Data.FlaggedCustomers)
	assert.Equal(t, 2, overview.Data.Sites[model.AdtySiteStatusActive])
	assert.EqualValues(t, 2, overview.Data.CustomHostnames)
	assert.True(t, overview.Data.HostnameMonthlyFeeUSD.IsZero(), "got %s", overview.Data.HostnameMonthlyFeeUSD)
	assert.EqualValues(t, 3, overview.Data.EarningHookFailures)
	require.Len(t, overview.Data.SettlementHolds, 1)
	assert.Equal(t, big.Id, overview.Data.SettlementHolds[0].SiteId)
	assert.Equal(t, "usage mismatch", overview.Data.SettlementHolds[0].SettleHoldReason)

	stats := adtyJSON(t, f.engine, http.MethodGet, fmt.Sprintf("/api/adty/admin/sites/%d/stats", small.Id), "", f.token).Body.String()
	assert.Contains(t, stats, `"revenue_usd":"15"`)
	assert.Contains(t, stats, `"breakdown"`)
	assert.NotContains(t, stats, "withdrawn_usd")
	f.grant(t, authz.AdtySiteRead, authz.AdtyReferralRead)
	statsWithWithdrawal := adtyJSON(t, f.engine, http.MethodGet, fmt.Sprintf("/api/adty/admin/sites/%d/stats", small.Id), "", f.token).Body.String()
	assert.Contains(t, statsWithWithdrawal, `"withdrawn_usd":"12.5"`)
	assert.Contains(t, stats, `"monthly"`)
	flagged := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites/flagged-customers", "", f.token).Body.String()
	assert.Contains(t, flagged, `"risk_reason":"same IP"`, "admins see the reason")
	assert.Contains(t, flagged, `"skipped_usd":"1.00"`)

	// 只有查看权限时，复核和处理举报都是 403。
	report := model.AdtySiteReport{SiteId: small.Id, UserId: customer.Id, ReporterId: 1, Reason: "abuse", Status: model.AdtySiteReportPending}
	require.NoError(t, f.db.Create(&report).Error)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/sites/reports", "", f.token).Body.String(), `"reason":"abuse"`)
	handlePath := fmt.Sprintf("/api/adty/admin/sites/reports/%d/handle", report.Id)
	reviewPath := fmt.Sprintf("/api/adty/admin/sites/customers/%d/review", customer.Id)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, handlePath, `{"status":"resolved"}`, f.token).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, reviewPath, `{"convert_skipped":true}`, f.token).Code)

	f.grant(t, authz.AdtySiteRead, authz.AdtySiteReview)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPost, handlePath, `{"status":"bogus"}`, f.token).Body.String(), `"success":false`)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPost, handlePath, `{"status":"resolved","note":"warned"}`, f.token).Body.String(), `"success":true`)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPost, handlePath, `{"status":"dismissed"}`, f.token).Body.String(), "already been handled")
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPost, reviewPath, `{"convert_skipped":true}`, f.token).Body.String(), `"success":true`)
	var reviewed model.AdtySiteCustomer
	require.NoError(t, f.db.First(&reviewed, customer.Id).Error)
	assert.True(t, reviewed.Reviewed)
	var skipped int64
	require.NoError(t, f.db.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ? AND kind = ?", small.Id, model.AdtySiteEarningSkipped).Count(&skipped).Error)
	assert.Zero(t, skipped, "pending skipped rows became usage")
}

// 一键下线公告（任务 11.3、11.9）：要分站·调整权限；只清空公告，其余品牌字段不动；写审计。
func TestSiteNoticeTakedown(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)
	owner := model.User{Username: "notice-owner", Password: "x", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&owner).Error)
	site, err := service.OpenAdtySite(owner.Id, "notice", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", site.Id).
		Update("brand", `{"name":"Notice AI","logo_url":"/uploads/site-assets/site-1-a.png","notice":"Buy cheap tokens at evil.example"}`).Error)
	path := fmt.Sprintf("/api/adty/admin/sites/%d/notice", site.Id)

	f.grant(t, authz.AdtySiteRead)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodDelete, path, "", f.token).Code)

	f.grant(t, authz.AdtySiteRead, authz.AdtySiteAdjust)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodDelete, path, `{"reason":"spam"}`, f.token).Code, "adjust cannot replace delete")
	f.grant(t, authz.AdtySiteRead, authz.AdtySiteDelete)
	resp := adtyJSON(t, f.engine, http.MethodDelete, path, `{"reason":"spam"}`, f.token)
	require.Contains(t, resp.Body.String(), `"cleared":true`, resp.Body.String())
	stored, err := service.GetAdtySite(site.Id)
	require.NoError(t, err)
	brand := service.AdtyBrandFor(stored)
	assert.Empty(t, brand.Notice)
	assert.Equal(t, "Notice AI", brand.Name)
	assert.Equal(t, "/uploads/site-assets/site-1-a.png", brand.LogoURL)

	var audit model.AdtySiteAudit
	require.NoError(t, f.db.Where("site_id = ? AND action = ?", site.Id, "notice_takedown").First(&audit).Error)
	assert.Contains(t, audit.OldValue, "Buy cheap tokens")
	assert.Equal(t, "spam", audit.Reason)
	assert.Equal(t, f.admin.Id, audit.OperatorId)

	again := adtyJSON(t, f.engine, http.MethodDelete, path, "", f.token)
	assert.Contains(t, again.Body.String(), `"cleared":false`, "nothing left to take down")
}

// 名额用量（任务 11.3、11.8、11.9）：发件域名按状态计数；widget 已登记数含第一个 widget 里的平台域名；剩余少于 5 个时提醒。
func TestResellerUsage(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	cfg.RootDomains = []string{"sites.example.com"}
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "w1", SiteKey: "0xA"}, {Id: "w2", SiteKey: "0xB"}}
	oldAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://main.example.com"
	t.Cleanup(func() { system_setting.ServerAddress = oldAddress })
	now := time.Now().Unix()
	require.NoError(t, f.db.Create(&[]model.AdtySiteSenderDomain{
		{SiteId: 1, Domain: "a.example", Status: model.AdtySenderDomainVerified, CreatedAt: now},
		{SiteId: 2, Domain: "b.example", Status: model.AdtySenderDomainPending, CreatedAt: now},
		{SiteId: 3, Domain: "c.example", Status: model.AdtySenderDomainFailed, CreatedAt: now},
		{SiteId: 4, Domain: "d.example", Status: model.AdtySenderDomainDisabled, CreatedAt: now},
	}).Error)
	domains := []model.AdtySiteDomain{}
	for i := range 8 {
		domains = append(domains, model.AdtySiteDomain{SiteId: 1, Hostname: fmt.Sprintf("w1-%d.example", i), Status: model.AdtySiteDomainActive, TurnstileWidget: "w1", CreatedAt: now})
	}
	for i := range 7 {
		domains = append(domains, model.AdtySiteDomain{SiteId: 1, Hostname: fmt.Sprintf("w2-%d.example", i), Status: model.AdtySiteDomainActive, TurnstileWidget: "w2", CreatedAt: now})
	}
	domains = append(domains,
		model.AdtySiteDomain{SiteId: 1, Hostname: "pending.example", Status: "pending", TurnstileWidget: "w2", CreatedAt: now},
		model.AdtySiteDomain{SiteId: 1, Hostname: "nowidget.example", Status: model.AdtySiteDomainActive, CreatedAt: now},
	)
	require.NoError(t, f.db.Create(&domains).Error)

	f.grant(t)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/reseller/usage", "", f.token).Code)
	f.grant(t, authz.AdtySiteRead)
	var envelope struct {
		Data service.AdtyResellerUsage `json:"data"`
	}
	resp := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/reseller/usage", "", f.token)
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
	usage := envelope.Data
	assert.EqualValues(t, 3, usage.Sender.InUse, "pending, verified and failed all hold a Resend slot")
	assert.EqualValues(t, 1, usage.Sender.Verified)
	assert.EqualValues(t, 1, usage.Sender.Failed)
	assert.EqualValues(t, 1, usage.Sender.Disabled)
	require.Len(t, usage.Widgets, 2)
	assert.Equal(t, 10, usage.Widgets[0].Used, "8 custom domains plus the main host and one root domain")
	assert.Equal(t, 7, usage.Widgets[1].Used, "pending domains are not registered yet")
	assert.Equal(t, 10, usage.Widgets[1].Limit)
	assert.Equal(t, 3, usage.WidgetSlotsLeft)
	assert.True(t, usage.WidgetSlotsLow, "fewer than 5 slots left")
	assert.EqualValues(t, 1, usage.UnassignedDomains)
}
