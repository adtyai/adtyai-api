package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestAdjustRejectsSubtractHiddenByPendingBatch(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := &User{
		Username: "batch-floor", Password: "x", Group: "default",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000,
	}
	require.NoError(t, DB.Create(user).Error)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -900)

	_, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 500, "pending floor", "pending-floor-1")
	require.ErrorIs(t, err, ErrAdtyQuotaFloor)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, 1000, stored.Quota)
	var ledgers int64
	require.NoError(t, DB.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgers).Error)
	require.Zero(t, ledgers)
}
