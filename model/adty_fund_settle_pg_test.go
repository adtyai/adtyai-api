package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fundSettleFixture 造一个带本金批次和预扣的用户。
// 所有用例都在隔离 PostgreSQL 上跑：分摊与批次是同事务变更，SQLite 无法证明。
type fundSettleFixture struct {
	db   *gorm.DB
	user User
}

func newFundSettleFixture(t *testing.T, principal int64) *fundSettleFixture {
	t.Helper()
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{},
		&BillingOperation{}, &BillingOperationMovement{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	user := User{Username: "fund-settle", Password: "pg-acceptance-pass", Quota: int(principal)}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, user.Id, "order-A", principal, 0)
	}))
	return &fundSettleFixture{db: db, user: user}
}

func (f *fundSettleFixture) reserve(t *testing.T, quota int, key string) {
	t.Helper()
	ok, err := TryReserveUserQuotaForOperation(f.user.Id, quota, key)
	require.NoError(t, err)
	require.True(t, ok)
}

func (f *fundSettleFixture) settle(t *testing.T, key string, actual int) {
	t.Helper()
	op := BillingOperation{
		OperationKey: key, UserId: f.user.Id, Kind: "request",
		ReservedQuota: actual + 0, ActualQuota: actual, State: BillingSettlePending,
	}
	require.NoError(t, f.db.Create(&op).Error)
	_, _, err := ApplyBillingSettlement(key)
	require.NoError(t, err)
}

func (f *fundSettleFixture) remaining(t *testing.T) int64 {
	t.Helper()
	var row AdtyFundBatch
	require.NoError(t, f.db.Where("user_id = ? AND kind = ?", f.user.Id, "principal").Order("id").First(&row).Error)
	return row.Remaining
}

func (f *fundSettleFixture) quota(t *testing.T) int {
	t.Helper()
	var user User
	require.NoError(t, f.db.First(&user, f.user.Id).Error)
	return user.Quota
}

func (f *fundSettleFixture) held(t *testing.T, key string) int64 {
	t.Helper()
	var sum struct{ Held int64 }
	require.NoError(t, f.db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("operation_id = ?", key).Scan(&sum).Error)
	return sum.Held
}

// 结算等于预扣：批次按实际消耗扣，分摊转实扣，批次总量与活余额一致。
func TestAdtyFundSettleConsumesBatchPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	f.reserve(t, 30_000_000, "request:settle-1")
	op := BillingOperation{OperationKey: "request:settle-1", UserId: f.user.Id, Kind: "request",
		ReservedQuota: 30_000_000, ActualQuota: 30_000_000, State: BillingSettlePending}
	require.NoError(t, f.db.Create(&op).Error)
	_, _, err := ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err)

	assert.EqualValues(t, 20_000_000, f.quota(t), "users.quota 扣掉 30,000,000")
	assert.EqualValues(t, 20_000_000, f.remaining(t), "批次同步扣掉 30,000,000")
	assert.EqualValues(t, 0, f.held(t, "request:settle-1"), "分摊全部转实扣")
	sums, err := SumAdtyFundBatchRemainingByKind(f.user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, int64(f.quota(t)), sums.Principal+sums.Bonus, "批次合计必须等于活余额")
}

// 实际小于预扣（退款）：批次不动，只释放没用到的那部分分摊。
func TestAdtyFundSettleRefundsWithoutTouchingBatchPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	f.reserve(t, 30_000_000, "request:refund-1")
	op := BillingOperation{OperationKey: "request:refund-1", UserId: f.user.Id, Kind: "request",
		ReservedQuota: 30_000_000, ActualQuota: 12_000_000, State: BillingSettlePending}
	require.NoError(t, f.db.Create(&op).Error)
	_, _, err := ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err)

	assert.EqualValues(t, 38_000_000, f.quota(t), "只消费 12,000,000，其余退回")
	assert.EqualValues(t, 38_000_000, f.remaining(t), "批次只扣实际消耗")
	assert.EqualValues(t, 0, f.held(t, "request:refund-1"))
}

// 全额退款（请求失败）：批次一分不动，分摊全释放。
func TestAdtyFundFullRefundLeavesBatchUntouchedPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	spec := BillingOperation{OperationKey: "request:failed-1", UserId: f.user.Id, Kind: "request"}
	_, err := ReserveBillingOperation(spec, 30_000_000)
	require.NoError(t, err)
	require.NoError(t, QueueBillingSettlement("request:failed-1", 0, true))
	_, _, err = ApplyBillingSettlement("request:failed-1")
	require.NoError(t, err)

	assert.EqualValues(t, 50_000_000, f.quota(t), "预扣全额退回")
	assert.EqualValues(t, 50_000_000, f.remaining(t), "批次从未被扣，不需要回补")
	assert.EqualValues(t, 0, f.held(t, "request:failed-1"))
}

// 补扣：先预扣 20,000,000，再补到 30,000,000，结算按 30,000,000 扣。
func TestAdtyFundTopUpReservationSettlesFullAmountPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	spec := BillingOperation{OperationKey: "request:grow-1", UserId: f.user.Id, Kind: "request"}
	_, err := ReserveBillingOperation(spec, 20_000_000)
	require.NoError(t, err)
	_, err = ReserveBillingOperation(spec, 30_000_000)
	require.NoError(t, err)

	assert.EqualValues(t, 20_000_000, f.quota(t), "两次预扣共扣 30,000,000")
	require.NoError(t, f.db.Model(&BillingOperation{}).
		Where("operation_key = ?", "request:grow-1").
		Updates(map[string]any{"actual_quota": 30_000_000, "state": BillingSettlePending}).Error)
	_, _, err = ApplyBillingSettlement("request:grow-1")
	require.NoError(t, err)

	assert.EqualValues(t, 20_000_000, f.remaining(t), "结算按累计 30,000,000 扣批次")
	sums, sumErr := SumAdtyFundBatchRemainingByKind(f.user.Id)
	require.NoError(t, sumErr)
	assert.EqualValues(t, int64(f.quota(t)), sums.Principal+sums.Bonus)
}

// 批次不够覆盖实际消耗时必须失败回滚，不能悄悄少扣。
func TestAdtyFundSettleFailsWhenBatchesCannotCoverPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	f.reserve(t, 30_000_000, "request:short-1")
	// 人为把批次改小，模拟历史漂移；此时结算应报错并整体回滚。
	require.NoError(t, f.db.Model(&AdtyFundBatch{}).
		Where("user_id = ?", f.user.Id).Update("remaining", 5_000_000).Error)
	op := BillingOperation{OperationKey: "request:short-1", UserId: f.user.Id, Kind: "request",
		ReservedQuota: 30_000_000, ActualQuota: 30_000_000, State: BillingSettlePending}
	require.NoError(t, f.db.Create(&op).Error)
	_, _, err := ApplyBillingSettlement(op.OperationKey)
	require.Error(t, err)

	var after BillingOperation
	require.NoError(t, f.db.Where("operation_key = ?", "request:short-1").First(&after).Error)
	assert.Equal(t, BillingSettlePending, after.State, "失败不能把状态推进到已结算")
	assert.EqualValues(t, 20_000_000, f.quota(t), "回滚后余额保持预扣后的值")
}

// 结算幂等：同一次结算被重放时，批次只能被扣一次。
// 恢复任务、重复 Apply、上游重试都可能走到这里。
func TestAdtyFundSettleIsIdempotentPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	f.reserve(t, 30_000_000, "request:idem-1")
	op := BillingOperation{OperationKey: "request:idem-1", UserId: f.user.Id, Kind: "request",
		ReservedQuota: 30_000_000, ActualQuota: 30_000_000, State: BillingSettlePending}
	require.NoError(t, f.db.Create(&op).Error)

	_, _, err := ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err)
	first := f.remaining(t)

	for i := 0; i < 3; i++ {
		_, _, err = ApplyBillingSettlement(op.OperationKey)
		require.NoError(t, err)
		require.EqualValues(t, first, f.remaining(t), "重放结算不能再扣一次批次")
	}
	assert.EqualValues(t, 20_000_000, first)
	assert.EqualValues(t, 20_000_000, f.quota(t))
}

// legacy 扣费路径（没有 operationId 的分摊）仍然要扣批次。
//
// 这条是「把 chargeUserQuotaUpToDB 改成只扣余额、批次全交给分摊」那个方案的反证：
// 该路径没有分摊可依，若批次不再由它扣，这些消费就永远不递减本金，
// 正是本单要修的缺陷本身。
// 团队 Key 由团队共享余额付款，成员的个人本金批次不能参与。
//
// 这是「扣错人」级别的缺陷：如果结算时按 UserId 去扣批次，团队消费会把员工
// 自己的本金吃掉。ReserveBillingOperation 的团队分支在登记分摊之前就返回了，
// 所以这个 operation 名下本来就没有分摊；结算必须据此跳过，而不是把 applied
// 当成「没登记过的旧账单」去扣个人批次。
func TestTeamKeyConsumptionLeavesMemberBatchesUntouchedPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	require.NoError(t, f.db.AutoMigrate(&AdtyTeamKey{}, &Token{}))
	require.NoError(t, f.db.Create(&AdtyTeamKey{
		TokenId: 9001, TeamId: 1, UserId: f.user.Id, Status: "active",
	}).Error)

	previousHook := AdtyTeamReserveHook
	AdtyTeamReserveHook = func(tx *gorm.DB, tokenId, userId, delta int, ref string) (bool, error) {
		return true, nil // 模拟团队余额已扣
	}
	t.Cleanup(func() { AdtyTeamReserveHook = previousHook })

	spec := BillingOperation{OperationKey: "request:team-1", UserId: f.user.Id, TokenId: 9001, Kind: "request"}
	_, err := ReserveBillingOperation(spec, 30_000_000)
	require.NoError(t, err)

	assert.EqualValues(t, 50_000_000, f.quota(t), "团队付款不动成员个人余额")
	assert.EqualValues(t, 50_000_000, f.remaining(t), "预扣阶段不动个人批次")
	assert.EqualValues(t, 0, f.held(t, "request:team-1"), "团队付款不登记个人分摊")

	require.NoError(t, f.db.Model(&BillingOperation{}).
		Where("operation_key = ?", "request:team-1").
		Updates(map[string]any{"actual_quota": 30_000_000, "state": BillingSettlePending}).Error)
	_, _, err = ApplyBillingSettlement("request:team-1")
	require.NoError(t, err)

	assert.EqualValues(t, 50_000_000, f.remaining(t), "结算也不能扣成员个人批次")
	assert.EqualValues(t, 50_000_000, f.quota(t), "团队付款不在成员个人账上再扣一次")
}

// ISS-R4-0033：两个并发预扣不能把同一笔批次额度各登记一次。
//
// 修前 :102 的可用量只取 batch.Remaining，不减别人已占用的 held，于是两个
// operation 都能登记满额；结算时条件更新只允许一个成功，另一个整事务回滚，
// 那个请求已经调过上游、却再也结不了账。
func TestAdtyFundAllocationDoesNotDoubleClaimBatchPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)

	for _, spec := range []struct {
		key   string
		quota int
	}{
		{"request:claim-1", 30_000_000},
		{"request:claim-2", 20_000_000},
	} {
		_, err := ReserveBillingOperation(BillingOperation{
			OperationKey: spec.key, UserId: f.user.Id, Kind: "request",
		}, spec.quota)
		require.NoError(t, err)
	}

	var total struct{ Held int64 }
	require.NoError(t, f.db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as held").
		Where("user_id = ?", f.user.Id).Scan(&total).Error)
	assert.EqualValues(t, 50_000_000, total.Held, "所有预扣登记加起来不能超过批次余量")

	// 两条都已登记的预扣都能正常结算，不需要任何一条回滚。
	for _, spec := range []struct {
		key   string
		quota int
	}{
		{"request:claim-1", 30_000_000},
		{"request:claim-2", 20_000_000},
	} {
		require.NoError(t, QueueBillingSettlement(spec.key, spec.quota, false))
		_, _, err := ApplyBillingSettlement(spec.key)
		require.NoError(t, err, "已登记的预扣必须都能结清")
	}
	assert.EqualValues(t, 0, f.remaining(t))
}

// 32 个 goroutine 抢同一批次：登记总量不能超过批次余量。
//
// 关键在夹具：**让 users.quota 大于批次余量**（drift 状态，正是存量数据的形状）。
// 账面一致时 quota 那道闸会先把总量卡在 remaining 上，批次守卫根本没被考到——
// 我第一版就是这么写的，把守卫去掉测试照样绿，是个假阳性。
func TestAdtyFundAllocationConcurrentClaimStaysUnderBatchPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	// 只留 20,000,000 批次余量，而 quota 仍是 50,000,000：模拟 drift。
	require.NoError(t, f.db.Model(&AdtyFundBatch{}).
		Where("user_id = ?", f.user.Id).Update("remaining", 20_000_000).Error)

	const workers = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	var unexpected []error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("request:burst-%d", idx)
			_, err := ReserveBillingOperation(BillingOperation{
				OperationKey: key, UserId: f.user.Id, Kind: "request",
			}, 2_000_000)
			if err != nil && !errors.Is(err, ErrBillingWalletInsufficient) {
				mu.Lock()
				unexpected = append(unexpected, err)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, unexpected, "并发下只允许「余额不足」这一种拒绝")

	var held struct{ Total int64 }
	require.NoError(t, f.db.Model(&AdtyFundAllocation{}).
		Select("coalesce(sum(held - settled),0) as total").
		Where("user_id = ?", f.user.Id).Scan(&held).Error)
	assert.LessOrEqual(t, held.Total, int64(20_000_000),
		"登记的预扣总额不得超过批次余量（%d > 20,000,000 即为双登记）", held.Total)
	assert.Positive(t, held.Total, "至少有一笔应当登记成功")
}

// 老用户：升级前就有额度、从来没有过批次。结算必须能走通。
//
// 这条是 `TestAdtyBalanceFloor*` 在 service 包暴露的回归在 model 层的等价物：
// 预扣把余额扣成 0 之后，若把「此刻余额」当成补建 legacy 批次的基数，补出来的
// 批次只有 0，本次消耗一分都扣不动，整笔结算失败、重试永远失败——而用户其实
// 只是「老、没有批次」，不是账面有问题。
func TestLegacyUserWithoutBatchesCanSettlePG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{},
		&BillingOperation{}, &BillingOperationMovement{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	// 只有余额，一行批次都没有。
	const principal = 20_000
	user := User{Username: "legacy-no-lot", Password: "pg-acceptance-pass", Quota: principal}
	require.NoError(t, db.Create(&user).Error)

	// 预扣把余额扣到 0（这正是「全花完」的最坏情形）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return AllocateAdtyFundAllocationsTx(tx, user.Id, "request:legacy-1", principal, principal)
	}))
	reserved, err := TryReserveUserQuotaForOperation(user.Id, principal, "request:legacy-1")
	require.NoError(t, err)
	require.True(t, reserved)

	var afterReserve User
	require.NoError(t, db.First(&afterReserve, user.Id).Error)
	require.EqualValues(t, 0, afterReserve.Quota, "全部预扣")

	op := BillingOperation{OperationKey: "request:legacy-1", UserId: user.Id, Kind: "request",
		ReservedQuota: principal, ActualQuota: principal, State: BillingSettlePending}
	require.NoError(t, db.Create(&op).Error)
	_, _, err = ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err, "老用户没有批次也必须能结算")

	var userAfter User
	require.NoError(t, db.First(&userAfter, user.Id).Error)
	assert.EqualValues(t, 0, userAfter.Quota)

	// 补建的 legacy 批次应正好被本次消耗吃干净。
	sums, err := SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, int64(userAfter.Quota)+int64(userAfter.QuotaPending), sums.Principal+sums.Bonus,
		"批次合计必须等于活余额")
	assert.Zero(t, sums.Principal, "本金批次应被吃干净")
}

// 老用户 + 预扣额超过半余额：这是生产上的常见形态，也是 63e169d 的回归点。
//
// 余额 100,000、预扣 80,000 之后只剩 20,000；如果补建 legacy 批次时用「此刻
// 余额」，补出来的批次只有 20,000，扣不动 80,000，结算整笔失败。修法是补建
// 基数取「这次业务消耗之前的余额」= 此刻余额 + 已预扣的 base。
func TestLegacyUserPartialReserveSettlesPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{},
		&BillingOperation{}, &BillingOperationMovement{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	const principal = 100_000
	const reserved = 80_000
	user := User{Username: "legacy-partial", Password: "pg-acceptance-pass", Quota: principal}
	require.NoError(t, db.Create(&user).Error)

	ok, err := TryReserveUserQuotaForOperation(user.Id, reserved, "request:legacy-partial-1")
	require.NoError(t, err)
	require.True(t, ok)

	var afterReserve User
	require.NoError(t, db.First(&afterReserve, user.Id).Error)
	require.EqualValues(t, principal-reserved, afterReserve.Quota, "预扣 80,000 后剩 20,000")

	op := BillingOperation{OperationKey: "request:legacy-partial-1", UserId: user.Id, Kind: "request",
		ReservedQuota: reserved, ActualQuota: reserved, State: BillingSettlePending}
	require.NoError(t, db.Create(&op).Error)
	_, _, err = ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err, "预扣额超过半余额时老用户也必须能结算")

	var after User
	require.NoError(t, db.First(&after, user.Id).Error)
	assert.EqualValues(t, principal-reserved, after.Quota, "全部消费后余额为 20,000")

	sums, err := SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, int64(after.Quota)+int64(after.QuotaPending), sums.Principal+sums.Bonus,
		"补建批次扣完本次消耗后，剩余必须等于活余额")
	assert.EqualValues(t, principal-reserved, sums.Principal)
}

// 账本缺口必须落库，不能只写日志。
//
// 场景：用户有批次，但批次总量小于活余额（存量 drift 的形状）。
// 本次消耗 30,000，批次只支撑得起 20,000 → fund_batch_gap = 10,000。
// 对账口径是 `select user_id, sum(fund_batch_gap) group by user_id`，
// 所以这一列必须非零且能定位到 operation。
func TestAdtyFundSettlementRecordsBatchGapPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{},
		&BillingOperation{}, &BillingOperationMovement{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	const principal = 50_000_000
	const batchOnly = 20_000_000 // 只给 20M 批次，活余额仍是 50M
	const spend = 30_000_000

	user := User{Username: "gap-shape", Password: "pg-acceptance-pass", Quota: principal}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, user.Id, "order-gap", batchOnly, 0)
	}))

	ok, err := TryReserveUserQuotaForOperation(user.Id, spend, "request:gap-1")
	require.NoError(t, err)
	require.True(t, ok)

	op := BillingOperation{OperationKey: "request:gap-1", UserId: user.Id, Kind: "request",
		ReservedQuota: spend, ActualQuota: spend, State: BillingSettlePending}
	require.NoError(t, db.Create(&op).Error)
	settled, applied, err := ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, BillingSettled, settled.State)

	assert.EqualValues(t, spend-batchOnly, settled.FundBatchGap,
		"批次只支撑 %d，本次实扣 %d，缺口应为 %d", batchOnly, spend, spend-batchOnly)

	// 对账查询本身也要能跑：按用户聚合，非零即漂移。
	var drift struct{ Total int64 }
	require.NoError(t, db.Model(&BillingOperation{}).
		Select("coalesce(sum(fund_batch_gap),0) as total").
		Where("user_id = ?", user.Id).Scan(&drift).Error)
	assert.EqualValues(t, spend-batchOnly, drift.Total)

	var batch AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&batch).Error)
	assert.Zero(t, batch.Remaining, "批次被扣到 0，差额就是撑不住的那部分")
}

// 账是平的（批次 = 活余额）时，gap 必须是 0——不能因为「走过这条路径」就记一笔。
func TestAdtyFundSettlementGapIsZeroWhenBooksBalancePG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{},
		&BillingOperation{}, &BillingOperationMovement{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	const principal = 50_000_000
	const spend = 30_000_000
	user := User{Username: "gap-zero", Password: "pg-acceptance-pass", Quota: principal}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, user.Id, "order-even", principal, 0)
	}))
	ok, err := TryReserveUserQuotaForOperation(user.Id, spend, "request:gap-0")
	require.NoError(t, err)
	require.True(t, ok)

	op := BillingOperation{OperationKey: "request:gap-0", UserId: user.Id, Kind: "request",
		ReservedQuota: spend, ActualQuota: spend, State: BillingSettlePending}
	require.NoError(t, db.Create(&op).Error)
	settled, _, err := ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err)
	assert.Zero(t, settled.FundBatchGap, "批次足够时不该记缺口")
}

// 花光批次 → 退款回额度 → 再消费：这是懒建 legacy 批次撞唯一索引的那条路。
//
// `adty_fund_batches` 的唯一索引是 (user_id, source, kind)。修前懒建用的是裸
// source='legacy'，所以每个用户只能成功一次：第二次进入这条分支时 Create 撞
// 唯一索引，原始 DB 错误让整笔扣费失败。退款走 IncreaseUserQuota，不会补批次，
// 所以「花光 + 退款 + 再消费」必然复现。
func TestLegacyBatchSurvivesExhaustionAndRefundPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{},
		&BillingOperation{}, &BillingOperationMovement{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	const first = 20_000
	user := User{Username: "legacy-refund-cycle", Password: "pg-acceptance-pass", Quota: first}
	require.NoError(t, db.Create(&user).Error)

	settleOnce := func(t *testing.T, key string, quota int) {
		t.Helper()
		ok, err := TryReserveUserQuotaForOperation(user.Id, quota, key)
		require.NoError(t, err)
		require.True(t, ok)
		op := BillingOperation{OperationKey: key, UserId: user.Id, Kind: "request",
			ReservedQuota: quota, ActualQuota: quota, State: BillingSettlePending}
		require.NoError(t, db.Create(&op).Error)
		_, _, err = ApplyBillingSettlement(op.OperationKey)
		require.NoError(t, err, "第 %s 次消费必须能结算", key)
	}

	// 第一次：零批次用户消费掉全部余额，懒建出 source='legacy' 的那一行。
	settleOnce(t, "request:legacy-cycle-1", first)

	var afterFirst User
	require.NoError(t, db.First(&afterFirst, user.Id).Error)
	require.EqualValues(t, 0, afterFirst.Quota)
	var lots []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&lots).Error)
	require.Len(t, lots, 1, "只应有一条 legacy 批次")
	require.EqualValues(t, 0, lots[0].Remaining, "第一次消费把它花光了")

	// 退款回额度：IncreaseUserQuota 不建批次，这正是撞车的触发条件。
	require.NoError(t, IncreaseUserQuota(user.Id, first, false))

	// 退款这一刻账是**不平**的，而且差额正好等于退款金额——这是实测结果，
	// 不是假设：`IncreaseUserQuota` 只加 `users.quota`，不建/不补批次。
	//
	// 所以「消费前 Σremaining == quota + pending」这条断言在这里不成立；
	// 真正的补偿发生在下一次扣费（懒建按 pre-operation 余额补齐）。这里把
	// 已知差额显式断言出来，它既是事实记录，也是「为什么必须有懒建」的证据。
	var afterRefund User
	require.NoError(t, db.First(&afterRefund, user.Id).Error)
	sumsAfterRefund, err := SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, first, int64(afterRefund.Quota)+int64(afterRefund.QuotaPending),
		"退款后活余额回到原值")
	require.Zero(t, sumsAfterRefund.Principal+sumsAfterRefund.Bonus,
		"退款不建批次：这是已知差额的来源")
	assert.EqualValues(t, first,
		int64(afterRefund.Quota)+int64(afterRefund.QuotaPending)-(sumsAfterRefund.Principal+sumsAfterRefund.Bonus),
		"退款后的已知差额等于退款金额")

	// 第二次：批次存在但 remaining=0，必须能补额度并继续消费，而不是撞唯一索引。
	settleOnce(t, "request:legacy-cycle-2", first)

	var afterSecond User
	require.NoError(t, db.First(&afterSecond, user.Id).Error)
	assert.EqualValues(t, 0, afterSecond.Quota)

	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&lots).Error)
	require.Len(t, lots, 1, "补额度不能新插一行：唯一索引 (user_id, source, kind)")
	assert.EqualValues(t, 2*first, lots[0].Original, "同一条 legacy 批次累加两次")
	assert.EqualValues(t, 0, lots[0].Remaining)

	// 这条用例守的核心事实：**退款再消费必须记成账本缺口，且金额正好等于退款**。
	//
	// 我最初在这里断言 gap == 0，理由是「结算按 pre-operation 余额补齐了」。
	// 那个理由是循环论证：补齐是消费路径自己凭空造出来的额度，用造出来的额度
	// 去证明「没有缺口」，等于让 `fund_batch_gap` 结构上恒为 0。把 backed 改成
	// 「补建前真实覆盖」之后，这里读出 20,000 —— 而这正是事实：
	// `IncreaseUserQuota` 只加 `users.quota`，不建也不补批次，A.12 要求的
	// 「退款按原分摊回退并保留原批次」在退款路径上根本没实现。
	// 这条 gap 就是那个缺陷的可观测症状，不是误报。
	var gap struct{ Total int64 }
	require.NoError(t, db.Model(&BillingOperation{}).
		Select("coalesce(sum(fund_batch_gap),0) as total").
		Where("user_id = ?", user.Id).Scan(&gap).Error)
	assert.EqualValues(t, first, gap.Total,
		"退款路径不回补批次，缺口必须等于退款金额；gap 读到 0 说明这一列被抵消了")
}
