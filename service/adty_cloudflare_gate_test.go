package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `0069` 的三条连锁后果全都在两个「总开关」上：这两处读 `CloudflareConfigured()`，
// 一旦它把 Turnstile 的字段算进来，开自定义域名但不用 Turnstile 的部署就会
// **静默停摆**——新增域名被关掉、定期检查任务每 5 分钟空转。
//
// 所以这里直接验这两处放行，而不是只验判据本身。
func TestCloudflareGatesAllowDomainWithoutTurnstile(t *testing.T) {
	announcementTestDB(t)
	previous := *adty_setting.GetResellerSetting()
	t.Cleanup(func() { *adty_setting.GetResellerSetting() = previous })

	cfg := adty_setting.GetResellerSetting()
	cfg.Enabled = true
	cfg.CustomDomainEnabled = true
	cfg.RootDomains = []string{"example.com"}
	cfg.CloudflareApiToken = "cf-token"
	cfg.CloudflareZoneId = "zone-1"
	cfg.CnameTarget = "edge.example.com"
	// 不给 account / widget：这个部署不用 Turnstile。
	cfg.CloudflareAccountId = ""
	cfg.TurnstileWidgets = nil

	// ① 定期检查任务不能因为「没配 Turnstile」而整轮返回。
	// 它会去查 pending 的域名；库刚建好、没有域名时应当正常返回 nil，而不是提前 return。
	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Now()))

	// ② 新增自定义域名不能被关掉。这里不造完整站点，只验「没有被 Gate 拦下」：
	// 被拦下时返回的是 ErrAdtyCustomDomainOff，其它错误说明已经过了这道门。
	_, err := AddAdtySiteDomain(context.Background(), 999_999, "shop.example.com")
	assert.NotErrorIs(t, err, ErrAdtyCustomDomainOff,
		"开自定义域名但不用 Turnstile 时，新增域名不该被判成「自定义域名已关闭」")
}
