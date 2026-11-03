package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 推广设置的保存接口：和钱有关的项没有内置值，可以清回「未设置」；没有默认比例时不能打开（约定 I）。
func TestReferralSettingsRequireRateToEnable(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := adty_setting.GetReferralSetting()
	saved := *cfg
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		*cfg = saved
		common.OptionMap = oldMap
	})
	cfg.Enabled, cfg.DefaultRate = false, nil

	put := func(body string) (bool, string) {
		t.Helper()
		resp := adtyJSON(t, f.engine, http.MethodPut, "/api/user/admin/referral/settings", body, f.root)
		require.Equal(t, http.StatusOK, resp.Code)
		var envelope struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
		return envelope.Success, envelope.Message
	}

	ok, message := put(`{"enabled":true}`)
	assert.False(t, ok)
	assert.Equal(t, "Set the default commission rate before turning the referral program on", message)

	ok, _ = put(`{"enabled":true,"default_rate":10,"min_withdraw_usd":20}`)
	require.True(t, ok)
	require.NotNil(t, cfg.DefaultRate)
	assert.EqualValues(t, 10, *cfg.DefaultRate)
	assert.True(t, cfg.Enabled)

	ok, _ = put(`{"clear":["default_rate"]}`)
	assert.False(t, ok, "the rate cannot be cleared while the program is on")

	ok, _ = put(`{"enabled":false,"clear":["default_rate","min_withdraw_usd"]}`)
	require.True(t, ok)
	assert.Nil(t, cfg.DefaultRate)
	assert.Nil(t, cfg.MinWithdrawUSD)
	assert.False(t, cfg.Enabled)
}

// 汇率不合法时，同一次提交里的推广设置不能先落库。合法时设置、汇率和审计一起提交。
func TestReferralSettingsAndRateCommitTogether(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := adty_setting.GetReferralSetting()
	saved := *cfg
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		*cfg = saved
		common.OptionMap = oldMap
	})
	ten := 10.0
	twenty := 20.0
	cfg.Enabled, cfg.DefaultRate, cfg.MinWithdrawUSD = true, &ten, &twenty
	cfg.WithdrawCNYRate = ""

	put := func(body string) (bool, string) {
		t.Helper()
		resp := adtyJSON(t, f.engine, http.MethodPut, "/api/user/admin/referral/settings", body, f.root)
		require.Equal(t, http.StatusOK, resp.Code)
		var envelope struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
		return envelope.Success, envelope.Message
	}

	ok, message := put(`{"default_rate":12.5,"min_withdraw_usd":99,"withdraw_cny_rate":"-1"}`)
	assert.False(t, ok)
	assert.Equal(t, "The exchange rate must be greater than 0", message)
	require.NotNil(t, cfg.DefaultRate)
	assert.EqualValues(t, 10, *cfg.DefaultRate)
	require.NotNil(t, cfg.MinWithdrawUSD)
	assert.EqualValues(t, 20, *cfg.MinWithdrawUSD)
	assert.Empty(t, cfg.WithdrawCNYRate)
	var storedRate int64
	require.NoError(t, f.db.Model(&model.Option{}).Where("key = ? AND value = ?", "adty_referral_setting.default_rate", "12.5").Count(&storedRate).Error)
	assert.Zero(t, storedRate)

	ok, message = put(`{"default_rate":11,"withdraw_cny_rate":"abc"}`)
	assert.False(t, ok)
	assert.Equal(t, "The exchange rate must be a number", message)
	assert.EqualValues(t, 10, *cfg.DefaultRate)

	ok, _ = put(`{"default_rate":11,"min_withdraw_usd":25,"withdraw_cny_rate":"7.25"}`)
	require.True(t, ok)
	require.NotNil(t, cfg.DefaultRate)
	assert.EqualValues(t, 11, *cfg.DefaultRate)
	require.NotNil(t, cfg.MinWithdrawUSD)
	assert.EqualValues(t, 25, *cfg.MinWithdrawUSD)
	assert.Equal(t, "7.25", cfg.WithdrawCNYRate)
	var option model.Option
	require.NoError(t, f.db.Where("key = ?", adty_setting.ReferralKeyWithdrawCNYRate).First(&option).Error)
	assert.Equal(t, "7.25", option.Value)
	var audits int64
	require.NoError(t, f.db.Model(&model.ReferralAdminAuditLog{}).Where("action = ?", "withdraw_cny_rate").Count(&audits).Error)
	assert.EqualValues(t, 1, audits)

	ok, _ = put(`{"cookie_ttl_days":14,"withdraw_cny_rate":"7.25"}`)
	require.True(t, ok)
	assert.Equal(t, 14, cfg.CookieTTLDays)
	require.NoError(t, f.db.Model(&model.ReferralAdminAuditLog{}).Where("action = ?", "withdraw_cny_rate").Count(&audits).Error)
	assert.EqualValues(t, 1, audits, "an unchanged rate does not write another audit row")
}

// 打开推广时以数据库里的默认比例为准，不看本进程里还没同步掉的旧值。
func TestReferralEnableUsesDatabaseRate(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := adty_setting.GetReferralSetting()
	saved := *cfg
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		*cfg = saved
		common.OptionMap = oldMap
	})
	require.NoError(t, model.UpdateOption("adty_referral_setting.default_rate", "null"))
	require.NoError(t, model.UpdateOption("adty_referral_setting.enabled", "false"))
	stale := 10.0
	cfg.DefaultRate = &stale
	cfg.Enabled = false

	resp := adtyJSON(t, f.engine, http.MethodPut, "/api/user/admin/referral/settings", `{"enabled":true}`, f.root)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "Set the default commission rate before turning the referral program on")
	assert.False(t, cfg.Enabled)
	var enabled model.Option
	require.NoError(t, f.db.Where("key = ?", "adty_referral_setting.enabled").First(&enabled).Error)
	assert.Equal(t, "false", enabled.Value)

	require.NoError(t, model.UpdateOption("adty_referral_setting.default_rate", "10"))
	cfg.DefaultRate = nil
	cfg.Enabled = false
	resp = adtyJSON(t, f.engine, http.MethodPut, "/api/user/admin/referral/settings", `{"enabled":true}`, f.root)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"success":true`)
	assert.True(t, cfg.Enabled)
	require.NotNil(t, cfg.DefaultRate)
	assert.EqualValues(t, 10, *cfg.DefaultRate)
}

func TestRestoreReferralClearsSiteOwnerSince(t *testing.T) {
	f := newAdtyGuardFixture(t)
	adtySessionToken(t, f.db, "site-owner", common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "site-owner").First(&user).Error)
	affiliate, err := service.Referral.EnsureAffiliate(user.Id)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(affiliate).Update("site_owner_since", 1_700_000_000).Error)
	require.NoError(t, f.db.Create(&model.AdtySite{OwnerUserId: user.Id, Subdomain: "still-open", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive, OpenedBy: model.AdtySiteOpenedByAdmin}).Error)

	path := fmt.Sprintf("/api/user/admin/referral/affiliates/%d/update", user.Id)
	body := `{"action":"restore_referral","reason":"site closed"}`
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, path, body, f.token).Code, "needs referral adjust")
	f.grant(t, authz.AdtyReferralAdjust)
	blocked := adtyJSON(t, f.engine, http.MethodPost, path, body, f.token)
	assert.Contains(t, blocked.Body.String(), "Close the site before restoring referral commissions")
	var stillOwner model.ReferralAffiliate
	require.NoError(t, f.db.First(&stillOwner, affiliate.Id).Error)
	assert.NotZero(t, stillOwner.SiteOwnerSince)
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("owner_user_id = ?", user.Id).Update("status", model.AdtySiteStatusClosed).Error)
	resp := adtyJSON(t, f.engine, http.MethodPost, path, body, f.token)
	require.Equal(t, http.StatusOK, resp.Code)
	decodeAdtyData[any](t, resp.Body)

	var stored model.ReferralAffiliate
	require.NoError(t, f.db.First(&stored, affiliate.Id).Error)
	assert.Zero(t, stored.SiteOwnerSince)
}
