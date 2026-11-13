package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 分站定价：按供货倍率和加价比例计费（ISS-R1-0009 / G.2）。

func sitePrice(t *testing.T, db *gorm.DB, siteId, userId int, group, markup string) {
	t.Helper()
	require.NoError(t, db.Create(&model.AdtySitePrice{SiteId: siteId, UserId: userId, Group: group, Markup: decimal.RequireFromString(markup), Version: 1}).Error)
	InvalidateAdtySitePriceCache()
}

func TestSiteMarkupResolveOrder(t *testing.T) {
	prices := []model.AdtySitePrice{
		{UserId: 7, Group: "vip", Markup: decimal.RequireFromString("2.0")},
		{UserId: 7, Group: "", Markup: decimal.RequireFromString("1.5")},
		{UserId: 0, Group: "vip", Markup: decimal.RequireFromString("1.3")},
		{UserId: 0, Group: "", Markup: decimal.RequireFromString("1.2")},
		{UserId: 9, Group: "default", Markup: decimal.RequireFromString("1.8")},
	}
	for _, tc := range []struct {
		name   string
		prices []model.AdtySitePrice
		user   int
		group  string
		want   string
	}{
		{"customer + group", prices, 7, "vip", "2"},
		{"customer", prices, 7, "default", "1.5"},
		{"site + group", prices, 8, "vip", "1.3"},
		{"site", prices, 8, "default", "1.2"},
		{"customer group beats site group", prices, 9, "default", "1.8"},
		{"customer default beats site group", prices[1:], 7, "vip", "1.5"},
		{"nothing set", nil, 7, "vip", "1"},
		{"stored k below 1 is used", []model.AdtySitePrice{{UserId: 0, Group: "", Markup: decimal.RequireFromString("0.9")}}, 8, "default", "0.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveAdtySiteMarkup(tc.prices, tc.user, tc.group)
			assert.True(t, got.Equal(decimal.RequireFromString(tc.want)), "got %s", got)
		})
	}
}

func TestHandleGroupRatioAppliesMarkup(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "ratio")
	customer := siteCustomer(t, db, site, "ratio-customer")
	mainUser := referralUser(t, db, "ratio-main")
	sitePrice(t, db, site.Id, 0, "", "1.2")

	for _, tc := range []struct {
		name   string
		user   int
		status string
		want   float64
		k      float64
	}{
		{"site customer", customer.Id, model.AdtySiteStatusActive, 0.96, 1.2},
		{"paused site keeps the site price", customer.Id, model.AdtySiteStatusPaused, 0.96, 1.2},
		{"main-site customer", mainUser.Id, model.AdtySiteStatusActive, 1, 1},
		{"closed site", customer.Id, model.AdtySiteStatusClosed, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", tc.status).Error)
			InvalidateAdtySiteCache()
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ratio, k := AdtyApplySiteMarkup(ctx, tc.user, "default", 1.0)
			assert.InDelta(t, tc.want, ratio, 1e-12)
			assert.InDelta(t, tc.k, k, 1e-12)
			if tc.k > 1 {
				assert.InDelta(t, 1.2, ctx.GetFloat64(AdtySiteMarkupContextKey), 1e-12)
				assert.InDelta(t, 0.8, ctx.GetFloat64(AdtySiteCostFactorContextKey), 1e-12)
			} else {
				_, _, ok := AdtySiteMarkupFromContext(ctx)
				assert.Equal(t, tc.status != model.AdtySiteStatusClosed && tc.user == customer.Id, ok)
			}
		})
	}
}

// markupBillingCase 跑一次扣费，返回扣掉的额度。
type markupBillingCase struct {
	name string
	run  func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int
}

// TestMarkupAppliedOnEveryBillingPath：同一个分站客户和主站客户，同样的用量走各条计费路径，
// 分站客户扣的都是主站客户的 供货倍率×(1+加价比例) 倍，也就是存储 k × d。
func TestMarkupAppliedOnEveryBillingPath(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Channel{}, &model.Task{}))
	_, site := openActiveSite(t, db, "paths")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	const k, d = 1.25, 0.8
	const sellFactor = k * d

	savedRatios, savedPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	savedGrok := *model_setting.GetGrokSettings()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
		*model_setting.GetGrokSettings() = savedGrok
		operation_setting.DeleteToolPriceForTest("markup_tool")
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"markup-text":2,"markup-task":3}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"markup-call":0.02}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	model_setting.GetGrokSettings().ViolationDeductionEnabled = true
	model_setting.GetGrokSettings().ViolationDeductionAmount = 0.04
	operation_setting.SetToolPriceForTest("markup_tool", 4)

	const tieredExpr = `tier("base", p * 2 + c * 6)`
	newInfo := func(ctx *gin.Context, user *model.User, token *model.Token, modelName string) *relaycommon.RelayInfo {
		info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, ChannelMeta: &relaycommon.ChannelMeta{},
			OriginModelName: modelName, UsingGroup: "default", UserGroup: "default", StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAI,
			UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
		info.PriceData.GroupRatioInfo = handleGroupRatioForTest(ctx, info)
		return info
	}
	cases := []markupBillingCase{
		{"ratio text", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-text")
			info.PriceData.ModelRatio, info.PriceData.CompletionRatio = 2, 1
			PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 1000, CompletionTokens: 1000, TotalTokens: 2000}, nil)
			return 0
		}},
		{"per call", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-call")
			info.PriceData.UsePrice, info.PriceData.ModelPrice = true, 0.02
			PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20}, nil)
			return 0
		}},
		{"tiered expression", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-tiered")
			group := info.PriceData.GroupRatioInfo.GroupRatio
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: tieredExpr, ExprHash: billingexpr.ExprHashString(tieredExpr),
				QuotaPerUnit: common.QuotaPerUnit, GroupRatio: group, EstimatedTier: "base"}
			PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 100000, CompletionTokens: 20000, TotalTokens: 120000}, nil)
			return 0
		}},
		{"image", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-call")
			info.PriceData.UsePrice, info.PriceData.ModelPrice = true, 0.02
			info.PriceData.AddOtherRatio("n", 3)
			PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 10, TotalTokens: 10}, nil)
			return 0
		}},
		{"tool surcharge", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-text")
			info.PriceData.ModelRatio, info.PriceData.CompletionRatio = 2, 1
			info.ResponsesUsageInfo = &relaycommon.ResponsesUsageInfo{BuiltInTools: map[string]*relaycommon.BuildInToolInfo{"markup_tool": {CallCount: 5}}}
			PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 1, TotalTokens: 1}, nil)
			return 0
		}},
		{"violation fee", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-text")
			charged := ChargeViolationFeeIfNeeded(ctx, info, types.NewError(fmt.Errorf("blocked"), types.ErrorCodeViolationFeeGrokCSAM))
			require.True(t, charged)
			return 0
		}},
		{"realtime pre-consume", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, OriginModelName: "markup-text",
				UsingGroup: "default", UserGroup: "default", StartTime: time.Now()}
			usage := &dto.RealtimeUsage{InputTokens: 1000, OutputTokens: 1000, TotalTokens: 2000}
			usage.InputTokenDetails.TextTokens, usage.OutputTokenDetails.TextTokens = 1000, 1000
			require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
			return 0
		}},
		{"task submit", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			info := newInfo(ctx, user, token, "markup-call")
			quota := common.QuotaRound(0.02 * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio)
			require.NoError(t, PostConsumeQuota(info, quota, 0, false))
			return 0
		}},
		{"task recalculated by tokens", func(t *testing.T, ctx *gin.Context, user *model.User, token *model.Token) int {
			task := &model.Task{TaskID: fmt.Sprintf("recalc-%d", user.Id), UserId: user.Id, Group: "default", Quota: 0, Status: model.TaskStatusSuccess}
			task.PrivateData.TokenId = token.Id
			task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "markup-task"}
			require.NoError(t, db.Create(task).Error)
			require.True(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000))
			return 0
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			charged := map[bool]int{}
			for _, onSite := range []bool{false, true} {
				name := fmt.Sprintf("path-%s-%t", tc.name, onSite)
				var user *model.User
				if onSite {
					user = siteCustomer(t, db, site, name)
				} else {
					user = referralUser(t, db, name)
				}
				const start = 100_000_000
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", start).Error)
				token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("key%d", user.Id), Name: "t", RemainQuota: start, UnlimitedQuota: true, Status: common.TokenStatusEnabled}
				require.NoError(t, db.Create(token).Error)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				tc.run(t, ctx, user, token)
				var stored model.User
				require.NoError(t, db.First(&stored, user.Id).Error)
				charged[onSite] = start - stored.Quota
			}
			require.Positive(t, charged[false], "the main-site customer pays something")
			assert.InDelta(t, float64(charged[false])*sellFactor, float64(charged[true]), 1, "site %d vs main %d", charged[true], charged[false])
		})
	}
}

func TestMarkupNotAppliedTwiceOnTaskRecalc(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Task{}, &model.Channel{}))
	_, site := openActiveSite(t, db, "recalc")
	customer := siteCustomer(t, db, site, "recalc-customer")
	sitePrice(t, db, site.Id, 0, "", "1.5")
	savedRatios, savedGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"recalc-model":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)

	// 提交时 k = 1.5 已经乘进快照的分组倍率；之后站长把 k 改成 3，重算仍按 1.5，不会乘成 1.5 × 3 或 1.5²。
	task := &model.Task{TaskID: "recalc-snap", UserId: customer.Id, Group: "default", Status: model.TaskStatusSuccess}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "recalc-model", GroupRatio: 1.5, SiteMarkup: 1.5, SiteCostFactor: 0.8}
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(3)).Error)
	InvalidateAdtySitePriceCache()
	require.True(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000))
	assert.Equal(t, 3000, task.Quota, "1000 tokens × model 2 × snapshot 1.5")
}

func TestTaskRecalcKeepsFrozenModelRatio(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Task{}, &model.Channel{}))
	savedRatios, savedGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"frozen-model":10}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	user := &model.User{Username: "frozen-ratio", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 10_000_000}
	require.NoError(t, db.Create(user).Error)
	task := &model.Task{TaskID: "frozen-ratio", UserId: user.Id, Group: "default", Status: model.TaskStatusSuccess, Quota: 1000}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "frozen-model", ModelRatio: 2, GroupRatio: 1}
	require.NoError(t, db.Create(task).Error)
	require.True(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000))
	assert.Equal(t, 2000, task.Quota, "settlement must keep the model ratio frozen at submit time")
}

func TestCostFactorResolveOrder(t *testing.T) {
	prepareSiteDB(t)
	cfg := adty_setting.GetResellerSetting()
	cfg.CostFactorTiers = []adty_setting.ResellerCostTier{{MinMonthlyUSD: 500, CostFactor: 0.7}}
	for _, tc := range []struct {
		name string
		site model.AdtySite
		want string
	}{
		{"site override", model.AdtySite{CostFactor: dec("0.6"), TierCostFactor: dec("0.7")}, "0.6"},
		{"tier", model.AdtySite{TierCostFactor: dec("0.7")}, "0.7"},
		{"global", model.AdtySite{}, "0.8"},
		{"below the minimum uses the minimum", model.AdtySite{CostFactor: dec("0.3")}, "0.5"},
		{"above 1 is honored", model.AdtySite{CostFactor: dec("1.2")}, "1.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := AdtySiteCostFactor(&tc.site)
			require.True(t, ok)
			assert.True(t, got.Equal(decimal.RequireFromString(tc.want)), "got %s", got)
		})
	}
	cfg.CostFactorTiers = nil
	got, _ := AdtySiteCostFactor(&model.AdtySite{TierCostFactor: dec("0.7")})
	assert.True(t, got.Equal(decimal.RequireFromString("0.8")), "tiers switched off fall back to the global d")
	cfg.CostFactor = nil
	_, ok := AdtySiteCostFactor(&model.AdtySite{})
	assert.False(t, ok)
}

func TestMonthlyTierAssignment(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tiered")
	cfg := adty_setting.GetResellerSetting()

	changed, err := AssignAdtySiteCostTiers("2026-10", map[int]decimal.Decimal{site.Id: decimal.NewFromInt(900)})
	require.NoError(t, err)
	assert.Empty(t, changed, "no tiers configured: nothing is written")
	stored, _ := GetAdtySite(site.Id)
	assert.Nil(t, stored.TierCostFactor)
	assert.Empty(t, stored.TierMonth)

	cfg.CostFactorTiers = []adty_setting.ResellerCostTier{{MinMonthlyUSD: 0, CostFactor: 0.8}, {MinMonthlyUSD: 500, CostFactor: 0.75}, {MinMonthlyUSD: 1000, CostFactor: 0.7}}
	changed, err = AssignAdtySiteCostTiers("2026-10", map[int]decimal.Decimal{site.Id: decimal.NewFromInt(900)})
	require.NoError(t, err)
	require.Len(t, changed, 1)
	stored, _ = GetAdtySite(site.Id)
	require.NotNil(t, stored.TierCostFactor)
	assert.True(t, stored.TierCostFactor.Equal(decimal.RequireFromString("0.75")), "$900 lands in the $500 tier")
	assert.Equal(t, "2026-10", stored.TierMonth)
	d, _ := AdtySiteCostFactor(stored)
	assert.True(t, d.Equal(decimal.RequireFromString("0.75")))
}

func TestHigherTierCostFactorWaits(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tier-raise")
	cfg := adty_setting.GetResellerSetting()
	cfg.CostFactorTiers = []adty_setting.ResellerCostTier{{MinMonthlyUSD: 0, CostFactor: 0.9}}
	t.Cleanup(func() { cfg.CostFactorTiers = nil })

	_, err := AssignAdtySiteCostTiers("2026-10", map[int]decimal.Decimal{site.Id: decimal.NewFromInt(10)})
	require.NoError(t, err)
	stored, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	current, ok := AdtySiteCostFactor(stored)
	require.True(t, ok)
	assert.True(t, current.Equal(decimal.RequireFromString("0.8")), "a higher tier d must not bill immediately, got %s", current)
	require.NotNil(t, stored.PendingTierCostFactor)
	assert.True(t, stored.PendingTierCostFactor.Equal(decimal.RequireFromString("0.9")))
	assert.Greater(t, stored.PendingTierCostFactorAt, time.Now().Unix())

	applied, err := ApplyDueAdtySiteTierCostFactors(stored.PendingTierCostFactorAt)
	require.NoError(t, err)
	assert.Equal(t, int64(1), applied)
	stored, err = GetAdtySite(site.Id)
	require.NoError(t, err)
	current, ok = AdtySiteCostFactor(stored)
	require.True(t, ok)
	assert.True(t, current.Equal(decimal.RequireFromString("0.9")))
	assert.Nil(t, stored.PendingTierCostFactor)
}

func TestScheduledCostTierUsesLastMonthRevenue(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tier-job")
	cfg := adty_setting.GetResellerSetting()
	cfg.CostFactorTiers = []adty_setting.ResellerCostTier{{MinMonthlyUSD: 100, CostFactor: 0.72}}
	t.Cleanup(func() { cfg.CostFactorTiers = nil })
	month := time.Now().AddDate(0, -1, 0).Format("2006-01")
	require.NoError(t, db.Create(&model.AdtySiteEarningDaily{
		SiteId: site.Id, UserId: 1, Day: month + "-15", Kind: model.AdtySiteEarningUsage,
		Revenue: decimal.NewFromInt(200), Status: model.AdtySiteEarningPending,
	}).Error)

	require.NoError(t, RunAdtySiteCostTierAssignment(time.Now()))
	stored, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	require.NotNil(t, stored.TierCostFactor)
	assert.True(t, stored.TierCostFactor.Equal(decimal.RequireFromString("0.72")))
	assert.Equal(t, month, stored.TierMonth)

	want := reflect.ValueOf(startAdtySiteCostTierTask).Pointer()
	scheduled := false
	for _, task := range adtyMasterTasks {
		if reflect.ValueOf(task).Pointer() == want {
			scheduled = true
		}
	}
	assert.True(t, scheduled, "the monthly tier job must be started with the other master tasks")
}

func savePricing(t *testing.T, siteId, userId int, def string, groups map[string]string, version int64, now int64) (*AdtySitePricingResult, error) {
	t.Helper()
	input := AdtySitePricingInput{SiteId: siteId, UserId: userId, ExpectedVersion: version, Now: now, GroupMarkups: map[string]decimal.Decimal{}}
	if def != "" {
		input.DefaultMarkup = dec(def)
	}
	for group, value := range groups {
		input.GroupMarkups[group] = decimal.RequireFromString(value)
	}
	return SaveAdtySitePricing(input)
}

func TestMarkupReadClampsToCurrentLimit(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "cap-read")
	customer := siteCustomer(t, db, site, "cap-read-customer")
	sitePrice(t, db, site.Id, 0, "", "2.5")
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("2.5")))

	resellerSetting().MaxMarkup = new(1.5)
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.5")), "global cap is lower than the stored price")

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("max_markup", decimal.RequireFromString("2")).Error)
	InvalidateAdtySiteCache()
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.5")), "the lower of the site cap and the global cap wins")

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("max_markup", decimal.RequireFromString("1.2")).Error)
	InvalidateAdtySiteCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ratio, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", 2)
	assert.InDelta(t, 1.2, k, 1e-9)
	assert.InDelta(t, 1.92, ratio, 1e-9)
}

func TestMarkupIncreaseDelayed(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "raise")
	customer := siteCustomer(t, db, site, "raise-customer")
	now := time.Now().Unix()
	first, err := savePricing(t, site.Id, 0, "1.2", nil, 0, now)
	require.NoError(t, err)
	assert.Equal(t, now+24*3600, first.Pending[""], "the first price waits when the site already has customers")
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.NewFromInt(1)))
	appliedNow, err := ApplyDueAdtySiteMarkups(now + 24*3600)
	require.NoError(t, err)
	assert.EqualValues(t, 1, appliedNow)
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.2")))
	var afterFirst model.AdtySitePrice
	require.NoError(t, db.Where("site_id = ? AND user_id = 0 AND \"group\" = ?", site.Id, "").First(&afterFirst).Error)
	assert.Greater(t, afterFirst.Version, first.Version)

	var notified []model.AdtySitePrice
	saved := AdtyOnSiteMarkupIncrease
	AdtyOnSiteMarkupIncrease = func(_ *model.AdtySite, _ int, raised []model.AdtySitePrice) { notified = raised }
	t.Cleanup(func() { AdtyOnSiteMarkupIncrease = saved })
	raised, err := savePricing(t, site.Id, 0, "1.8", nil, afterFirst.Version, now)
	require.NoError(t, err)
	assert.Equal(t, now+24*3600, raised.Pending[""])
	require.Len(t, notified, 1, "customers are told about the increase")
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.2")), "the old price holds during the delay")

	applied, err := ApplyDueAdtySiteMarkups(now + 24*3600 - 1)
	require.NoError(t, err)
	assert.Zero(t, applied)
	applied, err = ApplyDueAdtySiteMarkups(now + 24*3600)
	require.NoError(t, err)
	assert.EqualValues(t, 1, applied)
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.8")))
	var stored model.AdtySitePrice
	require.NoError(t, db.Where("site_id = ? AND user_id = 0 AND \"group\" = ?", site.Id, "").First(&stored).Error)
	assert.Greater(t, stored.Version, raised.Version, "taking effect moves the pricing version")
	_, err = savePricing(t, site.Id, 0, "1.2", nil, raised.Version, now+24*3600)
	require.ErrorIs(t, err, ErrAdtySitePricingConflict)
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.8")))
}

func TestMarkupDecreaseImmediate(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "lower")
	first, err := savePricing(t, site.Id, 0, "2", map[string]string{"vip": "2.5"}, 0, 0)
	require.NoError(t, err)
	customer := siteCustomer(t, db, site, "lower-customer")
	lowered, err := savePricing(t, site.Id, 0, "1.4", map[string]string{"vip": "1.1"}, first.Version, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"", "vip"}, lowered.Immediate)
	assert.Empty(t, lowered.Pending)
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.4")))
	assert.True(t, AdtySiteMarkup(customer.Id, "vip").Equal(decimal.RequireFromString("1.1")))
}

func TestDecreaseCancelsPendingIncrease(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "cancel")
	now := time.Now().Unix()
	first, err := savePricing(t, site.Id, 0, "1.5", nil, 0, now)
	require.NoError(t, err)
	customer := siteCustomer(t, db, site, "cancel-customer")
	raised, err := savePricing(t, site.Id, 0, "2.5", nil, first.Version, now)
	require.NoError(t, err)
	lowered, err := savePricing(t, site.Id, 0, "1.3", nil, raised.Version, now)
	require.NoError(t, err)
	assert.Equal(t, []string{""}, lowered.Immediate)
	applied, err := ApplyDueAdtySiteMarkups(now + 48*3600)
	require.NoError(t, err)
	assert.Zero(t, applied, "the pending increase is gone")
	assert.True(t, AdtySiteMarkup(customer.Id, "default").Equal(decimal.RequireFromString("1.3")))
}

func TestMarkupBounds(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "bounds")
	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"-0.1", false}, {"0.99", true}, {"1", true}, {"3", true}, {"3.01", false},
	} {
		var version int64
		var rows []model.AdtySitePrice
		require.NoError(t, db.Where("site_id = ?", site.Id).Find(&rows).Error)
		for _, row := range rows {
			version = max(version, row.Version)
		}
		_, err := savePricing(t, site.Id, 0, "", map[string]string{"default": tc.value}, version, 0)
		if tc.ok {
			assert.NoError(t, err, tc.value)
		} else {
			assert.Error(t, err, tc.value)
		}
	}
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("max_markup", decimal.NewFromInt(2)).Error)
	_, err := savePricing(t, site.Id, 0, "2.5", nil, 0, 0)
	assert.Error(t, err, "the per-site limit applies")

	_, err = SetAdtySiteStatus(site.Id, 1, model.AdtySiteStatusPaused, "")
	assert.ErrorIs(t, err, ErrAdtySitePauseRetired)
	var still model.AdtySite
	require.NoError(t, db.First(&still, site.Id).Error)
	assert.Equal(t, model.AdtySiteStatusActive, still.Status)

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusPaused).Error)
	var version int64
	var rows []model.AdtySitePrice
	require.NoError(t, db.Where("site_id = ?", site.Id).Find(&rows).Error)
	for _, row := range rows {
		version = max(version, row.Version)
	}
	_, err = savePricing(t, site.Id, 0, "1.1", nil, version, 0)
	assert.NoError(t, err, "leftover paused rows must not freeze pricing")
}

func TestPricingVersionConflict(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "version")
	first, err := savePricing(t, site.Id, 0, "1.2", nil, 0, 0)
	require.NoError(t, err)
	_, err = savePricing(t, site.Id, 0, "1.1", nil, first.Version, 0)
	require.NoError(t, err)
	_, err = savePricing(t, site.Id, 0, "1.05", nil, first.Version, 0)
	assert.ErrorIs(t, err, ErrAdtySitePricingConflict)
}

func TestUserLogHidesMarkup(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "hidden")
	customer := siteCustomer(t, db, site, "hidden-customer")
	sitePrice(t, db, site.Id, 0, "", "1.4")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	AdtyApplySiteMarkup(ctx, customer.Id, "default", 1)
	model.RecordConsumeLog(ctx, customer.Id, model.RecordConsumeLogParams{ModelName: "m", Quota: 1400, Other: model.NewLogOther()})

	var log model.Log
	require.NoError(t, db.Where("user_id = ?", customer.Id).First(&log).Error)
	assert.Contains(t, log.Other, `"site_markup":1.4`, "the admin log records k")
	assert.Contains(t, log.Other, `"site_cost_factor":0.8`)
	model.AdtyProjectUserLog(&log)
	assert.NotContains(t, log.Other, "site_markup")
	assert.NotContains(t, log.Other, "site_cost_factor")
	assert.NotContains(t, log.Other, "admin_info")
}

func TestPricingShowsSitePrice(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "showcase")
	customer := siteCustomer(t, db, site, "showcase-customer")
	sitePrice(t, db, site.Id, 0, "", "1.5")
	sitePrice(t, db, site.Id, 0, "vip", "2")
	sitePrice(t, db, site.Id, customer.Id, "vip", "1.2")
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups)) })
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))

	ratios := func(ctx *gin.Context) map[string]float64 {
		values := map[string]float64{"default": 1, "vip": 2}
		AdtyApplySitePricing(ctx, values)
		return values
	}
	visitor, _ := gin.CreateTestContext(httptest.NewRecorder())
	visitor.Set(AdtySiteContextKey, AdtySiteById(site.Id))
	assert.InDelta(t, 1.2, ratios(visitor)["default"], 1e-9, "visitors see supply × (1+markup ratio)")
	assert.InDelta(t, 3.2, ratios(visitor)["vip"], 1e-9)

	loggedIn, _ := gin.CreateTestContext(httptest.NewRecorder())
	loggedIn.Set("id", customer.Id)
	got := ratios(loggedIn)
	assert.InDelta(t, 1.2, got["default"], 1e-9)
	assert.InDelta(t, 1.92, got["vip"], 1e-9, "a signed-in customer sees their own price")

	mainVisitor, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.Equal(t, map[string]float64{"default": 1, "vip": 2}, ratios(mainVisitor))
	assert.InDelta(t, 1.92, AdtyUserGroupRatio(customer.Id, "default", "vip"), 1e-9)
}

// handleGroupRatioForTest 和 relay/helper.HandleGroupRatio 一样先算分组倍率再套供货/加价公式（helper 引用 service，这里不能反向引用）。
// HandleGroupRatio 本身在 relay/helper/price_test.go 的 TestHandleGroupRatioAppliesSiteMarkup 里测。
func handleGroupRatioForTest(ctx *gin.Context, info *relaycommon.RelayInfo) hosttypes.GroupRatioInfo {
	ratio := ratio_setting.GetGroupRatio(info.UsingGroup)
	marked, k := AdtyApplySiteMarkup(ctx, info.UserId, info.UsingGroup, ratio)
	return hosttypes.GroupRatioInfo{GroupRatio: marked, GroupSpecialRatio: -1, SiteMarkup: k}
}

func TestSiteSupplyAndSellFormula(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "formula")
	customer := siteCustomer(t, db, site, "formula-customer")

	// Stored k=1.25 means a 25% markup ratio, not 125%. Cost factor 0.8 is the supply multiplier.
	sitePrice(t, db, site.Id, 0, "", "1.25")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	const baseGroup, supplyFactor, storedK = 2.0, 0.8, 1.25
	supply := baseGroup * supplyFactor
	sell := supply * (1 + (storedK - 1))

	gotSell, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", baseGroup)
	assert.InDelta(t, storedK, k, 1e-12)
	assert.InDelta(t, supply, baseGroup*supplyFactor, 1e-12, "supply = base × cost factor × group ratio")
	assert.InDelta(t, sell, gotSell, 1e-12, "sell = supply × (1 + markup ratio)")
	assert.InDelta(t, 2.0, gotSell, 1e-12)

	// Markup ratio 0 (stored k=1) => sell equals supply, and a cost factor below 1 is not forced up.
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(1)).Error)
	InvalidateAdtySitePriceCache()
	zeroMarkup, k := AdtyApplySiteMarkup(nil, customer.Id, "default", baseGroup)
	assert.InDelta(t, 1, k, 1e-12)
	assert.InDelta(t, supply, zeroMarkup, 1e-12, "markup ratio 0 keeps the sell price equal to the supply price")
	assert.NotEqual(t, baseGroup, zeroMarkup, "cost factor 0.8 must not be forced up to 1")

	// Old k<1 fallback no longer applies: stored 0.9 is a -10% ratio.
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.RequireFromString("0.9")).Error)
	InvalidateAdtySitePriceCache()
	markdown, k := AdtyApplySiteMarkup(nil, customer.Id, "default", baseGroup)
	assert.InDelta(t, 0.9, k, 1e-12)
	assert.InDelta(t, supply*0.9, markdown, 1e-12, "k<1 is no longer replaced with 1")
	assert.NotEqual(t, supply, markdown)

	// A supply multiplier above 1 is honored once stored on the site.
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("cost_factor", decimal.RequireFromString("1.2")).Error)
	InvalidateAdtySiteCache()
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(1)).Error)
	InvalidateAdtySitePriceCache()
	raisedSupply, _ := AdtyApplySiteMarkup(nil, customer.Id, "default", baseGroup)
	assert.InDelta(t, baseGroup*1.2, raisedSupply, 1e-12, "supply factor 1.2 is used as a multiplier")
}

func TestSiteCostFactorAboveOneIsReadable(t *testing.T) {
	prepareSiteDB(t)
	got, ok := AdtySiteCostFactor(&model.AdtySite{CostFactor: dec("1.2")})
	require.True(t, ok)
	assert.True(t, got.Equal(decimal.RequireFromString("1.2")))
}

// bindSiteTeamKey 给用户建一个分站或主站团队，并把一张启用的令牌绑到这个团队上，便于走团队计费。
func bindSiteTeamKey(t *testing.T, db *gorm.DB, user *model.User, siteId int, name, discount, key string) *gin.Context {
	t.Helper()
	created, err := CreateAdtyTeam(user.Id, siteId, name, decimal.RequireFromString(discount))
	require.NoError(t, err)
	require.Equal(t, siteId, created.Team.SiteId)
	token := model.Token{UserId: user.Id, Key: key, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, BindAdtyTeamKey(user.Id, token.Id))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
	return ctx
}

func TestSiteTeamPriceUsesSupplyFloor(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "team-floor")
	const baseGroup, supplyFactor, storedK = 2.0, 0.8, 1.25
	supply := baseGroup * supplyFactor
	sell := supply * (1 + (storedK - 1))
	sitePrice(t, db, site.Id, 0, "", "1.25")

	t.Run("discounted price above supply is charged as the discounted price", func(t *testing.T) {
		customer := siteCustomer(t, db, site, "team-floor-above")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Above", "0.90", "team-floor-above-key")
		got, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", baseGroup)
		assert.InDelta(t, storedK, k, 1e-12)
		assert.InDelta(t, sell*0.90, got, 1e-12)
		assert.Greater(t, got, supply)
		assert.False(t, AdtyTeamPriceBlocked(ctx))
		assert.InDelta(t, 0.90, ctx.GetFloat64(AdtyTeamDiscountContextKey), 1e-12)
	})

	t.Run("discounted price below supply is charged as the supply price", func(t *testing.T) {
		customer := siteCustomer(t, db, site, "team-floor-below")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Below", "0.70", "team-floor-below-key")
		got, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", baseGroup)
		assert.InDelta(t, storedK, k, 1e-12)
		assert.InDelta(t, supply, got, 1e-12, "final price is max(supply, sell × pay ratio)")
		assert.Less(t, sell*0.70, supply)
		assert.False(t, AdtyTeamPriceBlocked(ctx), "a price below supply must be billed, not rejected")
		assert.InDelta(t, 0.70, ctx.GetFloat64(AdtyTeamDiscountContextKey), 1e-12)
	})

	t.Run("zero markup keeps the team final price at supply", func(t *testing.T) {
		require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(1)).Error)
		InvalidateAdtySitePriceCache()
		t.Cleanup(func() {
			require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.RequireFromString("1.25")).Error)
			InvalidateAdtySitePriceCache()
		})
		customer := siteCustomer(t, db, site, "team-floor-zero")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Zero", "0.80", "team-floor-zero-key")
		got, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", baseGroup)
		assert.InDelta(t, 1, k, 1e-12)
		assert.InDelta(t, supply, got, 1e-12, "markup ratio 0: sell equals supply, team final stays at supply")
		assert.False(t, AdtyTeamPriceBlocked(ctx))
	})

	t.Run("main-site teams have no supply floor", func(t *testing.T) {
		leader := referralUser(t, db, "team-floor-main")
		require.NoError(t, db.Model(leader).Update("site_id", 0).Error)
		ctx := bindSiteTeamKey(t, db, leader, 0, "Main", "0.50", "team-floor-main-key")
		got, k := AdtyApplySiteMarkup(ctx, leader.Id, "default", baseGroup)
		assert.InDelta(t, 1, k, 1e-12)
		assert.InDelta(t, baseGroup*0.50, got, 1e-12, "main-site team pays the discounted price with no new floor")
		assert.Less(t, got, supply)
		assert.False(t, AdtyTeamPriceBlocked(ctx))
	})

	t.Run("invalid pay ratio still blocks the request", func(t *testing.T) {
		customer := siteCustomer(t, db, site, "team-floor-invalid")
		ctx := bindSiteTeamKey(t, db, customer, site.Id, "Invalid", "0.90", "team-floor-invalid-key")
		require.NoError(t, db.Model(&model.AdtyTeam{}).Where("leader_user_id = ?", customer.Id).Update("discount", decimal.RequireFromString("1.50")).Error)
		got, k := AdtyApplySiteMarkup(ctx, customer.Id, "default", baseGroup)
		assert.InDelta(t, storedK, k, 1e-12)
		assert.InDelta(t, sell, got, 1e-12, "invalid ratio leaves the ordinary sell price and blocks billing")
		assert.True(t, AdtyTeamPriceBlocked(ctx))
	})
}
