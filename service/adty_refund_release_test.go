package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 走真实的 `WalletFunding` 退款路径：退钱必须连同预扣分摊一起释放。
//
// 这条盯的是「两个动作在同一个事务里」这件事本身，而不是某个函数内部怎么实现。
// 把 `Refund` 改回只调 `IncreaseUserQuota`，这条立刻红——那正是修之前的形态。
func TestWalletFundingRefundReleasesAllocationsPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	user := ticketTestUser(t, db, "funding-refund", 1)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 100_000).Error)
	require.NoError(t, db.Create(&model.AdtyFundBatch{
		UserId: user.Id, Kind: "principal", Source: "topup", Original: 100_000, Remaining: 100_000,
	}).Error)

	funding := &WalletFunding{userId: user.Id, operationKey: "request:refund-release-1"}
	require.NoError(t, funding.PreConsume(50_000))

	var before struct{ Held int64 }
	require.NoError(t, db.Model(&model.AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", user.Id).Scan(&before).Error)
	require.EqualValues(t, 50_000, before.Held, "预扣先登记分摊")

	require.NoError(t, funding.Refund())

	var after struct{ Held int64 }
	require.NoError(t, db.Model(&model.AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", user.Id).Scan(&after).Error)
	assert.Zero(t, after.Held, "退款必须释放预扣分摊，否则容量被永久占住")

	var reloaded model.User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.EqualValues(t, 100_000, reloaded.Quota, "钱照样要退回来")

	// 容量完整可用：同一批次上再预扣一整笔应当能登记满。
	funding2 := &WalletFunding{userId: user.Id, operationKey: "request:refund-release-2"}
	require.NoError(t, funding2.PreConsume(100_000))
	var second struct{ Held int64 }
	require.NoError(t, db.Model(&model.AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ? AND operation_id = ?", user.Id, "request:refund-release-2").Scan(&second).Error)
	assert.EqualValues(t, 100_000, second.Held, "释放之后容量不该还被占着一半")
}

// 没有 operationKey 的老调用面：只退钱，不能因为释放语句炸掉。
func TestWalletFundingRefundWithoutOperationKeyPG(t *testing.T) {
	db := announcementTestDB(t)
	user := ticketTestUser(t, db, "funding-refund-legacy", 1)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error)

	funding := &WalletFunding{userId: user.Id}
	funding.consumed = 30_000
	require.NoError(t, funding.Refund())

	var reloaded model.User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.EqualValues(t, 30_000, reloaded.Quota)
}
