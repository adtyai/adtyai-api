package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestAdtyFundAllocationRecordsPreChargeSharesPG is ISS-R4-0005 step 1.
//
// A.12.1 requires every fund batch to keep its 冻结/预扣 relation and A.12.2.6
// requires releases and refunds to return "按原分摊". This asserts the schema and
// the pre-charge bookkeeping on a real PostgreSQL instance: one reservation must
// write one share per batch it draws from, FIFO, principal before bonus, and it
// must not touch the batch remaining that settlement still consumes.
func TestAdtyFundAllocationRecordsPreChargeSharesPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyFundBatch{}, &AdtyFundAllocation{}))
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false

	user := User{Username: "iss-r4-0005-share", Password: "pg-acceptance-pass", Quota: 70_000_000}
	require.NoError(t, db.Create(&user).Error)
	// Order A: 40 principal lots, created first.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, user.Id, "order-A", 20_000_000, 20_000_000)
	}))
	// Order B: later lots, so FIFO must drain A before B.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, user.Id, "order-B", 30_000_000, 0)
	}))
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("source = ?", "order-A").Update("created_at", 1).Error)
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("source = ?", "order-B").Update("created_at", 2).Error)

	// Pre-charge 30,000,000: A holds 20 principal + 20 bonus, then B holds 10.
	reserved, err := TryReserveUserQuotaForOperation(user.Id, 30_000_000, "request:share-1")
	require.NoError(t, err)
	require.True(t, reserved)

	var shares []AdtyFundAllocation
	require.NoError(t, DB.Where("user_id = ?", user.Id).Order("seq").Find(&shares).Error)
	// Principal lots total 50,000,000, so 30,000,000 never reaches the bonus lot.
	require.Len(t, shares, 2, "principal before bonus, FIFO across batches")
	require.Equal(t, "principal", shares[0].Kind)
	require.Equal(t, "order-A", shares[0].Source)
	require.EqualValues(t, 20_000_000, shares[0].Held)
	require.Equal(t, "principal", shares[1].Kind)
	require.Equal(t, "order-B", shares[1].Source)
	require.EqualValues(t, 10_000_000, shares[1].Held)
	for _, share := range shares {
		require.Equal(t, "request:share-1", share.OperationId)
		require.EqualValues(t, 0, share.Settled)
	}

	// The batch rows are untouched at pre-charge: settlement still consumes them.
	var batch AdtyFundBatch
	require.NoError(t, DB.Where("user_id = ? AND source = ? AND kind = ?", user.Id, "order-A", "principal").First(&batch).Error)
	require.EqualValues(t, 20_000_000, batch.Remaining)

	// A second reservation records its own shares under its own operation key, so
	// a release can tell the two apart.
	reserved, err = TryReserveUserQuotaForOperation(user.Id, 25_000_000, "request:share-2")
	require.NoError(t, err)
	require.True(t, reserved)
	var second []AdtyFundAllocation
	require.NoError(t, DB.Where("operation_id = ?", "request:share-2").Order("seq").Find(&second).Error)
	var claimed int64
	for _, share := range second {
		claimed += share.Held
	}
	require.EqualValues(t, 25_000_000, claimed, "the whole amount is recorded against its own operation")

	// Held totals are readable for the reconciliation check that comes later.
	// 本金批次总额 50M，第一笔占走 30M，所以第二笔只能再拿 20M 本金；剩下 5M
	// 由赠金批次补足。把已被占用的 30M 从可用量里扣掉，正是这里该守住的不变量。
	heldPrincipal, heldBonus, err := SumAdtyFundAllocationHeld(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, heldPrincipal)
	require.EqualValues(t, 5_000_000, heldBonus)

	// Pre-charging must not make a user's usable quota disappear: capacity stays
	// with users.quota and adty_fund_batches.remaining until settlement lands.
	var afterUser User
	require.NoError(t, DB.First(&afterUser, user.Id).Error)
	require.EqualValues(t, 70_000_000-30_000_000-25_000_000, afterUser.Quota)
}

// TestAdtyFundAllocationTableIsMigratedPG asserts the new table arrives through
// the official migrate on an empty database and survives a repeated startup.
func TestAdtyFundAllocationTableIsMigratedPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)

	require.NoError(t, migrateDB())
	require.True(t, db.Migrator().HasTable(&AdtyFundAllocation{}), "empty-db migrate must create adty_fund_allocations")
	require.True(t, db.Migrator().HasIndex(&AdtyFundAllocation{}, "ux_adty_fund_allocation_share"))
	for _, column := range []string{"user_id", "batch_id", "operation_id", "seq", "kind", "source", "held", "settled"} {
		require.True(t, db.Migrator().HasColumn(&AdtyFundAllocation{}, column), "column %s", column)
	}

	before := snapshotAdtyMigrateSchema(t, db)
	require.NoError(t, migrateDB(), "repeated startup must be idempotent")
	assertAdtyMigrateSchemaUnchanged(t, db, before)
}

// ISS-R4-0049：MigrateAdtyTablesOn 必须在调用方的连接上建表，一处都不能漏。
//
// 它内部还要建索引、回填、删旧索引、建分站表——这些调用以前传的是包级 DB，
// 于是「换一条连接迁移」只对前一半表成立，后半程悄悄落到了另一个库上，
// 调用方随后撞「表不存在」，而现象和「迁移没跑」一模一样。
//
// 这条测试不看实现，只看结果：把包级 DB 指到一个空库，让 On() 迁移另一条
// 连接，然后断言「目标连接上该有的都在，包级 DB 上一个都不该有」。
func TestMigrateAdtyTablesOnUsesOnlyCallerConnectionPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)

	// 被迁移的目标库。
	target := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	// 另一条完全独立的库，冒充「包级 DB」。
	decoy := openAdtyThrowawayMigrateDatabase(t, admin, dsn)

	previousDB := DB
	DB = decoy
	t.Cleanup(func() { DB = previousDB })

	// 先在目标库上铺一张「遗留状态」的工单，用来验第三处副作用（状态重映射）。
	require.NoError(t, target.AutoMigrate(&AdtyTicket{}))
	require.NoError(t, target.Create(&AdtyTicket{
		UserId: 1, Type: "other", Title: "legacy",
		Status: AdtyTicketLegacyAssigned, Priority: "normal",
	}).Error)

	require.NoError(t, MigrateAdtyTablesOn(target))

	// 三处曾经传错连接的调用各自留下可观察的产物：
	//   ① 兑换码订单号唯一索引
	//   ② 活跃团队成员唯一索引
	//   ③ 遗留工单状态重映射
	assert.True(t, target.Migrator().HasIndex(&AdtyVoucher{}, adtyVoucherTradeNoUniqueIndex),
		"兑换码唯一索引必须建在目标连接上")
	assert.True(t, target.Migrator().HasIndex(&AdtyTeamMember{}, activeTeamMembershipUniqueIndex),
		"团队唯一索引必须建在目标连接上")
	var remapped AdtyTicket
	require.NoError(t, target.Where("title = ?", "legacy").First(&remapped).Error)
	assert.Equal(t, AdtyTicketInProgress, remapped.Status,
		"遗留状态 assigned 必须被重映射成 in_progress")

	// 诱饵库这三个产物一个都不该有。
	assert.False(t, decoy.Migrator().HasTable(&AdtyVoucher{}),
		"兑换码表被建到了包级 DB 上：索引那一步仍在用全局 DB")
	assert.False(t, decoy.Migrator().HasTable(&AdtyTeamMember{}),
		"团队成员表被建到了包级 DB 上：索引那一步仍在用全局 DB")

	required := []any{
		&AdtyFundBatch{}, &AdtyFundAllocation{}, &AdtyBillingHold{},
		&AdtySite{}, &AdtySitePrice{}, &AdtySiteEarningDaily{},
		&AdtyVoucher{}, &AdtyTeamMember{}, &AdtyTicket{}, &AdtyPromoCode{},
	}
	for _, entity := range required {
		assert.True(t, target.Migrator().HasTable(entity),
			"目标连接上缺少 %T：说明 On() 有分支落到了包级 DB", entity)
	}

	// 反向：包级 DB 不该被动过。
	for _, entity := range required {
		assert.False(t, decoy.Migrator().HasTable(entity),
			"%T 被建到了包级 DB 上：On() 有分支仍在用全局 DB", entity)
	}
}

// 新列必须出现在空库迁移与重复启动之后（AutoMigrate 加列）。
func TestBillingOperationFundBatchGapColumnMigratesPG(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)

	require.NoError(t, MigrateAdtyTablesOn(db))
	require.True(t, db.Migrator().HasColumn(&BillingOperation{}, "fund_batch_gap"),
		"empty-db migrate must create billing_operations.fund_batch_gap")

	before := snapshotAdtyMigrateSchema(t, db)
	require.NoError(t, MigrateAdtyTablesOn(db), "repeated startup must be idempotent")
	assertAdtyMigrateSchemaUnchanged(t, db, before)
}
