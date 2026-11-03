package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
)

func TestDeleteModelPricingDropsPluginOverride(t *testing.T) {
	drop := billing_setting.PluginBillingExprKey("matrix", "m1")
	keep := billing_setting.PluginBillingExprKey("matrix", "m2")
	values := map[string]map[string]any{
		"ModelPrice": {"m1": 1.0, "m2": 2.0},
		billing_setting.PluginBillingExprOption: {
			drop: "tier(\"gone\")",
			keep: "tier(\"stay\")",
		},
	}

	deleteModelPricing(values, map[string]struct{}{"m1": {}})

	assert.NotContains(t, values["ModelPrice"], "m1")
	assert.Equal(t, 2.0, values["ModelPrice"]["m2"])
	plugin := values[billing_setting.PluginBillingExprOption]
	assert.NotContains(t, plugin, drop)
	assert.Equal(t, "tier(\"stay\")", plugin[keep])
}
