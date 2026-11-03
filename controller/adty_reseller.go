package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// 站长侧的分站接口（P11）。站长中心的其余接口在 11.6。
// 方案里写的 controller/adty_site.go 已经是主站站点配置接口，分站的站长侧接口放这个文件。

// ApplyAdtySite 申请开站：POST /api/adty/site/apply {subdomain, contact_info, agree_version, invite_code?}。
// 带有效邀请码时直接开成 active，否则等管理员审核。
func ApplyAdtySite(c *gin.Context) {
	var body struct {
		Subdomain    string `json:"subdomain"`
		ContactInfo  string `json:"contact_info"`
		AgreeVersion string `json:"agree_version"`
		InviteCode   string `json:"invite_code"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	site, err := service.ApplyAdtySite(c.GetInt("id"), body.Subdomain, body.ContactInfo, body.AgreeVersion, body.InviteCode)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": site.Id, "status": site.Status, "subdomain": site.Subdomain, "host": service.AdtySiteDisplayHost(site)})
}

// RequestAdtySiteClose 站长申请关站：POST /api/adty/site/close {reason}。客户去向由管理员定。
func RequestAdtySiteClose(c *gin.Context) {
	var body struct {
		Reason string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	site, err := service.RequestAdtySiteClose(c.GetInt("id"), body.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": site.Id, "close_requested_at": site.CloseRequestedAt})
}

// GetAdtySiteDomains 列出站长自己站上的自定义域名（任务 11.8），带要加的 DNS 记录和状态。
func GetAdtySiteDomains(c *gin.Context) {
	domains, err := service.ListAdtySiteDomains(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, domains)
}

// AddAdtySiteDomain 绑定自定义域名：POST /api/adty/site/domains {hostname}（任务 11.8）。
func AddAdtySiteDomain(c *gin.Context) {
	var body struct {
		Hostname string `json:"hostname"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	domain, err := service.AddAdtySiteDomain(c.Request.Context(), c.GetInt("id"), body.Hostname)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, domain)
}

// RemoveAdtySiteDomain 删除自定义域名：DELETE /api/adty/site/domains/:id（任务 11.8）。
func RemoveAdtySiteDomain(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	if err := service.RemoveAdtySiteDomain(c.Request.Context(), c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

// AdtyPayResult 是易支付付完跳回的主站页面：GET /pay/result/:token，302 到订单所属站点的钱包（任务 11.3）。
func AdtyPayResult(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, service.AdtyPayResultTarget(c.Param("token")))
}

// GetAdtySiteSenderDomain 是站长的发件域名和要加的 DNS 记录（任务 11.3）；没有时 data 为 null。
func GetAdtySiteSenderDomain(c *gin.Context) {
	domain, err := service.GetAdtySiteSenderDomain(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, domain)
}

// AddAdtySiteSenderDomain 设置发件域名：POST /api/adty/site/sender-domain {domain}（任务 11.3）。
func AddAdtySiteSenderDomain(c *gin.Context) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	domain, err := service.AddAdtySiteSenderDomain(c.Request.Context(), c.GetInt("id"), body.Domain)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, domain)
}

// RemoveAdtySiteSenderDomain 删除发件域名，回到平台发件地址：DELETE /api/adty/site/sender-domain（任务 11.3）。
func RemoveAdtySiteSenderDomain(c *gin.Context) {
	if err := service.RemoveAdtySiteSenderDomain(c.Request.Context(), c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// DisableAdtyAdminSiteSenderDomain 是管理员停用一个站的发件域名（分站·调整，任务 11.3、11.9）。
func DisableAdtyAdminSiteSenderDomain(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	if err := service.DisableAdtySiteSenderDomain(c.Request.Context(), id, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.sender_domain_disable", map[string]any{"site_id": id})
	common.ApiSuccess(c, nil)
}
