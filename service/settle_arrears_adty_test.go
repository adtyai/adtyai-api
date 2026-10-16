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
