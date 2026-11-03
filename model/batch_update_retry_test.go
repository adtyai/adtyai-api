package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestBatchUpdateRequeuesFailedQuotaAndFlushAppliesIt(t *testing.T) {
	opened, err := OpenAdtyTestDatabase()
	if errors.Is(err, ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	require.NoError(t, opened.DB.AutoMigrate(&User{}))
	resetBatchUpdateTestState(t)

	user := &User{Username: "batch-retry", Password: "x", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000}
	require.NoError(t, DB.Create(user).Error)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -100)

	require.NoError(t, DB.Exec("ALTER TABLE users RENAME TO users_hidden").Error)
	batchUpdate()
	require.NoError(t, DB.Exec("ALTER TABLE users_hidden RENAME TO users").Error)

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	queued := batchUpdateStores[BatchUpdateTypeUserQuota][user.Id]
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	require.Equal(t, -100, queued, "写库失败要把差额放回队列")

	var during User
	require.NoError(t, DB.First(&during, user.Id).Error)
	require.Equal(t, 1000, during.Quota)

	FlushBatchUpdates()
	var after User
	require.NoError(t, DB.First(&after, user.Id).Error)
	require.Equal(t, 900, after.Quota)
}
