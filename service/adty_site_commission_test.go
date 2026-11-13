package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

func seedSiteInvite(t *testing.T, db *gorm.DB, rate decimal.Decimal) (*model.AdtySite, *model.User) {
	t.Helper()
	owner := referralUser(t, db, "comm-owner")
	inviter := referralUser(t, db, "comm-inviter")
	invitee := referralUser(t, db, "comm-invitee")
	site := &model.AdtySite{OwnerUserId: owner.Id, Subdomain: "comm", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, db.Model(inviter).Update("site_id", site.Id).Error)
	require.NoError(t, db.Model(invitee).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.AdtySiteInvite{
		SiteId: site.Id, InviterId: inviter.Id, InviteeId: invitee.Id, Rate: rate,
	}).Error)
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
