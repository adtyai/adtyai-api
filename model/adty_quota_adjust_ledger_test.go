package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// 人工调额的流水必须和充值/退款/兑换码/佣金转余额一样按逻辑余额（raw + pending）
// 记 before/after（ISS-R4-0082）。否则 quota_pending 非零的账号，同一张流水表里
// 两种口径并存，相邻两行满足不了 after(n) == before(n+1)，对账复算不出来。
func TestAdjustLedgerContinuesFromAdjacentLogicalBalanceRows(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "ledger-continuity", 1_000_000)
	// 典型在途态：raw 1,000,000、pending -400,000，逻辑余额 600,000。
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).
		Update("quota_pending", -400_000).Error)

	// 上一笔真实充值（入账 51,250,000）：写流水时用逻辑余额，after = 600,000 + 51,250,000。
	seedAdtyOrder(t, user.Id, "EPU-continuity", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := CompleteAdtyTopUp("EPU-continuity", usdtFacts("EPU-continuity", "50.01"), "")
	require.NoError(t, err)

	var rows []AdtyBalanceLedger
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.EqualValues(t, 600_000, rows[0].BalanceBefore)
	require.EqualValues(t, 51_850_000, rows[0].BalanceAfter)

	// 管理员加 100,000：流水要接着上一行，而不是跳回 raw。
	adjusted, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 100_000, "align ledger", "continuity-add-1")
	require.NoError(t, err)
	require.False(t, adjusted.Replayed)

	rows = nil
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	prev, adj := rows[0], rows[1]
	require.EqualValues(t, 100_000, adj.DeltaQuota)
	require.Equal(t, prev.BalanceAfter, adj.BalanceBefore, "相邻两行必须首尾相接")
	require.EqualValues(t, adj.BalanceBefore+adj.DeltaQuota, adj.BalanceAfter)
	require.EqualValues(t, 51_950_000, adj.BalanceAfter)

	// 流水 after 就是账户当前的逻辑余额（raw + pending）。
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	logical, err := userWalletBalance(stored.Quota, stored.QuotaPending)
	require.NoError(t, err)
	require.EqualValues(t, logical, adj.BalanceAfter)

	// 减额同样按逻辑余额，且仍与上一行连续。
	_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 50_000, "align ledger down", "continuity-sub-1")
	require.NoError(t, err)
	rows = nil
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id").Find(&rows).Error)
	require.Len(t, rows, 3)
	require.Equal(t, rows[1].BalanceAfter, rows[2].BalanceBefore)
	require.EqualValues(t, rows[2].BalanceBefore-50_000, rows[2].BalanceAfter)
}
