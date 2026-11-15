package model

import (
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
func TestLegacyChargeStillConsumesBatchPG(t *testing.T) {
	f := newFundSettleFixture(t, 50_000_000)
	charged, err := ChargeUserQuotaUpTo(f.user.Id, 10_000_000)
	require.NoError(t, err)
	require.EqualValues(t, 10_000_000, charged)

	assert.EqualValues(t, 40_000_000, f.quota(t))
	assert.EqualValues(t, 40_000_000, f.remaining(t), "无分摊的扣费路径必须自己扣批次")
	sums, err := SumAdtyFundBatchRemainingByKind(f.user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, int64(f.quota(t)), sums.Principal+sums.Bonus)
}

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
