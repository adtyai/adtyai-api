package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ISS-R4-0013：加赠配置以前只改内存，options 表一个字都不写，接口却回 success，
// 重启后配置消失。这里验证三件事：落库、同事务审计、内存与落库值一致。
func TestAdtyMethodBonusPersistsAndAuditsPG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.AuditIntent{}, &model.AuditLog{}))
	previousDB := model.DB
	model.DB = db
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMap = previousOptions
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.PUT("/api/adty/admin/topup/method-bonus", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("username", "root")
		c.Set("role", common.RoleRootUser)
		PutAdtyMainMethodBonus(c)
	})
	engine.PUT("/api/adty/admin/sites/:id/method-bonus-auth", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("username", "root")
		c.Set("role", common.RoleRootUser)
		PutAdtySiteBonusMethodAuth(c)
	})

	mainRec := adtyBonusPut(t, engine, "/api/adty/admin/topup/method-bonus", `{"method":"alipay","enabled":true,"rate":"0.02"}`)
	require.Equal(t, http.StatusOK, mainRec.Code, mainRec.Body.String())
	authRec := adtyBonusPut(t, engine, "/api/adty/admin/sites/7/method-bonus-auth", `{"method":"usdt","allowed":true}`)
	require.Equal(t, http.StatusOK, authRec.Code, authRec.Body.String())

	var methodBonus, siteAuth model.Option
	require.NoError(t, db.Where("key = ?", adty_setting.TopupSettingKeyMethodBonus).First(&methodBonus).Error)
	require.NoError(t, db.Where("key = ?", adty_setting.TopupSettingKeySiteMethodAuth).First(&siteAuth).Error)
	assert.Contains(t, methodBonus.Value, `"alipay"`)
	assert.Contains(t, methodBonus.Value, `"rate":"0.02"`)
	assert.Contains(t, siteAuth.Value, `"7"`)

	// 内存由配置层从落库值同步，所以重启前后的读数一致。
	rule, ok := adty_setting.MainMethodBonus("alipay")
	require.True(t, ok)
	assert.True(t, rule.Enabled)
	assert.Equal(t, "0.02", rule.Rate)
	assert.True(t, adty_setting.SiteMethodBonusAllowed(7, "usdt"))

	// 配置落库与审计意图必须成对出现，不能只有其中一半。
	var intents int64
	require.NoError(t, db.Model(&model.AuditIntent{}).Count(&intents).Error)
	assert.EqualValues(t, 2, intents)
}

func adtyBonusPut(t *testing.T, engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// 写库失败必须报错，不能回一个 success——这正是这条 P0 的核心：
// 用户以为保存了，重启后配置却不见。
func TestAdtyMethodBonusReportsWriteFailurePG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.AuditIntent{}, &model.AuditLog{}))
	previousDB := model.DB
	model.DB = db
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMap = previousOptions
	})
	require.NoError(t, db.Migrator().DropTable(&model.Option{}))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.PUT("/api/adty/admin/topup/method-bonus", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("username", "root")
		c.Set("role", common.RoleRootUser)
		PutAdtyMainMethodBonus(c)
	})

	rec := adtyBonusPut(t, engine, "/api/adty/admin/topup/method-bonus", `{"method":"wxpay","enabled":true,"rate":"0.03"}`)
	require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"success":false`)
	// 落库失败时内存不能先改：否则进程会在“配置已生效”的假象里跑下去。
	_, ok := adty_setting.MainMethodBonus("wxpay")
	assert.False(t, ok, "a failed write must not leave the new rate in memory")
}
