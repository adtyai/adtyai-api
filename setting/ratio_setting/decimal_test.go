package ratio_setting

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimalGettersConvertAtBoundaryOnce(t *testing.T) {
	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"vip":1.1}`))
	t.Cleanup(func() {
		_ = UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`)
	})

	got := GetGroupRatioDecimal("vip")
	assert.Equal(t, "1.1", got.String())
	assert.Equal(t, GetGroupRatio("vip"), got.InexactFloat64())
}

func TestJSONIngestKeepsExactDecimalLiterals(t *testing.T) {
	const literal = "0.1234567890123456789"
	floatLost := DecimalFromFloat(0.1234567890123456789)
	require.False(t, floatLost.Equal(decimal.RequireFromString(literal)), "the test coefficient must actually be inexact in float64")

	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"vip":`+literal+`}`))
	require.NoError(t, UpdateModelRatioByJSONString(`{"probe-ratio":`+literal+`}`))
	require.NoError(t, UpdateModelPriceByJSONString(`{"probe-price":0.1}`))
	require.NoError(t, UpdateCompletionRatioByJSONString(`{"probe-completion":`+literal+`}`))
	require.NoError(t, UpdateAudioRatioByJSONString(`{"probe-audio":`+literal+`}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"special":0.1}}`))
	t.Cleanup(func() {
		_ = UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`)
		_ = UpdateModelRatioByJSONString(`{}`)
		_ = UpdateModelPriceByJSONString(`{}`)
		_ = UpdateCompletionRatioByJSONString(`{}`)
		_ = UpdateAudioRatioByJSONString(`{}`)
		_ = UpdateGroupGroupRatioByJSONString(`{"vip":{"edit_this":0.9}}`)
	})

	assert.True(t, GetGroupRatioDecimal("vip").Equal(decimal.RequireFromString(literal)))
	ratio, ok, match := GetModelRatioDecimal("probe-ratio")
	require.True(t, ok)
	assert.Equal(t, "probe-ratio", match)
	assert.True(t, ratio.Equal(decimal.RequireFromString(literal)))
	price, ok := GetModelPriceDecimal("probe-price", false)
	require.True(t, ok)
	assert.True(t, price.Equal(decimal.RequireFromString("0.1")), "got %s", price)
	assert.True(t, GetCompletionRatioDecimal("probe-completion").Equal(decimal.RequireFromString(literal)))
	assert.True(t, GetAudioRatioDecimal("probe-audio").Equal(decimal.RequireFromString(literal)))
	grouped, ok := GetGroupGroupRatioDecimal("vip", "special")
	require.True(t, ok)
	assert.True(t, grouped.Equal(decimal.RequireFromString("0.1")))

	// Float getters remain available as an inexact compatibility view.
	assert.Equal(t, GetGroupRatio("vip"), GetGroupRatioDecimal("vip").InexactFloat64())
}

func TestJSONIngestRejectsInvalidPayloadsWithoutChangingAuthority(t *testing.T) {
	require.NoError(t, UpdateModelPriceByJSONString(`{"keep":0.1}`))
	t.Cleanup(func() { _ = UpdateModelPriceByJSONString(`{}`) })

	require.Error(t, UpdateModelPriceByJSONString(`{`))
	require.Error(t, UpdateModelPriceByJSONString(`[]`))
	got, ok := GetModelPriceDecimal("keep", false)
	require.True(t, ok)
	assert.True(t, got.Equal(decimal.RequireFromString("0.1")))
}

func TestLegacyFloatLoadConvertsOnceIntoDecimalAuthority(t *testing.T) {
	modelPriceMap.ReplaceAll(map[string]float64{"legacy": 0.1})
	modelPriceStore.syncFromFloats()
	t.Cleanup(func() { _ = UpdateModelPriceByJSONString(`{}`) })

	got, ok := GetModelPriceDecimal("legacy", false)
	require.True(t, ok)
	assert.True(t, got.Equal(DecimalFromFloat(0.1)))
	assert.True(t, got.Equal(decimal.RequireFromString("0.1")))
}
