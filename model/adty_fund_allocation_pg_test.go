package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	heldPrincipal, heldBonus, err := SumAdtyFundAllocationHeld(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, 55_000_000, heldPrincipal)
	require.EqualValues(t, 0, heldBonus)

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
