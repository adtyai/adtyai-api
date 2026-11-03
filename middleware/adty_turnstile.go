package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// AdtySiteContextKey 是当前站点在 gin 上下文里的键（11.2 的 AdtySiteContext 写入，*model.AdtySite；主站没有这个键）。
const AdtySiteContextKey = "adty_site"

// adtyTurnstileSiteverifyURL 是 TurnstileCheck 调的 siteverify 地址，测试里换成假服务器。
var adtyTurnstileSiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// AdtyTurnstileSecretFor 按当前站点和请求域名取 widget 的 secret；空串表示用上游的 TurnstileSecretKey。测试里可以换掉。
var AdtyTurnstileSecretFor = func(site *model.AdtySite, host string) string {
	return service.AdtyTurnstileFor(site, host).Secret
}

func adtySiteFromContext(c *gin.Context) *model.AdtySite {
	value, ok := c.Get(AdtySiteContextKey)
	if !ok {
		return nil
	}
	site, _ := value.(*model.AdtySite)
	return site
}

// adtyTurnstileCheckSite 用分站的 secret 校验，并要求 siteverify 返回的 hostname 是这次请求的域名（任务 11.8）。
func adtyTurnstileCheckSite(c *gin.Context, secret, token string) {
	ok, err := service.AdtyTurnstileVerifyAt(c.Request.Context(), adtyTurnstileSiteverifyURL, secret, token, c.ClientIP(), c.Request.Host)
	if err != nil {
		common.SysError(common.LogText("turnstile verify failed: %s", err.Error()))
	}
	if !ok {
		common.ApiErrorT(c, "Turnstile verification failed, please refresh and try again!")
		c.Abort()
		return
	}
	c.Next()
}
