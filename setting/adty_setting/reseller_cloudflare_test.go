package adty_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 两条判据必须分开：**自定义域名**要三项，**Turnstile**要三项（token + account + widget）。
//
// 它们服务两条不同的通路，缺项的后果也不同：
//   - 自定义域名缺项 → 域名加不上，保存被拒，有明确文案；
//   - Turnstile 缺项 → **没有任何报错**：`service/adty_turnstile_hosts.go` 的
//     `adtyTurnstileAssign` 走 `if account == "" { return "", nil }`，无错、无日志、
//     无标记，5 分钟一轮永远走同一空分支，而管理页仍显示「已配置」。
//
// **把 account_id 塞进 `CloudflareConfigured` 是错的**：它会拒绝「三项齐全但
// 不用 Turnstile」的部署保存，而且拒绝文案说的是「token、zone ID 和 CNAME target」
// ——判据和文案当场不一致（`ISS-R4-0069`）。所以我原来那条断言就是这个缺陷本身。
func TestCloudflareConfiguredNeedsOnlyTheThreeDnsFields(t *testing.T) {
	complete := ResellerSetting{
		CloudflareApiToken: "cf-token",
		CloudflareZoneId:   "zone-1",
		CnameTarget:        "edge.example.com",
	}
	assert.True(t, complete.CloudflareConfigured(), "三项齐全就该能开自定义域名")

	// 这一条是 `0069` 的核心：没有 account_id 不该影响自定义域名的判据。
	noAccount := complete
	noAccount.CloudflareAccountId = ""
	assert.True(t, noAccount.CloudflareConfigured(),
		"account_id 只服务 Turnstile；缺它不该拒绝自定义域名的保存")

	for name, mutate := range map[string]func(*ResellerSetting){
		"api token": func(s *ResellerSetting) { s.CloudflareApiToken = "  " },
		"zone id":   func(s *ResellerSetting) { s.CloudflareZoneId = "" },
		"cname":     func(s *ResellerSetting) { s.CnameTarget = "" },
	} {
		broken := complete
		mutate(&broken)
		assert.False(t, broken.CloudflareConfigured(), "缺 %s 也算没配好", name)
	}
}

// Turnstile 那一侧的判据单独一条，且它必须能**说出**「没配全」。
//
// 这个函数存在的意义不是「校验保存」，是让「静默跳过登记」有一个可判定的入口——
// 那个空分支今天仍然存在，但至少现在是「能被问出来的」。
func TestTurnstileConfiguredRequiresAccountAndWidget(t *testing.T) {
	complete := ResellerSetting{
		CloudflareApiToken:  "cf-token",
		CloudflareAccountId: "acct-1",
		TurnstileWidgets:    []ResellerTurnstileWidget{{Id: "w1"}},
	}
	assert.True(t, complete.TurnstileConfigured())

	for name, mutate := range map[string]func(*ResellerSetting){
		"account id": func(s *ResellerSetting) { s.CloudflareAccountId = "" },
		"api token":  func(s *ResellerSetting) { s.CloudflareApiToken = "" },
		"widget":     func(s *ResellerSetting) { s.TurnstileWidgets = nil },
	} {
		broken := complete
		mutate(&broken)
		assert.False(t, broken.TurnstileConfigured(), "缺 %s 就是没配全，登记会静默跳过", name)
	}
}
