package adty_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 三个入口现在都只「算」不「写」：返回要落库的 JSON，内存由配置层在提交后
// 从落库值同步。测试同样走「build + apply」两步，不绕过落库形状。
func TestBuildMainAndSiteMethodBonus(t *testing.T) {
	value, err := BuildMainMethodBonus("usdt.trc20", true, "0.02")
	require.NoError(t, err)
	require.Contains(t, value, `"usdt"`)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeyMethodBonus, value))
	rule, ok := MainMethodBonus("usdt")
	require.True(t, ok)
	require.True(t, rule.Enabled)
	require.Equal(t, "0.02", rule.Rate)

	_, err = BuildMainMethodBonus("paypal", true, "0.1")
	require.Error(t, err, "unsupported payment method")
	_, err = BuildSiteMethodBonus(9, "alipay", true, "0.1")
	require.Error(t, err, "a site cannot configure a method Root did not authorize")

	authValue, err := BuildSiteMethodAuth(9, "alipay", true)
	require.NoError(t, err)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeySiteMethodAuth, authValue))
	require.True(t, SiteMethodBonusAllowed(9, "alipay"))

	siteValue, err := BuildSiteMethodBonus(9, "alipay", true, "0.1")
	require.NoError(t, err)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeySiteMethodBonus, siteValue))
	saved, ok := SiteMethodBonus(9, "alipay")
	require.True(t, ok)
	require.Equal(t, "0.1", saved.Rate)
}

// 写进 options 的键必须和配置注册名一致，否则重启读不回来。
func TestTopupSettingKeysMatchJSONTags(t *testing.T) {
	require.Equal(t, "adty_topup_setting.method_bonus", TopupSettingKeyMethodBonus)
	require.Equal(t, "adty_topup_setting.site_method_auth", TopupSettingKeySiteMethodAuth)
	require.Equal(t, "adty_topup_setting.site_method_bonus", TopupSettingKeySiteMethodBonus)
}
