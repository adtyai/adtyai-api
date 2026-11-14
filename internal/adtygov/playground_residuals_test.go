package adtygov

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfficialPlaygroundResidualCallSitesAreMarked(t *testing.T) {
	sites := OfficialPlaygroundResidualCallSites()
	require.NotEmpty(t, sites)
	require.Equal(t, "RETIRE-playground-residuals", OfficialPlaygroundResidualInventoryID)

	required := map[string]string{
		"middleware/adty_site_context.go": "adtyAPIPathPrefixes /pg/",
		"relay/constant/relay_mode.go":    "Path2RelayMode /pg/chat/completions",
		"relay/common/relay_info.go":      "RelayInfo.IsPlayground",
		"service/quota.go":                "IsPlayground branches",
		"service/billing_session.go":      "IsPlayground branches",
		"service/midjourney.go":           "IsPlayground branch",
		"pkg/jsplugin/routing.go":         "reserved /pg and /playground",
		"router/web-router.go":            "rejectRetiredFeaturePath",
	}
	byPathSymbol := map[string]ResidualCallSite{}
	for _, site := range sites {
		require.NotEmpty(t, site.Path)
		require.NotEmpty(t, site.Symbol)
		require.NotEmpty(t, site.Retention)
		require.NotEmpty(t, site.Reason)
		require.False(t, site.UserFeature, "%s %s must not be marked a user feature", site.Path, site.Symbol)
		byPathSymbol[site.Path+"\x00"+site.Symbol] = site
	}
	for path, symbol := range required {
		_, ok := byPathSymbol[path+"\x00"+symbol]
		require.True(t, ok, "missing official residual mark for %s %s", path, symbol)
	}

	require.False(t, ClaimedClosed(CapRetiredChain))
	require.False(t, ClaimedClosed(CapSmoke))
	require.False(t, ClaimedClosed(CapScale))
	require.False(t, HasLandedEntry(CapRetiredChain))
	campaign := OfficialCleanupCampaign()
	require.False(t, campaign.Executed)
	require.False(t, campaign.MassDeleted)
}
