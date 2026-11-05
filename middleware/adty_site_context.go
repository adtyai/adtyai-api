package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
)

// adtyAPIPathPrefixes 是接口请求的前缀：这些请求在任何 Host 上都照常处理（API 调用不限域名，执行层决定 29）。
var adtyAPIPathPrefixes = []string{"/api/", "/v1/", "/v1beta/", "/mj/", "/pg/"}

func adtyIsSessionAPI(path string) bool {
	switch path {
	case "/api/verification", "/api/reset_password", "/api/user/reset",
		"/api/user/register", "/api/user/auth/refresh", "/api/user/auth/logout":
		return true
	}
	return strings.HasPrefix(path, "/api/user/login") ||
		strings.HasPrefix(path, "/api/user/passkey/login") ||
		strings.HasPrefix(path, "/api/oauth/")
}

func adtyIsAPIPath(path string) bool {
	if path == "/api" || path == "/v1" || path == "/v1beta" {
		return true
	}
	for _, prefix := range adtyAPIPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return strings.Contains(path, "/mj/")
}

// AdtySiteContext 按请求的 Host 识别当前分站（任务 11.2），放进 c.Set("adty_site", site)；主站或查不到时不放。
// 只认站点表（带缓存），不信任 X-Forwarded-Host 之类的头（执行层决定 28）。分站功能没打开时什么都不做。
//   - active、paused、closing 的站照常识别；pending 的站还没开，当作查不到。
//   - closed 的站：页面请求 302 到客户的新归属（转回主站就跳主站），接口请求照常处理。
//   - 查不到的 Host：unknown_host_mode=reject 时页面请求返回 404，其余当成主站。
func AdtySiteContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := adty_setting.GetResellerSetting()
		if !cfg.Enabled {
			c.Next()
			return
		}
		host := service.AdtyNormalizeHost(c.Request.Host)
		if host == "" || service.AdtyIsMainHost(host) {
			c.Next()
			return
		}
		if service.AdtyIsCustomDomainHost(host) {
			adtyHandleCustomDomain(c, host)
			return
		}
		if adtyIsSessionAPI(c.Request.URL.Path) {
			if status := service.AdtyRejectSessionAPIStatus(host); status != 0 {
				c.AbortWithStatus(status)
				return
			}
		}
		site := service.AdtySiteForHost(host)
		api := adtyIsAPIPath(c.Request.URL.Path)
		switch {
		case service.AdtySiteServesCustomers(site):
			c.Set(service.AdtySiteContextKey, site)
		case site != nil && site.Status == model.AdtySiteStatusClosed:
			// 新归属：转给别的站就跳那个站，转回主站就跳主站。ServerAddress 没填时没法跳，当主站处理。
			target := service.AdtySiteBaseURL(service.AdtySiteById(site.CloseTransferTo))
			if !api && target != "" {
				c.Redirect(http.StatusFound, target+c.Request.URL.RequestURI())
				c.Abort()
				return
			}
		default:
			if cfg.UnknownHostMode == adty_setting.ResellerUnknownHostReject && !api {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
		}
		c.Next()
	}
}

func adtyHandleCustomDomain(c *gin.Context, host string) {
	site := service.AdtySiteForHost(host)
	if site != nil {
		c.Set(service.AdtySiteContextKey, site)
	}
	path := c.Request.URL.Path
	if adtyIsRelayPath(path) || adtyCustomDomainPublicPage(path) || adtyCustomDomainPublicAPI(path) {
		c.Next()
		return
	}
	if adtyIsAPIPath(path) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if site != nil {
		if target := service.AdtySiteBaseURL(site); target != "" {
			c.Redirect(http.StatusFound, target+c.Request.URL.RequestURI())
			c.Abort()
			return
		}
	}
	c.AbortWithStatus(http.StatusForbidden)
}

// AdtyRequireCurrentSiteTerms blocks account and relay use until a site customer
// accepts the current terms. Logout, self, and the accept endpoint stay open.
func AdtyRequireCurrentSiteTerms() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api/user/self" || path == "/api/user/auth/logout" || path == "/api/adty/site/terms/accept" || path == "/api/status" {
			c.Next()
			return
		}
		userID := c.GetInt("id")
		if userID <= 0 {
			c.Next()
			return
		}
		required, _ := service.AdtySiteTermsStatus(userID)
		if required {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Accept the current site customer terms first",
			})
			return
		}
		c.Next()
	}
}

func adtyCustomDomainPublicAPI(path string) bool {
	switch path {
	case "/api/status", "/api/uptime/status", "/api/ratio_config", "/api/adty/groups/display", "/api/adty/site":
		return true
	}
	return strings.HasPrefix(path, "/api/adty/legal/") || strings.HasPrefix(path, "/api/adty/pricing")
}

func adtyIsRelayPath(path string) bool {
	return strings.HasPrefix(path, "/v1/") || path == "/v1" ||
		strings.HasPrefix(path, "/v1beta/") || path == "/v1beta" ||
		strings.HasPrefix(path, "/mj/") || strings.HasPrefix(path, "/pg/")
}

func adtyCustomDomainPublicPage(path string) bool {
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	switch path {
	case "/", "/pricing", "/docs", "/about", "/rankings", "/status",
		"/user-agreement", "/privacy-policy", "/refund-policy",
		"/zh-cn", "/zh-tw", "/fr", "/ja", "/ko", "/ru", "/vi",
		"/robots.txt", "/sitemap.xml", "/favicon.ico":
		return true
	}
	if strings.HasPrefix(path, "/pricing/") || strings.HasPrefix(path, "/docs/") ||
		strings.HasPrefix(path, "/.well-known/") || strings.HasPrefix(path, "/uploads/") {
		return true
	}
	base := path[strings.LastIndex(path, "/")+1:]
	return strings.Contains(base, ".")
}
