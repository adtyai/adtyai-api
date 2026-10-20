package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBillingRefundRestoresQuotaBeforeReturn(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	const userID = 881
	const tokenID = 88101
	const charged = 1000
	const tokenKey = "billing-token-sync"
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, tokenKey, 5000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId:          userID,
			TokenId:         tokenID,
			TokenKey:        tokenKey,
			RequestId:       "req-billing-sync",
			ForcePreConsume: true,
		},
		funding: &WalletFunding{userId: userID},
	}

	require.Nil(t, session.preConsume(ctx, charged))
	require.Equal(t, 4000, getUserQuota(t, userID))

	session.Refund(ctx)

	require.Equal(t, 5000, getUserQuota(t, userID))
	require.Equal(t, 5000, tokenRemain(t, tokenID))
	var job model.BillingRefundJob
	require.NoError(t, model.DB.Where("request_id = ?", "req-billing-sync").First(&job).Error)
	require.Equal(t, model.BillingRefundRefunded, job.Status)

	session.Refund(ctx)
	require.Equal(t, 5000, getUserQuota(t, userID))
	require.Equal(t, 5000, tokenRemain(t, tokenID))
}

func TestBillingRefundSweepSkipsLiveNodeAndRefundsDeadNodeOnce(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	prevNode := common.NodeName
	common.NodeName = "billing-sweep-node"
	t.Cleanup(func() { common.NodeName = prevNode })

	const userID = 882
	const tokenID = 88201
	const tokenKey = "billing-token-sweep"
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, tokenKey, 5000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId:          userID,
			TokenId:         tokenID,
			TokenKey:        tokenKey,
			RequestId:       "req-billing-sweep",
			ForcePreConsume: true,
		},
		funding: &WalletFunding{userId: userID},
	}
	require.Nil(t, session.preConsume(ctx, 1000))
	require.NoError(t, model.DB.Create(&model.SystemInstance{
		NodeName:   common.NodeName,
		LastSeenAt: common.GetTimestamp(),
	}).Error)
	require.NoError(t, model.DB.Model(&model.BillingRefundJob{}).Where("request_id = ?", "req-billing-sweep").
		Update("updated_at", common.GetTimestamp()-model.BillingHoldDeadAfterSeconds-10).Error)

	_, err := model.SweepBillingRefunds(common.GetTimestamp())
	require.NoError(t, err)
	require.Equal(t, 4000, getUserQuota(t, userID), "a live node must keep its open hold")

	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", common.NodeName).
		Update("last_seen_at", common.GetTimestamp()-model.SystemInstanceStaleAfterSeconds-10).Error)
	_, err = model.SweepBillingRefunds(common.GetTimestamp())
	require.NoError(t, err)
	require.Equal(t, 5000, getUserQuota(t, userID))
	_, err = model.SweepBillingRefunds(common.GetTimestamp())
	require.NoError(t, err)
	require.Equal(t, 5000, getUserQuota(t, userID))
	require.Equal(t, 5000, tokenRemain(t, tokenID))
}

func TestBillingSettleClosesHoldSoSweepDoesNotRefund(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	prevNode := common.NodeName
	common.NodeName = ""
	t.Cleanup(func() { common.NodeName = prevNode })

	const userID = 883
	const tokenID = 88301
	const tokenKey = "billing-token-settled"
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, tokenKey, 5000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId:          userID,
			TokenId:         tokenID,
			TokenKey:        tokenKey,
			RequestId:       "req-billing-settled",
			ForcePreConsume: true,
		},
		funding: &WalletFunding{userId: userID},
	}
	require.Nil(t, session.preConsume(ctx, 1000))
	require.NoError(t, session.Settle(1000))
	require.NoError(t, model.DB.Model(&model.BillingRefundJob{}).Where("request_id = ?", "req-billing-settled").
		Update("updated_at", time.Now().Unix()-model.BillingHoldDeadAfterSeconds-10).Error)

	_, err := model.SweepBillingRefunds(time.Now().Unix())
	require.NoError(t, err)
	require.Equal(t, 4000, getUserQuota(t, userID))
	session.Refund(ctx)
	require.Equal(t, 4000, getUserQuota(t, userID))
}

func tokenRemain(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, id).Error)
	return token.RemainQuota
}
