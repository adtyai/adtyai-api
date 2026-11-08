package service

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settleFailFunding 让钱包差额更新失败，退款仍走原来的预扣。
type settleFailFunding struct {
	inner FundingSource
}

func (f *settleFailFunding) Source() string { return f.inner.Source() }
func (f *settleFailFunding) PreConsume(amount int) error {
	return f.inner.PreConsume(amount)
}
func (f *settleFailFunding) Settle(delta int) (int, error) {
	return 0, errors.New("quota update failed")
}
func (f *settleFailFunding) Refund() error { return f.inner.Refund() }

// 结算差额写库失败时不能记成功消费。预扣还在，退款后余额和令牌回到预扣前。
func TestPostTextConsumeQuotaSettleFailureRefundsPrecharge(t *testing.T) {
	testPostConsumeQuotaSettleFailureRefundsPrecharge(t, false)
}

func TestPostWssConsumeQuotaSettleFailureRefundsPrecharge(t *testing.T) {
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
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Channel{}))

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
	session.funding = &settleFailFunding{inner: session.funding}

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
	require.True(t, info.Billing.NeedsRefund())

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
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		after, err := model.GetUserQuota(user.Id, true)
		assert.NoError(collect, err)
		assert.Equal(collect, startQuota, after)
		stored, err := model.GetTokenById(token.Id)
		assert.NoError(collect, err)
		assert.Equal(collect, startQuota, stored.RemainQuota)
		assert.Zero(collect, stored.UsedQuota)
	}, 5*time.Second, 10*time.Millisecond)
}
