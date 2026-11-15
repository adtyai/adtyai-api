package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 请求预扣后走失败退款，预扣时登记的分摊必须一起释放。
//
// 这条原本是**测量**：它先量出「退款后 held 仍残留」这个缺陷，钉住有问题的行为，
// 注释里写明「修好之后它会红，届时改成断言 held 归零」。修好之后我按那句话改的——
// 现在它是验收：held 归零，且同一批次的容量立刻全部可用。
func TestRefundReleasesPrechargedAllocationsPG(t *testing.T) {
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

	var reserved struct{ Held int64 }
	require.NoError(t, db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", userID).Scan(&reserved).Error)
	require.EqualValues(t, amount, reserved.Held, "预扣必须登记分摊")

	// 普通失败退款：`WalletFunding.Refund` 现在走这一句（同一事务退钱 + 释放分摊）。
	require.NoError(t, RefundUserQuotaForOperation(userID, amount, key))

	// 这就是要塞进那条 SQL 的数：退款之后还挂着多少没释放的分摊。
	var left struct {
		Held    int64
		Settled int64
	}
	require.NoError(t, db.Raw(
		"select coalesce(sum(held - settled),0) as held, coalesce(sum(settled),0) as settled "+
			"from adty_fund_allocations where user_id = ? and operation_id = ?", userID, key,
	).Scan(&left).Error)

	assert.Zero(t, left.Held, "退款必须释放这次预扣登记的分摊")
	assert.Zero(t, left.Settled)

	var after User
	require.NoError(t, db.First(&after, userID).Error)
	assert.EqualValues(t, 100_000, after.Quota, "余额确实退回去了")

	// 两边一起看才是完整现象：钱退了，占用也还回去了。
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
	assert.EqualValues(t, 100_000, secondHeld.Held,
		"退款释放了占用，同一批次的容量应当完整可用")
	t.Logf("第二次预扣 100000，实际登记 %d", secondHeld.Held)

	var stillStranded struct{ Held int64 }
	require.NoError(t, db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ? AND operation_id = ?", userID, key).Scan(&stillStranded).Error)
	assert.Zero(t, stillStranded.Held, "第一次那份不该再占着")
}
