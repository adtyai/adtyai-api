package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFlushBatchFailureIsNotReportedAsSuccess(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := &model.User{Username: "flush-failure", Password: "x", Quota: 100, QuotaPending: -10}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Exec("ALTER TABLE users RENAME TO users_hidden").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("ALTER TABLE users_hidden RENAME TO users").Error) })
	t.Setenv("ADTY_FLUSH_TOKEN", "synthetic-flush-token")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/adty/batch/flush", nil)
	ctx.Request.Header.Set("X-Adty-Flush-Token", "synthetic-flush-token")
	FlushAdtyBatchUpdates(ctx)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
}

func callBilling(handler func(*gin.Context), keys map[string]int) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/dashboard/billing/subscription", nil)
	for key, value := range keys {
		ctx.Set(key, value)
	}
	handler(ctx)
	return recorder
}

// 批量更新还没刷进数据库时，订阅和用量也要带上已扣差额。
func TestBillingIncludesPendingBatchQuota(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))

	previousBatch := common.BatchUpdateEnabled
	previousDisplay := common.DisplayTokenStatEnabled
	previousRedis := common.RedisEnabled
	previousUnit := common.QuotaPerUnit
	common.BatchUpdateEnabled = true
	common.DisplayTokenStatEnabled = true
	common.RedisEnabled = false
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		model.FlushBatchUpdates()
		common.BatchUpdateEnabled = previousBatch
		common.DisplayTokenStatEnabled = previousDisplay
		common.RedisEnabled = previousRedis
		common.QuotaPerUnit = previousUnit
	})

	user := &model.User{Username: "bill-batch", Password: "x", Quota: 1000000, UsedQuota: 0, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "sk-bill-batch", Name: "bill", RemainQuota: 1000000, UsedQuota: 0, Status: 1}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, model.DecreaseTokenQuota(token.Id, token.Key, 200000))

	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.Equal(t, 1000000, stored.RemainQuota)
	require.Equal(t, 0, stored.UsedQuota)

	subscription := callBilling(GetSubscription, map[string]int{"token_id": token.Id})
	var sub OpenAISubscriptionResponse
	require.NoError(t, json.Unmarshal(subscription.Body.Bytes(), &sub))
	require.Equal(t, 2.0, sub.HardLimitUSD)

	usage := callBilling(GetUsage, map[string]int{"token_id": token.Id})
	var used OpenAIUsageResponse
	require.NoError(t, json.Unmarshal(usage.Body.Bytes(), &used))
	require.Equal(t, 40.0, used.TotalUsage)

	common.DisplayTokenStatEnabled = false
	require.NoError(t, model.DecreaseUserQuota(user.Id, 200000, false))
	model.UpdateUserUsedQuota(user.Id, 200000)
	var storedUser model.User
	require.NoError(t, db.Select("quota", "used_quota").First(&storedUser, user.Id).Error)
	require.Equal(t, 1000000, storedUser.Quota)
	require.Equal(t, 0, storedUser.UsedQuota)

	userUsage := callBilling(GetUsage, map[string]int{"id": user.Id})
	var userUsed OpenAIUsageResponse
	require.NoError(t, json.Unmarshal(userUsage.Body.Bytes(), &userUsed))
	require.Equal(t, 40.0, userUsed.TotalUsage)

	userSub := callBilling(GetSubscription, map[string]int{"id": user.Id})
	var userSubscription OpenAISubscriptionResponse
	require.NoError(t, json.Unmarshal(userSub.Body.Bytes(), &userSubscription))
	require.Equal(t, 2.0, userSubscription.HardLimitUSD)
}
