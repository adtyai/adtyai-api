package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"

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

// 分摊表不存在时，退款照常放行，但必须在日志里留一条可聚合的痕迹。
//
// 这条分支的取舍是「钱正确优先于账本完整」：报错会把退款一起回滚，用户白扣一次。
// 但放行不等于没发生——「这次退款没有分摊记录可释放」是对账要能查到的事实，
// 不能只活在代码注释里。
func TestRefundWithoutAllocationTableStillTracesPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	user := ticketTestUser(t, db, "refund-no-table", 1)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error)

	// 模拟迁移没跑全：把分摊表删掉。
	require.NoError(t, db.Migrator().DropTable(&model.AdtyFundAllocation{}))
	require.False(t, db.Migrator().HasTable(&model.AdtyFundAllocation{}))

	logged := captureSysLog(t)
	require.NoError(t, model.RefundUserQuotaForOperation(user.Id, 40_000, "request:no-table-1"),
		"表不存在不能让退款失败——钱正确优先")

	var after model.User
	require.NoError(t, db.First(&after, user.Id).Error)
	assert.EqualValues(t, 40_000, after.Quota, "钱照样退回去")
	assert.Contains(t, logged.String(), "refund released without allocation table",
		"留痕：没有分摊记录可释放这件事必须可查")
	assert.Contains(t, logged.String(), "request:no-table-1", "痕迹要能定位到具体 operation")
}

// 抓 SysLog/SysError 的输出：它们写 gin 的两个全局 writer，测试里换成内存缓冲。
// 换 writer 必须持 LogWriterMu —— 那正是它在生产里防日志轮转并发的原因。
func captureSysLog(t *testing.T) *strings.Builder {
	t.Helper()
	buffer := &strings.Builder{}
	common.LogWriterMu.Lock()
	previousOut, previousErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = buffer, buffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = previousOut, previousErr
		common.LogWriterMu.Unlock()
	})
	return buffer
}
