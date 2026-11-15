package model

import (
	"testing"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 结算时的「补记」必须算出差额（`ISS-R4-0081`）。
//
// 修前 `amount -= taken.Held + taken.Settled`：`settled` 是 `held` 的子集，
// 减两遍让差额恒为负、直接 `return nil`。
//
// **这个洞只在结算路径里触发，不在「连续两次预扣」里**——我第一版就是这么写的，
// 反向验证时把 bug 改回去仍然绿，因为两次预扣之间 `settled` 还是 0，减两遍
// 等于减一遍。真正的触发条件是：结算**已经**把首批分摊转成 settled（held==settled），
// 此时 `left > 0` 触发补记，`sum(held)+sum(settled)` 就是两倍。
//
// 形态（总管给的复现）：预扣 5,000,000 → 结算 applied = 6,000,000。
func TestAdtyFundAllocationSettleTopUpRecordsTheDeltaPG(t *testing.T) {
	db := adtyTestDBWithSchema(t)
	pgtest.RecordServerVersion(t, db)

	const (
		userID = 8801
		first  = 5_000_000
		second = 6_000_000
		key    = "request:topup-delta-1"
	)
	require.NoError(t, db.Create(&User{Id: userID, Username: "topup-delta-8801", Password: "password1", Quota: 0}).Error)
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: userID, Kind: "principal", Source: "topup", Original: 10_000_000, Remaining: 10_000_000,
	}).Error)

	// 第一次预扣：登记 5,000,000。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return AllocateAdtyFundAllocationsTx(tx, userID, key, first, 10_000_000)
	}))
	sumHeld := func() (int64, int64) {
		var row struct{ Held, Settled int64 }
		require.NoError(t, db.Model(&AdtyFundAllocation{}).
			Select("coalesce(sum(held),0) as held, coalesce(sum(settled),0) as settled").
			Where("user_id = ? AND operation_id = ?", userID, key).Scan(&row).Error)
		return row.Held, row.Settled
	}
	held, settled := sumHeld()
	require.EqualValues(t, first, held, "首次预扣登记 5,000,000")
	require.Zero(t, settled)

	// 结算 applied = 6,000,000：首批 5,000,000 转成 settled 之后，left=1,000,000
	// 会触发补记——**这里就是 0081 的触发点**。
	var backed int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		backed, err = SettleAdtyFundAllocationsTx(tx, userID, key, second, 10_000_000)
		return err
	}))
	assert.EqualValues(t, second, backed,
		"补记成功才可能 backing 全额：修前 backed 只有 5,000,000，另外 1,000,000 被记成账本缺口")
	held, settled = sumHeld()
	assert.EqualValues(t, second, held)
	assert.EqualValues(t, second, settled, "结算后 held 与 settled 相等（收窄语义）")

	var remaining int64
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("user_id = ?", userID).
		Select("coalesce(sum(remaining),0)").Scan(&remaining).Error)
	// 批次 10,000,000，本次结算扣 6,000,000 → 余 4,000,000。
	// **这条断言是「只扣了一次」的证据**：如果第二次预扣没登记，结算只能扣首次的
	// 5,000,000，余量会停在 5,000,000 而 backed 只有 5,000,000。
	assert.EqualValues(t, 4_000_000, remaining, "批次余量 = 10,000,000 − 6,000,000")
}
