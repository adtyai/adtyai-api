package service

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Representative pre-finance site columns and data. Migration must add the
// new foundation without replacing the site's identity or historical factor.
type adtySiteBeforeFinance struct {
	Id             int
	OwnerUserId    int              `gorm:"index"`
	Subdomain      string           `gorm:"uniqueIndex;type:varchar(63)"`
	RootDomain     string           `gorm:"type:varchar(253)"`
	Status         string           `gorm:"type:varchar(16);index"`
	TierCostFactor *decimal.Decimal `gorm:"type:decimal(10,6)"`
}

func (adtySiteBeforeFinance) TableName() string { return "adty_sites" }

func TestStaffServiceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			owner := referralUser(t, db, "staff-owner")
			site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "staff-site", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
			require.NoError(t, db.Create(&site).Error)
			created, err := CreateAdtyStaff(owner.Id, site.Id, AdtyStaffInput{Username: "staff-matrix", DisplayName: "Support", Password: "employee-password-42", Role: "support"})
			require.NoError(t, err)
			assert.Equal(t, []string{"customers.read"}, created.Permissions)
			_, _, err = AdtyStaffAccess(created.UserId, 1, "customers.read")
			require.NoError(t, err)
			_, _, err = AdtyStaffAccess(created.UserId, 1, "customers.balance")
			assert.ErrorIs(t, err, ErrAdtyStaffDenied)
			var group sync.WaitGroup
			results := make(chan error, 2)
			for range 2 {
				group.Go(func() {
					_, err := MutateAdtyStaff(owner.Id, site.Id, created.Id, AdtySiteOwnerActionStaffUpdate, AdtyStaffInput{DisplayName: "Finance", Role: "finance", Status: "enabled", ExpectedVersion: 1})
					results <- err
				})
			}
			group.Wait()
			close(results)
			success, conflict := 0, 0
			for err := range results {
				if err == nil {
					success++
				} else {
					require.ErrorIs(t, err, ErrAdtyStaffConflict)
					conflict++
				}
			}
			assert.Equal(t, 1, success)
			assert.Equal(t, 1, conflict)
			_, _, err = AdtyStaffAccess(created.UserId, 1, "customers.read")
			assert.ErrorIs(t, err, ErrAdtyStaffDenied)
			_, _, err = AdtyStaffAccess(created.UserId, 2, "customers.balance")
			require.NoError(t, err)
			none := []string{}
			_, err = MutateAdtyStaff(owner.Id, site.Id, created.Id, AdtySiteOwnerActionStaffUpdate, AdtyStaffInput{DisplayName: "No access", Role: "finance", Permissions: &none, Status: "enabled", ExpectedVersion: 2})
			require.NoError(t, err)
			_, _, err = AdtyStaffAccess(created.UserId, 3, "customers.read")
			assert.ErrorIs(t, err, ErrAdtyStaffDenied)
			_, err = MutateAdtyStaff(owner.Id+1000, site.Id, created.Id, AdtySiteOwnerActionStaffDelete, AdtyStaffInput{ExpectedVersion: 3})
			require.Error(t, err)
			// Failed audit writes roll back both grants and auth generation.
			const callback = "test:staff-audit-rollback"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "adty_site_audits" {
					tx.AddError(errors.New("audit unavailable"))
				}
			}))
			_, err = MutateAdtyStaff(owner.Id, site.Id, created.Id, AdtySiteOwnerActionStaffDelete, AdtyStaffInput{ExpectedVersion: 3})
			require.ErrorContains(t, err, "audit unavailable")
			require.NoError(t, db.Callback().Create().Remove(callback))
			_, _, err = AdtyStaffAccess(created.UserId, 3, "")
			require.NoError(t, err)
			for column, value := range map[string]any{"owner_user_id": owner.Id + 1000, "status": model.AdtySiteStatusClosed, "agreement_ver": "old"} {
				require.NoError(t, db.Model(&site).Update(column, value).Error)
				_, _, err = AdtyStaffAccess(created.UserId, 3, "")
				assert.ErrorIs(t, err, ErrAdtyStaffDenied, column)
				require.NoError(t, db.Model(&site).Updates(map[string]any{"owner_user_id": owner.Id, "status": model.AdtySiteStatusActive, "agreement_ver": AdtyCurrentSiteAgreementVersion()}).Error)
			}
			var user model.User
			require.NoError(t, db.First(&user, created.UserId).Error)
			assert.Zero(t, user.Quota, "employee creation never grants money")
			assert.Equal(t, site.Id, user.StaffSiteId)
			var audits []model.AdtySiteAudit
			require.NoError(t, db.Where("site_id = ?", site.Id).Find(&audits).Error)
			assert.Len(t, audits, 3, "only committed changes create audit events")
		})
	}
}

func TestAdtySiteFinanceMigrationPreservesExistingSite(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgresql"} {
		t.Run(dialect, func(t *testing.T) {
			var db *gorm.DB
			if dialect == "postgresql" {
				db = adtyTestDB(t)
			} else {
				var err error
				db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				require.NoError(t, err)
				previousDB, previousLogDB := model.DB, model.LOG_DB
				previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
				model.DB, model.LOG_DB = db, db
				common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
				t.Cleanup(func() {
					model.DB, model.LOG_DB = previousDB, previousLogDB
					common.SetDatabaseTypes(previousMain, previousLog)
					sqlDB, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, sqlDB.Close())
				})
			}
			require.NoError(t, db.AutoMigrate(&adtySiteBeforeFinance{}))
			legacy := adtySiteBeforeFinance{OwnerUserId: 805, Subdomain: "existing-site", RootDomain: "sites.example.com",
				Status: model.AdtySiteStatusActive, TierCostFactor: dec("0.75")}
			require.NoError(t, db.Create(&legacy).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(model.AdtySiteTables()...))
			}
			var upgraded model.AdtySite
			require.NoError(t, db.First(&upgraded, legacy.Id).Error)
			assert.Equal(t, 805, upgraded.OwnerUserId)
			assert.Equal(t, "existing-site", upgraded.Subdomain)
			require.NotNil(t, upgraded.TierCostFactor)
			assert.Equal(t, "0.75", upgraded.TierCostFactor.String())
			assert.Empty(t, upgraded.TierCode)
			require.NoError(t, EnsureAdtySiteWallets(upgraded.Id))
			_, err := ApplyAdtySiteWalletChange(upgraded.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(7), AdtySiteWalletChange{
				Type: "upgrade_credit", IdempotencyKey: "upgrade-credit",
			})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(model.AdtySiteTables()...))
			var wallet model.AdtySiteWallet
			require.NoError(t, db.Where("site_id = ? AND kind = ?", upgraded.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
			assert.Equal(t, "7", wallet.Balance.String())
			assert.Error(t, db.Create(&model.AdtySiteWallet{SiteId: upgraded.Id, Kind: model.AdtySiteWalletOperating, Currency: "USD"}).Error)
			assert.Error(t, db.Create(&model.AdtySite{Subdomain: "existing-site"}).Error)
		})
	}
}

func TestAdtySiteFinanceSQLiteMigrationAndMutation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	require.NoError(t, db.AutoMigrate(model.AdtySiteTables()...))
	site := &model.AdtySite{OwnerUserId: 904, Subdomain: "sqlite-finance", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	entry, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(4), AdtySiteWalletChange{
		Type: "sqlite_credit", IdempotencyKey: "sqlite-finance-1",
	})
	require.NoError(t, err)
	assert.True(t, entry.BalanceAfter.Equal(decimal.NewFromInt(4)))
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.ErrorIs(t, db.Model(&model.AdtySiteWalletLedger{}).Where("id = ?", entry.Id).Update("reason", "tamper").Error, model.ErrAdtySiteWalletLedgerImmutable)
	for range 2 {
		require.NoError(t, db.AutoMigrate(model.AdtySiteTables()...))
		require.NoError(t, EnsureAdtySiteWallets(site.Id))
	}
	var walletCount int64
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).Where("site_id = ?", site.Id).Count(&walletCount).Error)
	assert.EqualValues(t, 3, walletCount)
	var retained model.AdtySiteWalletLedger
	require.NoError(t, db.First(&retained, entry.Id).Error)
	assert.Equal(t, "4", retained.BalanceAfter.String())
	duplicate := model.AdtySiteWalletLedger{IdempotencyKey: "sqlite-finance-1"}
	assert.Error(t, db.Create(&duplicate).Error, "migration must preserve ledger uniqueness")
}

func TestAdtySiteWalletConcurrentReplayAndDebits(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "wallet-concurrent")
	var workers sync.WaitGroup
	creditErrors := make(chan error, 12)
	creditIDs := make(chan int64, 12)
	for range 12 {
		workers.Go(func() {
			entry, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(20), AdtySiteWalletChange{
				Type: "real_payment", IdempotencyKey: "concurrent-payment",
			})
			creditErrors <- err
			if entry != nil {
				creditIDs <- entry.Id
			}
		})
	}
	workers.Wait()
	close(creditErrors)
	close(creditIDs)
	for err := range creditErrors {
		require.NoError(t, err)
	}
	var firstID int64
	for id := range creditIDs {
		if firstID == 0 {
			firstID = id
		}
		assert.Equal(t, firstID, id, "concurrent retries must return the original entry")
	}
	debitErrors := make(chan error, 25)
	for i := range 25 {
		workers.Go(func() {
			_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(-1), AdtySiteWalletChange{
				Type: "real_consumption", IdempotencyKey: fmt.Sprintf("concurrent-debit-%d", i),
			})
			debitErrors <- err
		})
	}
	workers.Wait()
	close(debitErrors)
	successful, insufficient := 0, 0
	for err := range debitErrors {
		switch {
		case err == nil:
			successful++
		case errors.Is(err, ErrAdtySiteWalletInsufficient):
			insufficient++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 20, successful)
	assert.Equal(t, 5, insufficient)
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	assert.Equal(t, "0", wallet.Balance.String())
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ?", site.Id).Count(&count).Error)
	assert.EqualValues(t, 21, count, "one payment plus exactly twenty funded debits")
}

func TestAdtySiteWalletsInitializeAndApplyIdempotently(t *testing.T) {
	db := prepareSiteDB(t)
	site := &model.AdtySite{OwnerUserId: 901, Subdomain: "wallets", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)

	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	var wallets []model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ?", site.Id).Order("kind").Find(&wallets).Error)
	require.Len(t, wallets, 3)
	assert.Equal(t, []string{model.AdtySiteWalletEarnings, model.AdtySiteWalletMargin, model.AdtySiteWalletOperating}, []string{wallets[0].Kind, wallets[1].Kind, wallets[2].Kind})
	for _, wallet := range wallets {
		assert.True(t, wallet.Balance.IsZero(), wallet.Kind)
	}

	change := AdtySiteWalletChange{Type: "test_credit", RefType: "test", RefID: "wallet-1", IdempotencyKey: "wallet-idem-1"}
	first, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(100), change)
	require.NoError(t, err)
	assert.True(t, first.BalanceBefore.IsZero())
	assert.True(t, first.BalanceAfter.Equal(decimal.NewFromInt(100)))

	replay, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(100), change)
	require.NoError(t, err)
	assert.Equal(t, first.Id, replay.Id)

	var ledgerCount int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ?", site.Id).Count(&ledgerCount).Error)
	assert.EqualValues(t, 1, ledgerCount)

	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(101), change)
	assert.ErrorIs(t, err, ErrAdtySiteWalletIdempotencyConflict)

	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	assert.True(t, wallet.Balance.Equal(decimal.NewFromInt(100)))
}

func TestAdtySiteWalletRejectsShortfallAndRepaysOverdraft(t *testing.T) {
	db := prepareSiteDB(t)
	site := &model.AdtySite{OwnerUserId: 902, Subdomain: "wallets-debt", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))

	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.NewFromInt(-1), AdtySiteWalletChange{Type: "charge", RefType: "test", RefID: "shortfall", IdempotencyKey: "wallet-shortfall"})
	assert.ErrorIs(t, err, ErrAdtySiteWalletInsufficient)

	overdraft, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(-25), AdtySiteWalletChange{
		Type: "commission_reversal", RefType: "test", RefID: "debt", IdempotencyKey: "wallet-debt", AllowOverdraft: true,
	})
	require.NoError(t, err)
	assert.True(t, overdraft.BalanceAfter.Equal(decimal.NewFromInt(-25)))

	repaid, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(30), AdtySiteWalletChange{
		Type: "commission_repayment", RefType: "test", RefID: "repay", IdempotencyKey: "wallet-repay",
	})
	require.NoError(t, err)
	assert.True(t, repaid.BalanceBefore.Equal(decimal.NewFromInt(-25)))
	assert.True(t, repaid.BalanceAfter.Equal(decimal.NewFromInt(5)))

	var rows []model.AdtySiteWalletLedger
	require.NoError(t, db.Where("site_id = ? AND wallet_kind = ?", site.Id, model.AdtySiteWalletEarnings).Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.True(t, rows[0].BalanceBefore.IsZero())
	assert.True(t, rows[0].BalanceAfter.Equal(decimal.NewFromInt(-25)))
	assert.True(t, rows[1].BalanceBefore.Equal(decimal.NewFromInt(-25)))
	assert.True(t, rows[1].BalanceAfter.Equal(decimal.NewFromInt(5)))
	assert.ErrorIs(t, db.Model(&rows[0]).Update("reason", "tamper").Error, model.ErrAdtySiteWalletLedgerImmutable)
}

func TestAdtySiteTierSelectionAndManualOverride(t *testing.T) {
	db := prepareSiteDB(t)
	starter := model.AdtySiteTier{Code: "starter", Name: "Starter", MinMonthlyUSD: decimal.Zero, DepositUSD: decimal.NewFromInt(10), Enabled: true, SortOrder: 1}
	growth := model.AdtySiteTier{Code: "growth", Name: "Growth", MinMonthlyUSD: decimal.NewFromInt(500), Enabled: true, SortOrder: 2}
	enterprise := model.AdtySiteTier{Code: "enterprise", Name: "Enterprise", MinMonthlyUSD: decimal.NewFromInt(2000), Enabled: true, SortOrder: 3}
	require.NoError(t, SaveAdtySiteTiers([]model.AdtySiteTier{starter, growth, enterprise}))

	site := &model.AdtySite{OwnerUserId: 903, Subdomain: "tiered", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	selected, err := EvaluateAdtySiteTier(site.Id, decimal.NewFromInt(750), 1_700_000_000)
	require.NoError(t, err)
	assert.Equal(t, "growth", selected.TierCode)

	manual, err := SetAdtySiteTier(site.Id, "starter", AdtySiteTierModeManual, 1_700_000_100)
	require.NoError(t, err)
	assert.Equal(t, "starter", manual.TierCode)
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.RequiredBalance.Equal(decimal.NewFromInt(10)))

	unchanged, err := EvaluateAdtySiteTier(site.Id, decimal.NewFromInt(2500), 1_700_000_200)
	require.NoError(t, err)
	assert.Equal(t, "starter", unchanged.TierCode)
	assert.Equal(t, AdtySiteTierModeManual, unchanged.TierMode)
}

func TestAdtySiteSettlementProjectsIntoEarningsWallet(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "wallet-settlement")
	require.NoError(t, db.Create(&model.AdtySiteEarningDaily{
		SiteId: site.Id, UserId: owner.Id, Day: "2020-01-01", Kind: model.AdtySiteEarningUsage,
		Earning: decimal.NewFromInt(12), Revenue: decimal.NewFromInt(20), Platform: decimal.NewFromInt(8),
		Status: model.AdtySiteEarningPending,
	}).Error)

	settled, err := RunAdtySiteSettlementOnce(time.Now().AddDate(0, 0, 30))
	require.NoError(t, err)
	assert.Equal(t, 1, settled)

	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&wallet).Error)
	assert.True(t, wallet.Balance.Equal(decimal.NewFromInt(12)))
	var ledger model.AdtySiteWalletLedger
	require.NoError(t, db.Where("site_id = ? AND wallet_kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&ledger).Error)
	assert.Equal(t, "site_earning_settlement", ledger.Type)
}

func TestAdtySiteSettlementCreditsLateBatchOnSameCutoff(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "wallet-late-settlement")
	now := time.Now().AddDate(0, 0, 30)
	for _, day := range []string{"2020-01-01", "2020-01-01+1"} {
		require.NoError(t, db.Create(&model.AdtySiteEarningDaily{
			SiteId: site.Id, UserId: owner.Id, Day: day, Kind: model.AdtySiteEarningUsage,
			Earning: decimal.NewFromInt(12), Revenue: decimal.NewFromInt(20), Platform: decimal.NewFromInt(8),
			Status: model.AdtySiteEarningPending,
		}).Error)
		settled, err := RunAdtySiteSettlementOnce(now)
		require.NoError(t, err)
		assert.Equal(t, 1, settled)
	}
	settled, err := RunAdtySiteSettlementOnce(now)
	require.NoError(t, err)
	assert.Zero(t, settled)
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&wallet).Error)
	assert.Equal(t, "24", wallet.Balance.String(), "equal late batches are separate business operations")
	var ledgerCount int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ?", site.Id).Count(&ledgerCount).Error)
	assert.EqualValues(t, 2, ledgerCount)
	var account model.ReferralCommissionAccount
	require.NoError(t, db.Where("user_id = ?", owner.Id).First(&account).Error)
	assert.InDelta(t, 24, account.AvailableAmount, 0.00000001)
}

func TestAdtySiteWalletAllowsPartialMarginTopupAndDebtRepayment(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "wallet-partial-funding")
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).
		Update("required_balance", decimal.NewFromInt(100)).Error)
	entry, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.NewFromInt(25), AdtySiteWalletChange{
		Type: "margin_payment", IdempotencyKey: "margin-partial-1",
	})
	require.NoError(t, err, "a deposit shortfall must not prevent paying part of it")
	assert.Equal(t, "25", entry.BalanceAfter.String())
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.NewFromInt(-1), AdtySiteWalletChange{
		Type: "margin_debit", IdempotencyKey: "margin-partial-reject",
	})
	assert.ErrorIs(t, err, ErrAdtySiteWalletInsufficient)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(-20), AdtySiteWalletChange{
		Type: "earning_reversal", IdempotencyKey: "partial-debt", AllowOverdraft: true,
	})
	require.NoError(t, err)
	entry, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(5), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "partial-repayment",
	})
	require.NoError(t, err)
	assert.Equal(t, "-15", entry.BalanceAfter.String())
}

func TestAdtySiteTierManualOverrideSurvivesLegacyTasks(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tier-manual-isolation")
	require.NoError(t, SaveAdtySiteTiers([]model.AdtySiteTier{{Code: "manual", Name: "Manual", CostFactor: dec("0.7"), Enabled: true}}))
	adty_setting.GetResellerSetting().CostFactorTiers = []adty_setting.ResellerCostTier{{MinMonthlyUSD: 0, CostFactor: 0.6}}
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(map[string]any{
		"pending_tier_cost_factor": dec("0.95"), "pending_tier_cost_factor_at": 1,
	}).Error)
	InvalidateAdtySiteCache()
	require.NotNil(t, AdtySiteById(site.Id))
	fixed, err := SetAdtySiteTier(site.Id, "manual", AdtySiteTierModeManual, time.Now().Unix())
	require.NoError(t, err)
	assert.Nil(t, fixed.PendingTierCostFactor)
	assert.Zero(t, fixed.PendingTierCostFactorAt)
	factor, valid := AdtySiteCostFactor(AdtySiteById(site.Id))
	require.True(t, valid)
	assert.Equal(t, "0.7", factor.String(), "new tier cost applies immediately to cached billing reads")
	_, err = AssignAdtySiteCostTiers("2026-10", map[int]decimal.Decimal{site.Id: decimal.NewFromInt(1000)})
	require.NoError(t, err)
	stored, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	require.NotNil(t, stored.TierCostFactor)
	assert.Equal(t, "0.7", stored.TierCostFactor.String(), "old monthly tiers must not override a fixed tier")
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(map[string]any{
		"pending_tier_cost_factor": dec("0.95"), "pending_tier_cost_factor_at": 1,
	}).Error)
	applied, err := ApplyDueAdtySiteTierCostFactors(time.Now().Unix())
	require.NoError(t, err)
	assert.Zero(t, applied, "legacy delayed writes must also respect the new tier boundary")
	stored, err = GetAdtySite(site.Id)
	require.NoError(t, err)
	assert.Equal(t, "0.7", stored.TierCostFactor.String())
}

func TestAdtySiteWalletRejectsUnrepresentableAmounts(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "wallet-precision")
	for i, raw := range []string{"0.000000001", "-0.000000001", "100000000000000000000", "-100000000000000000000"} {
		_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.RequireFromString(raw), AdtySiteWalletChange{
			Type: "precision_check", IdempotencyKey: fmt.Sprintf("invalid-amount-%d", i), AllowOverdraft: true,
		})
		assert.ErrorIs(t, err, ErrAdtySiteWalletInvalidOperation, raw)
	}
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ?", site.Id).Count(&count).Error)
	assert.Zero(t, count, "unrepresentable amounts must not create silently rounded ledger entries")
	maximum := decimal.RequireFromString("99999999999999999999.99999999")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, maximum, AdtySiteWalletChange{
		Type: "maximum_credit", IdempotencyKey: "wallet-maximum",
	})
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.RequireFromString("0.00000001"), AdtySiteWalletChange{
		Type: "overflow_credit", IdempotencyKey: "wallet-overflow",
	})
	assert.ErrorIs(t, err, ErrAdtySiteWalletInvalidOperation)
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	assert.True(t, maximum.Equal(wallet.Balance))
}

func TestAdtySiteTierDefinitionChangesReachExistingAssignments(t *testing.T) {
	db := prepareSiteDB(t)
	tier := model.AdtySiteTier{Code: "dynamic", Name: "Dynamic", CostFactor: dec("0.7"), MaxMarkup: dec("2.5"), DepositUSD: decimal.NewFromInt(10), Enabled: true}
	require.NoError(t, SaveAdtySiteTiers([]model.AdtySiteTier{tier}))
	var sites []*model.AdtySite
	for i, mode := range []string{AdtySiteTierModeAuto, AdtySiteTierModeManual} {
		_, site := openActiveSite(t, db, fmt.Sprintf("dynamic-tier-%d", i))
		_, err := SetAdtySiteTier(site.Id, tier.Code, mode, time.Now().Unix())
		require.NoError(t, err)
		sites = append(sites, site)
		require.NotNil(t, AdtySiteById(site.Id))
	}
	tier.CostFactor, tier.MaxMarkup, tier.DepositUSD = dec("0.6"), dec("1.5"), decimal.NewFromInt(20)
	require.NoError(t, SaveAdtySiteTiers([]model.AdtySiteTier{tier}))
	for _, site := range sites {
		current := AdtySiteById(site.Id)
		require.NotNil(t, current)
		require.NotNil(t, current.TierCostFactor)
		assert.Equal(t, "0.6", current.TierCostFactor.String())
		require.NotNil(t, current.TierMaxMarkup)
		assert.Equal(t, "1.5", current.TierMaxMarkup.String())
		var margin model.AdtySiteWallet
		require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
		assert.Equal(t, "20", margin.RequiredBalance.String())
		assert.Equal(t, "0", margin.Balance.String(), "requirement updates are not deposit payments")
	}
}

func TestAdtySiteTierRejectsValuesRoundedByDatabase(t *testing.T) {
	for _, tier := range []model.AdtySiteTier{
		{Code: "precision", Name: "Precision", CostFactor: dec("0.0000001")},
		{Code: "precision", Name: "Precision", CostFactor: dec("0.1234567")},
		{Code: "precision", Name: "Precision", MaxMarkup: dec("1.00001")},
		{Code: "precision", Name: "Precision", MaxMarkup: dec("1000000")},
		{Code: "precision", Name: "Precision", MinMonthlyUSD: decimal.RequireFromString("0.000000001")},
		{Code: "precision", Name: "Precision", DepositUSD: decimal.RequireFromString("1000000000000")},
	} {
		assert.ErrorIs(t, ValidateAdtySiteTier(tier), ErrAdtySiteTierInvalid, "%+v", tier)
	}
}

func TestAdtySiteTierAssignmentUsesCurrentDefinition(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled_%t", enabled), func(t *testing.T) {
			db := prepareSiteDB(t)
			_, site := openActiveSite(t, db, "tier-concurrent-definition")
			tier := model.AdtySiteTier{Code: "current", Name: "Current", CostFactor: dec("0.7"), MaxMarkup: dec("2.5"), DepositUSD: decimal.NewFromInt(10), Enabled: true}
			require.NoError(t, SaveAdtySiteTiers([]model.AdtySiteTier{tier}))

			// Pause assignment before it locks the site, allowing a Root
			// definition edit to commit first. Both operations use real SQL.
			paused, proceed := make(chan struct{}), make(chan struct{})
			var pausedAssignment atomic.Bool
			var resumeOnce sync.Once
			resume := func() { resumeOnce.Do(func() { close(proceed) }) }
			defer resume()
			const callback = "adty_test:pause_tier_assignment"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "adty_sites" && pausedAssignment.CompareAndSwap(false, true) {
					close(paused)
					<-proceed
				}
			}))
			defer func() { require.NoError(t, db.Callback().Query().Remove(callback)) }()
			done := make(chan error, 1)
			go func() {
				_, err := SetAdtySiteTier(site.Id, tier.Code, AdtySiteTierModeManual, time.Now().Unix())
				done <- err
			}()
			select {
			case <-paused:
			case <-time.After(10 * time.Second):
				t.Fatal("assignment did not reach the site lock")
			}
			tier.CostFactor, tier.MaxMarkup, tier.DepositUSD, tier.Enabled = dec("0.6"), dec("1.5"), decimal.NewFromInt(20), enabled
			saveErr := SaveAdtySiteTiers([]model.AdtySiteTier{tier})
			resume()
			select {
			case err := <-done:
				require.NoError(t, saveErr)
				if !enabled {
					require.ErrorIs(t, err, ErrAdtySiteTierNotFound)
				} else {
					require.NoError(t, err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("assignment did not finish")
			}
			current, err := GetAdtySite(site.Id)
			require.NoError(t, err)
			if !enabled {
				assert.Empty(t, current.TierCode, "a disabled definition must not acquire a new assignment")
				return
			}
			require.NotNil(t, current.TierCostFactor)
			assert.Equal(t, "0.6", current.TierCostFactor.String())
			require.NotNil(t, current.TierMaxMarkup)
			assert.Equal(t, "1.5", current.TierMaxMarkup.String())
			var margin model.AdtySiteWallet
			require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
			assert.Equal(t, "20", margin.RequiredBalance.String())
		})
	}
}

func TestAdtySiteWalletConcurrentDebitReplayReturnsOriginalEntry(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "wallet-debit-replay")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(20), AdtySiteWalletChange{
		Type: "real_payment", IdempotencyKey: "debit-replay-funding",
	})
	require.NoError(t, err)
	const workers = 3 // The integration fixture permits four SQL connections.
	ready, proceed := make(chan struct{}, workers), make(chan struct{})
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(proceed) }) }
	defer resume()
	const callback = "adty_test:pause_debit_replay"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "adty_site_wallet_ledgers" && tx.RowsAffected == 0 {
			ready <- struct{}{}
			<-proceed
		}
	}))
	defer func() { require.NoError(t, db.Callback().Query().Remove(callback)) }()
	type outcome struct {
		entry *model.AdtySiteWalletLedger
		err   error
	}
	results := make(chan outcome, workers)
	for range workers {
		go func() {
			entry, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletOperating, decimal.NewFromInt(-20), AdtySiteWalletChange{
				Type: "real_consumption", IdempotencyKey: "same-debit-replayed",
			})
			results <- outcome{entry: entry, err: err}
		}()
	}
	for range workers {
		select {
		case <-ready:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent debits did not reach the initial idempotency lookup")
		}
	}
	resume()
	var entryID int64
	for range workers {
		select {
		case result := <-results:
			if !assert.NoError(t, result.err, "a replay of a successful debit must not report insufficient funds") {
				continue
			}
			require.NotNil(t, result.entry)
			if entryID == 0 {
				entryID = result.entry.Id
			}
			assert.Equal(t, entryID, result.entry.Id)
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent debits did not finish")
		}
	}
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&wallet).Error)
	assert.True(t, wallet.Balance.IsZero())
	var count int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key = ?", "same-debit-replayed").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
