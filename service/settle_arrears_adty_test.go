package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 审查 P0-1：预扣后结算补扣超过余额时照扣（赠送→0，实充→负），不可整笔免单。
func TestWalletSettleShortfallChargesIntoArrears(t *testing.T) {
	truncate(t)
	const uid = 9101
	seedUser(t, uid, 100)
	require.NoError(t, model.IncreaseUserBonusQuota(uid, 300, "topup_bonus", "seed", "test", "p0-1-settle"))

	w := &WalletFunding{userId: uid}
	require.NoError(t, w.PreConsume(50))
	require.NoError(t, w.Settle(625)) // 实际成本 675

	q, err := model.GetUserQuota(uid, true)
	require.NoError(t, err)
	assert.Equal(t, -275, q, "total wallet = 400 - 675")
	bonus, err := model.GetUserBonusBalance(uid)
	require.NoError(t, err)
	assert.Equal(t, int64(0), bonus)
	b, p := w.walletSplit()
	assert.Equal(t, 675, b+p)

	// 欠费后下一次预扣必须 403（ErrInsufficientWalletQuota）
	w2 := &WalletFunding{userId: uid}
	assert.ErrorIs(t, w2.PreConsume(1), ErrInsufficientWalletQuota)
}

// 审查 P0-1：PostConsumeQuota（音频/WSS 旧路径）同样照扣。
func TestPostConsumeQuotaWalletShortfallChargesIntoArrears(t *testing.T) {
	truncate(t)
	const uid = 9102
	seedUser(t, uid, 100)
	info := &relaycommon.RelayInfo{UserId: uid, IsPlayground: true}
	_, err := postConsumeQuotaWithResult(info, 675, 0, false)
	require.NoError(t, err)
	q, err := model.GetUserQuota(uid, true)
	require.NoError(t, err)
	assert.Equal(t, -575, q)
	assert.Equal(t, 675, info.WalletBonusConsumed+info.WalletPaidConsumed)
}

// 审查 R-P2-4：扣成负数时写 consume_arrears 标记流水（delta=0，balance_after 为负）。
func TestSettleIntoArrearsWritesLedgerMarker(t *testing.T) {
	truncate(t)
	const uid = 9103
	seedUser(t, uid, 100)
	w := &WalletFunding{userId: uid}
	require.NoError(t, w.PreConsume(50))
	require.NoError(t, w.Settle(625))
	var e model.LedgerEntry
	require.NoError(t, model.DB.Where("user_id = ? AND kind = ?", uid, model.LedgerKindConsumeArrears).First(&e).Error)
	assert.Equal(t, int64(0), e.Delta)
	assert.Equal(t, int64(-575), e.BalanceAfter)
}

// 审查 R-P2-3：低余额时单用户并发位有上限，释放后可再占。
func TestLowBalanceSlotLimit(t *testing.T) {
	resetLowBalanceSlotsForTest()
	prev := LowBalanceMaxInflight
	LowBalanceMaxInflight = 2
	t.Cleanup(func() { LowBalanceMaxInflight = prev; resetLowBalanceSlotsForTest() })
	assert.True(t, isLowBalance(700))
	r1, ok := acquireLowBalanceSlot(77)
	require.True(t, ok)
	_, ok = acquireLowBalanceSlot(77)
	require.True(t, ok)
	_, ok = acquireLowBalanceSlot(77)
	require.False(t, ok, "third concurrent request must be rejected")
	r1()
	r1() // 幂等
	_, ok = acquireLowBalanceSlot(77)
	require.True(t, ok)
	_, ok = acquireLowBalanceSlot(78)
	require.True(t, ok, "other users unaffected")
}
