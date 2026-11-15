package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// adtyTestDB 把当前测试换到独立的 PostgreSQL schema，结束时换回并删掉 schema。
// 没设 ADTY_TEST_PG_DSN 就跳过。不要和 t.Parallel 一起用：换的是包级 DB。
func adtyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	opened, err := OpenAdtyTestDatabase()
	if errors.Is(err, ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	return opened.DB
}

// adtyTestDBWithSchema 和 adtyTestDB 一样开一个独立 schema，但额外把二开
// 的全部表建好。
//
// 为什么不直接塞进 adtyTestDB：有些测试**故意**要先看到「表不存在」或先建一张
// 历史结构的表再让迁移去改它（`TestEnsureAdtySitePriceUniqueIndexMigratesLegacyIndex`
// 就是），无条件迁移会让它们撞「表已存在」。要全量 schema 的测试显式用这个。
func adtyTestDBWithSchema(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	migrateAdtyTestSchema(t, db)
	return db
}

// migrateAdtyTestSchema 一次建齐计费链路会用到的表。
//
// 逐个测试自己挑表是个陷阱：实现每给扣费路径加一张表，就有一批测试在运行时
// 撞出「adty fund batch table is missing」这种一句话错误，看不出缺的是哪张。
// 这里统一建齐，测试只管造数据。
func migrateAdtyTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &TopUp{}, &Option{}, &Log{}))
	// 必须用 On(db) 而不是全局版本：调用方传进来的库不一定是包级 DB，
	// 全局版本会「成功」地迁移另一套表，随后测试撞「表不存在」。
	require.NoError(t, MigrateAdtyTablesOn(db))
	require.True(t, db.Migrator().HasTable(&AdtyFundBatch{}), "migration must land on the caller's connection")
	require.True(t, db.Migrator().HasTable(&AdtyFundAllocation{}))
}
