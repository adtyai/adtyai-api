package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const usd = 500000

func b03User(t *testing.T, paid, gift int) int {
	t.Helper()
	u := createReserveTestUser(t, paid)
	if gift > 0 {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			if err := EnsureUserBonusWallet(tx, u.Id); err != nil {
				return err
			}
			return tx.Model(&UserBonusWallet{}).Where("user_id = ?", u.Id).Update("balance", gift).Error
		}))
	}
	return u.Id
}

func b03At(t *testing.T, at time.Time) {
	t.Helper()
	old := adtyWalletConsumeNow
	adtyWalletConsumeNow = func() time.Time { return at }
	t.Cleanup(func() { adtyWalletConsumeNow = old })
}

func b03Balances(t *testing.T, uid int) (paid, gift int64) {
	t.Helper()
	paid = int64(getUserQuotaFromDB(t, uid))
	require.NoError(t, DB.Model(&UserBonusWallet{}).Where("user_id = ?", uid).Select("COALESCE(balance,0)").Scan(&gift).Error)
	return
}

func b03Split(t *testing.T, uid int, hb time.Time) AdtyWalletConsumeHourly {
	t.Helper()
	m, err := loadWalletConsumeForHour(DB, hb)
	require.NoError(t, err)
	return m[uid]
}

// 写汇总行并返回 account→delta
func b03Rollup(t *testing.T, uid int, total int64, hb time.Time) map[string]int64 {
	t.Helper()
	split := b03Split(t, uid, hb)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, _, err := writeHourlyRollupUserTx(tx, uid, total, split, hb)
		return err
	}))
	var rows []LedgerEntry
	require.NoError(t, DB.Where("user_id = ? AND kind = ? AND hour_bucket = ?", uid, LedgerKindAPIConsumeHourly, hb).Find(&rows).Error)
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Account] += r.Delta
	}
	return out
}

func b03Setup(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
}

// (a) 实充 $100 + 赠送 $10，10:50 扣 $3、11:10 扣 $4 → 10 点 bonus −3，11 点 bonus −4
func TestB03ScenarioA_GiftFirstAcrossHours(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 100*usd, 10*usd)
	h10 := time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)
	h11 := h10.Add(time.Hour)

	b03At(t, h10.Add(50*time.Minute))
	_, _, ok, err := TryReserveUserWallet(uid, 3*usd)
	require.NoError(t, err)
	require.True(t, ok)
	b03At(t, h11.Add(10*time.Minute))
	_, _, ok, err = TryReserveUserWallet(uid, 4*usd)
	require.NoError(t, err)
	require.True(t, ok)

	require.Equal(t, map[string]int64{"bonus": -3 * usd}, b03Rollup(t, uid, 3*usd, h10))
	require.Equal(t, map[string]int64{"bonus": -4 * usd}, b03Rollup(t, uid, 4*usd, h11))
	paid, gift := b03Balances(t, uid)
	require.Equal(t, int64(100*usd), paid)
	require.Equal(t, int64(3*usd), gift)
}

// (b) 实充 50 赠送 2，同一小时扣 5 → bonus −2，paid −3
func TestB03ScenarioB_SplitWithinHour(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 50*usd, 2*usd)
	hb := time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)
	b03At(t, hb.Add(5*time.Minute))
	_, _, err := DecreaseUserWallet(uid, 5*usd)
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"bonus": -2 * usd, "paid": -3 * usd}, b03Rollup(t, uid, 5*usd, hb))
	// 幂等：重跑不重复写
	require.Equal(t, map[string]int64{"bonus": -2 * usd, "paid": -3 * usd}, b03Rollup(t, uid, 5*usd, hb))
}

// (c) 预扣后部分退回：实充 100 赠送 1，预扣 3（赠送1+实充2），退回 2.5 → 先退实充 2 再退赠送 0.5，净 bonus 0.5
func TestB03ScenarioC_ReserveThenPartialReturn(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 100*usd, 1*usd)
	hb := time.Date(2026, 10, 16, 4, 0, 0, 0, time.UTC)
	b03At(t, hb.Add(time.Minute))
	b, p, ok, err := TryReserveUserWallet(uid, 3*usd)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1*usd, b)
	require.Equal(t, 2*usd, p)
	// 模拟 WalletFunding.Settle(-2.5$)：先退实充再退赠送；请求内退回不写流水
	require.NoError(t, ReturnReservedUserWallet(uid, usd/2, 2*usd))
	var refundRows int64
	require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND kind IN ?", uid, []string{"wallet_refund", "wallet_return"}).Count(&refundRows).Error)
	require.Equal(t, int64(0), refundRows)
	s := b03Split(t, uid, hb)
	require.Equal(t, int64(usd/2), s.Bonus)
	require.Equal(t, int64(0), s.Paid)
	require.Equal(t, map[string]int64{"bonus": -usd / 2}, b03Rollup(t, uid, usd/2, hb))
	paid, gift := b03Balances(t, uid)
	require.Equal(t, int64(100*usd), paid)
	require.Equal(t, int64(usd/2), gift)
}

// (d) 充值订单退款（CreditUserWallet/AdjustQuota 等非扣费路径）不写出资拆分
func TestB03ScenarioD_RefundPathUnaffected(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 10*usd, 0)
	require.NoError(t, CreditUserWallet(uid, 0, 5*usd, "topup", "test", "order", "x"))
	var n int64
	require.NoError(t, DB.Model(&AdtyWalletConsumeHourly{}).Where("user_id = ?", uid).Count(&n).Error)
	require.Equal(t, int64(0), n)
}

// 切换前 / 无拆分：差额全记 paid；拆分与日志不一致时余数记 paid
func TestB03FallbackRemainderToPaid(t *testing.T) {
	b, p, r := planHourlyRollupSplit(100, AdtyWalletConsumeHourly{})
	require.Equal(t, [3]int64{0, 100, 100}, [3]int64{b, p, r})
	b, p, r = planHourlyRollupSplit(100, AdtyWalletConsumeHourly{Bonus: 30, Paid: 60})
	require.Equal(t, [3]int64{30, 70, 10}, [3]int64{b, p, r})
	b, p, r = planHourlyRollupSplit(100, AdtyWalletConsumeHourly{Bonus: 150})
	require.Equal(t, [3]int64{100, 0, -50}, [3]int64{b, p, r})
	b, p, _ = planHourlyRollupSplit(100, AdtyWalletConsumeHourly{Bonus: -5, Paid: 105})
	require.Equal(t, [2]int64{0, 100}, [2]int64{b, p})
}

// 切换前已有旧 paid 行的小时 force 重跑：不补 bonus 行（不重复扣账）
func TestB03LegacyRowSkipsWholeUserHour(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 10*usd, 5*usd)
	hb := time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, err := createHourlyRollupLedgerTx(tx, uid, "paid", -3*usd, 0, hb)
		return err
	}))
	b03At(t, hb.Add(time.Minute))
	_, _, _, err := TryReserveUserWallet(uid, 3*usd)
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"paid": -3 * usd}, b03Rollup(t, uid, 3*usd, hb))
}

// H1：开批量也走双钱包。实充 110 赠送 10，扣 150 → 整笔失败不扣负；扣 115 → 赠送 0、实充 5
func TestH1BatchModeStillDualWallet(t *testing.T) {
	b03Setup(t)
	common.BatchUpdateEnabled = true
	uid := b03User(t, 110, 10)
	err := DecreaseUserQuota(uid, 150, false)
	require.ErrorIs(t, err, ErrInsufficientWalletBalance)
	paid, gift := b03Balances(t, uid)
	require.Equal(t, [2]int64{110, 10}, [2]int64{paid, gift})
	require.NoError(t, DecreaseUserQuota(uid, 115, false))
	paid, gift = b03Balances(t, uid)
	require.Equal(t, [2]int64{5, 0}, [2]int64{paid, gift})
	for i := range BatchUpdateTypeCount {
		require.Empty(t, batchUpdateStores[i])
	}
}

// H2/H-B：实充 100 赠送 10，扣 10（扣赠送）后任务失败按记录拆分退 10 → 100/10，写 bonus 流水，不碰拆分表
func TestH2TaskRefundGoesBackToGift(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 100, 10)
	hb := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	b03At(t, hb.Add(time.Minute))
	b, p, err := DecreaseUserWallet(uid, 10)
	require.NoError(t, err)
	b03At(t, hb.Add(90*time.Minute))
	rb, rp, legacy := SplitRefundByRecorded(10, b, p)
	require.False(t, legacy)
	require.NoError(t, RefundUserWalletAfterTheFact(uid, rb, rp, "task_refund", "t1"))
	paid, gift := b03Balances(t, uid)
	require.Equal(t, [2]int64{100, 10}, [2]int64{paid, gift})
	var n int64
	require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND account = ? AND kind = ? AND delta = ?", uid, "bonus", "wallet_refund", 10).Count(&n).Error)
	require.Equal(t, int64(1), n)
	// M-A：消费小时拆分不变，退款小时无记录
	require.Equal(t, int64(10), b03Split(t, uid, hb).Bonus)
	require.Equal(t, AdtyWalletConsumeHourly{}, b03Split(t, uid, hb.Add(time.Hour)))
}

// H2b：实充 100 赠送 0，任务扣 10（全实充）；之后获赠 10 并被别的请求用掉；任务失败退 10 → 实充 100 赠送 0
func TestH2bTaskRefundKeepsPaid(t *testing.T) {
	b03Setup(t)
	uid := b03User(t, 100, 0)
	b, p, err := DecreaseUserWallet(uid, 10)
	require.NoError(t, err)
	require.NoError(t, IncreaseUserBonusQuota(uid, 10, "topup_bonus", "seed", "test", "h2b"))
	_, _, err = DecreaseUserWallet(uid, 10)
	require.NoError(t, err)
	rb, rp, _ := SplitRefundByRecorded(10, b, p)
	require.NoError(t, RefundUserWalletAfterTheFact(uid, rb, rp, "task_refund", "t2"))
	paid, gift := b03Balances(t, uid)
	require.Equal(t, [2]int64{100, 0}, [2]int64{paid, gift})
}

func TestSplitRefundByRecorded(t *testing.T) {
	b, p, l := SplitRefundByRecorded(10, 0, 0)
	require.Equal(t, []any{10, 0, true}, []any{b, p, l})
	b, p, l = SplitRefundByRecorded(30, 10, 20)
	require.Equal(t, []any{10, 20, false}, []any{b, p, l})
	b, p, _ = SplitRefundByRecorded(15, 10, 20)
	require.Equal(t, []int{0, 15}, []int{b, p})
	b, p, _ = SplitRefundByRecorded(25, 3, 20)
	require.Equal(t, []int{5, 20}, []int{b, p})
}
