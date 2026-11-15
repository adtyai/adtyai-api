package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ISS-R4-0026：批次扣减必须是数据库级保证，不是「内存里算好绝对值再写回」。
//
// consumeAdtyFundBatchesTx 与 ReclaimAdtyFundBatchesBySourceTx 都换成
// deductAdtyFundBatchTx 那套三件套（WHERE remaining >= ? + gorm.Expr +
// RowsAffected 校验）。三件事分开验：
//
//  1. 扣到刚好够：take == remaining 的边界必须成功。
//  2. 扣不动：批次不够时必须是 ErrAdtyFundBatchShortfall，不能静默成功。
//  3. 丢更新：另一笔事务先提交了扣减时，这里必须报 shortfall，而不是把对方
//     已提交的 remaining 覆盖回自己读到的旧值。第 3 条是裸 Update 唯一会
//     「安静地错」的地方——把实现改回裸 Update，这两条 PG 用例必红。
//
// 前提说明：五条生产路径当前都持 users 行锁，所以丢更新今天不会发生。
// 这些用例验的不是「今天坏了」，而是「把未声明的约定换成数据库自己的保证」。

func TestConsumeFundBatchesTakesExactlyTheRemainingLot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: 2601, Kind: "principal", Source: "exact", Original: 100, Remaining: 100,
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 2601, 100, 100)
	}), "批次与余额都是 100，扣 100 是「刚好够」，必须成功")

	var lot AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 2601).First(&lot).Error)
	require.EqualValues(t, 0, lot.Remaining)

	// 再扣 1：批次已空、余额也是 0，必须报 shortfall，不能扣成 -1。
	err = db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 2601, 1, 0)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	require.NoError(t, db.First(&lot, lot.Id).Error)
	require.EqualValues(t, 0, lot.Remaining, "扣不动时库里不许出现负数")
}

func TestReclaimFundBatchesTakesExactlyTheRemainingLot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 2602, Kind: "principal", Source: "order-exact", Original: 80, Remaining: 80},
		{UserId: 2602, Kind: "bonus", Source: "order-exact", Original: 20, Remaining: 20},
		{UserId: 2602, Kind: "principal", Source: "order-other", Original: 50, Remaining: 50},
	}).Error)

	var taken int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		taken, err = ReclaimAdtyFundBatchesBySourceTx(tx, 2602, "order-exact", 80, 20)
		return err
	}))
	require.EqualValues(t, 100, taken, "本金 80 + 赠送 20 刚好是这一来源的全部剩余")

	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 2602).Order("id").Find(&rows).Error)
	got := map[string]int64{}
	for _, row := range rows {
		got[row.Source+":"+row.Kind] = row.Remaining
	}
	require.EqualValues(t, 0, got["order-exact:principal"])
	require.EqualValues(t, 0, got["order-exact:bonus"])
	require.EqualValues(t, 50, got["order-other:principal"], "别的来源不许被动")
}

// ── 以下两条必须真实 PostgreSQL：行级锁与「条件更新重新求值」是 PG 语义，
// SQLite 的单写者模型证明不了这两件事。

func openAdtyFundBatchAtomicPG(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	return db
}

// holdAdtyFundBatchRow 在独立事务里占住批次行，模拟「另一个事务已经扣过、
// 但提交还没发生」的瞬间。
func holdAdtyFundBatchRow(t *testing.T, db *gorm.DB, batchID int64) *gorm.DB {
	t.Helper()
	held := db.Begin()
	require.NoError(t, held.Error)
	require.NoError(t, held.Exec("SELECT id FROM adty_fund_batches WHERE id = ? FOR UPDATE", batchID).Error)
	t.Cleanup(func() { _ = held.Rollback().Error })
	return held
}

// waitAdtyBlockedOnRowLock 等到扣减语句确实卡在行锁上再放行。这样「读旧值 →
// 写入被挡 → 解挡后按新值重判条件」是必然而不是靠 sleep 撞运气。
func waitAdtyBlockedOnRowLock(t *testing.T, db *gorm.DB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int64
		require.NoError(t, db.Raw("SELECT count(*) FROM pg_locks WHERE NOT granted").Scan(&waiting).Error)
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("扣减的 UPDATE 没有在 10s 内等到行锁：用例前提不成立")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitAdtyTxResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatalf("持锁方提交后扣减仍未返回（疑似死锁）")
		return nil
	}
}

func TestConsumeFundBatchesDoesNotOverwriteConcurrentDeductionPG(t *testing.T) {
	db := openAdtyFundBatchAtomicPG(t)

	const userID = 2603
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: userID, Kind: "principal", Source: "race", Original: 100, Remaining: 100,
	}).Error)
	var lot AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", userID, "race").First(&lot).Error)

	held := holdAdtyFundBatchRow(t, db, lot.Id)

	done := make(chan error, 1)
	go func() {
		done <- db.Transaction(func(tx *gorm.DB) error {
			return ConsumeAdtyFundBatchesTx(tx, userID, 60, 100)
		})
	}()

	waitAdtyBlockedOnRowLock(t, db)

	// 另一笔扣减把这一行扣到 30 后提交：此刻本次扣的 60 已经扣不动了。
	require.NoError(t, held.Exec("UPDATE adty_fund_batches SET remaining = 30 WHERE id = ?", lot.Id).Error)
	require.NoError(t, held.Commit().Error)

	err := waitAdtyTxResult(t, done)
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall,
		"读到 100 但已被别人扣到 30，扣 60 必须报 shortfall；裸 Update 会「成功」并把 30 覆盖成 40")

	var after AdtyFundBatch
	require.NoError(t, db.First(&after, lot.Id).Error)
	require.EqualValues(t, 30, after.Remaining, "已提交的扣减被覆盖 = 丢更新")
}

func TestReclaimFundBatchesDoesNotOverwriteConcurrentDeductionPG(t *testing.T) {
	db := openAdtyFundBatchAtomicPG(t)

	const userID = 2604
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: userID, Kind: "principal", Source: "race", Original: 100, Remaining: 100,
	}).Error)
	var lot AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", userID, "race").First(&lot).Error)

	held := holdAdtyFundBatchRow(t, db, lot.Id)

	done := make(chan error, 1)
	go func() {
		done <- db.Transaction(func(tx *gorm.DB) error {
			_, err := ReclaimAdtyFundBatchesBySourceTx(tx, userID, "race", 60, 0)
			return err
		})
	}()

	waitAdtyBlockedOnRowLock(t, db)

	require.NoError(t, held.Exec("UPDATE adty_fund_batches SET remaining = 30 WHERE id = ?", lot.Id).Error)
	require.NoError(t, held.Commit().Error)

	err := waitAdtyTxResult(t, done)
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall,
		"读到 100 但已被别人扣到 30，回收 60 必须报 shortfall；裸 Update 会静默把 30 覆盖成 40")

	var after AdtyFundBatch
	require.NoError(t, db.First(&after, lot.Id).Error)
	require.EqualValues(t, 30, after.Remaining, "已提交的扣减被覆盖 = 丢更新")
}
