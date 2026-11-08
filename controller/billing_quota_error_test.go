package controller

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestSubscriptionKeepsDurableQuotaReadError(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	previousDisplay := common.DisplayTokenStatEnabled
	previousRedis := common.RedisEnabled
	common.DisplayTokenStatEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() { common.DisplayTokenStatEnabled = previousDisplay; common.RedisEnabled = previousRedis })
	user := model.User{Username: "durable-bill-err", Quota: 500000, UsedQuota: 250000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:durable_quota_read_error", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && len(tx.Statement.Selects) == 2 && tx.Statement.Selects[0] == "quota" {
			tx.AddError(errors.New("durable wallet read unavailable"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("test:durable_quota_read_error")) })

	recorder := callBilling(GetSubscription, map[string]int{"id": user.Id})
	require.Contains(t, recorder.Body.String(), "durable wallet read unavailable")
	require.NotContains(t, recorder.Body.String(), "billing_subscription")
}
