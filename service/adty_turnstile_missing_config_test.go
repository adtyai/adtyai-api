package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common/logsinktest"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 运行期那条日志必须点名**缺的是哪一块**，否则读的人只看到「又一条」。
func TestMissingTurnstileConfigNamesTheGap(t *testing.T) {
	cfg := &adty_setting.ResellerSetting{}
	assert.Equal(t, "cloudflare_account_id is empty and no turnstile widgets configured",
		missingTurnstileConfig(cfg))

	cfg.CloudflareAccountId = "acct-1"
	assert.Equal(t, "no turnstile widgets configured", missingTurnstileConfig(cfg))

	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "w1"}}
	assert.Empty(t, missingTurnstileConfig(cfg), "配齐就没有缺项")
}

// 缺配时登记要打一条 SysError，而且要说清「域名照常能用、但没有验证」。
// 名额满那条路径保持静默——那是正常降级，不是配置缺陷。
func TestTurnstileAssignLogsMissingConfig(t *testing.T) {
	db := announcementTestDB(t)
	buffer := logsinktest.Capture(t)

	previous := *adty_setting.GetResellerSetting()
	t.Cleanup(func() { *adty_setting.GetResellerSetting() = previous })
	setting := adty_setting.GetResellerSetting()
	setting.CloudflareAccountId = ""
	setting.TurnstileWidgets = nil

	_, err := adtyTurnstileAssign(context.Background(), db, "shop.example.com")
	require.NoError(t, err, "缺配不是错误：域名照常能用")

	logged := buffer.String()
	assert.Contains(t, logged, "turnstile assignment skipped")
	assert.Contains(t, logged, "shop.example.com", "要能定位到是哪个域名")
	assert.Contains(t, logged, "cloudflare_account_id is empty", "要点名缺的字段")
	assert.Contains(t, logged, "no human verification", "要说清后果")
}
