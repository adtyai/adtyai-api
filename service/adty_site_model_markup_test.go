package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func siteModelPrice(t *testing.T, siteId, userId int, group, modelName, markup string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.AdtySitePrice{
		SiteId: siteId, UserId: userId, Group: group, Model: modelName,
		Markup: decimal.RequireFromString(markup), Version: 1,
	}).Error)
	InvalidateAdtySitePriceCache()
}

func TestSiteMarkupResolveOrderWithModelOverride(t *testing.T) {
	prices := []model.AdtySitePrice{
		{UserId: 0, Group: "", Model: "", Markup: decimal.RequireFromString("1.2")},
		{UserId: 0, Group: "", Model: "gpt-4", Markup: decimal.RequireFromString("1.5")},
		{UserId: 0, Group: "vip", Model: "", Markup: decimal.RequireFromString("1.3")},
		{UserId: 7, Group: "", Model: "", Markup: decimal.RequireFromString("1.1")},
		{UserId: 7, Group: "", Model: "gpt-4", Markup: decimal.RequireFromString("1.8")},
	}
	for _, tc := range []struct {
		name, group, model, want string
		user                     int
	}{
		{name: "global applies when the model has no override", user: 8, group: "default", model: "claude-3", want: "1.2"},
		{name: "per-model override covers only that model", user: 8, group: "default", model: "gpt-4", want: "1.5"},
		{name: "unlisted model inherits global", user: 8, group: "default", model: "new-model", want: "1.2"},
		{name: "empty model name uses the global row", user: 8, group: "default", model: "", want: "1.2"},
		{name: "group default beats a less specific per-model row", user: 8, group: "vip", model: "gpt-4", want: "1.3"},
		{name: "group default still applies to models without a group override", user: 8, group: "vip", model: "claude-3", want: "1.3"},
		{name: "user default beats site-level per-model", user: 7, group: "default", model: "claude-3", want: "1.1"},
		{name: "user per-model at the same specificity overrides the user default", user: 7, group: "default", model: "gpt-4", want: "1.8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveAdtySiteMarkup(prices, tc.user, tc.group, tc.model)
			assert.True(t, got.Equal(decimal.RequireFromString(tc.want)), "got %s", got)
		})
	}
}

func TestPerModelMarkupBillingDisplayAndDeleteRestore(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.BillingOperation{}, &model.BillingOperationMovement{}))
	_, site := openActiveSite(t, db, "model-k")
	customer := siteCustomer(t, db, site, "model-k-customer")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)

	now := time.Now().Unix()
	first, err := savePricing(t, site.Id, 0, "1.2", nil, 0, now)
	require.NoError(t, err)
	_, err = ApplyDueAdtySiteMarkups(now + 24*3600)
	require.NoError(t, err)
	var afterGlobal model.AdtySitePrice
	require.NoError(t, db.Where("site_id = ? AND user_id = 0 AND \"group\" = ? AND model = ?", site.Id, "", "").First(&afterGlobal).Error)

	saved, err := SaveAdtySitePricing(AdtySitePricingInput{
		SiteId: site.Id, ExpectedVersion: afterGlobal.Version, Now: now + 24*3600,
		ModelMarkups: map[string]decimal.Decimal{"gpt-4": decimal.RequireFromString("1.5")},
	})
	require.NoError(t, err)
	require.Contains(t, saved.ModelPending, "gpt-4")
	applied, err := ApplyDueAdtySiteMarkups(now + 48*3600)
	require.NoError(t, err)
	assert.EqualValues(t, 1, applied)

	assert.True(t, AdtySiteMarkup(customer.Id, "default", "").Equal(decimal.RequireFromString("1.2")))
	assert.True(t, AdtySiteMarkup(customer.Id, "default", "claude-3").Equal(decimal.RequireFromString("1.2")), "models without an override inherit global")
	assert.True(t, AdtySiteMarkup(customer.Id, "default", "gpt-4").Equal(decimal.RequireFromString("1.5")))
	assert.True(t, AdtySiteMarkup(customer.Id, "default", "brand-new").Equal(decimal.RequireFromString("1.2")), "a new model inherits global without an auto-inserted row")
	var overrideCount int64
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ? AND model = ?", site.Id, "brand-new").Count(&overrideCount).Error)
	assert.Zero(t, overrideCount)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set(AdtySiteMarkupContextKey, 9.9)
	billed, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", 1, "gpt-4")
	assert.InDelta(t, 1.5, k, 1e-12)
	assert.InDelta(t, 1.2, billed, 1e-12, "1 * 0.8 * 1.5")
	assert.InDelta(t, 1.5, ctx.GetFloat64(AdtySiteMarkupContextKey), 1e-12, "a client-supplied markup in context cannot bypass the backend")

	other, kOther := AdtyApplySiteMarkup(nil, customer.Id, "default", 1, "claude-3")
	assert.InDelta(t, 1.2, kOther, 1e-12)
	assert.InDelta(t, 0.96, other, 1e-12, "1 * 0.8 * 1.2")

	pricing := []model.Pricing{
		{ModelName: "gpt-4", ModelPrice: 2, ModelRatio: ptrFloat(1)},
		{ModelName: "claude-3", ModelPrice: 2, ModelRatio: ptrFloat(1)},
	}
	req, _ := gin.CreateTestContext(httptest.NewRecorder())
	req.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	req.Set("id", customer.Id)
	adjusted := AdtyAdjustPricingForSiteModels(req, "default", pricing)
	require.Len(t, adjusted, 2)
	assert.InDelta(t, 2.5, adjusted[0].ModelPrice, 1e-12, "gpt-4 scales by 1.5/1.2")
	assert.InDelta(t, 1.25, *adjusted[0].ModelRatio, 1e-12)
	assert.InDelta(t, 2, adjusted[1].ModelPrice, 1e-12, "claude-3 keeps the global markup already in group_ratio")
	assert.InDelta(t, 1, *adjusted[1].ModelRatio, 1e-12)
	assert.Equal(t, float64(2), pricing[0].ModelPrice, "the cached pricing slice is not mutated")

	snapCtx, info := snapshotRelay(t, customer.Id, "default", hosttypes.PriceData{
		ModelPrice:     2,
		UsePrice:       true,
		GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
	})
	info.OriginModelName = "gpt-4"
	snapBilled, storedK := AdtyApplySiteMarkupDecimal(snapCtx, customer.Id, "default", decimal.NewFromInt(1), "gpt-4")
	require.True(t, storedK.Equal(decimal.RequireFromString("1.5")))
	info.PriceData.GroupRatioInfo.GroupRatio = snapBilled.InexactFloat64()
	info.PriceData.GroupRatioInfo.SetBilledGroupRatio(snapBilled)
	AdtyFreezeRequestPriceSnapshot(snapCtx, info)
	snap := AdtyFrozenPriceSnapshot(snapCtx)
	require.NotNil(t, snap)
	assert.Equal(t, "0.5", snap.SiteMarkupRatio, "the snapshot stores the effective per-model markup ratio")
	assert.Equal(t, "1.5", snap.StoredMarkupK)

	costBefore, ok := AdtySiteCostFactor(site)
	require.True(t, ok)
	var version int64
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ? AND user_id = 0", site.Id).Select("MAX(version)").Scan(&version).Error)
	deleted, err := SaveAdtySitePricing(AdtySitePricingInput{
		SiteId: site.Id, ExpectedVersion: version, Now: now + 48*3600,
		ModelMarkups: map[string]decimal.Decimal{"gpt-4": decimal.Zero},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-4"}, deleted.ModelImmediate)
	assert.Empty(t, deleted.ModelPending)
	assert.True(t, AdtySiteMarkup(customer.Id, "default", "gpt-4").Equal(decimal.RequireFromString("1.2")), "deleting the override restores global immediately")
	restored, kRestored := AdtyApplySiteMarkup(nil, customer.Id, "default", 1, "gpt-4")
	assert.InDelta(t, 1.2, kRestored, 1e-12)
	assert.InDelta(t, 0.96, restored, 1e-12, "no stale cached per-model price remains")
	var leftover int64
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ? AND model = ?", site.Id, "gpt-4").Count(&leftover).Error)
	assert.Zero(t, leftover)
	costAfter, ok := AdtySiteCostFactor(site)
	require.True(t, ok)
	assert.True(t, costAfter.Equal(costBefore), "saving model markup cannot change the supply ratio")
	_ = first
}

func TestSaveAdtySitePricingRejectsInvalidModelNameAndDoesNotTouchCostFactor(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "model-bad")
	var stored model.AdtySite
	require.NoError(t, db.First(&stored, site.Id).Error)
	cost := stored.CostFactor

	_, err := SaveAdtySitePricing(AdtySitePricingInput{
		SiteId: site.Id, ExpectedVersion: 0, Now: 1,
		ModelMarkups: map[string]decimal.Decimal{"   ": decimal.RequireFromString("1.2")},
	})
	require.Error(t, err)

	_, err = SaveAdtySitePricing(AdtySitePricingInput{
		SiteId: site.Id, ExpectedVersion: 0, Now: 1,
		ModelMarkups: map[string]decimal.Decimal{string(make([]byte, 192)): decimal.RequireFromString("1.2")},
	})
	require.Error(t, err)

	var after model.AdtySite
	require.NoError(t, db.First(&after, site.Id).Error)
	assert.Equal(t, cost, after.CostFactor)
}

func TestPerModelMarkupIncreaseStillUsesExistingDelay(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "model-delay")
	customer := siteCustomer(t, db, site, "model-delay-customer")
	now := time.Now().Unix()
	first, err := savePricing(t, site.Id, 0, "1.2", nil, 0, now)
	require.NoError(t, err)
	_, err = ApplyDueAdtySiteMarkups(now + 24*3600)
	require.NoError(t, err)
	var version int64
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ? AND user_id = 0", site.Id).Select("MAX(version)").Scan(&version).Error)

	raised, err := SaveAdtySitePricing(AdtySitePricingInput{
		SiteId: site.Id, ExpectedVersion: version, Now: now + 24*3600,
		ModelMarkups: map[string]decimal.Decimal{"gpt-4": decimal.RequireFromString("1.5")},
	})
	require.NoError(t, err)
	require.Contains(t, raised.ModelPending, "gpt-4")
	assert.True(t, AdtySiteMarkup(customer.Id, "default", "gpt-4").Equal(decimal.RequireFromString("1.2")), "a higher per-model markup waits for the existing delay")
	_ = first
}

func TestHandleGroupRatioUsesOriginModelMarkup(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "model-hook")
	customer := siteCustomer(t, db, site, "model-hook-customer")
	sitePrice(t, db, site.Id, 0, "", "1.2")
	siteModelPrice(t, site.Id, 0, "", "gpt-4", "1.5")

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 0)
	info := &relaycommon.RelayInfo{UserId: customer.Id, UsingGroup: "default", OriginModelName: "gpt-4"}
	// Exercise the production hook signature used by HandleGroupRatio.
	ratio, k := AdtyApplySiteMarkupHook(ctx, info.UserId, info.UsingGroup, 1, info.OriginModelName)
	assert.InDelta(t, 1.5, k, 1e-12)
	assert.InDelta(t, 1.2, ratio, 1e-12)
}

func ptrFloat(v float64) *float64 { return &v }
