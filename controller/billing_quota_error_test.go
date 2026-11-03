package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// 关掉按令牌展示后，读剩余额度失败不能被随后的已用额度读成功盖掉。
func TestSubscriptionKeepsQuotaReadError(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	server := miniredis.RunT(t)
	previousRedis := common.RedisEnabled
	previousRDB := common.RDB
	previousDisplay := common.DisplayTokenStatEnabled
	previousBatch := common.BatchUpdateEnabled
	previousUnit := common.QuotaPerUnit
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.DisplayTokenStatEnabled = false
	common.BatchUpdateEnabled = false
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousRedis
		common.RDB = previousRDB
		common.DisplayTokenStatEnabled = previousDisplay
		common.BatchUpdateEnabled = previousBatch
		common.QuotaPerUnit = previousUnit
	})

	user := &model.User{
		Username: "bill-err", Password: "x", Quota: 500000, UsedQuota: 250000,
		Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, model.SetUserAuthVersionFence(user.Id, 5))

	recorder := callBilling(GetSubscription, map[string]int{"id": user.Id})
	body := recorder.Body.String()
	require.Contains(t, body, "user authentication state update is pending")
	require.NotContains(t, body, "billing_subscription")
}
