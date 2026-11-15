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

	consumeAdtyTestQuota(t, customer.Id, 300000)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.RequireFromString("0.4")), "consumption debits only the principal share")

	consumeAdtyTestQuota(t, customer.Id, 200000)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero), "the rest of the principal clears the operating wallet")

	require.NoError(t, db.Create(&model.AdtyFundBatch{
		UserId: customer.Id, Kind: "bonus", Source: "gift-1", Original: 100000, Remaining: 100000,
	}).Error)
	require.NoError(t, db.Model(customer).Update("quota", 100000).Error)
	consumeAdtyTestQuota(t, customer.Id, 100000) // gift consumption is not refused when the operating wallet is empty
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero), "gift consumption does not debit the operating wallet")
}

func TestManualQuotaAdjustDoesNotDebitOperatingWallet(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	customer := referralUser(t, db, "op-adjust-customer")
	site := &model.AdtySite{OwnerUserId: customer.Id, Subdomain: "op-adjust", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))

	require.NoError(t, db.Create(&model.TopUp{
		UserId: customer.Id, Amount: 1, Money: 1, TradeNo: "OP-ADJ-1",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderAdtyEpay,
		CreateTime: 1, Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: "OP-ADJ-1", UserId: customer.Id, SiteId: site.Id, Provider: model.PaymentProviderAdtyEpay,
		Method: "alipay", FaceUSD: decimal.NewFromInt(1), CreditQuota: 500000,
		PayAmount: decimal.NewFromInt(1), PayCurrency: "USD",
	}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, _, _, err := model.CompleteAdtyTopUpTx(tx, "OP-ADJ-1", model.AdtyPaymentFacts{
			Provider: model.PaymentProviderAdtyEpay, Source: model.AdtyPaymentSourceManual,
		})
		return err
	}))

	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.NewFromInt(1)))

	// Empty the operating wallet so a mistaken usage-style debit would fail the adjust.
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(-1), AdtySiteWalletChange{
		Type: "adjustment", RefType: "test", RefID: "drain-op-adj", IdempotencyKey: "drain-op-adj",
	})
	require.NoError(t, err)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero))

	adjusted, err := model.AdjustUserQuotaWithLedger(customer.Id, 1, common.RoleRootUser, "subtract", 300000, "manual trim", "op-adjust-sub-1")
	require.NoError(t, err)
	require.Equal(t, 500000, adjusted.Before)
	require.Equal(t, 200000, adjusted.After)

	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero), "manual adjust must not debit the operating wallet")

	var consumeLedgers int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).
		Where("site_id = ? AND wallet_kind = ? AND type = ?", site.Id, model.AdtySiteWalletOperating, "principal_consume").
		Count(&consumeLedgers).Error)
	require.EqualValues(t, 0, consumeLedgers)

	var remaining int64
	require.NoError(t, db.Model(&model.AdtyFundBatch{}).Where("user_id = ?", customer.Id).Select("COALESCE(SUM(remaining),0)").Scan(&remaining).Error)
	require.EqualValues(t, 200000, remaining)

	// Usage charges must still debit the operating wallet once it has cover again.
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.RequireFromString("0.4"), AdtySiteWalletChange{
		Type: "adjustment", RefType: "test", RefID: "restore-op-adj", IdempotencyKey: "restore-op-adj",
	})
	require.NoError(t, err)
	consumeAdtyTestQuota(t, customer.Id, 200000)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.Zero), "a later usage charge still debits the operating wallet")
}

// consumeAdtyTestQuota 驱动一次「用量消耗」：同一事务里先扣批次（会触发
// principal-spent hook，也就是这两条用例真正在测的东西），再扣用户钱包余额。
//
// 原来的驱动是 `model.ChargeUserQuotaUpTo`；它在真实流量上不可达，已随
// ISS-R4-0054 连同钉住它的测试一起删除，所以这里直接落在它做过的那两步上。
func consumeAdtyTestQuota(t *testing.T, userID, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Select("id", "quota").First(&user, userID).Error; err != nil {
			return err
		}
		if err := model.ConsumeAdtyFundBatchesTx(tx, userID, int64(quota), int64(user.Quota)); err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", userID).
			Update("quota", gorm.Expr("quota - ?", quota)).Error
	}))
}
