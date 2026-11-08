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
	require.NoError(t, opened.DB.AutoMigrate(&User{}, &Token{}))
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := &User{Username: "batch-retry", Password: "x", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DecreaseUserQuota(user.Id, 100, false))

	require.NoError(t, DB.Exec("ALTER TABLE users RENAME TO users_hidden").Error)
	batchUpdate()
	require.NoError(t, DB.Exec("ALTER TABLE users_hidden RENAME TO users").Error)

	var during User
	require.NoError(t, DB.First(&during, user.Id).Error)
	require.Equal(t, 1000, during.Quota)
	require.Equal(t, -100, during.QuotaPending, "failed flush must retain the durable amount")
	// Simulate process loss after the failed flush. Recovery cannot depend on
	// either the old local hint or requeueing an amount in process memory.
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	FlushBatchUpdates()
	var after User
	require.NoError(t, DB.First(&after, user.Id).Error)
	require.Equal(t, 900, after.Quota)
	require.Zero(t, after.QuotaPending)
	FlushBatchUpdates()
	require.NoError(t, DB.First(&after, user.Id).Error)
	require.Equal(t, 900, after.Quota, "a repeated flush must not apply the amount twice")
}
