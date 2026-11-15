package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 量一件事：请求预扣后走普通失败退款（`WalletFunding.Refund` → IncreaseUserQuota），
// 预扣时登记的分摊（held）有没有被释放。
//
// 这条不是「断言正确行为」，是**测量**：先读出一个数，再决定这条该派给谁、
// 修在哪一层。所以断言写的是实测值，理由写在旁边。
func TestRefundLeavesPrechargedAllocationsHeldPG(t *testing.T) {
	db := adtyTestDBWithSchema(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = false })

	const (
		userID = 771
		amount = 50_000
		key    = "request:refund-held-1"
	)
	require.NoError(t, db.Create(&User{Id: userID, Username: "refund-held-771", Password: "password1", Quota: 100_000}).Error)
	// 这一条批次是分水岭：`AllocateAdtyFundAllocationsTx` 在 `len(batches)==0` 时
	// 直接 return，没有批次的老用户预扣时什么都不登记，也就没有 held 可残留。
	// 要测「退款是否泄漏 held」，账户必须**有**批次。
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: userID, Kind: "principal", Source: "topup", Original: 100_000, Remaining: 100_000,
	}).Error)

	// 预扣：扣余额 + 登记 held 分摊。
	ok, err := TryReserveUserQuotaForOperation(userID, amount, key)
	require.NoError(t, err)
	require.True(t, ok)

	var mid User
	require.NoError(t, db.First(&mid, userID).Error)
	t.Logf("quota after reserve = %d", mid.Quota)
	var total int64
	require.NoError(t, db.Model(&AdtyFundAllocation{}).Count(&total).Error)
	t.Logf("total allocation rows in schema = %d", total)
	var allocs []AdtyFundAllocation
	require.NoError(t, db.Where("user_id = ?", userID).Find(&allocs).Error)
	t.Logf("allocation rows after reserve: %d, op=%q", len(allocs), key)
	var batches []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", userID).Find(&batches).Error)
	for _, b := range batches {
		t.Logf("batch id=%d kind=%s source=%s original=%d remaining=%d", b.Id, b.Kind, b.Source, b.Original, b.Remaining)
	}
	var reserved struct{ Held int64 }
	require.NoError(t, db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", userID).Scan(&reserved).Error)
	require.EqualValues(t, amount, reserved.Held, "预扣必须登记分摊")

	// 普通失败退款：`WalletFunding.Refund` 走的就是这一句。
	require.NoError(t, IncreaseUserQuota(userID, amount, false))

	// 这就是要塞进那条 SQL 的数：退款之后还挂着多少没释放的分摊。
	var left struct {
		Held    int64
		Settled int64
	}
	require.NoError(t, db.Raw(
		"select coalesce(sum(held - settled),0) as held, coalesce(sum(settled),0) as settled "+
			"from adty_fund_allocations where user_id = ? and operation_id = ?", userID, key,
	).Scan(&left).Error)

	assert.EqualValues(t, amount, left.Held,
		"实测：普通失败退款不释放 held，预扣登记的分摊原样挂着")
	assert.Zero(t, left.Settled)

	var after User
	require.NoError(t, db.First(&after, userID).Error)
	assert.EqualValues(t, 100_000, after.Quota, "余额确实退回去了")

	// 两边一起看才是完整现象：钱退了、分摊没退。
	t.Logf("退款后 quota=%d, 残留 held=%d", after.Quota, left.Held)

	// 残留的后果不是「账上多一行」：同一批次上别人还占着的额度必须先扣掉
	// （outstandingAdtyFundAllocationByBatchTx），所以那 50,000 会**永久**占住
	// 一半容量。量一下：余额已经退回到 100,000，再预扣 100,000 时只能登记一半。
	const second = "request:refund-held-2"
	ok, err = TryReserveUserQuotaForOperation(userID, 100_000, second)
	require.NoError(t, err)
	require.True(t, ok, "余额够，预扣本身成功")

	var secondHeld struct{ Held int64 }
	require.NoError(t, db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ? AND operation_id = ?", userID, second).Scan(&secondHeld).Error)
	assert.EqualValues(t, 50_000, secondHeld.Held,
		"残留 50,000 让它只能登记一半：失败退款每来一次，可用批次容量就少一块")
	t.Logf("第二次预扣 100000，实际只登记 %d", secondHeld.Held)

	var stillStranded struct{ Held int64 }
	require.NoError(t, db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ? AND operation_id = ?", userID, key).Scan(&stillStranded).Error)
	assert.EqualValues(t, 50_000, stillStranded.Held, "第一次的那份仍然挂着，不会自己消失")
}
