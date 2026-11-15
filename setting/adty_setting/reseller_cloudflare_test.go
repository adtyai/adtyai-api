package adty_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 自定义域名的「已配置」判据必须包含 account_id —— Turnstile 登记用它拼 API 路径。
//
// 缺它时的形态最糟：后台显示「已配置」，而 `adtyTurnstileAssign` 走
// `if account == "" { return "", nil }`，无错、无日志、无标记，5 分钟一轮
// 永远走同一空分支。也就是说「配置完成」这件事被一个静默失败伪装了。
func TestCloudflareConfiguredRequiresAccountId(t *testing.T) {
	complete := ResellerSetting{
		CloudflareApiToken:  "cf-token",
		CloudflareZoneId:    "zone-1",
		CloudflareAccountId: "acct-1",
		CnameTarget:         "edge.example.com",
	}
	assert.True(t, complete.CloudflareConfigured(), "四项齐全才算已配置")

	missingAccount := complete
	missingAccount.CloudflareAccountId = "  "
	assert.False(t, missingAccount.CloudflareConfigured(),
		"account_id 缺了就是没配好：Turnstile 会用空 account 拼路径，那一支是静默返回")

	for name, mutate := range map[string]func(*ResellerSetting){
		"api token": func(s *ResellerSetting) { s.CloudflareApiToken = "" },
		"zone id":   func(s *ResellerSetting) { s.CloudflareZoneId = "" },
		"cname":     func(s *ResellerSetting) { s.CnameTarget = "" },
	} {
		broken := complete
		mutate(&broken)
		assert.False(t, broken.CloudflareConfigured(), "缺 %s 也算没配好", name)
	}
}
