package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHistoricalRequestPriceSnapshotCopiesStoredRatios(t *testing.T) {
	snap, reason := BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{
		ModelName:      "hist-model",
		BillingGroup:   "default",
		ModelRatio:     2,
		GroupRatio:     1.25,
		SiteMarkup:     1.25,
		SiteCostFactor: 0.8,
		SiteId:         9,
		AcceptedAt:     1700000000,
	})
	require.Equal(t, "", reason)
	require.NotNil(t, snap)
	require.True(t, snap.HasOfficialFields())
	assert.Equal(t, "2", snap.ModelBasePrice)
	assert.Equal(t, "2", snap.ModelRatio)
	assert.Equal(t, "1.25", snap.GroupRatio)
	assert.Equal(t, "1.25", snap.BilledGroupRatio)
	assert.Equal(t, "0.8", snap.SiteCostFactor)
	assert.Equal(t, "0.25", snap.SiteMarkupRatio)
	assert.Equal(t, "1.25", snap.StoredMarkupK)
	assert.Equal(t, "2.5", snap.FinalPrice)
	assert.Equal(t, BillingUnitPerToken, snap.BillingUnit)
	assert.Equal(t, int64(1700000000), snap.AcceptedAt)
	assert.Equal(t, RequestPriceCurrencyUSD, snap.Currency)
	assert.False(t, snap.MainSite)
}

func TestBuildHistoricalRequestPriceSnapshotMainSiteEncoding(t *testing.T) {
	snap, reason := BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{
		ModelRatio:   3,
		GroupRatio:   1.2,
		AcceptedAt:   1,
		BillingGroup: "vip",
	})
	require.Equal(t, "", reason)
	require.NotNil(t, snap)
	assert.True(t, snap.MainSite)
	assert.Equal(t, "1", snap.SiteCostFactor)
	assert.Equal(t, "0", snap.SiteMarkupRatio)
	assert.Equal(t, "3.6", snap.FinalPrice)
}

func TestBuildHistoricalRequestPriceSnapshotSkipsUnsafeRows(t *testing.T) {
	_, reason := BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{ModelRatio: 2, AcceptedAt: 1})
	assert.Equal(t, PriceSnapshotBackfillSkipMissingGroupRatio, reason)

	_, reason = BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{GroupRatio: 1.2, AcceptedAt: 1})
	assert.Equal(t, PriceSnapshotBackfillSkipMissingBasePrice, reason)

	_, reason = BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{
		ModelRatio: 2, GroupRatio: 1.2, SiteId: 4, SiteMarkup: 1.25, AcceptedAt: 1,
	})
	assert.Equal(t, PriceSnapshotBackfillSkipIncompleteSiteRates, reason)

	_, reason = BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{ModelRatio: 2, GroupRatio: 1.2})
	assert.Equal(t, PriceSnapshotBackfillSkipMissingAcceptedAt, reason)

	existing := &RequestPriceSnapshot{
		SchemaVersion: RequestPriceSnapshotVersion, ModelBasePrice: "2", GroupRatio: "1",
		SiteCostFactor: "1", SiteMarkupRatio: "0", FinalPrice: "2", Currency: RequestPriceCurrencyUSD,
		BillingUnit: BillingUnitPerToken, AcceptedAt: 9, RuleVersion: RequestPriceSnapshotVersion,
		BilledGroupRatio: "1",
	}
	got, reason := BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{Existing: existing, ModelRatio: 9, GroupRatio: 4, AcceptedAt: 9})
	assert.Equal(t, PriceSnapshotBackfillSkipAlreadyComplete, reason)
	require.Equal(t, existing, got)
	assert.Equal(t, "2", got.ModelBasePrice)
}

// 历史 tiered 行没有 base 也不该被丢掉：与 accept 时同一判据，建一条不带零价的快照。
func TestBuildHistoricalRequestPriceSnapshotTieredWithoutBaseOmitsPrice(t *testing.T) {
	snap, reason := BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{
		ModelName:      "hist-tiered",
		BillingGroup:   "default",
		GroupRatio:     1.2,
		SiteMarkup:     1.25,
		SiteCostFactor: 0.8,
		SiteId:         9,
		Tiered:         true,
		AcceptedAt:     1700000000,
	})
	require.Equal(t, "", reason)
	require.NotNil(t, snap)
	require.True(t, snap.HasOfficialFields())
	assert.Equal(t, BillingUnitTieredExpr, snap.BillingUnit)
	assert.Equal(t, "", snap.ModelBasePrice)
	assert.Equal(t, "", snap.FinalPrice)
	assert.Equal(t, "1.2", snap.GroupRatio)
	assert.Equal(t, "1.25", snap.StoredMarkupK, "the frozen factors are still worth backfilling")

	// 非 tiered 行缺 base 仍然跳过：留空只对"由表达式定价"的记录成立。
	_, reason = BuildHistoricalRequestPriceSnapshot(HistoricalPriceSnapshotSource{GroupRatio: 1.2, AcceptedAt: 1})
	assert.Equal(t, PriceSnapshotBackfillSkipMissingBasePrice, reason)
}
