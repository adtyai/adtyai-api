package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProjectWalletLotsSeparatesPrincipalAndBonus(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtyFundBatch{}))

	both := createLotUser(t, "lots-both", 0)
	require.NoError(t, db.Create([]model.AdtyFundBatch{
		{UserId: both.Id, Kind: "principal", Source: "order-a", Original: 800000, Remaining: 500000},
		{UserId: both.Id, Kind: "principal", Source: "order-b", Original: 500000, Remaining: 500000},
		{UserId: both.Id, Kind: "bonus", Source: "order-a", Original: 250000, Remaining: 250000},
	}).Error)
	proj, ok, err := ProjectWalletLotsForUser(both)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, WalletLotProjectionSourceBatches, proj.Source)
	assert.EqualValues(t, 1_000_000, proj.PrincipalQuota)
	assert.EqualValues(t, 250_000, proj.BonusQuota)
	assert.EqualValues(t, 1_250_000, proj.TotalQuota)
	assert.True(t, proj.PrincipalUSD.Equal(decimal.NewFromInt(2)), proj.PrincipalUSD.String())
	assert.True(t, proj.BonusUSD.Equal(decimal.RequireFromString("0.5")), proj.BonusUSD.String())
	assert.True(t, proj.TotalUSD.Equal(decimal.RequireFromString("2.5")), proj.TotalUSD.String())
	assert.True(t, proj.TotalUSD.Equal(proj.PrincipalUSD.Add(proj.BonusUSD)))

	principalOnly := createLotUser(t, "lots-principal", 700_000)
	require.NoError(t, db.Create(&model.AdtyFundBatch{
		UserId: principalOnly.Id, Kind: "principal", Source: "legacy", Original: 700000, Remaining: 700000,
	}).Error)
	proj, ok, err = ProjectWalletLotsForUser(principalOnly)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, 700_000, proj.PrincipalQuota)
	assert.EqualValues(t, 0, proj.BonusQuota)
	assert.True(t, proj.BonusUSD.IsZero())
	assert.True(t, proj.TotalUSD.Equal(proj.PrincipalUSD))

	bonusOnly := createLotUser(t, "lots-bonus", 0)
	require.NoError(t, db.Create(&model.AdtyFundBatch{
		UserId: bonusOnly.Id, Kind: "bonus", Source: "gift", Original: 500000, Remaining: 500000,
	}).Error)
	proj, ok, err = ProjectWalletLotsForUser(bonusOnly)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, 0, proj.PrincipalQuota)
	assert.EqualValues(t, 500_000, proj.BonusQuota)
	assert.True(t, proj.PrincipalUSD.IsZero())
	assert.True(t, proj.BonusUSD.Equal(decimal.NewFromInt(1)))

	zeroBonus := createLotUser(t, "lots-zero-bonus", 0)
	require.NoError(t, db.Create([]model.AdtyFundBatch{
		{UserId: zeroBonus.Id, Kind: "principal", Source: "paid", Original: 500000, Remaining: 250000},
		{UserId: zeroBonus.Id, Kind: "bonus", Source: "paid", Original: 100000, Remaining: 0},
	}).Error)
	proj, ok, err = ProjectWalletLotsForUser(zeroBonus)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, 250_000, proj.PrincipalQuota)
	assert.EqualValues(t, 0, proj.BonusQuota)
	assert.Equal(t, WalletLotProjectionSourceBatches, proj.Source)

	legacy := createLotUser(t, "lots-legacy", 1_500_000)
	proj, ok, err = ProjectWalletLotsForUser(legacy)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, WalletLotProjectionSourceLegacy, proj.Source)
	assert.EqualValues(t, 1_500_000, proj.PrincipalQuota)
	assert.EqualValues(t, 0, proj.BonusQuota)
	assert.True(t, proj.PrincipalUSD.Equal(decimal.RequireFromString("3")))
}

func TestProjectWalletLotsDoesNotChangeSpendOrCredit(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtyFundBatch{}))
	user := createLotUser(t, "lots-spend", 90)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.CreateAdtyFundBatchesTx(tx, user.Id, "credit-1", 50, 40)
	}))
	before, err := model.SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, 50, before.Principal)
	require.EqualValues(t, 40, before.Bonus)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, user.Id, 45, 90)
	}))
	after, err := model.SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, 5, after.Principal)
	require.EqualValues(t, 40, after.Bonus)
	proj, ok, err := ProjectWalletLotsForUser(user)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, after.Principal, proj.PrincipalQuota)
	assert.EqualValues(t, after.Bonus, proj.BonusQuota)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.CreateAdtyFundBatchesTx(tx, user.Id, "credit-2", 10, 0)
	}))
	created, err := model.SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, 15, created.Principal)
	require.EqualValues(t, 40, created.Bonus)
}

func TestOwnerCustomerLotSplitIsScopedAndHonorsVisibility(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "lot-vis")
	_, other := openActiveSite(t, db, "lot-other")
	customer := siteCustomer(t, db, site, "lot-customer")
	require.NoError(t, db.Model(customer).Updates(map[string]any{"quota": 1_000_000, "quota_pending": 0}).Error)
	customer.Quota = 1_000_000
	require.NoError(t, db.Create([]model.AdtyFundBatch{
		{UserId: customer.Id, Kind: "principal", Source: "paid", Original: 750000, Remaining: 750000},
		{UserId: customer.Id, Kind: "bonus", Source: "paid", Original: 250000, Remaining: 250000},
	}).Error)

	view, err := GetAdtyOwnerCustomer(site, customer.Id)
	require.NoError(t, err)
	require.NotNil(t, view.Balance)
	require.NotNil(t, view.WalletLotProjection)
	assert.EqualValues(t, 750_000, view.WalletLotProjection.PrincipalQuota)
	assert.EqualValues(t, 250_000, view.WalletLotProjection.BonusQuota)
	assert.True(t, view.WalletLotProjection.TotalUSD.Equal(decimal.NewFromInt(2)))

	views, total, err := ListAdtyOwnerCustomers(site, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, views, 1)
	require.NotNil(t, views[0].WalletLotProjection)
	assert.EqualValues(t, 750_000, views[0].WalletLotProjection.PrincipalQuota)

	_, err = GetAdtyOwnerCustomer(other, customer.Id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Customer not found")

	site.Visibility = `{"customer_balance":"off"}`
	require.NoError(t, db.Model(site).Update("visibility", site.Visibility).Error)
	hidden, err := GetAdtyOwnerCustomer(site, customer.Id)
	require.NoError(t, err)
	assert.Nil(t, hidden.Balance)
	assert.Nil(t, hidden.WalletLotProjection)
}

func createLotUser(t *testing.T, name string, quota int) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com", Quota: quota}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}
