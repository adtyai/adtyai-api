package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaUSDConversionIgnoresMutatedFloatScale(t *testing.T) {
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = old })

	quota, err := common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(2).Mul(common.QuotaPerUnitDecimal()))
	require.NoError(t, err)
	assert.Equal(t, 1_000_000, quota)

	info := QuotaInfo{UsePrice: true, ModelPrice: 2, GroupRatio: 1}
	info = withBilledGroupRatio(info, decimal.NewFromInt(1))
	got, clamp := calculateAudioQuota(info)
	assert.Nil(t, clamp)
	assert.Equal(t, 1_000_000, got, "per-request price must use QuotaPerUnitDecimal, not the mutated float")
}

func TestInexactFloatProductIsBilledAtExactDecimal(t *testing.T) {
	// 0.1, 0.7 and 1.15 are short decimals after a one-time NewFromFloat,
	// but their product is not exactly representable in float64, and the
	// float bill rounds to a different integer than the exact decimal.
	group := decimal.RequireFromString("0.1")
	cost := decimal.RequireFromString("0.7")
	storedK := decimal.RequireFromString("1.15") // markup ratio = 0.15
	sell := group.Mul(cost).Mul(storedK)
	require.Equal(t, "0.0805", sell.String())

	floatProduct := float64(0.1) * float64(0.7) * float64(1.15)
	require.NotEqual(t, sell.String(), decimal.NewFromFloat(floatProduct).String(),
		"the test case must be one where a float import of the product is inexact")

	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "precision")
	customer := siteCustomer(t, db, site, "precision-customer")
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("cost_factor", cost).Error)
	InvalidateAdtySiteCache()
	sitePrice(t, db, site.Id, 0, "", storedK.String())

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", nil)
	billed, k := AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", group, "")
	require.True(t, k.Equal(storedK))
	require.True(t, billed.Equal(sell), "got %s want %s", billed, sell)
	fromCtx, ok := AdtyBilledGroupRatioFromContext(ctx)
	require.True(t, ok)
	require.True(t, fromCtx.Equal(sell))

	// Markup ratio 0 => sell equals supply, with exact decimal (no float equality).
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(1)).Error)
	InvalidateAdtySitePriceCache()
	zeroMarkup, k := AdtyApplySiteMarkupDecimal(nil, customer.Id, "default", group, "")
	require.True(t, k.Equal(decimal.NewFromInt(1)))
	require.True(t, zeroMarkup.Equal(group.Mul(cost)), "got %s", zeroMarkup)

	// Restore the stored k used for the billed-amount proof.
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", storedK).Error)
	InvalidateAdtySitePriceCache()
	billed, _ = AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", group, "")
	require.True(t, billed.Equal(sell))

	// Final billed amount: only one rounding at the official helper.
	// 0.07 * 500000 * 0.0805 = 2817.5, which rounds half-away-from-zero to 2818.
	// The float product instead yields 2817.499... and would round to 2817.
	price := decimal.RequireFromString("0.07")
	exact := price.Mul(common.QuotaPerUnitDecimal()).Mul(sell)
	require.Equal(t, "2817.5", exact.String())
	wantQuota, err := common.QuotaFromDecimalStrict(exact)
	require.NoError(t, err)
	require.Equal(t, 2818, wantQuota)

	relayInfo := &relaycommon.RelayInfo{}
	relayInfo.PriceData = hosttypes.PriceData{
		UsePrice:   true,
		ModelPrice: 0.07,
		GroupRatioInfo: hosttypes.GroupRatioInfo{
			GroupRatio: sell.InexactFloat64(),
		},
	}
	relayInfo.PriceData.GroupRatioInfo.SetBilledGroupRatio(sell)
	summary := calculateTextQuotaSummary(ctx, relayInfo, &dto.Usage{PromptTokens: 1, TotalTokens: 1})
	assert.Equal(t, 2818, summary.Quota)

	floatOnly := hosttypes.GroupRatioInfo{GroupRatio: floatProduct}
	floatQuota, _ := common.QuotaFromDecimalChecked(price.Mul(common.QuotaPerUnitDecimal()).Mul(floatOnly.BilledGroupRatio()))
	require.NotEqual(t, wantQuota, floatQuota, "importing the float product would bill a different amount")
}

func TestSiteTeamFormulasHoldInDecimal(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "precision-team")
	group := decimal.RequireFromString("1.1")
	cost := decimal.RequireFromString("0.7")
	storedK := decimal.RequireFromString("1.15")
	supply := group.Mul(cost)
	sell := supply.Mul(storedK)
	require.Equal(t, "0.77", supply.String())
	require.Equal(t, "0.8855", sell.String())
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("cost_factor", cost).Error)
	InvalidateAdtySiteCache()
	sitePrice(t, db, site.Id, 0, "", storedK.String())

	t.Run("discounted below supply is billed at supply and not rejected", func(t *testing.T) {
		customer := siteCustomer(t, db, site, "precision-team-below")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Below", "0.7", "precision-team-below-key")
		billed, k := AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", group, "")
		require.True(t, k.Equal(storedK))
		require.True(t, billed.Equal(supply), "got %s want supply %s (sell*0.7=%s)", billed, supply, sell.Mul(decimal.RequireFromString("0.7")))
		assert.False(t, AdtyTeamPriceBlocked(ctx))
	})

	t.Run("discounted above supply is billed at the exact discounted price", func(t *testing.T) {
		customer := siteCustomer(t, db, site, "precision-team-above")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Above", "0.95", "precision-team-above-key")
		want := sell.Mul(decimal.RequireFromString("0.95"))
		billed, _ := AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", group, "")
		require.True(t, billed.Equal(want), "got %s want %s", billed, want)
		assert.True(t, billed.GreaterThan(supply))
		assert.False(t, AdtyTeamPriceBlocked(ctx))
	})

	t.Run("main-site team has no supply floor", func(t *testing.T) {
		leader := referralUser(t, db, "precision-team-main")
		require.NoError(t, db.Model(leader).Update("site_id", 0).Error)
		ctx := bindSiteTeamKey(t, db, leader, 0, "Main", "0.5", "precision-team-main-key")
		billed, k := AdtyApplySiteMarkupDecimal(ctx, leader.Id, "default", group, "")
		require.True(t, k.Equal(decimal.NewFromInt(1)))
		require.True(t, billed.Equal(group.Mul(decimal.RequireFromString("0.5"))), "got %s", billed)
		assert.True(t, billed.LessThan(supply))
		assert.False(t, AdtyTeamPriceBlocked(ctx))
	})

	t.Run("invalid pay ratio still fails closed", func(t *testing.T) {
		customer := siteCustomer(t, db, site, "precision-team-invalid")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Invalid", "0.95", "precision-team-invalid-key")
		require.NoError(t, db.Model(&model.AdtyTeam{}).Where("leader_user_id = ?", customer.Id).Update("discount", decimal.RequireFromString("1.50")).Error)
		billed, _ := AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", group, "")
		require.True(t, billed.Equal(sell), "got %s", billed)
		assert.True(t, AdtyTeamPriceBlocked(ctx))
	})
}

func TestSiteSellPriceOverflowFailsClosed(t *testing.T) {
	group := decimal.RequireFromString("1.5")
	huge := decimal.RequireFromString("1e400")
	billed, k, ok := adtySiteOrdinarySellRatioDec(group, decimal.NewFromInt(1), huge, 1, "default")
	require.False(t, ok)
	require.True(t, billed.Equal(group), "overflow must leave the group ratio unchanged, got %s", billed)
	require.True(t, k.Equal(decimal.NewFromInt(1)))
}
