package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 字段级可见性要同时看实时授权和当前访问令牌的 scope（ISS-R4-0039）：
// 只勾 adty_site:read 的令牌不能读到客户邮箱原文和站长提现额。
func TestAdminSiteFieldVisibilityRespectsTokenScope(t *testing.T) {
	f := newAdtyGuardFixture(t)
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)

	owner := model.User{Username: "scope-owner", Password: "x", Email: "owner@example.com", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&owner).Error)
	site, err := service.OpenAdtySite(owner.Id, "scope", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	customer := model.User{Username: "scope-customer", Email: "bob@example.com", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, SiteId: site.Id, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&customer).Error)
	require.NoError(t, f.db.Model(&model.ReferralCommissionAccount{}).Where("user_id = ?", site.OwnerUserId).Update("withdrawn_amount", 12.5).Error)

	f.grant(t, authz.AdtySiteRead, authz.AdtyUserRead, authz.AdtyReferralRead)
	customersPath := fmt.Sprintf("/api/adty/admin/sites/%d/customers", site.Id)
	statsPath := fmt.Sprintf("/api/adty/admin/sites/%d/stats", site.Id)

	siteOnly := createAdtyScopedToken(t, f.admin.Id, "adty_site:read")
	masked := adtyJSON(t, f.engine, http.MethodGet, customersPath, "", siteOnly)
	require.Equal(t, http.StatusOK, masked.Code)
	assert.Contains(t, masked.Body.String(), "b***@example.com")
	assert.NotContains(t, masked.Body.String(), "bob@example.com")
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, statsPath, "", siteOnly).Body.String(), "withdrawn_usd")

	// 会话没有令牌 scope，只看实时授权。
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, customersPath, "", f.token).Body.String(), "bob@example.com")
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, statsPath, "", f.token).Body.String(), `"withdrawn_usd":"12.5"`)

	// 令牌带上对应 scope 后，字段恢复；两个 scope 互不通用。
	withUser := createAdtyScopedToken(t, f.admin.Id, "adty_site:read", "adty_user:read")
	withReferral := createAdtyScopedToken(t, f.admin.Id, "adty_site:read", "adty_referral:read")
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, customersPath, "", withUser).Body.String(), "bob@example.com")
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, customersPath, "", withReferral).Body.String(), "bob@example.com")
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, statsPath, "", withReferral).Body.String(), `"withdrawn_usd":"12.5"`)
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, statsPath, "", withUser).Body.String(), "withdrawn_usd")
}
