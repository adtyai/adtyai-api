package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteBrandSettingsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("database DSN not configured")
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			require.NoError(t, db.AutoMigrate(&model.AdtySite{}, &model.AdtySitePrice{}, &model.AdtySiteDomain{}))
			owner := model.User{Username: "brand-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&owner).Error)
			site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "brand", RootDomain: "sites.example.test", Status: model.AdtySiteStatusActive, AgreementVer: service.AdtyCurrentSiteAgreementVersion(), Brand: `{"name":"Old name","logo_url":"/uploads/site-assets/old.png"}`}
			require.NoError(t, db.Create(&site).Error)
			other := model.AdtySite{OwnerUserId: owner.Id + 1000, Subdomain: "other-brand", RootDomain: site.RootDomain, Status: model.AdtySiteStatusActive, Brand: `{"name":"Other site"}`}
			require.NoError(t, db.Create(&other).Error)
			t.Cleanup(service.InvalidateAdtySiteCache)
			request := func(handler gin.HandlerFunc, method, body string, userID int, scopedSite *model.AdtySite) (int, map[string]any) {
				t.Helper()
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(method, "/api/adty/site/self", strings.NewReader(body))
				c.Set("id", userID)
				if scopedSite != nil {
					c.Set(service.AdtySiteContextKey, scopedSite)
				}
				handler(c)
				var envelope map[string]any
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope), recorder.Body.String())
				return recorder.Code, envelope
			}
			_, legacy := request(GetAdtyOwnerSite, http.MethodGet, "", owner.Id, nil)
			require.Equal(t, true, legacy["success"])
			brand := legacy["data"].(map[string]any)["brand"].(map[string]any)
			assert.Equal(t, "Old name", brand["name"])
			_, saved := request(PutAdtyOwnerSite, http.MethodPut, `{"site_id":`+fmt.Sprint(other.Id)+`,"brand":{"name":"Brand AI","title":"  <Desk> AI  ","logo_url":"/uploads/site-assets/logo.png","favicon_url":"/uploads/site-assets/icon.webp","theme_color":"#225588"}}`, owner.Id, nil)
			require.Equal(t, true, saved["success"], saved)
			brand = saved["data"].(map[string]any)["brand"].(map[string]any)
			assert.Equal(t, "<Desk> AI", brand["title"])
			assert.Equal(t, "/uploads/site-assets/icon.webp", brand["favicon_url"])
			assert.Equal(t, "#225588", brand["theme_color"])
			require.NoError(t, db.First(&site, site.Id).Error)
			_, public := request(GetStatus, http.MethodGet, "", 0, &site)
			status := public["data"].(map[string]any)
			assert.Equal(t, "<Desk> AI", status["site_title"])
			assert.Equal(t, "/uploads/site-assets/icon.webp", status["favicon"])
			assert.Equal(t, "/uploads/site-assets/logo.png", status["logo"])
			assert.Equal(t, "#225588", status["theme_color"])
			_, publicOther := request(GetStatus, http.MethodGet, "", 0, &other)
			assert.NotEqual(t, "#225588", publicOther["data"].(map[string]any)["theme_color"])
			for _, invalid := range []string{
				`{"theme_color":"red; background:url(https://example.test)"}`, `{"theme_color":"#12345g"}`,
				`{"favicon_url":"https://example.test/icon.png"}`, `{"favicon_url":"javascript:alert(1)"}`,
				`{"favicon_url":"/uploads/site-assets/../private"}`, `{"title":"` + strings.Repeat("字", 81) + `"}`,
			} {
				_, rejected := request(PutAdtyOwnerSite, http.MethodPut, `{"brand":`+invalid+`}`, owner.Id, nil)
				assert.Equal(t, false, rejected["success"], invalid)
				var unchanged model.AdtySite
				require.NoError(t, db.First(&unchanged, site.Id).Error)
				assert.Equal(t, site.Brand, unchanged.Brand, "invalid save must preserve every existing field")
			}
			code, denied := request(PutAdtyOwnerSite, http.MethodPut, `{"brand":{"title":"Foreign"}}`, other.OwnerUserId+1, nil)
			assert.Equal(t, http.StatusForbidden, code)
			assert.Equal(t, false, denied["success"])
			_, cleared := request(PutAdtyOwnerSite, http.MethodPut, `{"brand":{"name":"Brand AI","title":"","favicon_url":"","theme_color":""}}`, owner.Id, nil)
			require.Equal(t, true, cleared["success"])
			require.NoError(t, db.First(&site, site.Id).Error)
			_, public = request(GetStatus, http.MethodGet, "", 0, &site)
			status = public["data"].(map[string]any)
			assert.Equal(t, "", status["site_title"])
			assert.Equal(t, "", status["favicon"])
			assert.Equal(t, "", status["theme_color"])
			require.NoError(t, db.First(&other, other.Id).Error)
			assert.Equal(t, `{"name":"Other site"}`, other.Brand)
		})
	}
}

// 站点公开配置（任务 8.3 的后端，在 8.6 先建）。

func TestAdtySitePublicEndpoint(t *testing.T) {
	cfg := config.GlobalConfig.Get("adty_site_setting")
	require.NotNil(t, cfg, "adty_site_setting 要登记到 config")
	old := *adty_setting.GetSiteSetting()
	oldMinTopUp := operation_setting.MinTopUp
	t.Cleanup(func() {
		*adty_setting.GetSiteSetting() = old
		operation_setting.MinTopUp = oldMinTopUp
	})
	// 首页数字条的「$N 起充」读这个，后台「充值设置」里的最低充值金额
	operation_setting.MinTopUp = 5
	// 和后台保存走同一条路：PUT /api/option/ → handleConfigUpdate → UpdateConfigFromMap
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"api_base_url":          "https://api.example.com",
		"support_email":         "support@example.com",
		"telegram_url":          "https://t.me/example",
		"support_hours":         "10:00-22:00 UTC+8",
		"tawk_property_id":      "prop",
		"tawk_widget_id":        "widget",
		"refund_policy_enabled": "true",
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/adty/site", nil)
	GetAdtySite(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.Equal(t, map[string]any{
		"api_base_url":           "https://api.example.com",
		"support_email":          "support@example.com",
		"telegram_url":           "https://t.me/example",
		"support_hours":          "10:00-22:00 UTC+8",
		"tawk_property_id":       "prop",
		"tawk_widget_id":         "widget",
		"refund_policy_enabled":  true,
		"min_topup":              float64(5),
		"site_agreement_version": float64(0),
	}, envelope.Data)
}
