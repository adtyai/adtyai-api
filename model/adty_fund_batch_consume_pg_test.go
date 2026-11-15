package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestAdtyFundBatchFollowsWalletConsumptionPG 是 ISS-R4-0005 的实证测试。
//
// 争议点：正常中转消费只扣 users.quota，资金批次 adty_fund_batches.remaining
// 是否跟着递减。若批次不递减，退款时 adtyOrderLotRemainingTx 读到的剩余本金
// 会是全额，从而多退。
//
// 本测试按主管给的步骤在真实 PostgreSQL 上跑：
//  1. 造用户，本金批次 100 USD = 50,000,000 quota，users.quota 同为 50,000,000
//  2. 预扣 60 USD = 30,000,000 quota
//  3. 结算 ActualQuota == 预扣（delta = 0）
//  4. 断言批次 remaining 与 users.quota 一起递减到 20,000,000
//
// 失败即等于缺陷成立：测试信息里会直接给出四个实际数值。
func TestAdtyFundBatchFollowsWalletConsumptionPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)

	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{}, &BillingOperation{}, &BillingOperationMovement{}))

	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	const (
		principalQuota = int64(50_000_000) // 100 USD，1 USD = 500000 quota
		spentQuota     = int64(30_000_000) // 60 USD
		wantRemaining  = principalQuota - spentQuota
	)

	user := User{Username: "iss-r4-0005", Password: "pg-acceptance-pass", Quota: int(principalQuota)}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, user.Id, "order-A", principalQuota, 0)
	}))

	before := batchRemainingOrFail(t, user.Id)
	require.EqualValues(t, principalQuota, before, "fixture: 批次应为全额本金")

	// 第 2 步：真实预扣入口（不是直接改库）
	reserved, err := TryReserveUserQuota(user.Id, int(spentQuota))
	require.NoError(t, err)
	require.True(t, reserved, "余额足够，预扣必须成功")

	var afterReserve User
	require.NoError(t, DB.First(&afterReserve, user.Id).Error)
	batchAfterReserve := batchRemainingOrFail(t, user.Id)
	t.Logf("预扣后：users.quota=%d（应为 %d）adty_fund_batches.remaining=%d（应递减到 %d）",
		afterReserve.Quota, wantRemaining, batchAfterReserve, wantRemaining)

	// 第 3 步：走真实结算入口，实际等于预扣
	op := BillingOperation{
		OperationKey:  "iss-r4-0005:settle",
		UserId:        user.Id,
		Kind:          "request",
		ReservedQuota: int(spentQuota),
		ActualQuota:   int(spentQuota),
		State:         BillingSettlePending,
	}
	require.NoError(t, DB.Create(&op).Error)
	settled, applied, err := ApplyBillingSettlement(op.OperationKey)
	require.NoError(t, err, "结算必须成功")
	require.True(t, applied, "结算应被应用")
	require.EqualValues(t, BillingSettled, settled.State)

	var afterSettle User
	require.NoError(t, DB.First(&afterSettle, user.Id).Error)
	batchAfterSettle := batchRemainingOrFail(t, user.Id)
	t.Logf("结算后：users.quota=%d adty_fund_batches.remaining=%d",
		afterSettle.Quota, batchAfterSettle)

	// 第 4 步：断言。批次若没减，这里就是缺陷现场。
	require.EqualValues(t, wantRemaining, afterSettle.Quota,
		"users.quota 应按消费额递减（预扣已扣，结算 delta=0）")
	require.EqualValues(t, wantRemaining, batchAfterSettle,
		"ISS-R4-0005：消费 %d quota 后批次 remaining 应为 %d，实际 %d（批次未递减则退款会多退）",
		spentQuota, wantRemaining, batchAfterSettle)

	// 批次合计必须等于活余额，否则偏差会永久累积且无人发现。
	sums, err := SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, int64(afterSettle.Quota)+int64(afterSettle.QuotaPending), sums.Principal+sums.Bonus,
		"批次合计必须等于 quota + quota_pending（当前 principal=%d bonus=%d quota=%d pending=%d）",
		sums.Principal, sums.Bonus, afterSettle.Quota, afterSettle.QuotaPending)
}

func batchRemainingOrFail(t *testing.T, userID int) int64 {
	t.Helper()
	var row AdtyFundBatch
	require.NoError(t, DB.Where("user_id = ? AND kind = ?", userID, "principal").Order("id").First(&row).Error)
	return row.Remaining
}
