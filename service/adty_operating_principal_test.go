package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSiteOperatingWalletTracksCustomerPrincipalOnly(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	customer := referralUser(t, db, "op-customer")
	site := &model.AdtySite{OwnerUserId: customer.Id, Subdomain: "op-principal", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))

	require.NoError(t, db.Create(&model.TopUp{
		UserId: customer.Id, Amount: 1, Money: 1, TradeNo: "OP-1",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderAdtyEpay,
		CreateTime: 1, Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: "OP-1", UserId: customer.Id, SiteId: site.Id, Provider: model.PaymentProviderAdtyEpay,
		Method: "alipay", FaceUSD: decimal.NewFromInt(1), CreditQuota: 500000,
		PayAmount: decimal.NewFromInt(1), PayCurrency: "USD",
	}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, _, _, err := model.CompleteAdtyTopUpTx(tx, "OP-1", model.AdtyPaymentFacts{
			Provider: model.PaymentProviderAdtyEpay, Source: model.AdtyPaymentSourceManual,
		})
		return err
	}))

	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.NewFromInt(1)), "site top-up principal credits the operating wallet")

	charged, err := model.ChargeUserQuotaUpTo(customer.Id, 300000)
	require.NoError(t, err)
	require.Equal(t, 300000, charged)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.RequireFromString("0.4")), "consumption debits only the principal share")

	charged, err = model.ChargeUserQuotaUpTo(customer.Id, 200000)
	require.NoError(t, err)
	require.Equal(t, 200000, charged)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero), "the rest of the principal clears the operating wallet")

	require.NoError(t, db.Create(&model.AdtyFundBatch{
		UserId: customer.Id, Kind: "bonus", Source: "gift-1", Original: 100000, Remaining: 100000,
	}).Error)
	require.NoError(t, db.Model(customer).Update("quota", 100000).Error)
	charged, err = model.ChargeUserQuotaUpTo(customer.Id, 100000)
	require.NoError(t, err)
	require.Equal(t, 100000, charged, "gift consumption is not refused when the operating wallet is empty")
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero), "gift consumption does not debit the operating wallet")
}
