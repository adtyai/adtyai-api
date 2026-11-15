package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adtyMethodBonusBody struct {
	Method  string `json:"method"`
	Enabled bool   `json:"enabled"`
	Rate    string `json:"rate"`
	Allowed *bool  `json:"allowed"`
}

// saveAdtyBonusMethodOption 把一个加赠配置写进 options，并与审计同事务。
//
// 先落库、提交后由配置层按落库值更新内存：写库失败时内存保持旧值，接口返回
// 错误，不会出现「界面显示已保存、重启后配置消失」。
//
// 合并基准在事务内用行锁从库里读（merge 收到的 current），不取本进程的内存
// 快照：整串 JSON 覆盖写时，取内存当基准会让另一个副本刚写入的规则被静默抹掉
// （ISS-R4-0099）。
func saveAdtyBonusMethodOption(c *gin.Context, key, action string, params map[string]any, after any, merge func(current string) (string, error)) bool {
	event := prepareManageAuditChange(c, 0, action, params, nil, after, "", "")
	var value string
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		current, err := model.LockOptionValueTx(tx, key)
		if err != nil {
			return err
		}
		merged, err := merge(current)
		if err != nil {
			return err
		}
		if err := model.SaveOptionsTx(tx, map[string]string{key: merged}); err != nil {
			return err
		}
		value = merged
		return model.AppendAuditIntentTx(tx, event)
	})
	if err != nil {
		// 写库失败必须是 500：这条接口以前谎报 success，配置静默丢失。
		common.ApiErrorStatus(c, http.StatusInternalServerError, err)
		return false
	}
	if err := model.ApplyOptionValues(map[string]string{key: value}); err != nil {
		common.SysError(common.LogText("apply %s after it was stored: %s", key, err.Error()))
	}
	return true
}

// PutAdtyMainMethodBonus lets Root set one main-site payment-method gift rule.
func PutAdtyMainMethodBonus(c *gin.Context) {
	var body adtyMethodBonusBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	merge := func(current string) (string, error) {
		return adty_setting.MergeMainMethodBonus(current, body.Method, body.Enabled, body.Rate)
	}
	// 非法参数要在事务外就变成 400：事务里的错误一律按写库失败回 500。
	if _, err := merge(""); err != nil {
		common.ApiError(c, err)
		return
	}
	if !saveAdtyBonusMethodOption(c, adty_setting.TopupSettingKeyMethodBonus, "adty.topup.method_bonus.update",
		map[string]any{"method": body.Method, "enabled": body.Enabled, "rate": body.Rate},
		map[string]any{"method": body.Method, "enabled": body.Enabled}, merge) {
		return
	}
	common.ApiSuccess(c, adty_setting.GetTopupSetting().MethodBonus)
}

// PutAdtySiteBonusMethodAuth lets Root authorize one method for a site.
func PutAdtySiteBonusMethodAuth(c *gin.Context) {
	siteID, err := strconv.Atoi(c.Param("id"))
	if err != nil || siteID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body adtyMethodBonusBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.Allowed == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	merge := func(current string) (string, error) {
		return adty_setting.MergeSiteMethodAuth(current, siteID, body.Method, *body.Allowed)
	}
	if _, err := merge(""); err != nil {
		common.ApiError(c, err)
		return
	}
	if !saveAdtyBonusMethodOption(c, adty_setting.TopupSettingKeySiteMethodAuth, "adty.topup.site_method_auth.update",
		map[string]any{"method": body.Method, "allowed": *body.Allowed, "site_id": siteID},
		map[string]any{"method": body.Method, "allowed": *body.Allowed}, merge) {
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": siteID, "allowed": *body.Allowed})
}

// PutAdtySiteMethodBonus lets a site owner save a gift rule inside Root's authorization.
func PutAdtySiteMethodBonus(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body adtyMethodBonusBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	merge := func(current string) (string, error) {
		return adty_setting.MergeSiteMethodBonus(current, site.Id, body.Method, body.Enabled, body.Rate)
	}
	if _, err := merge(""); err != nil {
		common.ApiError(c, err)
		return
	}
	if !saveAdtyBonusMethodOption(c, adty_setting.TopupSettingKeySiteMethodBonus, "adty.topup.site_method_bonus.update",
		map[string]any{"method": body.Method, "enabled": body.Enabled, "rate": body.Rate, "site_id": site.Id},
		map[string]any{"method": body.Method, "enabled": body.Enabled}, merge) {
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": site.Id})
}
