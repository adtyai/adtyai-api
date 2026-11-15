package adty_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 三个入口都只「算」不「写」：以调用方给的当前值（生产里是事务内加锁读到的
// options 行）为基准合并，返回要落库的 JSON，内存由配置层在提交后从落库值同步。
func TestMergeMainAndSiteMethodBonus(t *testing.T) {
	value, err := MergeMainMethodBonus("", "usdt.trc20", true, "0.02")
	require.NoError(t, err)
	require.Contains(t, value, `"usdt"`)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeyMethodBonus, value))
	rule, ok := MainMethodBonus("usdt")
	require.True(t, ok)
	require.True(t, rule.Enabled)
	require.Equal(t, "0.02", rule.Rate)

	_, err = MergeMainMethodBonus("", "paypal", true, "0.1")
	require.Error(t, err, "unsupported payment method")
	_, err = MergeSiteMethodBonus("", 9, "alipay", true, "0.1")
	require.Error(t, err, "a site cannot configure a method Root did not authorize")

	authValue, err := MergeSiteMethodAuth("", 9, "alipay", true)
	require.NoError(t, err)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeySiteMethodAuth, authValue))
	require.True(t, SiteMethodBonusAllowed(9, "alipay"))

	siteValue, err := MergeSiteMethodBonus("", 9, "alipay", true, "0.1")
	require.NoError(t, err)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeySiteMethodBonus, siteValue))
	saved, ok := SiteMethodBonus(9, "alipay")
	require.True(t, ok)
	require.Equal(t, "0.1", saved.Rate)
}

// ISS-R4-0099：合并的基准是库里读到的当前值，不是本进程的内存快照。另一个
// 副本刚写进去的规则必须原样留下，不能被自己这一项的整串覆盖抹掉。
func TestMergeKeepsRulesWrittenByAnotherNode(t *testing.T) {
	first, err := MergeMainMethodBonus("", "alipay", true, "0.02")
	require.NoError(t, err)
	second, err := MergeMainMethodBonus(first, "wxpay", true, "0.03")
	require.NoError(t, err)
	require.Contains(t, second, `"alipay"`, "另一节点写入的主站规则不能被覆盖")
	require.Contains(t, second, `"wxpay"`)

	auth, err := MergeSiteMethodAuth("", 7, "usdt", true)
	require.NoError(t, err)
	auth, err = MergeSiteMethodAuth(auth, 8, "usdt", true)
	require.NoError(t, err)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeySiteMethodAuth, auth))
	require.True(t, SiteMethodBonusAllowed(7, "usdt"), "另一节点授权的站点不能被覆盖")
	require.True(t, SiteMethodBonusAllowed(8, "usdt"))

	site, err := MergeSiteMethodBonus("", 7, "usdt", true, "0.1")
	require.NoError(t, err)
	site, err = MergeSiteMethodBonus(site, 8, "usdt", true, "0.2")
	require.NoError(t, err)
	require.NoError(t, ApplyMethodBonusOption(TopupSettingKeySiteMethodBonus, site))
	rule7, ok := SiteMethodBonus(7, "usdt")
	require.True(t, ok, "另一节点写入的站点规则不能被覆盖")
	require.Equal(t, "0.1", rule7.Rate)
	rule8, ok := SiteMethodBonus(8, "usdt")
	require.True(t, ok)
	require.Equal(t, "0.2", rule8.Rate)
}

// 写进 options 的键必须和配置注册名一致，否则重启读不回来。
func TestTopupSettingKeysMatchJSONTags(t *testing.T) {
	require.Equal(t, "adty_topup_setting.method_bonus", TopupSettingKeyMethodBonus)
	require.Equal(t, "adty_topup_setting.site_method_auth", TopupSettingKeySiteMethodAuth)
	require.Equal(t, "adty_topup_setting.site_method_bonus", TopupSettingKeySiteMethodBonus)
}
