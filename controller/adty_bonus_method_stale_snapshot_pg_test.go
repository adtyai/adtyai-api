package controller

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ISS-R4-0099：加赠配置写回去的是整张表，合并基准若取本进程的内存快照，另一
// 副本刚保存的规则会被静默抹掉（接口还回 success）。
//
// 这里用「两个独立内存副本」复现，而不是并发：A 存完就把内存退回 A 存之前的
// 样子（模拟还没到 SyncOptions 周期的节点 B），B 再存另一条。当前代码下 A 写的
// 那条会消失；修好后两条都在。
func TestAdtyBonusMethodKeepsRulesWrittenByAnotherReplicaPG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.AuditIntent{}, &model.AuditLog{}, &model.AdtySite{}))
	previousDB := model.DB
	model.DB = db
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	setting := adty_setting.GetTopupSetting()
	previousMain, previousAuth, previousSite := setting.MethodBonus, setting.SiteMethodAuth, setting.SiteMethodBonus
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMap = previousOptions
		setting.MethodBonus, setting.SiteMethodAuth, setting.SiteMethodBonus = previousMain, previousAuth, previousSite
		service.InvalidateAdtySiteCache()
	})

	ownerA := model.User{Username: "bonus-replica-a", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	ownerB := model.User{Username: "bonus-replica-b", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&ownerA).Error)
	require.NoError(t, db.Create(&ownerB).Error)
	siteA := model.AdtySite{OwnerUserId: ownerA.Id, Subdomain: "bonus-replica-a", Status: model.AdtySiteStatusActive, AgreementVer: service.AdtyCurrentSiteAgreementVersion()}
	siteB := model.AdtySite{OwnerUserId: ownerB.Id, Subdomain: "bonus-replica-b", Status: model.AdtySiteStatusActive, AgreementVer: service.AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)

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
	engine.PUT("/api/adty/site/bonus-methods/a", func(c *gin.Context) {
		c.Set("id", ownerA.Id)
		c.Set("role", common.RoleCommonUser)
		PutAdtySiteMethodBonus(c)
	})
	engine.PUT("/api/adty/site/bonus-methods/b", func(c *gin.Context) {
		c.Set("id", ownerB.Id)
		c.Set("role", common.RoleCommonUser)
		PutAdtySiteMethodBonus(c)
	})

	stored := func(key string) string {
		t.Helper()
		var row model.Option
		require.NoError(t, db.Where("key = ?", key).First(&row).Error)
		return row.Value
	}

	// 主站规则：A 存 alipay，B 的陈旧内存里还没有它，B 存 wxpay。
	mainA := adtyBonusPut(t, engine, "/api/adty/admin/topup/method-bonus", `{"method":"alipay","enabled":true,"rate":"0.02"}`)
	require.Equal(t, http.StatusOK, mainA.Code, mainA.Body.String())
	setting.MethodBonus = map[string]adty_setting.MethodBonusRule{}
	mainB := adtyBonusPut(t, engine, "/api/adty/admin/topup/method-bonus", `{"method":"wxpay","enabled":true,"rate":"0.03"}`)
	require.Equal(t, http.StatusOK, mainB.Code, mainB.Body.String())
	main := stored(adty_setting.TopupSettingKeyMethodBonus)
	assert.Contains(t, main, `"alipay"`, "另一副本写入的主站规则不能被陈旧快照覆盖")
	assert.Contains(t, main, `"wxpay"`)

	// 站点授权：同样两个副本，各授权一个站点。
	authA := adtyBonusPut(t, engine, "/api/adty/admin/sites/"+strconv.Itoa(siteA.Id)+"/method-bonus-auth", `{"method":"usdt","allowed":true}`)
	require.Equal(t, http.StatusOK, authA.Code, authA.Body.String())
	setting.SiteMethodAuth = nil
	authB := adtyBonusPut(t, engine, "/api/adty/admin/sites/"+strconv.Itoa(siteB.Id)+"/method-bonus-auth", `{"method":"usdt","allowed":true}`)
	require.Equal(t, http.StatusOK, authB.Code, authB.Body.String())
	auth := stored(adty_setting.TopupSettingKeySiteMethodAuth)
	assert.Contains(t, auth, strconv.Itoa(siteA.Id), "另一副本授权的站点不能被陈旧快照覆盖")
	assert.Contains(t, auth, strconv.Itoa(siteB.Id))

	// 站长规则：复现 finding 里的「站点 A / 站点 B」场景。
	bonusA := adtyBonusPut(t, engine, "/api/adty/site/bonus-methods/a", `{"method":"usdt","enabled":true,"rate":"0.10"}`)
	require.Equal(t, http.StatusOK, bonusA.Code, bonusA.Body.String())
	setting.SiteMethodBonus = nil
	bonusB := adtyBonusPut(t, engine, "/api/adty/site/bonus-methods/b", `{"method":"usdt","enabled":true,"rate":"0.20"}`)
	require.Equal(t, http.StatusOK, bonusB.Code, bonusB.Body.String())
	bonus := stored(adty_setting.TopupSettingKeySiteMethodBonus)
	assert.Contains(t, bonus, `"rate":"0.1"`, "站点 A 刚保存的规则不能被站点 B 的写入抹掉")
	assert.Contains(t, bonus, `"rate":"0.2"`)
}
