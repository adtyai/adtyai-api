package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotWithPrices(unit, base, final string) *RequestPriceSnapshot {
	return &RequestPriceSnapshot{
		SchemaVersion:    RequestPriceSnapshotVersion,
		ModelName:        "snapshot-price-rule",
		ModelBasePrice:   base,
		BillingGroup:     "default",
		GroupRatio:       "1",
		SiteCostFactor:   "1",
		SiteMarkupRatio:  "0",
		FinalPrice:       final,
		Currency:         RequestPriceCurrencyUSD,
		BillingUnit:      unit,
		AcceptedAt:       1700000000,
		RuleVersion:      RequestPriceSnapshotVersion,
		BilledGroupRatio: "1",
	}
}

// 零价不是"没有价"，是"错价"：它会被 consumed_amount 里的真实金额当场反驳。
func TestRequestPriceSnapshotRejectsFabricatedZeroPrice(t *testing.T) {
	for _, unit := range []string{BillingUnitPerToken, BillingUnitPerCall} {
		snap := snapshotWithPrices(unit, "0", "0")
		assert.False(t, snap.HasOfficialFields(), "%s must not freeze a zero base price", unit)
	}

	tiered := snapshotWithPrices(BillingUnitTieredExpr, "0", "0")
	assert.False(t, tiered.HasOfficialFields(), "a tiered record must not store zeros either")

	negative := snapshotWithPrices(BillingUnitPerToken, "2", "-2")
	assert.False(t, negative.HasOfficialFields(), "a negative final price is never official")
}

// tiered_expr 由表达式定价，没有 base 可冻结：显式留空是合法的，也是最诚实的写法。
func TestRequestPriceSnapshotTieredMayOmitPriceButNotZero(t *testing.T) {
	omitted := snapshotWithPrices(BillingUnitTieredExpr, "", "")
	assert.True(t, omitted.HasOfficialFields())

	for _, price := range [][2]string{{"1.5", "1.5"}, {"1.5", ""}, {"", "1.5"}} {
		snap := snapshotWithPrices(BillingUnitTieredExpr, price[0], price[1])
		assert.True(t, snap.HasOfficialFields(), "%v", price)
	}

	garbage := snapshotWithPrices(BillingUnitTieredExpr, "abc", "")
	assert.False(t, garbage.HasOfficialFields())
}

// 普通记录仍要冻结真实的正价，且空字符串不算有价。
func TestRequestPriceSnapshotOfficialFieldsRequireRealPrice(t *testing.T) {
	require.True(t, snapshotWithPrices(BillingUnitPerToken, "2", "2.5").HasOfficialFields())
	require.True(t, snapshotWithPrices(BillingUnitPerToken, "0.000002", "0.000002").HasOfficialFields())
	// 免费分组的 final 为 0 是真实的 0，不构成矛盾。
	require.True(t, snapshotWithPrices(BillingUnitPerToken, "2", "0").HasOfficialFields())
	require.False(t, snapshotWithPrices(BillingUnitPerToken, "", "2").HasOfficialFields())
	require.False(t, snapshotWithPrices(BillingUnitPerToken, "2", "").HasOfficialFields())
	require.False(t, snapshotWithPrices(BillingUnitPerToken, "2", "  ").HasOfficialFields())
}

// 存储往返不能把"tiered 留空"变回不合法，否则重试会悄悄失去冻结倍率。
func TestRequestPriceSnapshotTieredEmptyPriceSurvivesRoundTrip(t *testing.T) {
	snap := snapshotWithPrices(BillingUnitTieredExpr, "", "")
	encoded, err := EncodeRequestPriceSnapshot(snap)
	require.NoError(t, err)
	decoded := DecodeRequestPriceSnapshot(encoded)
	require.NotNil(t, decoded)
	assert.Equal(t, "", decoded.ModelBasePrice)
	assert.Equal(t, "", decoded.FinalPrice)
}
