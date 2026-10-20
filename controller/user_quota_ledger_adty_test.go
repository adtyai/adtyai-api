package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestAdjustUserQuotaWritesPaidLedger(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Username: "ledger-quota", Quota: 1000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)

	adjustment, err := model.AdjustUserQuota(user.Id, common.RoleRootUser, 42, "add", 250)
	require.NoError(t, err)
	require.Equal(t, 1000, adjustment.Before)
	require.Equal(t, 1250, adjustment.After)

	var rows []model.LedgerEntry
	require.NoError(t, db.Where("user_id = ? AND kind = ?", user.Id, "admin_quota_adjust").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "paid", rows[0].Account)
	require.Equal(t, int64(250), rows[0].Delta)
	require.Equal(t, int64(1250), rows[0].BalanceAfter)
	require.Equal(t, 42, rows[0].OperatorId)

	_, err = model.AdjustUserQuota(user.Id, common.RoleRootUser, 42, "override", 1250)
	require.NoError(t, err)
	require.NoError(t, db.Where("user_id = ? AND kind = ?", user.Id, "admin_quota_adjust").Find(&rows).Error)
	require.Len(t, rows, 1)
}
