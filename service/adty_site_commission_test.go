package service

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSiteInviteCommissionKeepsObservationDaysFromRecordTime(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(5), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-snapshot",
	})
	require.NoError(t, err)
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-DAYS", decimal.NewFromInt(30))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0))
	}))
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Where("trade_no = ?", "SITE-DAYS").First(&row).Error)
	require.Equal(t, int64(7), row.ObservationDays)
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "30"))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, int64(7), row.ObservationDays)
	require.Equal(t, int64(1_700_000_100+7*86400), row.AvailableAt)
}

func TestSiteInviteCommissionStaysPendingUntilRootConfiguresCoverage(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(20), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-seed",
	})
	require.NoError(t, err)

	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-1", decimal.NewFromInt(30))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0))
	}))
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Where("trade_no = ?", "SITE-1").First(&row).Error)
	require.Equal(t, "pending_coverage", row.Status)
	require.True(t, row.AmountUSD.Equal(decimal.RequireFromString("3")))
	require.False(t, SiteInviteCommissionWithdrawable(&row, time.Unix(1_700_000_000, 0)))
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.NewFromInt(20)), "unset coverage rules must not spend earnings")

	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "covered", row.Status)
	require.Equal(t, int64(1_700_000_100+7*86400), row.AvailableAt)
	require.False(t, SiteInviteCommissionWithdrawable(&row, time.Unix(1_700_000_100, 0)))
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&wallet).Error)
	require.True(t, wallet.Balance.Equal(decimal.NewFromInt(17)))

	require.NoError(t, ReleaseAdtySiteInviteCommissions(time.Unix(row.AvailableAt, 0)))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "available", row.Status)
	require.True(t, SiteInviteCommissionWithdrawable(&row, time.Unix(row.AvailableAt, 0)))
	var user model.User
	require.NoError(t, db.First(&user, invitee.Id).Error)
	require.Equal(t, 0, user.Quota, "making commission available must not credit personal quota")
}

var siteInviteSeed int64

func seedSiteInvite(t *testing.T, db *gorm.DB, rate decimal.Decimal) (*model.AdtySite, *model.User) {
	t.Helper()
	suffix := strconv.FormatInt(atomic.AddInt64(&siteInviteSeed, 1), 10)
	owner := referralUser(t, db, "comm-owner-"+suffix)
	inviter := referralUser(t, db, "comm-inviter-"+suffix)
	invitee := referralUser(t, db, "comm-invitee-"+suffix)
	site := &model.AdtySite{OwnerUserId: owner.Id, Subdomain: "comm-" + suffix, RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, db.Model(inviter).Update("site_id", site.Id).Error)
	require.NoError(t, db.Model(invitee).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.AdtySiteInvite{
		SiteId: site.Id, InviterId: inviter.Id, InviteeId: invitee.Id, Rate: rate,
	}).Error)
	require.NoError(t, db.Create(&model.AdtySiteReferralRate{SiteId: site.Id, Rate: rate, Enabled: true}).Error)
	return site, invitee
}

func siteInviteOrder(siteID, inviteeID int, tradeNo string, face decimal.Decimal) *model.AdtyTopupOrder {
	return &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: inviteeID, SiteId: siteID, FaceUSD: face, TeamId: 0,
	}
}

func TestSiteInviteCommissionMarginFillsShortfallOnlyAfterRootAuthorizesThatBusiness(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(1), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-short",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.NewFromInt(100), "commission-margin-cycle"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))

	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-M", decimal.NewFromInt(30))
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", "SITE-M").First(&row).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "pending_coverage", row.Status)
	require.True(t, row.CoveredUSD.Equal(decimal.NewFromInt(1)))
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(100)), "gift margin authorization must not spend margin on commission")

	require.NoError(t, AuthorizeAdtySiteCommissionMargin(site.Id, true))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_200, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "covered", row.Status)
	require.True(t, row.CoveredUSD.Equal(decimal.RequireFromString("3")))
	require.Equal(t, int64(1_700_000_200+7*86400), row.AvailableAt)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(98)))
	require.True(t, margin.CycleUsedUSD.Equal(decimal.NewFromInt(2)))
	var ledger model.AdtySiteWalletLedger
	require.NoError(t, db.Where("idempotency_key = ?", "commission:SITE-M:margin").First(&ledger).Error)
	require.Equal(t, "commission_cover", ledger.Type)
	require.Equal(t, model.AdtySiteWalletMargin, ledger.WalletKind)
	require.True(t, ledger.Delta.Equal(decimal.NewFromInt(-2)))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_300, 0))
	}))
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(98)))

	extBoth := siteInviteOrder(site.Id, invitee.Id, "SITE-M2", decimal.NewFromInt(30))
	var both model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, extBoth, time.Unix(1_700_000_400, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", "SITE-M2").First(&both).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, both.Id, time.Unix(1_700_000_400, 0))
	}))
	require.NoError(t, db.First(&both, both.Id).Error)
	require.Equal(t, "covered", both.Status)
	require.True(t, both.CoveredUSD.Equal(decimal.RequireFromString("3")))
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.IsZero())
	var marginAfter model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&marginAfter).Error)
	require.True(t, marginAfter.Balance.Equal(decimal.NewFromInt(95)))
}

func TestSiteInviteCommissionMarginStopsAtTheOpenCycle(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(1), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-cycle",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.NewFromInt(100), "cycle-room"))
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).Update("cycle_used_usd", "89").Error)
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	require.NoError(t, AuthorizeAdtySiteCommissionMargin(site.Id, true))

	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-CYCLE", decimal.NewFromInt(30))
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", "SITE-CYCLE").First(&row).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "pending_coverage", row.Status)
	require.True(t, row.CoveredUSD.Equal(decimal.NewFromInt(2)))
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(99)))
	require.True(t, margin.CycleUsedUSD.Equal(decimal.NewFromInt(90)))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_200, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "pending_coverage", row.Status)
	var marginAgain model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&marginAgain).Error)
	require.True(t, marginAgain.Balance.Equal(decimal.NewFromInt(99)))
}

func TestSiteInviteCommissionRefundClawsBackByCompletedPrincipalRatio(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, db.AutoMigrate(model.ReferralTables()...))
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(1), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-claw",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.NewFromInt(100), "claw-cycle"))
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	require.NoError(t, AuthorizeAdtySiteCommissionMargin(site.Id, true))

	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-CLAW", decimal.NewFromInt(30))
	ext.CreditQuota = 30 * 500000
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", "SITE-CLAW").First(&row).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.Equal(t, "covered", row.Status)

	ext.RefundQuota = 15 * 500000
	topUp := &model.TopUp{TradeNo: "SITE-CLAW", UserId: invitee.Id}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.AdtyAfterTopUpRefundedTx(tx, topUp, ext)
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.True(t, row.ClawedUSD.Equal(decimal.RequireFromString("1.5")))
	require.True(t, row.RecoveredUSD.Equal(decimal.RequireFromString("1.5")))
	require.Equal(t, "covered", row.Status)
	var clawLedger model.AdtySiteWalletLedger
	require.NoError(t, db.Where("idempotency_key = ?", "commission-clawback:SITE-CLAW:margin:1.5").First(&clawLedger).Error)
	require.Equal(t, "commission_clawback", clawLedger.Type)
	require.Equal(t, "site_invite", clawLedger.RefType)
	require.True(t, clawLedger.Delta.Equal(decimal.RequireFromString("1.5")))
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("99.5")))
	require.True(t, margin.CycleUsedUSD.Equal(decimal.RequireFromString("0.5")))
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.IsZero())
	var user model.User
	require.NoError(t, db.First(&user, invitee.Id).Error)
	require.Equal(t, 0, user.Quota)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.AdtyAfterTopUpRefundedTx(tx, topUp, ext)
	}))
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("99.5")))

	ext.RefundQuota = 30 * 500000
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.AdtyAfterTopUpRefundedTx(tx, topUp, ext)
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.True(t, row.ClawedUSD.Equal(decimal.RequireFromString("3")))
	require.True(t, row.RecoveredUSD.Equal(decimal.RequireFromString("3")))
	require.Equal(t, "clawed_back", row.Status)
	require.False(t, SiteInviteCommissionWithdrawable(&row, time.Unix(row.AvailableAt, 0)))
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	require.True(t, margin.Balance.Equal(decimal.NewFromInt(100)))
	require.True(t, margin.CycleUsedUSD.IsZero())
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(1)))
	require.NoError(t, db.First(&user, invitee.Id).Error)
	require.Equal(t, 0, user.Quota)
}

func TestRefundPageFactsUseRecordedLotsAndOriginalRate(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	buyer := referralUser(t, db, "refund-facts-"+strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10))
	require.NoError(t, db.Model(buyer).Update("quota_pending", int64(500000)).Error)
	trade := "FACTS-" + strconv.Itoa(buyer.Id)
	require.NoError(t, db.Create(&model.TopUp{TradeNo: trade, UserId: buyer.Id, Status: common.TopUpStatusSuccess}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: trade, UserId: buyer.Id, SiteId: 4, FaceUSD: decimal.NewFromInt(100),
		PaidAmount: decimal.NewFromInt(700), PaidCurrency: "CNY", PayCurrency: "CNY",
		PriceSnapshot: decimal.NewFromInt(7), BonusUSD: decimal.NewFromInt(10),
		CreditQuota: 110 * 500000, RefundQuota: 5 * 500000,
	}).Error)
	require.NoError(t, db.Create(&[]model.AdtyFundBatch{
		{UserId: buyer.Id, Kind: "principal", Source: trade, Original: 100 * 500000, Remaining: 40 * 500000},
		{UserId: buyer.Id, Kind: "bonus", Source: trade, Original: 10 * 500000, Remaining: 4 * 500000},
	}).Error)
	facts, err := model.LoadAdtyRefundPageFacts(trade, decimal.NewFromInt(10))
	require.NoError(t, err)
	require.Equal(t, trade, facts.TradeNo)
	require.Equal(t, 4, facts.SiteId)
	require.Equal(t, buyer.Id, facts.UserId)
	require.Equal(t, "100", facts.FaceUSD)
	require.Equal(t, "700", facts.PaidAmount)
	require.Equal(t, "CNY", facts.PaidCurrency)
	require.Equal(t, "7", facts.OrderRate)
	require.Equal(t, "60", facts.PrincipalConsumedUSD)
	require.Equal(t, "40", facts.PrincipalRemainingUSD)
	require.Equal(t, int64(500000), facts.PendingQuota)
	require.Equal(t, int64(5*500000), facts.RefundOccupiedQuota)
	require.Equal(t, "10", facts.BonusUSD)
	require.Equal(t, int64(6*500000), facts.BonusUsedQuota)
	require.Equal(t, int64(4*500000), facts.BonusRemainingQuota)
	require.Equal(t, "70", facts.ForeignReference)
}

func TestRefundPreviewShowsIssuedWithdrawnAndExpectedClawback(t *testing.T) {
	issued, withdrawn, expected := model.SiteInviteCommissionRefundPreview(
		decimal.NewFromInt(3), decimal.NewFromInt(1), decimal.RequireFromString("0.5"), 15*500000, 30*500000,
	)
	require.Equal(t, "3", issued)
	require.Equal(t, "1", withdrawn)
	require.Equal(t, "1", expected)
	_, _, none := model.SiteInviteCommissionRefundPreview(decimal.Zero, decimal.Zero, decimal.Zero, 1, 1)
	require.Equal(t, "0", none)
}

func TestSiteInviteBatchRegisterUsesOnlyASavedRootThreshold(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, inviter, invitees := seedBatchInvite(t, db, 3)
	for i, invitee := range invitees {
		require.NoError(t, BindSiteInviteFromIP(site.Id, inviter.Id, invitee.Id, "ip-a"))
		var count int64
		require.NoError(t, db.Model(&model.AdtySiteInvite{}).Where("inviter_id = ?", inviter.Id).Count(&count).Error)
		require.Equal(t, int64(i+1), count)
	}
	require.NoError(t, SaveAdtySiteInviteBatchPolicy("2", "24"))
	other := referralUser(t, db, "batch-other-"+strconv.Itoa(site.Id))
	require.NoError(t, db.Model(other).Update("site_id", site.Id).Error)
	require.Error(t, BindSiteInviteFromIP(site.Id, inviter.Id, other.Id, "ip-a"))
	var blocked model.AdtySiteInvite
	require.Error(t, db.Where("invitee_id = ?", other.Id).First(&blocked).Error)
	var anomaly model.AdtyAnomalyDisposition
	require.NoError(t, db.Where("kind = ? AND ref = ?", "batch_register", strconv.Itoa(site.Id)).First(&anomaly).Error)
	require.Equal(t, "open", anomaly.Status)
	allowed := referralUser(t, db, "batch-ok-"+strconv.Itoa(site.Id))
	require.NoError(t, db.Model(allowed).Update("site_id", site.Id).Error)
	require.NoError(t, BindSiteInviteFromIP(site.Id, inviter.Id, allowed.Id, "ip-b"))
	var wallets int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Count(&wallets).Error)
	require.Zero(t, wallets)
}

func seedBatchInvite(t *testing.T, db *gorm.DB, n int) (*model.AdtySite, *model.User, []*model.User) {
	t.Helper()
	suffix := strconv.FormatInt(atomic.AddInt64(&siteInviteSeed, 1), 10)
	owner := referralUser(t, db, "batch-owner-"+suffix)
	inviter := referralUser(t, db, "batch-inviter-"+suffix)
	site := &model.AdtySite{OwnerUserId: owner.Id, Subdomain: "batch-" + suffix, RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, db.Model(inviter).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.AdtySiteReferralRate{SiteId: site.Id, Rate: decimal.RequireFromString("0.1"), Enabled: true}).Error)
	invitees := make([]*model.User, 0, n)
	for i := 0; i < n; i++ {
		user := referralUser(t, db, "batch-invitee-"+suffix+"-"+strconv.Itoa(i))
		require.NoError(t, db.Model(user).Update("site_id", site.Id).Error)
		invitees = append(invitees, user)
	}
	return site, inviter, invitees
}

func TestWithdrawnSiteInviteCommissionRefundRecordsLiabilityWithoutDebitingQuota(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(3), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-withdrawn-" + strconv.Itoa(site.Id),
	})
	require.NoError(t, err)
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-WD-"+strconv.Itoa(site.Id), decimal.NewFromInt(30))
	ext.CreditQuota = 30 * 500000
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", ext.TradeNo).First(&row).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.Model(&row).Update("withdrawn_usd", decimal.NewFromInt(1)).Error)
	ext.RefundQuota = 30 * 500000
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.AdtyAfterTopUpRefundedTx(tx, &model.TopUp{TradeNo: ext.TradeNo, UserId: invitee.Id}, ext)
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.True(t, row.WithdrawnUSD.Equal(decimal.NewFromInt(1)))
	require.True(t, row.RecoveredUSD.Equal(decimal.NewFromInt(2)))
	require.True(t, row.RecoveryUSD.IsZero())
	var liability model.AdtyCommissionRecovery
	require.NoError(t, db.Where("commission_id = ?", row.Id).First(&liability).Error)
	require.True(t, liability.AmountUSD.Equal(decimal.NewFromInt(1)))
	require.Equal(t, "open", liability.Status)
	require.Equal(t, "withdrawn", liability.Kind)
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(2)))
	var user model.User
	require.NoError(t, db.First(&user, row.InviterId).Error)
	require.Equal(t, 0, user.Quota)
}

func TestSiteInviteCommissionRefundCancelsUncoveredAmountWithoutMovingWallets(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, db.AutoMigrate(model.ReferralTables()...))
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-OPEN", decimal.NewFromInt(30))
	ext.CreditQuota = 30 * 500000
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1, 0))
	}))
	ext.RefundQuota = 15 * 500000
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.AdtyAfterTopUpRefundedTx(tx, &model.TopUp{TradeNo: "SITE-OPEN", UserId: invitee.Id}, ext)
	}))
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Where("trade_no = ?", "SITE-OPEN").First(&row).Error)
	require.Equal(t, "pending_coverage", row.Status)
	require.True(t, row.ClawedUSD.Equal(decimal.RequireFromString("1.5")))
	require.True(t, row.RecoveredUSD.IsZero())
	var ledgers int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("type = ?", "commission_clawback").Count(&ledgers).Error)
	require.Zero(t, ledgers)
	var wallets []model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ?", site.Id).Find(&wallets).Error)
	for _, wallet := range wallets {
		require.True(t, wallet.Balance.IsZero(), wallet.Kind)
	}
}

func TestSiteInviteCommissionConvertsAvailableRemainderToInviterQuota(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(3), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-convert",
	})
	require.NoError(t, err)
	require.NoError(t, SaveAdtySiteCommissionPolicy("10", "7"))
	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-CVT", decimal.NewFromInt(30))
	var row model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", "SITE-CVT").First(&row).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, row.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	var invite model.AdtySiteInvite
	require.NoError(t, db.Where("site_id = ? AND invitee_id = ?", site.Id, invitee.Id).First(&invite).Error)
	require.Error(t, ConvertAdtySiteInviteCommission(invite.InviterId, row.Id, decimal.NewFromInt(1), "cvt-1"))

	require.NoError(t, ReleaseAdtySiteInviteCommissions(time.Unix(row.AvailableAt, 0)))
	require.NoError(t, ConvertAdtySiteInviteCommission(invite.InviterId, row.Id, decimal.NewFromInt(1), "cvt-1"))
	require.NoError(t, ConvertAdtySiteInviteCommission(invite.InviterId, row.Id, decimal.NewFromInt(1), "cvt-1"))
	var inviter model.User
	require.NoError(t, db.First(&inviter, invite.InviterId).Error)
	require.Equal(t, 500000, inviter.Quota)
	require.NoError(t, db.First(&row, row.Id).Error)
	require.True(t, row.ConvertedUSD.Equal(decimal.NewFromInt(1)))
	var inviteeUser model.User
	require.NoError(t, db.First(&inviteeUser, invitee.Id).Error)
	require.Equal(t, 0, inviteeUser.Quota)
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.IsZero())
	require.Error(t, ConvertAdtySiteInviteCommission(invitee.Id, row.Id, decimal.NewFromInt(1), "cvt-other"))

	ext.CreditQuota = 30 * 500000
	ext.RefundQuota = 30 * 500000
	require.NoError(t, db.AutoMigrate(model.ReferralTables()...))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.AdtyAfterTopUpRefundedTx(tx, &model.TopUp{TradeNo: "SITE-CVT", UserId: invitee.Id}, ext)
	}))
	require.NoError(t, db.First(&row, row.Id).Error)
	require.True(t, row.ClawedUSD.Equal(decimal.NewFromInt(3)))
	require.True(t, row.RecoveredUSD.Equal(decimal.NewFromInt(2)))
	require.True(t, row.RecoveryUSD.Equal(decimal.NewFromInt(1)))
	require.NoError(t, db.First(&inviter, invite.InviterId).Error)
	require.Equal(t, 500000, inviter.Quota)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(2)))
}

func TestResumeSiteInviteCoverageFundsOriginalRowsInOrder(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(3), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-resume",
	})
	require.NoError(t, err)
	require.NoError(t, SaveAdtySiteCommissionPolicy("2", "7"))

	older := siteInviteOrder(site.Id, invitee.Id, "SITE-OLD", decimal.NewFromInt(30))
	newer := siteInviteOrder(site.Id, invitee.Id, "SITE-NEW", decimal.NewFromInt(30))
	var oldRow, newRow model.AdtySiteInviteCommission
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAdtySiteInviteCommissionTx(tx, older, time.Unix(1_700_000_000, 0)); err != nil {
			return err
		}
		if err := RecordAdtySiteInviteCommissionTx(tx, newer, time.Unix(1_700_000_010, 0)); err != nil {
			return err
		}
		if err := tx.Where("trade_no = ?", "SITE-OLD").First(&oldRow).Error; err != nil {
			return err
		}
		return CoverAdtySiteInviteCommissionTx(tx, oldRow.Id, time.Unix(1_700_000_100, 0))
	}))
	require.NoError(t, db.First(&oldRow, oldRow.Id).Error)
	require.Equal(t, "pending_coverage", oldRow.Status)
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.Equal(decimal.NewFromInt(3)), "a backlog at the cap does not spend earnings on one row")

	require.NoError(t, ResumeAdtySiteInviteCoverage(site.Id, time.Unix(1_700_000_200, 0)))
	require.NoError(t, db.First(&oldRow, oldRow.Id).Error)
	require.NoError(t, db.Where("trade_no = ?", "SITE-NEW").First(&newRow).Error)
	require.Equal(t, "covered", oldRow.Status)
	require.Equal(t, "pending_coverage", newRow.Status)
	require.Equal(t, int64(1_700_000_200+7*86400), oldRow.AvailableAt)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.IsZero())
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Count(&count).Error)
	require.Equal(t, int64(2), count)

	require.NoError(t, ResumeAdtySiteInviteCoverage(site.Id, time.Unix(1_700_000_300, 0)))
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.IsZero())
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestSiteReferralOffDoesNotRecordCommissionOnThatSite(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	require.NoError(t, SetAdtyOwnerSiteReferralEnabled(site.Id, site.OwnerUserId, false))
	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-OFF", decimal.NewFromInt(30))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1, 0))
	}))
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Count(&count).Error)
	require.Zero(t, count)

	other, otherInvitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	otherOrder := siteInviteOrder(other.Id, otherInvitee.Id, "SITE-ON", decimal.NewFromInt(20))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, otherOrder, time.Unix(2, 0))
	}))
	var otherRow model.AdtySiteInviteCommission
	require.NoError(t, db.Where("trade_no = ?", "SITE-ON").First(&otherRow).Error)
	require.Equal(t, other.Id, otherRow.SiteId)

	require.NoError(t, SetAdtyOwnerSiteReferralEnabled(site.Id, site.OwnerUserId, true))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(3, 0))
	}))
	var restored model.AdtySiteInviteCommission
	require.NoError(t, db.Where("trade_no = ?", "SITE-OFF").First(&restored).Error)
	require.Equal(t, site.Id, restored.SiteId)
}

func TestInviterSeesPendingSiteCommissionBeforeItIsWithdrawable(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	ext := siteInviteOrder(site.Id, invitee.Id, "SITE-SEE", decimal.NewFromInt(30))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1_700_000_000, 0))
	}))
	var invite model.AdtySiteInvite
	require.NoError(t, db.Where("site_id = ? AND invitee_id = ?", site.Id, invitee.Id).First(&invite).Error)
	rows, err := ListAdtySiteInviteCommissions(invite.InviterId, time.Unix(1_700_000_000, 0))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "pending_coverage", rows[0].Status)
	require.Equal(t, "SITE-SEE", rows[0].TradeNo)
	require.False(t, SiteInviteCommissionWithdrawable(&rows[0], time.Unix(1_700_000_000, 0)))
}

func TestBindSiteInviteRejectsCrossSiteAndPaidCustomers(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, _ := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	var invite model.AdtySiteInvite
	require.NoError(t, db.Where("site_id = ?", site.Id).First(&invite).Error)
	outsider := referralUser(t, db, "other-site-inviter")
	sameSite := referralUser(t, db, "same-site-invitee")
	require.NoError(t, db.Model(sameSite).Update("site_id", site.Id).Error)
	require.Error(t, BindSiteInvite(site.Id, outsider.Id, sameSite.Id))
	var created int64
	require.NoError(t, db.Model(&model.AdtySiteInvite{}).Where("invitee_id = ?", sameSite.Id).Count(&created).Error)
	require.Zero(t, created)

	require.NoError(t, db.Create(&model.TopUp{
		UserId: sameSite.Id, Amount: 5, Money: 5, TradeNo: "fresh-paid",
		Status: common.TopUpStatusSuccess,
	}).Error)
	require.Error(t, BindSiteInvite(site.Id, invite.InviterId, sameSite.Id))
	require.NoError(t, db.Model(&model.AdtySiteInvite{}).Where("invitee_id = ?", sameSite.Id).Count(&created).Error)
	require.Zero(t, created)
}

func TestSiteInviteCommissionIgnoresTeamTopups(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site, invitee := seedSiteInvite(t, db, decimal.RequireFromString("0.10"))
	ext := siteInviteOrder(site.Id, invitee.Id, "TEAM-1", decimal.NewFromInt(10))
	ext.TeamId = 9
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAdtySiteInviteCommissionTx(tx, ext, time.Unix(1, 0))
	}))
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Count(&count).Error)
	require.Zero(t, count)
	_ = common.QuotaPerUnit
}
