package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestCollectAdtyFundAnomaliesFlagsDuplicateCreditAndRefund(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "dup-fund")
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-credit", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "credited"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-credit", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "credited"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "once", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "credited"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "once", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "replayed"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-refund", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "received", EventType: "refund.succeeded"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-refund", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "received", EventType: "refund.succeeded"}).Error)

	kinds, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kinds, "duplicate_credit")
	require.Contains(t, kinds, "duplicate_refund")
}

func TestCollectAdtyFundAnomaliesFlagsMarginOverCap(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "fund-anomaly")
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("100"), "anomaly-cycle"))
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).Update("cycle_used_usd", "91").Error)
	kinds, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kinds, "margin_anomaly")
}

func TestMarginDebitStopsAtNinetyPercentOfOpenCycle(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "margin-cycle")
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("10000"), "cycle-open"))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.RequireFromString("-9000"), AdtySiteWalletChange{
		Type: "margin_draw", IdempotencyKey: "draw-9000",
	})
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.RequireFromString("-1"), AdtySiteWalletChange{
		Type: "margin_draw", IdempotencyKey: "draw-over",
	})
	require.ErrorIs(t, err, ErrAdtySiteWalletInsufficient)
}

func TestTransferEarningToMarginOpensNewCycle(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "margin-transfer")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("1000"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-1000",
	})
	require.NoError(t, err)
	require.NoError(t, TransferAdtyEarningToMargin(site.Id, 1, decimal.RequireFromString("1000"), decimal.RequireFromString("1000"), "xfer-1"))
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("1000")))
	require.True(t, margin.CycleTierUSD.Equal(decimal.RequireFromString("1000")))
	require.True(t, margin.CycleUsedUSD.IsZero())
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.RequireFromString("-901"), AdtySiteWalletChange{
		Type: "margin_draw", IdempotencyKey: "draw-over-transfer",
	})
	require.ErrorIs(t, err, ErrAdtySiteWalletInsufficient)
}
