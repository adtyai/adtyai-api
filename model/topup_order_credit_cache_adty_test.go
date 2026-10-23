package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// 预扣先改缓存合计。入账必须在已有缓存上加上实付和赠送，不能删掉缓存让在途预扣丢失，
// 也不能等缓存过期后才让新额度可调用。
func TestCompleteTopupOrderAddsCreditToExistingQuotaCache(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	require.NoError(t, DB.AutoMigrate(&TopupOrder{}))
	require.NoError(t, EnsureLedgerSchema(DB))
	t.Cleanup(func() { DB.Exec("DELETE FROM topup_orders") })

	user := createReserveTestUser(t, 1000)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, cacheDecrUserQuota(user.Id, 200))

	paid, err := DollarsToQuota(10)
	require.NoError(t, err)
	bonus, err := DollarsToQuota(2)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&TopupOrder{
		OrderNo:         "CACHE-CREDIT-1",
		UserId:          user.Id,
		AmountUSD:       10,
		PaidAmountUSD:   10,
		CreditAmountUSD: 10,
		BonusAmountUSD:  2,
		Status:          TopupOrderStatusPending,
		RefundStatus:    RefundStatusNone,
		PaymentProvider: PaymentProviderBepusdt,
	}).Error)

	done, err := CompleteTopupOrder("CACHE-CREDIT-1")
	require.NoError(t, err)
	require.Equal(t, TopupOrderStatusPaid, done.Status)

	want := strconv.FormatInt(800+int64(paid)+int64(bonus), 10)
	got, err := common.RDB.HGet(context.Background(), getUserCacheKey(user.Id), "Quota").Result()
	require.NoError(t, err)
	require.Equal(t, want, got)

	again, err := CompleteTopupOrder("CACHE-CREDIT-1")
	require.NoError(t, err)
	require.Equal(t, TopupOrderStatusPaid, again.Status)
	got, err = common.RDB.HGet(context.Background(), getUserCacheKey(user.Id), "Quota").Result()
	require.NoError(t, err)
	require.Equal(t, want, got)
}
