package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 保存**不拒绝**，但要让「这几项不会生效」在响应里看得见。
//
// 拒绝会让「先填一半、回头再补」变成不可能，而这几项的失败后果都是功能降级、
// 不是算错钱。所以走 warnings，不走 error。
func TestResellerWarningsSurfaceTurnstileGap(t *testing.T) {
	cfg := &adty_setting.ResellerSetting{
		CustomDomainEnabled: true,
		CloudflareApiToken:  "cf-token",
		CloudflareZoneId:    "zone-1",
		CnameTarget:         "edge.example.com",
		// account 和 widget 都没配：自定义域名能用，人机验证不会登记。
	}
	assert.True(t, cfg.CloudflareConfigured(), "三项齐全就该能开自定义域名")
	assert.False(t, cfg.TurnstileConfigured())

	warnings := adtyResellerSettingsWarnings(cfg)
	assert.Len(t, warnings, 1, "只该报 Turnstile 这一条")
	assert.Contains(t, warnings[0], "Turnstile is not configured")
	assert.Contains(t, warnings[0], "will not be registered for human verification",
		"要说清后果：域名能用，但不会登记人机验证")

	// 配齐之后不该再有这条。
	cfg.CloudflareAccountId = "acct-1"
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "w1", SiteKey: "k1"}}
	assert.Empty(t, adtyResellerSettingsWarnings(cfg), "配齐就不该有告警")
}

// 没开自定义域名也不该报——那条告警的前提是「开了但配不全」。
func TestResellerWarningsQuietWhenCustomDomainsOff(t *testing.T) {
	cfg := &adty_setting.ResellerSetting{CustomDomainEnabled: false}
	assert.Empty(t, adtyResellerSettingsWarnings(cfg))
}

// 发件域名那一条同理：开了但没填 Resend key 时要说，而且不拒绝。
func TestResellerWarningsSurfaceSenderDomainGap(t *testing.T) {
	cfg := &adty_setting.ResellerSetting{SenderDomainEnabled: true}
	warnings := adtyResellerSettingsWarnings(cfg)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "Resend API key is empty")
}

// 密钥被清空时只有一行 SysError，运营看到的现象却是「分站注册全挂了」，
// 根因还在另一条密钥上（ISS-R4-0122）。所以要把「哪一条读不出来」摆到设置页上。
func TestResellerSettingsWarnAboutClearedSecrets(t *testing.T) {
	db := modelManagementDB(t, "sqlite", "")
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	key := adty_setting.ResellerSettingModule + "." + adty_setting.ResellerKeyCloudflareTok
	require.NoError(t, db.Save(&model.Option{Key: key, Value: "adty:secret:v1:truncated"}).Error)

	model.InitOptionMap()
	warnings := adtyClearedSecretWarnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], key, "要说清是哪一条密钥被清空了")
	assert.Contains(t, warnings[0], "re-enter it")

	// 填好之后不再有这条告警（顺带把包级状态清干净）。
	require.NoError(t, db.Save(&model.Option{Key: key, Value: ""}).Error)
	model.InitOptionMap()
	assert.Empty(t, adtyClearedSecretWarnings())
}
