package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReserveSiteGiftUsesEarningThenAuthorizedMargin(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "gift-freeze")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-4",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "gift-cycle"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	ext := &model.AdtyTopupOrder{TradeNo: "GIFT-1", SiteId: site.Id, BonusUSD: decimal.RequireFromString("10"), Method: "usdt.trc20", BonusMethod: "usdt", BonusExpiresAt: 0}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReserveAdtySiteGiftTx(tx, ext)
	}))
	require.Equal(t, "earning+margin", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("4")))
	require.True(t, ext.GiftReserveMargin.Equal(decimal.RequireFromString("6")))
	require.EqualValues(t, 0, ext.BonusExpiresAt)
	var earning, margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, earning.Balance.IsZero())
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("14")))
}
