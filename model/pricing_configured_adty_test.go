package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 审查 N-P2-17：没配倍率/价格的模型 price_configured=false；已知模型为 true。
func TestPricingMarksUnconfiguredModels(t *testing.T) {
	resetPricingEndpointTestTables(t)
	ratio_setting.InitRatioSettings()
	prev := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"zz-configured-model":2}`))
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRatioByJSONString(prev) })
	insertPricingEndpointChannel(t, 902, constant.ChannelTypeOpenAI, dto.ChannelOtherSettings{})
	insertPricingEndpointAbility(t, 902, "zz-no-such-ratio-model-xyz")
	insertPricingEndpointAbility(t, 902, "zz-configured-model")
	p := pricingByModel(GetPricing())
	require.Contains(t, p, "zz-no-such-ratio-model-xyz")
	assert.False(t, p["zz-no-such-ratio-model-xyz"].PriceConfigured)
	assert.True(t, p["zz-configured-model"].PriceConfigured)
}
