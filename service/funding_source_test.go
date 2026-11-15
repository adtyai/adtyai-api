package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ISS-R4-0054：`WalletFunding.Settle` 的正数分支不再有扣费实现。
//
// 这条是**反向验证**：把正数分支改回「静默返回 (0, nil)」（删掉扣费实现后
// 最省事的写法）或随手接回一条非原子扣费路径，这里就红。静默不扣费是比报错
// 更坏的失败形态——它不报错，免费放行。
func TestLegacyWalletSettleRefusesToChargeSilently(t *testing.T) {
	funding := &WalletFunding{userId: 1}

	charged, err := funding.Settle(100)
	assert.ErrorIs(t, err, ErrLegacyWalletCharge)
	assert.Zero(t, charged, "不能报告一个没发生过的实扣额")

	// 负数分支（退还）不受影响，仍然走 IncreaseUserQuota；这里只验「不走到扣费」，
	// 不连库，所以用 delta == 0 的中性输入确认分支未变。
	charged, err = funding.Settle(0)
	assert.NoError(t, err)
	assert.Zero(t, charged)
}
