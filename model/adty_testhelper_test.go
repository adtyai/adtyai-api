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
	migrateAdtyTestSchema(t, opened.DB)
	return opened.DB
}

// migrateAdtyTestSchema 一次建齐计费链路会用到的表。
//
// 逐个测试自己挑表是个陷阱：实现每给扣费路径加一张表，就有一批测试在运行时
// 撞出「adty fund batch table is missing」这种一句话错误，看不出缺的是哪张。
// 这里统一建齐，测试只管造数据。
func migrateAdtyTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &TopUp{}, &Option{}, &Log{}))
	require.NoError(t, MigrateAdtyTables())
}
