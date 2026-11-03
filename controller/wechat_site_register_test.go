package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeChatRegistrationUsesRequestSiteAndRegisterHook(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousCache, previousRedis, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath
	previousRegister, previousWeChat := common.RegisterEnabled, common.WeChatAuthEnabled
	previousAddress, previousToken := common.WeChatServerAddress, common.WeChatServerToken
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath = previousMaster, previousCache, previousRedis, previousSQLite
		common.RegisterEnabled, common.WeChatAuthEnabled = previousRegister, previousWeChat
		common.WeChatServerAddress, common.WeChatServerToken = previousAddress, previousToken
		service.InvalidateAdtySiteCache()
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = true, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "wechat-site.db")
	require.NoError(t, model.InitDB())
	common.IsMasterNode = false
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, model.MigrateAdtyTables())
	require.NoError(t, database.AutoMigrate(&model.ReferralAffiliate{}, &model.ReferralCommissionAccount{}))

	cfg := adty_setting.GetResellerSetting()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	*cfg = adty_setting.DefaultResellerSetting()
	cfg.Enabled = true
	cfg.RootDomains = []string{"sites.example.com"}
	oldServer := system_setting.ServerAddress
	oldGenerate := constant.GenerateDefaultToken
	system_setting.ServerAddress = "https://main.example.com"
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		system_setting.ServerAddress = oldServer
		constant.GenerateDefaultToken = oldGenerate
	})
	common.RegisterEnabled = true
	common.WeChatAuthEnabled = true

	site := &model.AdtySite{OwnerUserId: 1, Subdomain: "shop", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	closing := &model.AdtySite{OwnerUserId: 2, Subdomain: "closing", RootDomain: "sites.example.com", Status: model.AdtySiteStatusClosing}
	require.NoError(t, database.Create(site).Error)
	require.NoError(t, database.Create(closing).Error)
	service.InvalidateAdtySiteCache()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":"` + r.URL.Query().Get("code") + `"}`))
	}))
	t.Cleanup(upstream.Close)
	common.WeChatServerAddress = upstream.URL

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.AdtySiteContext())
	engine.GET("/api/oauth/wechat", WeChatAuth)
	call := func(host, code string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat?code="+code, nil)
		req.Host = host
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	siteResponse := call("shop.sites.example.com", "wx-shop")
	require.Contains(t, siteResponse.Body.String(), `"success":true`, siteResponse.Body.String())
	var siteUser model.User
	require.NoError(t, database.Where("wechat_id = ?", "wx-shop").First(&siteUser).Error)
	assert.Equal(t, site.Id, siteUser.SiteId)
	var customer model.AdtySiteCustomer
	require.NoError(t, database.Where("user_id = ?", siteUser.Id).First(&customer).Error)
	assert.Equal(t, site.Id, customer.SiteId)
	var siteAffiliates int64
	require.NoError(t, database.Model(&model.ReferralAffiliate{}).Where("user_id = ?", siteUser.Id).Count(&siteAffiliates).Error)
	assert.Zero(t, siteAffiliates)

	closingResponse := call("closing.sites.example.com", "wx-closing")
	assert.Contains(t, closingResponse.Body.String(), `"success":false`)
	var closingCount int64
	require.NoError(t, database.Model(&model.User{}).Where("wechat_id = ?", "wx-closing").Count(&closingCount).Error)
	assert.Zero(t, closingCount)

	mainResponse := call("main.example.com", "wx-main")
	require.Contains(t, mainResponse.Body.String(), `"success":true`, mainResponse.Body.String())
	var mainUser model.User
	require.NoError(t, database.Where("wechat_id = ?", "wx-main").First(&mainUser).Error)
	assert.Zero(t, mainUser.SiteId)
	var mainAffiliates int64
	require.NoError(t, database.Model(&model.ReferralAffiliate{}).Where("user_id = ?", mainUser.Id).Count(&mainAffiliates).Error)
	assert.EqualValues(t, 1, mainAffiliates)

	squatter := model.User{Username: "squatter-wechat", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, database.Create(&squatter).Error)
	taken := "wechat_" + strconv.Itoa(squatter.Id+1)
	require.NoError(t, database.Model(&squatter).Update("username", taken).Error)
	retryResponse := call("main.example.com", "wx-retry")
	require.Contains(t, retryResponse.Body.String(), `"success":true`, retryResponse.Body.String())
	var retried model.User
	require.NoError(t, database.Where("wechat_id = ?", "wx-retry").First(&retried).Error)
	assert.NotEqual(t, taken, retried.Username)
	assert.NotEmpty(t, retried.Username)
}
