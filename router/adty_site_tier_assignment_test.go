package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootPinsASiteLevelAndHandsItBack(t *testing.T) {
	f := newOwnerFixture(t)
	root := adtySessionToken(t, f.db, "tier-root", common.RoleRootUser)
	saved := adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/reseller/tiers",
		`{"tiers":[{"code":"gold","name":"Gold","min_effective_users":10,"min_principal_usd":"100","min_consumption_usd":"50","cost_factor":"0.7","max_markup":"2.5","deposit_usd":"30","enabled":true,"sort_order":1},`+
			`{"code":"retired","name":"Retired","deposit_usd":"0","enabled":false}]}`, root)
	require.Contains(t, saved.Body.String(), `"success":true`, saved.Body.String())

	path := fmt.Sprintf("/api/adty/admin/sites/%d/tier", f.site.Id)
	pinned := adtyJSON(t, f.engine, http.MethodPut, path, `{"code":"gold","mode":"manual"}`, root)
	require.Contains(t, pinned.Body.String(), `"success":true`, pinned.Body.String())
	var site model.AdtySite
	require.NoError(t, f.db.First(&site, f.site.Id).Error)
	assert.Equal(t, "gold", site.TierCode)
	assert.Equal(t, "manual", site.TierMode)
	require.NotNil(t, site.TierCostFactor)
	assert.True(t, site.TierCostFactor.Equal(decimal.RequireFromString("0.7")))
	var margin model.AdtySiteWallet
	require.NoError(t, f.db.Where("site_id = ? AND kind = ?", f.site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.RequiredBalance.Equal(decimal.NewFromInt(30)))

	assert.Contains(t, adtyJSON(t, f.engine, http.MethodPut, path, `{"code":"retired","mode":"manual"}`, root).Body.String(),
		"This level does not exist or is disabled")
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodPut, path, `{"code":"gold","mode":"sometimes"}`, root).Body.String(), `"success":true`)
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodPut, path, `{"code":"","mode":"auto"}`, f.token).Body.String(), `"success":true`)

	auto := adtyJSON(t, f.engine, http.MethodPut, path, `{"code":"","mode":"auto"}`, root)
	require.Contains(t, auto.Body.String(), `"success":true`, auto.Body.String())
	var handedBack model.AdtySite
	require.NoError(t, f.db.First(&handedBack, f.site.Id).Error)
	assert.Equal(t, "", handedBack.TierCode)
	assert.Equal(t, "auto", handedBack.TierMode)
	assert.Nil(t, handedBack.TierCostFactor)
}
