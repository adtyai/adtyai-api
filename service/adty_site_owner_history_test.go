package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerCustomerBalancesIncludeDurablePending(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "pending-balance")
	customer := siteCustomer(t, db, site, "pending-customer")
	for _, tc := range []struct{ raw, pending int }{{100, -80}, {-80, 100}} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Updates(map[string]any{
			"quota": tc.raw, "quota_pending": tc.pending,
		}).Error)
		view, err := GetAdtyOwnerCustomer(site, customer.Id)
		require.NoError(t, err)
		require.NotNil(t, view.Balance)
		assert.Equal(t, 20, *view.Balance)
		views, total, err := ListAdtyOwnerCustomers(site, 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, views, 1)
		require.NotNil(t, views[0].Balance)
		assert.Equal(t, 20, *views[0].Balance)
	}
}

func TestOwnerCannotSeeHistoryBeforeTransfer(t *testing.T) {
	db := prepareSiteDB(t)
	_, oldSite := openActiveSite(t, db, "old-vis")
	_, newSite := openActiveSite(t, db, "new-vis")
	customer := siteCustomer(t, db, oldSite, "moved-vis")
	require.NoError(t, db.Model(customer).Update("used_quota", 100).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: customer.Id, Type: model.LogTypeConsume, Quota: 100, ModelName: "old-model", CreatedAt: 400, Other: "{}",
	}).Error)
	oldOrder := &model.AdtyTopupOrder{
		TradeNo: "OLD-VIS-TOP", UserId: customer.Id, SiteId: oldSite.Id, Provider: model.PaymentProviderAdtyBEpusdt,
		Method: "usdt.trc20", FaceUSD: decimal.NewFromInt(10), BonusUSD: decimal.Zero, CreditQuota: 5_000_000,
	}
	require.NoError(t, db.Create(oldOrder).Error)
	require.NoError(t, db.Model(oldOrder).Update("created_at", 400).Error)

	joined := time.Now().Unix()
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("site_id", newSite.Id).Error)
	require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Updates(map[string]any{
		"site_id": newSite.Id, "joined_at": joined,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: customer.Id, Type: model.LogTypeConsume, Quota: 7, ModelName: "new-model", CreatedAt: joined + 5, Other: "{}",
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: "NEW-VIS-TOP", UserId: customer.Id, SiteId: newSite.Id, Provider: model.PaymentProviderAdtyBEpusdt,
		Method: "usdt.trc20", FaceUSD: decimal.NewFromInt(3), BonusRate: decimal.RequireFromString("0.025"), BonusUSD: decimal.Zero, CreditQuota: 1_500_000,
	}).Error)

	view, err := GetAdtyOwnerCustomer(newSite, customer.Id)
	require.NoError(t, err)
	require.NotNil(t, view.UsedQuota)
	assert.Equal(t, 7, *view.UsedQuota)

	logs, total, err := ListAdtyOwnerCustomerLogs(newSite, customer.Id, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "new-model", logs[0].ModelName)

	topups, total, err := ListAdtyOwnerCustomerTopUps(newSite, customer.Id, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, topups, 1)
	assert.Equal(t, "NEW-VIS-TOP", topups[0].TradeNo)
	assert.True(t, topups[0].BonusRate.Equal(decimal.RequireFromString("0.025")))
}
