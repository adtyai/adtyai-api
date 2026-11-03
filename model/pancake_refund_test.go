package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundWaffoPancakeClawsBackQuota(t *testing.T) {
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = old })

	user := &User{Username: "pancake-refund", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1_000_000}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&TopUp{})
		DB.Delete(&User{}, user.Id)
	})
	order := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, TradeNo: "PAN-REFUND-1",
		PaymentMethod: "waffo_pancake", PaymentProvider: PaymentProviderWaffoPancake,
		Status: common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(order).Error)

	require.NoError(t, RefundWaffoPancake(order.TradeNo))
	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, 0, stored.Quota)
	assert.Equal(t, common.TopUpStatusRefunded, topUpStatus(t, DB, order.TradeNo))

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 1_000_000).Error)
	require.NoError(t, RefundWaffoPancake(order.TradeNo))
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, 1_000_000, stored.Quota, "a second refund does not deduct again")
}

func TestRefundWaffoPancakeOnDedicatedPostgres(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "pancake-pg", 1_000_000)
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 2, Money: 2, TradeNo: "PAN-PG-REFUND",
		PaymentMethod: "waffo_pancake", PaymentProvider: PaymentProviderWaffoPancake,
		Status: common.TopUpStatusSuccess,
	}).Error)
	assert.Equal(t, 1_000_000, userQuota(t, db, user.Id))
	require.NoError(t, RefundWaffoPancake("PAN-PG-REFUND"))
	assert.Equal(t, 0, userQuota(t, db, user.Id))
	assert.Equal(t, common.TopUpStatusRefunded, topUpStatus(t, db, "PAN-PG-REFUND"))
}

func TestAdminUserListHidesSameOrHigherRole(t *testing.T) {
	rows := []*User{
		{Username: "vis-common", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled},
		{Username: "vis-admin", Password: "x", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
		{Username: "vis-root", Password: "x", Role: common.RoleRootUser, Status: common.UserStatusEnabled},
	}
	for _, row := range rows {
		require.NoError(t, DB.Create(row).Error)
	}
	t.Cleanup(func() {
		for _, row := range rows {
			DB.Delete(&User{}, row.Id)
		}
	})

	visible, _, err := SearchUsers("vis-", "", nil, nil, 0, 20, common.RoleAdminUser)
	require.NoError(t, err)
	names := make([]string, 0, len(visible))
	for _, row := range visible {
		names = append(names, row.Username)
	}
	assert.Equal(t, []string{"vis-common"}, names)

	all, _, err := SearchUsers("vis-", "", nil, nil, 0, 20, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}
