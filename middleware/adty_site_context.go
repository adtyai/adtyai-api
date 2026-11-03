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
