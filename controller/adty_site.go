package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// adtySiteView 是站点配置加上首页要显示的公开数字。
type adtySiteView struct {
	adty_setting.SiteSetting
	// MinTopUp 是后台「充值设置」的最低充值金额（美元），首页数字条显示「$N 起充」。
	// 充值接口 /api/adty/topup/info 要登录，访客读不到，所以在这里公开一份。
	MinTopUp int `json:"min_topup"`
	// SiteAgreementVersion 是申请开站时必须回传的协议版本。
	SiteAgreementVersion int `json:"site_agreement_version"`
}

// GetAdtySite 是公开的站点配置（任务 8.3）：首页、文档、快速接入卡、客服和退款政策开关都读它。
func GetAdtySite(c *gin.Context) {
	common.ApiSuccess(c, adtySiteView{
		SiteSetting:          *adty_setting.GetSiteSetting(),
		MinTopUp:             operation_setting.MinTopUp,
		SiteAgreementVersion: adty_setting.GetLegalSetting().SiteAgreementVersion,
	})
}
