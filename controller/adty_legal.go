package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// GetAdtyLegal 返回一份条款（任务 8.6）：GET /api/adty/legal/:doc?lang=，doc 为
// user-agreement、privacy-policy、refund-policy，以及分站的 site-agreement、site-customer-terms（P11）。按「本语言 → en → 上游 legal.*」取第一个非空的；
// 退款政策和分站的两份没有上游文本，最后回退成空。返回格式和 ETag 跟上游 /api/user-agreement 一样。
func GetAdtyLegal(c *gin.Context) {
	texts := adty_setting.GetLegalSetting()
	upstream := system_setting.GetLegalSettings()
	var localized map[string]string
	var fallback string
	switch c.Param("doc") {
	case "user-agreement":
		localized, fallback = texts.UserAgreement, upstream.UserAgreement
	case "privacy-policy":
		localized, fallback = texts.PrivacyPolicy, upstream.PrivacyPolicy
	case "refund-policy":
		localized = texts.RefundPolicy
	case "site-agreement":
		localized = texts.SiteAgreement
	case "site-customer-terms":
		localized = texts.SiteCustomerTerms
	default:
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Invalid parameters"))
		return
	}

	lang := strings.TrimSpace(c.Query("lang"))
	for _, key := range []string{lang, "en"} {
		if text := strings.TrimSpace(localized[key]); key != "" && text != "" {
			serveRevalidatedJSON(c, text)
			return
		}
	}
	serveRevalidatedJSON(c, fallback)
}
