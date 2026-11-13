package adty_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetMainAndSiteMethodBonus(t *testing.T) {
	require.NoError(t, SetMainMethodBonus("usdt.trc20", true, "0.02"))
	rule, ok := MainMethodBonus("usdt")
	require.True(t, ok)
	require.True(t, rule.Enabled)
	require.Equal(t, "0.02", rule.Rate)
	require.Error(t, SetMainMethodBonus("paypal", true, "0.1"))
	require.Error(t, SetSiteMethodBonus(9, "alipay", true, "0.1"))
	require.NoError(t, AuthorizeSiteBonusMethod(9, "alipay", true))
	require.NoError(t, SetSiteMethodBonus(9, "alipay", true, "0.1"))
	saved, ok := SiteMethodBonus(9, "alipay")
	require.True(t, ok)
	require.Equal(t, "0.1", saved.Rate)
}
