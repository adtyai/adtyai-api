package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 删订阅前钉住只用钱包时的扣费数字，删完以后这些测试不改也必须通过。
// 特征测试按约定 F 的例外沿用本包 TestMain 的内存 SQLite 和 task_billing_test.go 的辅助函数。

// adtyWalletRelayContext 建一个 /v1/chat/completions 中转请求的 gin 上下文。
func adtyWalletRelayContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

// adtyWalletRelayInfo 是只用钱包的中转请求：UserSetting 留空走默认计费偏好，
// ForcePreConsume 关掉信任旁路，预扣全额。
func adtyWalletRelayInfo(userID, tokenID int, tokenKey string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		TokenUnlimited:  false,
		IsPlayground:    false,
		ForcePreConsume: true,
	}
}

func TestAdtyWalletPreConsumeSettle(t *testing.T) {
	truncate(t)
	const userID, tokenID = 9101, 9101
	const tokenKey = "sk-adty-wallet-settle"
	seedUser(t, userID, 1_000_000)
	seedToken(t, tokenID, userID, tokenKey, 100_000)
	c := adtyWalletRelayContext()
	info := adtyWalletRelayInfo(userID, tokenID, tokenKey)

	require.Nil(t, PreConsumeBilling(c, 10_000, info))
	require.NotNil(t, info.Billing)
	require.Equal(t, 990_000, getUserQuota(t, userID), "user quota after pre-consume")
	require.Equal(t, 10_000, getTokenUsedQuota(t, tokenID), "token used quota after pre-consume")

	require.NoError(t, SettleBilling(c, info, 7_500))
	require.Equal(t, 992_500, getUserQuota(t, userID), "user quota after settle")
	require.Equal(t, 7_500, getTokenUsedQuota(t, tokenID), "token used quota after settle")
}

func TestAdtyWalletRefund(t *testing.T) {
	truncate(t)
	const userID, tokenID = 9102, 9102
	const tokenKey = "sk-adty-wallet-refund"
	seedUser(t, userID, 1_000_000)
	seedToken(t, tokenID, userID, tokenKey, 100_000)
	c := adtyWalletRelayContext()
	info := adtyWalletRelayInfo(userID, tokenID, tokenKey)

	require.Nil(t, PreConsumeBilling(c, 10_000, info))
	require.Equal(t, 990_000, getUserQuota(t, userID), "user quota after pre-consume")
	require.Equal(t, 10_000, getTokenUsedQuota(t, tokenID), "token used quota after pre-consume")

	info.Billing.Refund(c)

	// 退款在 gopool 里异步执行，等钱包和令牌都退回去再结束测试
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		quota, err := model.GetUserQuota(userID, true)
		assert.NoError(collect, err)
		assert.Equal(collect, 1_000_000, quota, "user quota after refund")
		token, err := model.GetTokenById(tokenID)
		assert.NoError(collect, err)
		assert.Equal(collect, 0, token.UsedQuota, "token used quota after refund")
	}, 5*time.Second, 10*time.Millisecond)
}

func TestAdtyWalletInsufficientQuota(t *testing.T) {
	truncate(t)
	const userID, tokenID = 9103, 9103
	const tokenKey = "sk-adty-wallet-insufficient"
	seedUser(t, userID, 100)
	seedToken(t, tokenID, userID, tokenKey, 100_000)
	c := adtyWalletRelayContext()
	info := adtyWalletRelayInfo(userID, tokenID, tokenKey)

	apiErr := PreConsumeBilling(c, 10_000, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Nil(t, info.Billing)
	require.Equal(t, 100, getUserQuota(t, userID), "user quota")
	require.Equal(t, 0, getTokenUsedQuota(t, tokenID), "token used quota")
}

// adtyWalletTask 建一个提交时已从钱包预扣 10,000 的任务：用户额度从 1,000,000 扣到 990,000，
// 令牌、用户、渠道的已用额度都是 10,000。直接构造 model.Task，不设 BillingSource 和 SubscriptionId：
// 不是订阅任务就走 taskAdjustFunding 的钱包分支，删订阅时动这两个字段也不用改这里。
func adtyWalletTask(t *testing.T, userID, tokenID, channelID int) *model.Task {
	t.Helper()
	seedUser(t, userID, 990_000)
	seedToken(t, tokenID, userID, "sk-adty-task-wallet", 90_000)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, 10_000, 1)
	task := &model.Task{
		TaskID:      "task_adty_wallet",
		UserId:      userID,
		ChannelId:   channelID,
		Group:       "default",
		Quota:       10_000,
		Status:      model.TaskStatusInProgress,
		Properties:  model.Properties{OriginModelName: "test-model"},
		PrivateData: model.TaskPrivateData{TokenId: tokenID},
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestAdtyTaskBillingWalletOnly(t *testing.T) {
	const userID, tokenID, channelID = 9104, 9104, 9104
	for _, tc := range []struct {
		name          string
		refund        bool // 任务失败走 RefundTaskQuota，否则按 actualQuota 走 RecalculateTaskQuota
		actualQuota   int
		wantUserQuota int
		wantTokenUsed int
		wantTaskQuota int
	}{
		{name: "RefundTaskQuota refunds the pre-consumed quota", refund: true, wantUserQuota: 1_000_000, wantTokenUsed: 0, wantTaskQuota: 0},
		{name: "RecalculateTaskQuota refunds the difference", actualQuota: 7_500, wantUserQuota: 992_500, wantTokenUsed: 7_500, wantTaskQuota: 7_500},
		{name: "RecalculateTaskQuota charges the difference", actualQuota: 12_500, wantUserQuota: 987_500, wantTokenUsed: 12_500, wantTaskQuota: 12_500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			task := adtyWalletTask(t, userID, tokenID, channelID)
			ctx := context.Background()

			if tc.refund {
				require.True(t, RefundTaskQuota(ctx, task, "upstream task failed"))
			} else {
				RecalculateTaskQuota(ctx, task, tc.actualQuota, common.NewMessage("adaptor adjustment"))
			}

			require.Equal(t, tc.wantUserQuota, getUserQuota(t, userID), "user quota")
			require.Equal(t, tc.wantTokenUsed, getTokenUsedQuota(t, tokenID), "token used quota")
			require.Equal(t, tc.wantTaskQuota, task.Quota, "task quota")
			require.Equal(t, tc.wantTaskQuota, getTaskQuota(t, task.ID), "persisted task quota")
		})
	}
}

// ---------------------------------------------------------------------------
// 余额下限：钱包余额最多扣到 0，不透支。下面的测试都连 PostgreSQL（adtyTestDB）。
// ---------------------------------------------------------------------------

const (
	floorSiteMarkup = "1.25" // 分站价 k
	floorCostFactor = "0.8"  // 拿货价 d，prepareSiteDB 的默认值
	floorModelPrice = 0.04   // 按次计费的单价（美元）：主站价 20,000 额度，分站价 25,000
)

// prepareFloorDB 建测试库，关掉 Redis 和批量更新，并把信任门槛设成很小的正数，
// 让旧规则的信任旁路对任何正余额都生效：余额下限规则下钱包不再走信任旁路。
func prepareFloorDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Channel{}, &model.Task{}, &model.Midjourney{}))
	quotaSetting := operation_setting.GetQuotaSetting()
	oldTrust, oldBatch, oldLogConsume := quotaSetting.TrustQuotaUSD, common.BatchUpdateEnabled, common.LogConsumeEnabled
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		quotaSetting.TrustQuotaUSD, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldTrust, oldBatch, oldLogConsume
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	quotaSetting.TrustQuotaUSD = 1e-6 // 0.5 额度
	common.BatchUpdateEnabled, common.LogConsumeEnabled = false, true
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	return db
}

// useFloorMiniRedis 打开 Redis（miniredis），余额以缓存为准。
func useFloorMiniRedis(t *testing.T) {
	t.Helper()
	server := miniredis.RunT(t)
	oldEnabled, oldRDB, oldSync := common.RedisEnabled, common.RDB, common.SyncFrequency
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RedisEnabled, common.SyncFrequency = true, 60
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB, common.SyncFrequency = oldEnabled, oldRDB, oldSync
	})
}

// floorSite 开一个分站价 k = 1.25 的分站。
func floorSite(t *testing.T, db *gorm.DB, prefix string) *model.AdtySite {
	t.Helper()
	_, site := openActiveSite(t, db, prefix)
	sitePrice(t, db, site.Id, 0, "", floorSiteMarkup)
	return site
}

// floorCustomer 建一个钱包余额为 balance 的客户和一个不限额度的令牌（令牌额度不在余额下限范围内）。
// site 为 nil 时是主站客户。
func floorCustomer(t *testing.T, db *gorm.DB, site *model.AdtySite, name string, balance int) (*model.User, *model.Token) {
	t.Helper()
	var user *model.User
	if site != nil {
		user = siteCustomer(t, db, site, name)
	} else {
		user = referralUser(t, db, name)
	}
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", balance).Error)
	token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("floor%d", user.Id), Name: "floor", RemainQuota: 100_000_000, UnlimitedQuota: true, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(token).Error)
	return user, token
}

// floorRelay 建一个按次计费的文本请求：分组倍率经 AdtyApplySiteMarkup 乘 k，k、d 写进 gin 上下文，
// 和 relay/helper 的 HandleGroupRatio 一样。返回上下文、RelayInfo 和这次请求的应扣额度。
func floorRelay(t *testing.T, user *model.User, token *model.Token) (*gin.Context, *relaycommon.RelayInfo, int) {
	t.Helper()
	ctx := adtyWalletRelayContext()
	info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TokenUnlimited: true,
		ChannelMeta: &relaycommon.ChannelMeta{}, OriginModelName: "floor-call", UsingGroup: "default", UserGroup: "default",
		StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAI}
	info.PriceData.GroupRatioInfo = handleGroupRatioForTest(ctx, info)
	info.PriceData.UsePrice, info.PriceData.ModelPrice = true, floorModelPrice
	quota := common.QuotaRound(floorModelPrice * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio)
	return ctx, info, quota
}

// floorUsage 是按次计费请求的用量（按次计费不看 token 数，只要有用量）。
func floorUsage() *dto.Usage {
	return &dto.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20}
}

// floorBalance 读用户余额：开了 Redis 时读缓存（预扣以缓存为准），否则读数据库。
func floorBalance(t *testing.T, userID int) int {
	t.Helper()
	quota, err := model.GetUserQuota(userID, false)
	require.NoError(t, err)
	return quota
}

// assertFloorRejected 断言预扣被拒：上游的额度不足错误码、403，没有建计费会话（不会调上游）。
func assertFloorRejected(t *testing.T, apiErr *types.NewAPIError, info *relaycommon.RelayInfo) {
	t.Helper()
	if !assert.NotNil(t, apiErr, "pre-consume must be rejected") {
		return
	}
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Nil(t, info.Billing, "no billing session, the upstream is never called")
}

// assertFloorIntField 按名字读结构体的 int 字段并断言。约定的新字段（RelayInfo.BillingShortfall、
// postConsumeQuotaResult.Shortfall）实现加上之前不存在，用反射读，缺字段时报失败而不是编译不过。
// 等实现加上后可以改成直接引用字段。
func assertFloorIntField(t *testing.T, v any, field string, want int) {
	t.Helper()
	value := reflect.Indirect(reflect.ValueOf(v))
	got := value.FieldByName(field)
	if !got.IsValid() {
		assert.Failf(t, "missing field", "%s has no field %s yet", value.Type(), field)
		return
	}
	assert.EqualValues(t, want, got.Int(), "%s.%s", value.Type(), field)
}

// callTaskAdjustFunding 按约定的签名 taskAdjustFunding(task, delta) (shortfall int, err error) 调用。
// 实现改签名之前它只返回 error，用反射调，旧签名时报失败。等实现改好后可以改成直接调用。
func callTaskAdjustFunding(t *testing.T, task *model.Task, delta int) (int, bool) {
	t.Helper()
	out := reflect.ValueOf(taskAdjustFunding).Call([]reflect.Value{reflect.ValueOf(task), reflect.ValueOf(delta)})
	if len(out) != 2 {
		assert.Failf(t, "old signature", "taskAdjustFunding returns %d values, want (shortfall int, err error)", len(out))
		return 0, false
	}
	if err, _ := out[1].Interface().(error); err != nil {
		assert.NoError(t, err)
		return 0, false
	}
	return int(out[0].Int()), true
}

// floorLastAdminInfo 取用户最近一条 logType 日志，返回日志和它的 other.admin_info。
func floorLastAdminInfo(t *testing.T, db *gorm.DB, userID, logType int) (model.Log, map[string]any) {
	t.Helper()
	var log model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, logType).Order("id desc").First(&log).Error)
	other := map[string]any{}
	if log.Other != "" {
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	}
	adminInfo, _ := other["admin_info"].(map[string]any)
	return log, adminInfo
}

// floorTask 建一个提交时已从钱包预扣 preConsumed 的任务（没有分站快照，走钱包分支）。
func floorTask(t *testing.T, db *gorm.DB, user *model.User, token *model.Token, preConsumed int) *model.Task {
	t.Helper()
	task := &model.Task{TaskID: fmt.Sprintf("floor-task-%d", user.Id), UserId: user.Id, Group: "default", Quota: preConsumed,
		Status: model.TaskStatusInProgress, Properties: model.Properties{OriginModelName: "floor-task"},
		PrivateData: model.TaskPrivateData{TokenId: token.Id}}
	require.NoError(t, db.Create(task).Error)
	return task
}

// 1. 余额不够预扣就拒绝，返回上游的额度不足错误码，上游不会被调用。
func TestAdtyBalanceFloorRejectsInsufficientPreConsume(t *testing.T) {
	t.Run("balance below the pre-consume", func(t *testing.T) {
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-reject-low", 15_000)
		ctx, info, quota := floorRelay(t, user, token)
		require.Equal(t, 20_000, quota)

		assertFloorRejected(t, PreConsumeBilling(ctx, quota, info), info)
		assert.Equal(t, 15_000, floorBalance(t, user.Id))
	})
	t.Run("balance already reserved by an earlier request", func(t *testing.T) {
		// 余额 20,000 只够一次：第一次预扣占满，第二次必须拒绝。
		// 旧规则两次都走信任旁路不预扣，第二次也放行，结算后余额变成 −20,000。
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-reject-reserved", 20_000)
		firstCtx, first, quota := floorRelay(t, user, token)
		require.Nil(t, PreConsumeBilling(firstCtx, quota, first))
		assert.Equal(t, 0, floorBalance(t, user.Id), "balance after the first pre-consume")

		secondCtx, second, _ := floorRelay(t, user, token)
		assertFloorRejected(t, PreConsumeBilling(secondCtx, quota, second), second)
	})
}

// 2. 余额刚好等于预扣额：放行，余额 0；按同样的额度结算后仍是 0。
func TestAdtyBalanceFloorExactBalance(t *testing.T) {
	db := prepareFloorDB(t)
	user, token := floorCustomer(t, db, nil, "floor-exact", 20_000)
	ctx, info, quota := floorRelay(t, user, token)

	require.Nil(t, PreConsumeBilling(ctx, quota, info))
	assert.Equal(t, quota, info.Billing.GetPreConsumedQuota(), "the full price is reserved")
	assert.Equal(t, 0, floorBalance(t, user.Id), "balance after pre-consume")

	require.NoError(t, SettleBilling(ctx, info, quota))
	assert.Equal(t, 0, floorBalance(t, user.Id), "balance after settle")
}

// 3. 并发 20 个请求、每个预扣 1、余额 5：正好 5 个通过，余额 0。Redis 开、关各跑一次。
func TestAdtyBalanceFloorConcurrentPreConsume(t *testing.T) {
	for _, redisOn := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis=%t", redisOn), func(t *testing.T) {
			db := prepareFloorDB(t)
			if redisOn {
				useFloorMiniRedis(t)
			}
			user, token := floorCustomer(t, db, nil, "floor-concurrent", 5)
			const requests = 20
			ctxs := make([]*gin.Context, requests)
			infos := make([]*relaycommon.RelayInfo, requests)
			for i := range requests {
				ctxs[i], infos[i], _ = floorRelay(t, user, token)
			}

			var admitted atomic.Int32
			var wg sync.WaitGroup
			for i := range requests {
				wg.Go(func() {
					if PreConsumeBilling(ctxs[i], 1, infos[i]) == nil {
						admitted.Add(1)
					}
				})
			}
			wg.Wait()

			assert.EqualValues(t, 5, admitted.Load(), "admitted requests")
			assert.Equal(t, 0, floorBalance(t, user.Id), "balance")
			assert.Equal(t, 0, getUserQuota(t, user.Id), "database balance")
		})
	}
}

// 4. 结算实际 > 余额：余额 0，shortfall 正确，日志 admin_info 有 shortfall_quota，
// 分站客户的站长收益按实扣算。
func TestAdtyBalanceFloorSettleShortfall(t *testing.T) {
	db := prepareFloorDB(t)
	site := floorSite(t, db, "floorshort")
	user, token := floorCustomer(t, db, site, "floor-short-customer", 10_000)
	ctx, info, quota := floorRelay(t, user, token)
	require.Equal(t, 25_000, quota, "site price with k = 1.25")

	// 预扣只估了 4,000，实际 25,000：余额 10,000 只能实扣 10,000，差 15,000。
	require.Nil(t, PreConsumeBilling(ctx, 4_000, info))
	PostTextConsumeQuota(ctx, info, floorUsage(), nil)

	assert.Equal(t, 0, floorBalance(t, user.Id), "balance stops at 0")
	assertFloorIntField(t, info, "BillingShortfall", 15_000)

	log, adminInfo := floorLastAdminInfo(t, db, user.Id, model.LogTypeConsume)
	assert.Equal(t, 25_000, log.Quota, "the log keeps the price of the request")
	assert.EqualValues(t, 15_000, adminInfo["shortfall_quota"], "admin_info %v", adminInfo)

	charged := int64(10_000)
	_, _, wantEarning := AdtySiteEarningSplit(charged, usd(floorSiteMarkup), usd(floorCostFactor))
	var rows []model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", site.Id, user.Id).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, charged, rows[0].Quota, "earning is booked on the charged quota")
	assert.True(t, rows[0].Earning.Equal(wantEarning), "earning %s, want %s", rows[0].Earning, wantEarning)
}

// 5. 分站请求 k = 1.25：预扣和扣费都按乘 k 后的价格，余额下限也按这个价格判断。
func TestAdtyBalanceFloorSiteMarkupPrice(t *testing.T) {
	for _, tc := range []struct {
		name         string
		onSite       bool
		balance      int
		wantQuota    int
		wantAdmitted bool
		wantBalance  int
	}{
		{name: "site customer pays k times the main price", onSite: true, balance: 25_000, wantQuota: 25_000, wantAdmitted: true, wantBalance: 0},
		{name: "site customer with only the main price is rejected", onSite: true, balance: 20_000, wantQuota: 25_000, wantAdmitted: false, wantBalance: 20_000},
		{name: "main-site customer pays the main price", onSite: false, balance: 20_000, wantQuota: 20_000, wantAdmitted: true, wantBalance: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := prepareFloorDB(t)
			var site *model.AdtySite
			if tc.onSite {
				site = floorSite(t, db, "floorprice")
			}
			user, token := floorCustomer(t, db, site, "floor-price-customer", tc.balance)
			ctx, info, quota := floorRelay(t, user, token)
			require.Equal(t, tc.wantQuota, quota)

			apiErr := PreConsumeBilling(ctx, quota, info)
			if !tc.wantAdmitted {
				assertFloorRejected(t, apiErr, info)
				assert.Equal(t, tc.wantBalance, floorBalance(t, user.Id))
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tc.balance-quota, floorBalance(t, user.Id), "balance after pre-consume")

			PostTextConsumeQuota(ctx, info, floorUsage(), nil)
			assert.Equal(t, tc.wantBalance, floorBalance(t, user.Id), "balance after settle")
			log, adminInfo := floorLastAdminInfo(t, db, user.Id, model.LogTypeConsume)
			assert.Equal(t, quota, log.Quota)
			assert.NotContains(t, adminInfo, "shortfall_quota", "fully charged, no shortfall")
		})
	}
}

// 6. 回归：余额 10、预扣 10、实际 225，结算后余额 0（旧规则是 −215）。
// Redis 开关 × BatchUpdateEnabled 开关都跑。Redis 开时余额以缓存为准，批量模式下数据库
// 要等队列刷新，所以 Redis+批量只看缓存；其余组合数据库也必须是 0。
func TestAdtyBalanceFloorSettleRegression(t *testing.T) {
	for _, mode := range []struct {
		name  string
		redis bool
		batch bool
	}{
		{name: "direct", redis: false, batch: false},
		{name: "batch without redis", redis: false, batch: true},
		{name: "redis direct", redis: true, batch: false},
		{name: "redis batch", redis: true, batch: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			db := prepareFloorDB(t)
			if mode.redis {
				useFloorMiniRedis(t)
			}
			common.BatchUpdateEnabled = mode.batch
			user, token := floorCustomer(t, db, nil, "floor-regression", 10)
			ctx, info, _ := floorRelay(t, user, token)

			require.Nil(t, PreConsumeBilling(ctx, 10, info))
			require.NoError(t, SettleBilling(ctx, info, 225))

			assert.Equal(t, 0, floorBalance(t, user.Id), "balance after settle")
			if !(mode.redis && mode.batch) {
				assert.Equal(t, 0, getUserQuota(t, user.Id), "database balance after settle")
			}
			assertFloorIntField(t, info, "BillingShortfall", 215)
		})
	}
}

// 7. 异步任务差额结算、Realtime、MJ：余额不够时扣到 0。
func TestAdtyBalanceFloorAsyncPaths(t *testing.T) {
	t.Run("task settles the difference", func(t *testing.T) {
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-task", 1_000)
		task := floorTask(t, db, user, token, 10_000)

		// 实际 15,000，差额 5,000，余额只有 1,000：实扣 1,000，差 4,000。
		RecalculateTaskQuota(context.Background(), task, 15_000, common.NewMessage("adaptor adjustment"))

		assert.Equal(t, 0, floorBalance(t, user.Id), "balance stops at 0")
		_, adminInfo := floorLastAdminInfo(t, db, user.Id, model.LogTypeConsume)
		assert.EqualValues(t, 4_000, adminInfo["shortfall_quota"], "admin_info %v", adminInfo)
	})
	t.Run("taskAdjustFunding returns the shortfall", func(t *testing.T) {
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-task-funding", 500)
		task := floorTask(t, db, user, token, 0)

		if shortfall, ok := callTaskAdjustFunding(t, task, 2_000); ok {
			assert.Equal(t, 1_500, shortfall)
		}
		assert.Equal(t, 0, floorBalance(t, user.Id), "balance stops at 0")
	})
	t.Run("realtime usage charge", func(t *testing.T) {
		// Realtime 每段用量走 PostConsumeQuota；PreWssConsumeQuota 的余额检查不加锁，
		// 并发会话之间仍可能不够，扣费本身必须停在 0。
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-realtime", 1_000)
		_, info, _ := floorRelay(t, user, token)

		result, err := postConsumeQuotaWithResult(info, 3_000, 0, false)
		require.NoError(t, err)
		assert.True(t, result.FundingApplied)
		assertFloorIntField(t, result, "Shortfall", 2_000)
		assert.Equal(t, 0, floorBalance(t, user.Id), "balance stops at 0")
	})
	t.Run("midjourney settle", func(t *testing.T) {
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-mj", 1_000)
		_, info, _ := floorRelay(t, user, token)
		task := &model.Midjourney{UserId: user.Id, MjId: fmt.Sprintf("floor-mj-%d", user.Id), Action: "IMAGINE", Progress: "0%"}
		prepared, err := PrepareMidjourneyTaskBilling(info, task, 3_000, true)
		require.NoError(t, err)
		require.NoError(t, task.Insert())

		billed, err := SettleMidjourneyTaskBilling(info, task, prepared)
		require.NoError(t, err)
		assert.True(t, billed)
		assert.Equal(t, 0, floorBalance(t, user.Id), "balance stops at 0")
	})
}

func TestRealtimeIncrementalUsageIsNotChargedTwice(t *testing.T) {
	model.InitColumnNamesForTests()
	truncate(t)
	const userID, tokenID = 9401, 9401
	const tokenKey = "realtime-once"
	const opening = 1_000_000
	seedUser(t, userID, opening)
	seedToken(t, tokenID, userID, tokenKey, opening)
	c := adtyWalletRelayContext()
	info := adtyWalletRelayInfo(userID, tokenID, "sk-"+tokenKey)
	info.OriginModelName = "gpt-4o-realtime-preview"
	info.UsingGroup = "default"
	info.UserGroup = "default"
	info.StartTime = time.Now()

	require.Nil(t, PreConsumeBilling(c, 1_000, info))
	usage := &dto.RealtimeUsage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300}
	usage.InputTokenDetails.TextTokens = 200
	usage.OutputTokenDetails.TextTokens = 100
	require.NoError(t, PreWssConsumeQuota(c, info, usage))
	afterStream := getUserQuota(t, userID)
	streamCharge := (opening - 1_000) - afterStream
	require.Greater(t, streamCharge, 0)
	require.NoError(t, SettleBilling(c, info, streamCharge))
	require.Equal(t, opening-streamCharge, getUserQuota(t, userID),
		"settlement must not charge the same realtime usage again")
}

// 8. 有 shortfall 之后退款只退实扣的部分，不把平台没收到的钱退给用户。
func TestAdtyBalanceFloorRefundAfterShortfall(t *testing.T) {
	t.Run("task", func(t *testing.T) {
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-refund-task", 1_000)
		task := floorTask(t, db, user, token, 10_000)

		// 预扣 10,000，差额结算实扣 1,000（差 4,000），一共实扣 11,000。
		RecalculateTaskQuota(context.Background(), task, 15_000, common.NewMessage("adaptor adjustment"))
		assert.Equal(t, 0, floorBalance(t, user.Id), "balance after settle")
		assert.Equal(t, 11_000, task.Quota, "task quota is what the wallet actually paid")

		require.True(t, RefundTaskQuota(context.Background(), task, "upstream task failed"))
		log, _ := floorLastAdminInfo(t, db, user.Id, model.LogTypeRefund)
		assert.Equal(t, 11_000, log.Quota, "refund log")
		assert.Equal(t, 11_000, floorBalance(t, user.Id), "balance after refund")
	})
	t.Run("midjourney", func(t *testing.T) {
		db := prepareFloorDB(t)
		user, token := floorCustomer(t, db, nil, "floor-refund-mj", 1_000)
		_, info, _ := floorRelay(t, user, token)
		task := &model.Midjourney{UserId: user.Id, MjId: fmt.Sprintf("floor-refund-mj-%d", user.Id), Action: "IMAGINE", Progress: "0%"}
		prepared, err := PrepareMidjourneyTaskBilling(info, task, 3_000, true)
		require.NoError(t, err)
		require.NoError(t, task.Insert())
		_, err = SettleMidjourneyTaskBilling(info, task, prepared)
		require.NoError(t, err)
		assert.Equal(t, 0, floorBalance(t, user.Id), "balance after settle")

		require.True(t, RefundMidjourneyQuota(context.Background(), task, "upstream task failed"))
		log, _ := floorLastAdminInfo(t, db, user.Id, model.LogTypeRefund)
		assert.Equal(t, 1_000, log.Quota, "refund log")
		assert.Equal(t, 1_000, floorBalance(t, user.Id), "balance after refund")
	})
}

// TestAdtyBalanceFloorViolationFee：违规扣费 $0.04（20,000）、余额 1,000：扣到 0，日志记应扣 20,000、
// admin_info 记 shortfall 19,000。
func TestAdtyBalanceFloorViolationFee(t *testing.T) {
	db := prepareFloorDB(t)
	savedGrok := *model_setting.GetGrokSettings()
	t.Cleanup(func() { *model_setting.GetGrokSettings() = savedGrok })
	model_setting.GetGrokSettings().ViolationDeductionEnabled = true
	model_setting.GetGrokSettings().ViolationDeductionAmount = 0.04

	user, token := floorCustomer(t, db, nil, "floor-violation", 1000)
	ctx, info, _ := floorRelay(t, user, token)
	require.True(t, ChargeViolationFeeIfNeeded(ctx, info, types.NewError(fmt.Errorf("blocked"), types.ErrorCodeViolationFeeGrokCSAM)))
	assert.Equal(t, 0, floorBalance(t, user.Id))
	log, adminInfo := floorLastAdminInfo(t, db, user.Id, model.LogTypeConsume)
	assert.Equal(t, 20_000, log.Quota)
	assert.EqualValues(t, 19_000, adminInfo["shortfall_quota"])
}
