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
