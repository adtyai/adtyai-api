package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common/logsinktest"

	"github.com/QuantumNous/new-api/internal/pgtest"
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
	pgtest.RecordServerVersion(t, db)

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

// 分摊表不存在时：退款照常放行（钱正确优先），但**启动时必须点名缺表**。
//
// 这条分支的取舍是「钱正确优先于账本完整」：报错会把退款一起回滚，用户白扣一次。
// 但放行不等于没发生——缺表的库里**每一笔**退款都没有分摊可释放，整段部署窗口都
// 不可对账，那是对账必须提前知道的事实，所以它被提到启动期（`ISS-R4-0061`）。
//
// 断言换过一次：原来断言的是 `logged.String()` 里出现过一句话——那只证明「进程
// 内存里格式化过这个句子」，重启就没、轮转就没、也聚合不了。现在断言的是启动
// 检查的**类型化结论**：缺表要返回 `ErrAdtyFundAllocationTableMissing` 并点名表名。
func TestRefundWithoutAllocationTableIsNamedAtStartupPG(t *testing.T) {
	db := announcementTestDB(t)
	pgtest.RecordServerVersion(t, db)

	// 判据只有「表在不在」：表在的时候一条告警都不许有，否则这条检查会变成噪声。
	require.NoError(t, CheckAdtyStartupSchema(db), "表在的时候不能误报缺表")

	user := ticketTestUser(t, db, "refund-no-table", 1)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error)

	// 模拟迁移没跑全：把分摊表删掉。
	require.NoError(t, db.Migrator().DropTable(&model.AdtyFundAllocation{}))
	require.False(t, db.Migrator().HasTable(&model.AdtyFundAllocation{}))

	logged := captureSysLog(t)
	err := CheckAdtyStartupSchema(db)
	require.ErrorIs(t, err, model.ErrAdtyFundAllocationTableMissing,
		"缺表必须由启动检查点名，而不是等退款时才发现")
	assert.Contains(t, logged.String(), "adty_fund_allocations",
		"启动告警要指名缺的是哪张表，否则运维不知道要补哪条迁移")

	// 运行期：钱照样退，不许因为缺表失败。
	require.NoError(t, model.RefundUserQuotaForOperation(user.Id, 40_000, "request:no-table-1"),
		"表不存在不能让退款失败——钱正确优先")

	var after model.User
	require.NoError(t, db.First(&after, user.Id).Error)
	assert.EqualValues(t, 40_000, after.Quota, "钱照样退回去")
}

// captureSysLog 用共享的日志 sink 收 SysLog/SysError 的输出。
//
// 原来这里自己持 `common.LogWriterMu` 换 gin 的两个 writer——**那把锁是实现的
// 细节**，让每个要验日志的测试复述它，等于要求每个作者都记得同一件事。
// 现在换成 `logsinktest`，还原绑在 `t.Cleanup` 上。
func captureSysLog(t *testing.T) *logsinktest.Buffer {
	t.Helper()
	return logsinktest.Capture(t)
}
