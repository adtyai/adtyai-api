package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestPriceSnapshotOfficialFieldsAtReserve(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.BillingOperation{}, &model.BillingOperationMovement{}))
	_, site := openActiveSite(t, db, "snap-reserve")
	customer := siteCustomer(t, db, site, "snap-reserve-customer")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)
	sitePrice(t, db, site.Id, 0, "", "1.25")

	ctx, info := snapshotRelay(t, customer.Id, "default", hosttypes.PriceData{
		ModelPrice:     2,
		UsePrice:       true,
		GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
	})
	billed, storedK := AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", decimal.NewFromInt(1))
	require.True(t, storedK.Equal(decimal.RequireFromString("1.25")))
	info.PriceData.GroupRatioInfo.GroupRatio = billed.InexactFloat64()
	info.PriceData.GroupRatioInfo.SetBilledGroupRatio(billed)
	AdtyFreezeRequestPriceSnapshot(ctx, info)

	snap := AdtyFrozenPriceSnapshot(ctx)
	require.NotNil(t, snap)
	require.True(t, snap.HasOfficialFields())
	assert.Equal(t, "2", snap.ModelBasePrice)
	assert.Equal(t, "default", snap.BillingGroup)
	assert.Equal(t, "1", snap.GroupRatio)
	assert.Equal(t, "0.8", snap.SiteCostFactor)
	assert.Equal(t, "0.25", snap.SiteMarkupRatio)
	assert.Equal(t, "", snap.TeamPayRatio)
	assert.False(t, snap.TeamApplied)
	assert.Equal(t, "1.6", snap.SupplyCost)
	assert.Equal(t, "2", snap.SellPrice)
	assert.Equal(t, "2", snap.FinalPrice)
	assert.Equal(t, model.RequestPriceCurrencyUSD, snap.Currency)
	assert.Equal(t, model.BillingUnitPerCall, snap.BillingUnit)
	assert.Equal(t, model.RequestPriceSnapshotVersion, snap.RuleVersion)
	assert.NotZero(t, snap.AcceptedAt)
	assert.False(t, snap.MainSite)
	assert.Equal(t, site.Id, snap.SiteId)

	session, apiErr := NewBillingSession(ctx, info, 1_000_000)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	row, err := model.GetBillingOperation(session.operationKey)
	require.NoError(t, err)
	stored := row.RequestPriceSnapshot()
	require.NotNil(t, stored)
	assert.Equal(t, snap.SiteMarkupRatio, stored.SiteMarkupRatio)
	assert.Equal(t, snap.SiteCostFactor, stored.SiteCostFactor)
	assert.Equal(t, snap.FinalPrice, stored.FinalPrice)
	assert.Equal(t, "1.25", stored.StoredMarkupK)
	assert.InDelta(t, 1.25, row.SiteMarkup, 1e-12)
	assert.InDelta(t, 0.8, row.SiteCostFactor, 1e-12)
}

func TestRequestPriceSnapshotIgnoresLivePriceChangesOnRetryAndSettle(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.Task{}, &model.Channel{}, &model.BillingOperation{}, &model.BillingOperationMovement{}))
	_, site := openActiveSite(t, db, "snap-retry")
	customer := siteCustomer(t, db, site, "snap-retry-customer")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)
	sitePrice(t, db, site.Id, 0, "", "1.25")

	savedRatios, savedGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-retry-model":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	ctx, info := snapshotRelay(t, customer.Id, "default", hosttypes.PriceData{ModelRatio: 2})
	info.OriginModelName = "snap-retry-model"
	billed, _ := AdtyApplySiteMarkupDecimal(ctx, customer.Id, "default", decimal.NewFromInt(1))
	info.PriceData.GroupRatioInfo.SetBilledGroupRatio(billed)
	AdtyFreezeRequestPriceSnapshot(ctx, info)
	snap := AdtyFrozenPriceSnapshot(ctx)
	require.NotNil(t, snap)
	assert.Equal(t, "2", snap.ModelBasePrice)
	assert.Equal(t, "0.25", snap.SiteMarkupRatio)
	assert.Equal(t, "0.8", snap.SiteCostFactor)
	assert.Equal(t, "2", snap.FinalPrice)
	frozenBilled := snap.BilledGroupRatio

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-retry-model":9}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":3}`))
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(3)).Error)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("cost_factor", decimal.RequireFromString("0.5")).Error)
	InvalidateAdtySitePriceCache()
	InvalidateAdtySiteCache()

	live, _ := AdtyApplySiteMarkupDecimal(nil, customer.Id, "default", decimal.NewFromInt(3))
	assert.False(t, live.Equal(billed), "live config changed")
	AdtyFreezeRequestPriceSnapshot(ctx, info)
	unchanged := AdtyFrozenPriceSnapshot(ctx)
	require.NotNil(t, unchanged)
	assert.Equal(t, snap.ModelBasePrice, unchanged.ModelBasePrice)
	assert.Equal(t, snap.SiteMarkupRatio, unchanged.SiteMarkupRatio)
	assert.Equal(t, snap.SiteCostFactor, unchanged.SiteCostFactor)
	assert.Equal(t, snap.FinalPrice, unchanged.FinalPrice)
	reused := GroupRatioInfoFromSnapshot(unchanged)
	assert.Equal(t, frozenBilled, reused.BilledGroupRatio().String())

	task := &model.Task{TaskID: "snap-retry-task", UserId: customer.Id, Group: "default", Status: model.TaskStatusSuccess}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "snap-retry-model",
		ModelRatio:      2,
		GroupRatio:      1,
		SiteMarkup:      1.25,
		SiteCostFactor:  0.8,
		PriceSnapshot:   snap,
	}
	require.NoError(t, db.Create(task).Error)
	require.True(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000))
	assert.Equal(t, 2000, task.Quota, "1000 tokens × frozen model 2 × frozen billed group 1")
}

func TestRequestPriceSnapshotDecimalAuthority(t *testing.T) {
	precise := "0.123456789012345678"
	snap := &model.RequestPriceSnapshot{
		SchemaVersion:    model.RequestPriceSnapshotVersion,
		ModelBasePrice:   "1.15",
		BillingGroup:     "vip",
		GroupRatio:       "1.1",
		SiteCostFactor:   "0.7",
		SiteMarkupRatio:  "0.15",
		FinalPrice:       precise,
		Currency:         model.RequestPriceCurrencyUSD,
		BillingUnit:      model.BillingUnitPerToken,
		AcceptedAt:       time.Now().Unix(),
		RuleVersion:      model.RequestPriceSnapshotVersion,
		SupplyCost:       "0.8855",
		SellPrice:        "1.018325",
		BilledGroupRatio: precise,
		ModelRatio:       "1.15",
		StoredMarkupK:    "1.15",
	}
	require.True(t, snap.HasOfficialFields())
	encoded, err := model.EncodeRequestPriceSnapshot(snap)
	require.NoError(t, err)
	decoded := model.DecodeRequestPriceSnapshot(encoded)
	require.NotNil(t, decoded)
	final, ok := model.ParseSnapshotDecimal(decoded.FinalPrice)
	require.True(t, ok)
	assert.Equal(t, precise, final.String())
	assert.NotEqual(t, precise, decimal.NewFromFloat(final.InexactFloat64()).String(), "authority must not be reconstructed only from float64")
	billed, ok := decoded.BilledGroupRatioDecimal()
	require.True(t, ok)
	assert.True(t, billed.Equal(decimal.RequireFromString(precise)))

	info := applySnapshotToQuotaInfo(QuotaInfo{ModelRatio: 99, GroupRatio: 99}, decoded)
	assert.True(t, info.hasModelRatioDecimal)
	assert.True(t, info.hasGroupRatioDecimal)
	assert.True(t, info.ModelRatioDecimal.Equal(decimal.RequireFromString("1.15")))
	assert.True(t, info.GroupRatioDecimal.Equal(decimal.RequireFromString(precise)))
}

func TestRequestPriceSnapshotMainSiteAndTeamRepresentable(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	_, site := openActiveSite(t, db, "snap-repr")
	sitePrice(t, db, site.Id, 0, "", "1.2")
	mainUser := referralUser(t, db, "snap-repr-main")
	siteUser := siteCustomer(t, db, site, "snap-repr-site")
	mainTeam := bindSiteTeamKey(t, db, mainUser, 0, "MainSnap", "0.8", "snap-repr-main-key")
	siteTeam := bindSiteTeamKey(t, db, siteUser, site.Id, "SiteSnap", "0.5", "snap-repr-site-key")

	t.Run("main-site non-team", func(t *testing.T) {
		ctx, info := snapshotRelay(t, mainUser.Id, "default", hosttypes.PriceData{ModelRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1.5}})
		billed, _ := AdtyApplySiteMarkupDecimal(ctx, mainUser.Id, "default", decimal.RequireFromString("1.5"))
		info.PriceData.GroupRatioInfo.SetBilledGroupRatio(billed)
		AdtyFreezeRequestPriceSnapshot(ctx, info)
		snap := AdtyFrozenPriceSnapshot(ctx)
		require.NotNil(t, snap)
		assert.True(t, snap.MainSite)
		assert.Equal(t, "1", snap.SiteCostFactor)
		assert.Equal(t, "0", snap.SiteMarkupRatio)
		assert.False(t, snap.TeamApplied)
		assert.Equal(t, "", snap.TeamPayRatio)
		assert.Equal(t, "", snap.SupplyPriceFloor)
		assert.Equal(t, "3", snap.FinalPrice)
	})

	t.Run("main-site team has no supply floor", func(t *testing.T) {
		info := &relaycommon.RelayInfo{UserId: mainUser.Id, UsingGroup: "default", StartTime: time.Now(), PriceData: hosttypes.PriceData{ModelRatio: 2}}
		billed, _ := AdtyApplySiteMarkupDecimal(mainTeam, mainUser.Id, "default", decimal.RequireFromString("1.5"))
		info.PriceData.GroupRatioInfo.SetBilledGroupRatio(billed)
		AdtyFreezeRequestPriceSnapshot(mainTeam, info)
		snap := AdtyFrozenPriceSnapshot(mainTeam)
		require.NotNil(t, snap)
		assert.True(t, snap.MainSite)
		assert.True(t, snap.TeamApplied)
		assert.Equal(t, "0.8", snap.TeamPayRatio)
		assert.Equal(t, "", snap.SupplyPriceFloor)
		assert.Equal(t, "2.4", snap.FinalPrice) // 2 * 1.5 * 0.8
	})

	t.Run("site team records floor and final at supply", func(t *testing.T) {
		info := &relaycommon.RelayInfo{UserId: siteUser.Id, UsingGroup: "default", StartTime: time.Now(), PriceData: hosttypes.PriceData{ModelRatio: 2}}
		billed, _ := AdtyApplySiteMarkupDecimal(siteTeam, siteUser.Id, "default", decimal.RequireFromString("1.5"))
		info.PriceData.GroupRatioInfo.SetBilledGroupRatio(billed)
		AdtyFreezeRequestPriceSnapshot(siteTeam, info)
		snap := AdtyFrozenPriceSnapshot(siteTeam)
		require.NotNil(t, snap)
		assert.False(t, snap.MainSite)
		assert.True(t, snap.TeamApplied)
		assert.Equal(t, "0.5", snap.TeamPayRatio)
		assert.Equal(t, "0.2", snap.SiteMarkupRatio)
		assert.Equal(t, "0.8", snap.SiteCostFactor)
		// supply = 2 * 1.5 * 0.8 = 2.4; sell = 2.4 * 1.2 = 2.88; discounted = 1.44; final = max(2.4, 1.44) = 2.4
		assert.Equal(t, "2.4", snap.SupplyCost)
		assert.Equal(t, "2.4", snap.SupplyPriceFloor)
		assert.Equal(t, "2.4", snap.FinalPrice)
	})
}

func TestOldTaskWithoutSnapshotKeepsStoredRatio(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Task{}, &model.Channel{}))
	_, site := openActiveSite(t, db, "snap-old")
	customer := siteCustomer(t, db, site, "snap-old-customer")
	sitePrice(t, db, site.Id, 0, "", "1.5")
	savedRatios, savedGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-old-model":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":4}`))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)

	task := &model.Task{TaskID: "snap-old-task", UserId: customer.Id, Group: "default", Status: model.TaskStatusSuccess}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "snap-old-model", ModelRatio: 2, GroupRatio: 1.2}
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(3)).Error)
	InvalidateAdtySitePriceCache()
	require.True(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000))
	assert.Equal(t, 2400, task.Quota, "old rows must keep the stored billed ratio, not live markup")
}

func snapshotRelay(t *testing.T, userId int, group string, price hosttypes.PriceData) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 0)
	info := &relaycommon.RelayInfo{
		UserId:     userId,
		UsingGroup: group,
		RequestId:  "snap-" + group,
		StartTime:  time.Now(),
		PriceData:  price,
	}
	return ctx, info
}
