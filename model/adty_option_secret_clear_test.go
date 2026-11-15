package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	adtyClearTestTokenKey  = "adty_reseller_setting.cloudflare_apiToken"
	adtyClearTestWidgetKey = "adty_reseller_setting.turnstile_widget_secret"
	adtyClearTestCrypto    = "synthetic-encryption-key-for-local-tests-only"
)

// secretClearTestFixture 建一个独立的库并把两条站点密钥写到库里。
func secretClearTestFixture(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", adtyClearTestCrypto)
	db := isolatedMigrationTestDatabase(t, "sqlite")
	require.NoError(t, db.AutoMigrate(&Option{}))
	cfg := adty_setting.GetResellerSetting()
	saved, oldOptions := *cfg, common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		*cfg, common.OptionMap = saved, oldOptions
		recordAdtyClearedOptionSecrets(nil)
	})
	DB = db
	require.NoError(t, UpdateOptionsBulk(map[string]string{
		adtyClearTestTokenKey:  "synthetic-token",
		adtyClearTestWidgetKey: `{"widget":"synthetic-widget-secret"}`,
	}))
	require.Equal(t, "synthetic-token", cfg.CloudflareApiToken)
	require.Equal(t, map[string]string{"widget": "synthetic-widget-secret"}, cfg.TurnstileWidgetSecret)
	return db
}

// 一条密文坏掉只该影响它自己（ISS-R4-0122）：AllOption 是逐行解密的循环，
// 遇到第一条解不开就整体返回错误，原来的错误分支据此把**两个**密钥一起清空。
func TestUnreadableSecretClearsOnlyItself(t *testing.T) {
	db := secretClearTestFixture(t)
	cfg := adty_setting.GetResellerSetting()

	// 只把 token 的密文弄坏。
	require.NoError(t, db.Save(&Option{Key: adtyClearTestTokenKey, Value: "adty:secret:v1:truncated"}).Error)
	_, err := AllOption()
	require.Error(t, err)

	loadOptionsFromDatabase()

	assert.Empty(t, cfg.CloudflareApiToken, "解不开的那一条必须被清空（fail-closed）")
	assert.Equal(t, map[string]string{"widget": "synthetic-widget-secret"}, cfg.TurnstileWidgetSecret,
		"另一条解得好好的密钥不能被连带清空")
	assert.Equal(t, []string{adtyClearTestTokenKey}, ClearedAdtyOptionSecrets(),
		"要记下被清的是哪一条，后台才有东西可显示")
	var stored Option
	require.NoError(t, db.Where(&Option{Key: adtyClearTestTokenKey}).First(&stored).Error)
	assert.Equal(t, "adty:secret:v1:truncated", stored.Value, "清空只发生在内存里，密文不能被改写")

	// 重新填一条好的，两条密钥都回来，告警也消失。
	require.NoError(t, UpdateOptionsBulk(map[string]string{adtyClearTestTokenKey: "synthetic-token"}))
	loadOptionsFromDatabase()
	assert.Equal(t, "synthetic-token", cfg.CloudflareApiToken)
	assert.Equal(t, map[string]string{"widget": "synthetic-widget-secret"}, cfg.TurnstileWidgetSecret)
	assert.Empty(t, ClearedAdtyOptionSecrets())
}

// 反方向：解不开的是 widget secret 时，token 同样不能被连带清空。
func TestUnreadableWidgetSecretKeepsToken(t *testing.T) {
	db := secretClearTestFixture(t)
	cfg := adty_setting.GetResellerSetting()

	require.NoError(t, db.Save(&Option{Key: adtyClearTestWidgetKey, Value: "adty:secret:v1:truncated"}).Error)
	loadOptionsFromDatabase()

	assert.Empty(t, cfg.TurnstileWidgetSecret)
	assert.Equal(t, "synthetic-token", cfg.CloudflareApiToken, "完好的一条不受影响")
	assert.Equal(t, []string{adtyClearTestWidgetKey}, ClearedAdtyOptionSecrets())
}

// 读库失败时谁坏了还无从判断：不该把还好的密钥一起抹掉。
func TestTransientOptionReadErrorKeepsSecrets(t *testing.T) {
	db := secretClearTestFixture(t)
	cfg := adty_setting.GetResellerSetting()

	failure := errors.New("synthetic transient read failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("secret_read_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(failure)
		}
	}))
	loadOptionsFromDatabase()
	require.NoError(t, db.Callback().Query().Remove("secret_read_failure"))

	assert.Equal(t, "synthetic-token", cfg.CloudflareApiToken, "一次瞬时读库错误不该抹掉密钥")
	assert.Equal(t, map[string]string{"widget": "synthetic-widget-secret"}, cfg.TurnstileWidgetSecret)
	assert.Empty(t, ClearedAdtyOptionSecrets())
}
