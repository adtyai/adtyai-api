package relay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWssHelperSettlementFailureReachesRefundPolicy(t *testing.T) {
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN is unset")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	t.Cleanup(func() { common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch })
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Channel{}))
	user := model.User{Username: "ws-settle-failure", Password: "x", Group: "default", Quota: 1_000_000,
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, opened.DB.Create(&user).Error)
	// The process-wide notification cleaner owns one permanent pool worker.
	_, err = service.CheckNotificationLimit(user.Id, "ws-settle-test")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 1 }, 5*time.Second, time.Millisecond)
	})
	token := model.Token{UserId: user.Id, Key: "ws-settle-failure", RemainQuota: 1_000_000, Status: common.TokenStatusEnabled}
	require.NoError(t, opened.DB.Create(&token).Error)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, ForcePreConsume: true,
		OriginModelName: "gpt-realtime", UserGroup: "default", UsingGroup: "default", StartTime: time.Now(),
		RelayMode: relayconstant.RelayModeRealtime, RelayFormat: types.RelayFormatOpenAIRealtime,
		RequestURLPath: "/v1/realtime?model=gpt-realtime",
		PriceData:      hosttypes.PriceData{UsePrice: true, ModelPrice: 0.01, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}
	reserveCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	reserveCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	require.Nil(t, service.PreConsumeBilling(reserveCtx, 10_000, info))
	// Reject the final wallet adjustment in PostgreSQL itself, after reservation.
	require.NoError(t, opened.DB.Exec("ALTER TABLE users ADD CONSTRAINT ws_settle_fault CHECK (quota = 990000)").Error)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var upstreamConnections atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamConnections.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
	}))
	t.Cleanup(upstream.Close)
	type outcome struct {
		ctx *gin.Context
		err *types.NewAPIError
	}
	done := make(chan outcome, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			done <- outcome{err: types.NewError(err, types.ErrorCodeBadResponse)}
			return
		}
		defer conn.Close()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = r
		common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
		common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, info.OriginModelName)
		info.ClientWs = conn
		done <- outcome{ctx: ctx, err: WssHelper(ctx, info)}
	}))
	t.Cleanup(proxy.Close)
	client, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http"), nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	var result outcome
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime request did not finish after upstream close")
	}
	require.NotNil(t, result.err, "the controller needs the settlement error to invoke its refund policy")
	assert.Equal(t, types.ErrorCodeUpdateDataError, result.err.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(result.err))
	assert.EqualValues(t, 1, upstreamConnections.Load())
	require.True(t, info.Billing.NeedsRefund())
	var logs int64
	require.NoError(t, opened.DB.Model(&model.Log{}).Count(&logs).Error)
	assert.Zero(t, logs)
	require.NoError(t, opened.DB.Exec("ALTER TABLE users DROP CONSTRAINT ws_settle_fault").Error)
	require.NotNil(t, RefundFailedRequestBilling(result.ctx, info, result.err))
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var storedUser model.User
		var storedToken model.Token
		assert.NoError(collect, opened.DB.First(&storedUser, user.Id).Error)
		assert.NoError(collect, opened.DB.First(&storedToken, token.Id).Error)
		assert.Equal(collect, 1_000_000, storedUser.Quota)
		assert.Equal(collect, 1_000_000, storedToken.RemainQuota)
		assert.Zero(collect, storedUser.UsedQuota)
		assert.Zero(collect, storedToken.UsedQuota)
	}, 5*time.Second, time.Millisecond)
}
