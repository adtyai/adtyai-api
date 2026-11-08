package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A persisted settlement decision owns the reservation after a write failure.
// Recovery applies it once; an error handler cannot race it with a full refund.
func TestPostTextConsumeQuotaSettleFailureRecoversDurableDecision(t *testing.T) {
	testPostConsumeQuotaSettleFailureRefundsPrecharge(t, false)
}

func TestPostWssConsumeQuotaSettleFailureRecoversDurableDecision(t *testing.T) {
	testPostConsumeQuotaSettleFailureRefundsPrecharge(t, true)
}

func testPostConsumeQuotaSettleFailureRefundsPrecharge(t *testing.T, realtime bool) {
	t.Helper()
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Channel{}, &model.Task{}, &model.Midjourney{}, &model.BillingOperation{}, &model.BillingOperationMovement{}, &model.AdtySiteEarningEvent{}))

	const startQuota = 1000000
	const preConsumed = 10000
	user := &model.User{
		Username: "wssettle",
		Password: "x",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Quota:    startQuota,
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         "sk-ws-settle",
		Name:        "ws",
		Status:      common.TokenStatusEnabled,
		RemainQuota: startQuota,
	}
	require.NoError(t, model.DB.Create(token).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{},
		UserId:          user.Id,
		TokenId:         token.Id,
		TokenKey:        token.Key,
		OriginModelName: "gpt-4.1",
		UsingGroup:      "default",
		StartTime:       time.Now(),
		IsStream:        true,
		ForcePreConsume: true,
		PriceData: types.PriceData{
			UsePrice:   true,
			ModelPrice: 0.01,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}
	require.Nil(t, PreConsumeBilling(ctx, preConsumed, info))
	quota, quotaErr := model.GetUserQuota(user.Id, true)
	require.NoError(t, quotaErr)
	require.Equal(t, startQuota-preConsumed, quota)

	session := info.Billing.(*BillingSession)
	fail := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("test:durable-settlement-failure", func(tx *gorm.DB) {
		if fail && tx.Statement.Table == "users" {
			tx.AddError(errors.New("forced wallet write failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove("test:durable-settlement-failure") })

	if realtime {
		consumeErr := PostWssConsumeQuota(ctx, info, info.OriginModelName, &dto.RealtimeUsage{
			InputTokens: 10, OutputTokens: 10, TotalTokens: 20,
		}, nil)
		require.Error(t, consumeErr)
	} else {
		consumeErr := PostTextConsumeQuota(ctx, info, &dto.Usage{
			PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20,
		}, nil)
		require.Error(t, consumeErr)
	}
	require.False(t, info.Billing.NeedsRefund())

	quota, quotaErr = model.GetUserQuota(user.Id, true)
	require.NoError(t, quotaErr)
	require.Equal(t, startQuota-preConsumed, quota, "差额更新失败不能把余额再改掉")
	var used int
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Select("used_quota").Scan(&used).Error)
	require.Zero(t, used)
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&logs).Error)
	require.Zero(t, logs)

	info.Billing.Refund(ctx)
	fail = false
	require.NoError(t, RecoverBillingOperations(context.Background(), 100))
	op, err := model.GetBillingOperation(session.operationKey)
	require.NoError(t, err)
	require.Equal(t, model.BillingSettled, op.State)
	require.NoError(t, RecoverBillingOperations(context.Background(), 100))
	after, err := model.GetUserQuota(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, startQuota-op.AppliedQuota, after)
	stored, err := model.GetTokenById(token.Id)
	require.NoError(t, err)
	require.Equal(t, startQuota-op.TokenAppliedQuota, stored.RemainQuota)
}
