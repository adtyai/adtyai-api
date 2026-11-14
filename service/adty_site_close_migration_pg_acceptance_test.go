package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestOfficialPostgresCloseMigrationDSNRejectsNonPostgres 确认非 PostgreSQL
// 连接串不能作为 A.16.7 的关闭条件；缺 DSN 必须标成 BLK-ENV-PG。
func TestOfficialPostgresCloseMigrationDSNRejectsNonPostgres(t *testing.T) {
	t.Setenv("ADTY_TEST_PG_DSN", "root:pass@tcp(127.0.0.1:3306)/new-api")
	t.Setenv("TEST_POSTGRES_DSN", "")
	_, err := model.OfficialPostgresAcceptanceDSN()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not PostgreSQL")

	t.Setenv("ADTY_TEST_PG_DSN", "")
	t.Setenv("TEST_POSTGRES_DSN", "")
	_, err = model.OfficialPostgresAcceptanceDSN()
	require.ErrorIs(t, err, model.ErrPostgresAcceptanceDSNUnset)
	assert.Contains(t, err.Error(), "BLK-ENV-PG")

	t.Setenv("TEST_POSTGRES_DSN", "file:memory?mode=memory&cache=shared")
	_, err = model.OfficialPostgresAcceptanceDSN()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not PostgreSQL")
}

func requireOfficialPostgresCloseMigrationDSN(t *testing.T) string {
	t.Helper()
	dsn, err := model.OfficialPostgresAcceptanceDSN()
	if errors.Is(err, model.ErrPostgresAcceptanceDSNUnset) {
		t.Skip("BLK-ENV-PG: ADTY_TEST_PG_DSN / TEST_POSTGRES_DSN is unset; this acceptance is not recorded as passed")
	}
	require.NoError(t, err)
	return dsn
}

// TestAdtyPostgreSQLCloseMigrationMatrixAcceptance 是 A.16.7 在真实 PostgreSQL
// 上的仓库内关闭条件：12 个官方场景并逐笔核对余额、账本、订单、团队、Key 和
// 原站点映射。它不把 L0/L1、FLOW-013 或 5.2 上线记为已关闭。
func TestAdtyPostgreSQLCloseMigrationMatrixAcceptance(t *testing.T) {
	dsn := requireOfficialPostgresCloseMigrationDSN(t)

	admin, err := model.OpenOfficialPostgresAcceptance(dsn)
	if err != nil {
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		_ = model.CloseOfficialGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	if err := adminSQL.Ping(); err != nil {
		_ = model.CloseOfficialGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	var current string
	if err := admin.Raw("SELECT current_database()").Scan(&current).Error; err != nil {
		_ = model.CloseOfficialGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	require.NotEmpty(t, current)
	require.False(t, model.OfficialThrowawayPostgresName(current), "admin DSN must not already point at a throwaway database")

	versionNum, version, err := model.OfficialPostgresServerVersion(admin)
	require.NoError(t, err)
	require.NotEmpty(t, versionNum, "live server_version_num must be recorded as evidence")
	t.Logf("A.16.7 evidence only (not a pass/fail condition): server_version_num=%s server_version=%s driver=%s", versionNum, version, admin.Dialector.Name())
	t.Cleanup(func() {
		require.NoError(t, model.CloseOfficialGorm(admin))
	})

	name, err := model.NewOfficialThrowawayPostgresName(model.OfficialPostgresThrowawayPrefix1617)
	require.NoError(t, err)
	db, err := model.CreateOfficialThrowawayPostgresDatabase(admin, dsn, name)
	require.NoError(t, err, "create throwaway database %s", name)
	t.Logf("throwaway database: %s", name)

	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	savedReseller := *adty_setting.GetResellerSetting()
	savedLegal := *adty_setting.GetLegalSetting()
	savedSite := *adty_setting.GetSiteSetting()
	oldAddress := system_setting.ServerAddress
	restoreDB := model.SwapOfficialPackageDatabase(db, common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	dropped := false
	t.Cleanup(func() {
		common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch
		*adty_setting.GetResellerSetting() = savedReseller
		*adty_setting.GetLegalSetting() = savedLegal
		*adty_setting.GetSiteSetting() = savedSite
		system_setting.ServerAddress = oldAddress
		InvalidateAdtySiteCache()
		if dropped {
			return
		}
		dropped = true
		_ = model.CloseOfficialGorm(db)
		restoreDB()
		if dropErr := model.DropOfficialThrowawayPostgresDatabase(admin, name); dropErr != nil {
			t.Errorf("drop throwaway database %s: %v", name, dropErr)
		}
	})

	require.NoError(t, model.RunOfficialMigrateDB(), "official migrate must initialize the throwaway database")
	require.NoError(t, i18n.Init())

	cfg := adty_setting.GetResellerSetting()
	*cfg = adty_setting.DefaultResellerSetting()
	cfg.Enabled = true
	cfg.RootDomains = []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(2.0), new(5.0), new(20.0)
	adty_setting.GetLegalSetting().SiteAgreementVersion = 3
	adty_setting.GetSiteSetting().SupportEmail = "support@adtyai.com"
	system_setting.ServerAddress = "https://adtyai.com"
	InvalidateAdtySiteCache()

	t.Run("scene_01_ordinary_site_customer_to_main", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s01ord")
		customer := siteCustomer(t, db, site, "s01-customer")
		const principal, bonus, frozen int64 = 800_000, 200_000, 50_000
		seedInheritedWallet(t, customer, site.Id, "s01-order", principal, bonus, frozen)
		tokenID := seedUserAPIKey(t, customer.Id, "sk-s01-old")

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s01-order", principal, bonus, frozen)
		requireKeyRevokedButRetained(t, tokenID, "sk-s01-old")
		assert.Equal(t, 0, mustUser(t, customer.Id).SiteId)
	})

	t.Run("scene_02_main_has_same_email_independent_account", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s02same")
		const shared = "shared-s02@example.com"
		mainUser := referralUser(t, db, "s02-main")
		require.NoError(t, db.Model(mainUser).Update("email", shared).Error)
		seedInheritedWallet(t, mainUser, 0, "s02-main-order", 2_000_000, 0, 0)
		customer := siteCustomer(t, db, site, "s02-site")
		require.NoError(t, db.Model(customer).Update("email", "Shared-S02@Example.com").Error)
		const principal, bonus, frozen int64 = 700_000, 100_000, 0
		seedInheritedWallet(t, customer, site.Id, "s02-site-order", principal, bonus, frozen)

		closeSiteNow(t, site)
		inherited := mustUser(t, customer.Id)
		original := mustUser(t, mainUser.Id)
		assert.NotEqual(t, original.Id, inherited.Id)
		assert.Zero(t, inherited.SiteId)
		assert.Zero(t, original.SiteId)
		assert.Equal(t, site.Id, inherited.InheritedFromSiteId)
		assert.Zero(t, original.InheritedFromSiteId)
		assert.Equal(t, customer.Username, inherited.Username)
		assert.Equal(t, mainUser.Username, original.Username)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s02-site-order", principal, bonus, frozen)
		requireReconciledWallet(t, mainUser.Id, 0, "s02-main-order", 2_000_000, 0, 0)
		var mainMappings int64
		require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id = ?", mainUser.Id).Count(&mainMappings).Error)
		assert.Zero(t, mainMappings)
		_, err := model.GetUniqueUserByEmail(shared, 0)
		assert.ErrorIs(t, err, model.ErrEmailAmbiguous)
	})

	t.Run("scene_03_multiple_sites_same_email", func(t *testing.T) {
		_, siteA := openActiveSite(t, db, "s03a")
		_, siteB := openActiveSite(t, db, "s03b")
		const shared = "shared-s03@example.com"
		mainUser := referralUser(t, db, "s03-main")
		require.NoError(t, db.Model(mainUser).Updates(map[string]any{"email": shared, "quota": 1_000_000}).Error)
		userA := siteCustomer(t, db, siteA, "s03-site-a")
		userB := siteCustomer(t, db, siteB, "s03-site-b")
		require.NoError(t, db.Model(userA).Update("email", shared).Error)
		require.NoError(t, db.Model(userB).Update("email", shared).Error)
		seedInheritedWallet(t, userA, siteA.Id, "s03-a-order", 300_000, 50_000, 0)
		seedInheritedWallet(t, userB, siteB.Id, "s03-b-order", 400_000, 80_000, 0)

		closeSiteNow(t, siteA)
		requireInheritedIdentity(t, db, userA.Id, siteA.Id)
		requireReconciledWallet(t, userA.Id, siteA.Id, "s03-a-order", 300_000, 50_000, 0)
		stillB := mustUser(t, userB.Id)
		assert.Equal(t, siteB.Id, stillB.SiteId)
		assert.Zero(t, stillB.InheritedFromSiteId)
		requireReconciledWallet(t, userB.Id, siteB.Id, "s03-b-order", 400_000, 80_000, 0)
		assert.Zero(t, mustUser(t, mainUser.Id).InheritedFromSiteId)
		assert.Equal(t, 1_000_000, mustUser(t, mainUser.Id).Quota)

		closeSiteNow(t, siteB)
		requireInheritedIdentity(t, db, userB.Id, siteB.Id)
		requireReconciledWallet(t, userB.Id, siteB.Id, "s03-b-order", 400_000, 80_000, 0)
		assert.NotEqual(t, userA.Id, userB.Id)
		assert.NotEqual(t, userA.Id, mainUser.Id)
		assert.Equal(t, siteA.Id, mustUser(t, userA.Id).InheritedFromSiteId)
		assert.Equal(t, siteB.Id, mustUser(t, userB.Id).InheritedFromSiteId)
		var mapsA, mapsB int64
		require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id = ?", userA.Id).Count(&mapsA).Error)
		require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id = ?", userB.Id).Count(&mapsB).Error)
		assert.Equal(t, int64(1), mapsA)
		assert.Equal(t, int64(1), mapsB)
	})

	t.Run("scene_04_principal_permanent_bonus_and_freeze", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s04fund")
		customer := siteCustomer(t, db, site, "s04-customer")
		const principal, bonus, frozen int64 = 1_200_000, 300_000, 80_000
		seedInheritedWallet(t, customer, site.Id, "s04-order", principal, bonus, frozen)
		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s04-order", principal, bonus, frozen)
		stored := mustUser(t, customer.Id)
		assert.Equal(t, int(principal+bonus-frozen), stored.Quota)
		assert.Equal(t, int(frozen), stored.QuotaPending)
	})

	t.Run("scene_05_inflight_api_requests_and_async_tasks", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s05fly")
		customer := siteCustomer(t, db, site, "s05-customer")
		const principal, bonus, frozen int64 = 900_000, 100_000, 120_000
		seedInheritedWallet(t, customer, site.Id, "s05-order", principal, bonus, frozen)
		op := model.BillingOperation{
			OperationKey: "s05-inflight-request", UserId: customer.Id, TokenId: 0,
			Kind: "request", ReservedQuota: int(frozen), TokenReservedQuota: 0,
			State: model.BillingReserved, SiteId: site.Id,
		}
		require.NoError(t, db.Create(&op).Error)
		task := model.Task{TaskID: "s05-async-task", UserId: customer.Id, Group: "default", Quota: 30_000, Status: model.TaskStatusInProgress, Progress: "30%"}
		require.NoError(t, db.Create(&task).Error)
		event := model.AdtyOrderPaymentEvent{
			TradeNo: "s05-order", SiteId: site.Id, UserId: customer.Id,
			Provider: "manual", Source: model.AdtyPaymentSourceCallback, Outcome: "pending",
		}
		require.NoError(t, db.Create(&event).Error)

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s05-order", principal, bonus, frozen)

		var storedOp model.BillingOperation
		require.NoError(t, db.Where("operation_key = ?", "s05-inflight-request").First(&storedOp).Error)
		assert.Equal(t, customer.Id, storedOp.UserId)
		assert.Equal(t, model.BillingReserved, storedOp.State)
		assert.Equal(t, int(frozen), storedOp.ReservedQuota)
		assert.Equal(t, site.Id, storedOp.SiteId)

		var storedTask model.Task
		require.NoError(t, db.Where("task_id = ?", "s05-async-task").First(&storedTask).Error)
		assert.Equal(t, customer.Id, storedTask.UserId)
		assert.Equal(t, 30_000, storedTask.Quota)
		assert.Equal(t, model.TaskStatus("IN_PROGRESS"), storedTask.Status)

		var storedEvent model.AdtyOrderPaymentEvent
		require.NoError(t, db.Where("trade_no = ?", "s05-order").First(&storedEvent).Error)
		assert.Equal(t, customer.Id, storedEvent.UserId)
		assert.Equal(t, site.Id, storedEvent.SiteId)
		assert.Equal(t, "pending", storedEvent.Outcome)
	})

	t.Run("scene_06_team_membership", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s06team")
		leader := siteCustomer(t, db, site, "s06-leader")
		member := siteCustomer(t, db, site, "s06-member")
		seedInheritedWallet(t, leader, site.Id, "s06-leader-order", 500_000, 0, 0)
		seedInheritedWallet(t, member, site.Id, "s06-member-order", 200_000, 0, 0)
		created, err := CreateAdtyTeam(leader.Id, site.Id, "s06-team", decimal.RequireFromString("0.9"))
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.AdtyTeamMember{TeamId: created.Team.Id, UserId: member.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}).Error)
		teamKeyID := seedUserAPIKey(t, leader.Id, "sk-s06-team")
		require.NoError(t, db.Create(&model.AdtyTeamKey{TokenId: teamKeyID, TeamId: created.Team.Id, UserId: leader.Id, Status: model.AdtyTeamMemberActive}).Error)

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, leader.Id, site.Id)
		requireInheritedIdentity(t, db, member.Id, site.Id)
		requireReconciledWallet(t, leader.Id, site.Id, "s06-leader-order", 500_000, 0, 0)
		requireReconciledWallet(t, member.Id, site.Id, "s06-member-order", 200_000, 0, 0)

		var inherited model.AdtyTeam
		require.NoError(t, db.First(&inherited, created.Team.Id).Error)
		assert.Equal(t, created.Team.Id, inherited.Id)
		assert.Zero(t, inherited.SiteId)
		assert.Equal(t, site.Id, inherited.SourceSiteId)
		assert.Equal(t, model.AdtyTeamStatusActive, inherited.Status)
		assert.Equal(t, leader.Id, inherited.LeaderUserId)
		var members []model.AdtyTeamMember
		require.NoError(t, db.Where("team_id = ? AND status = ?", inherited.Id, model.AdtyTeamMemberActive).Order("user_id").Find(&members).Error)
		require.Len(t, members, 2)
		ids := []int{members[0].UserId, members[1].UserId}
		assert.ElementsMatch(t, []int{leader.Id, member.Id}, ids)
		requireKeyRevokedButRetained(t, teamKeyID, "sk-s06-team")
	})

	t.Run("scene_07_outstanding_commission_refund_and_withdrawal", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s07pay")
		customer := siteCustomer(t, db, site, "s07-customer")
		inviter := siteCustomer(t, db, site, "s07-inviter")
		seedInheritedWallet(t, customer, site.Id, "s07-order", 600_000, 50_000, 0)
		commission := model.AdtySiteInviteCommission{
			SiteId: site.Id, InviterId: inviter.Id, InviteeId: customer.Id, TradeNo: "s07-order",
			FaceUSD: decimal.NewFromInt(1), Rate: decimal.RequireFromString("0.10"),
			AmountUSD: decimal.RequireFromString("0.10"), Status: model.AdtySiteCommissionPendingCoverage,
		}
		require.NoError(t, db.Create(&commission).Error)
		refund := model.AdtyManualRefund{
			RefundId: "s07-refund", TradeNo: "s07-order", UserId: customer.Id, SiteId: site.Id,
			Status: model.AdtyManualRefundStatusSubmitted, RequestUSD: decimal.RequireFromString("0.20"), RequestQuota: 100_000,
			Reason: "customer request",
		}
		require.NoError(t, db.Create(&refund).Error)
		withdrawal := model.AdtySiteInviteWithdrawal{
			SiteId: site.Id, UserId: inviter.Id, AmountUSD: decimal.RequireFromString("0.10"),
			NetUSD: decimal.RequireFromString("0.10"), Method: "usdt", Status: model.AdtySiteInviteWithdrawPending,
			IdempotencyKey: "s07-withdraw",
		}
		require.NoError(t, db.Create(&withdrawal).Error)

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireInheritedIdentity(t, db, inviter.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s07-order", 600_000, 50_000, 0)

		var storedCommission model.AdtySiteInviteCommission
		require.NoError(t, db.First(&storedCommission, commission.Id).Error)
		assert.Equal(t, commission.Id, storedCommission.Id)
		assert.Equal(t, site.Id, storedCommission.SiteId)
		assert.Equal(t, inviter.Id, storedCommission.InviterId)
		assert.Equal(t, customer.Id, storedCommission.InviteeId)
		assert.True(t, storedCommission.AmountUSD.Equal(decimal.RequireFromString("0.10")))
		assert.Equal(t, model.AdtySiteCommissionPendingCoverage, storedCommission.Status)
		assert.True(t, storedCommission.ClawedUSD.IsZero(), "close must not auto-claw commission")

		var storedRefund model.AdtyManualRefund
		require.NoError(t, db.First(&storedRefund, refund.Id).Error)
		assert.Equal(t, "s07-refund", storedRefund.RefundId)
		assert.Equal(t, customer.Id, storedRefund.UserId)
		assert.Equal(t, site.Id, storedRefund.SiteId)
		assert.Equal(t, model.AdtyManualRefundStatusSubmitted, storedRefund.Status)
		assert.Equal(t, int64(100_000), storedRefund.RequestQuota)
		assert.Zero(t, storedRefund.CompletedAt, "close must not auto-refund")

		var storedWithdraw model.AdtySiteInviteWithdrawal
		require.NoError(t, db.First(&storedWithdraw, withdrawal.Id).Error)
		assert.Equal(t, inviter.Id, storedWithdraw.UserId)
		assert.Equal(t, site.Id, storedWithdraw.SiteId)
		assert.Equal(t, model.AdtySiteInviteWithdrawPending, storedWithdraw.Status)
		assert.True(t, storedWithdraw.AmountUSD.Equal(decimal.RequireFromString("0.10")))
	})

	t.Run("scene_08_old_api_keys", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s08key")
		customer := siteCustomer(t, db, site, "s08-customer")
		seedInheritedWallet(t, customer, site.Id, "s08-order", 100_000, 0, 0)
		tokenID := seedUserAPIKey(t, customer.Id, "sk-s08-personal")
		_, err := model.ValidateUserToken("sk-s08-personal")
		require.NoError(t, err)

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s08-order", 100_000, 0, 0)
		requireKeyRevokedButRetained(t, tokenID, "sk-s08-personal")
	})

	t.Run("scene_09_unredeemed_vouchers_bound_to_closed_site", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s09code")
		customer := siteCustomer(t, db, site, "s09-customer")
		outsider := referralUser(t, db, "s09-outsider")
		seedInheritedWallet(t, customer, site.Id, "s09-order", 100_000, 0, 0)
		customerCode := seedVoucher(t, "s09custcode", model.AdtyVoucherPurposeSiteCustomer, model.AdtyVoucherStatusPending, site.Id, site.OwnerUserId)
		ownerCode := seedVoucher(t, "s09ownercode", model.AdtyVoucherPurposeSiteOwner, model.AdtyVoucherStatusPending, site.Id, site.OwnerUserId)
		// Official platform-sourced owner codes freeze the main voucher liability
		// at issuance; cancel on close must be able to record that release.
		seedPlatformVoucherLiability(t, ownerCode.AmountUSD)

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s09-order", 100_000, 0, 0)

		var inherited, revoked model.AdtyVoucher
		require.NoError(t, db.First(&inherited, customerCode.Id).Error)
		require.NoError(t, db.First(&revoked, ownerCode.Id).Error)
		assert.True(t, inherited.InheritedByMain)
		assert.Equal(t, model.AdtyVoucherStatusPending, inherited.Status)
		assert.Equal(t, site.Id, inherited.TargetSiteId)
		assert.Equal(t, site.Id, inherited.SourceSiteId)
		assert.Equal(t, model.AdtyVoucherStatusCancelled, revoked.Status)
		var liability model.AdtyVoucherLiability
		require.NoError(t, db.Where("scope = ?", "main").First(&liability).Error)
		assert.True(t, liability.FrozenUSD.Equal(decimal.Zero), "owner-code cancel must record the liability release")
		var grants int64
		require.NoError(t, db.Model(&model.AdtyVoucherCloseGrant{}).Where("site_id = ? AND user_id = ?", site.Id, customer.Id).Count(&grants).Error)
		assert.Equal(t, int64(1), grants)
		inheritedUser := mustUser(t, customer.Id)
		require.NoError(t, adtyVoucherRedeemAllowed(db, inheritedUser, &inherited))
		assert.ErrorIs(t, adtyVoucherRedeemAllowed(db, mustUser(t, outsider.Id), &inherited), ErrAdtyVoucherWrongAccount)
	})

	t.Run("scene_10_batch_interrupt_retry_concurrent_repeat", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s10int")
		first := siteCustomer(t, db, site, "s10-first")
		second := siteCustomer(t, db, site, "s10-second")
		seedInheritedWallet(t, first, site.Id, "s10-first-order", 150_000, 20_000, 0)
		seedInheritedWallet(t, second, site.Id, "s10-second-order", 180_000, 30_000, 0)
		firstKey := seedUserAPIKey(t, first.Id, "sk-s10-first")
		secondKey := seedUserAPIKey(t, second.Id, "sk-s10-second")
		require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(map[string]any{
			"status": model.AdtySiteStatusClosing, "close_execute_at": 1, "close_transfer_to": 0,
		}).Error)

		drop := installCloseMigrationInterruptTrigger(t, "adty_fail_close_s10", second.Id)
		err := ExecuteAdtySiteClose(site.Id, 10)
		require.Error(t, err, "an interrupted close transaction must fail closed")
		assert.Equal(t, site.Id, mustUser(t, first.Id).SiteId, "rollback must not leave the first customer migrated alone")
		assert.Equal(t, site.Id, mustUser(t, second.Id).SiteId)
		assert.Zero(t, mustUser(t, first.Id).InheritedFromSiteId)
		assert.Zero(t, mustUser(t, second.Id).InheritedFromSiteId)
		var maps int64
		require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id IN ?", []int{first.Id, second.Id}).Count(&maps).Error)
		assert.Zero(t, maps)
		_, err = model.ValidateUserToken("sk-s10-first")
		require.NoError(t, err, "rolled-back close must not revoke keys")
		_, err = model.ValidateUserToken("sk-s10-second")
		require.NoError(t, err)
		stillClosing, err := GetAdtySite(site.Id)
		require.NoError(t, err)
		assert.Equal(t, model.AdtySiteStatusClosing, stillClosing.Status)
		requireReconciledWallet(t, first.Id, site.Id, "s10-first-order", 150_000, 20_000, 0)
		requireReconciledWallet(t, second.Id, site.Id, "s10-second-order", 180_000, 30_000, 0)

		drop()
		require.NoError(t, ExecuteAdtySiteClose(site.Id, 10))
		requireInheritedIdentity(t, db, first.Id, site.Id)
		requireInheritedIdentity(t, db, second.Id, site.Id)
		requireReconciledWallet(t, first.Id, site.Id, "s10-first-order", 150_000, 20_000, 0)
		requireReconciledWallet(t, second.Id, site.Id, "s10-second-order", 180_000, 30_000, 0)
		requireKeyRevokedButRetained(t, firstKey, "sk-s10-first")
		requireKeyRevokedButRetained(t, secondKey, "sk-s10-second")

		// Repeat execute is idempotent: each user and fund fact succeeds only once.
		require.NoError(t, ExecuteAdtySiteClose(site.Id, 20))
		requireInheritedIdentity(t, db, first.Id, site.Id)
		requireInheritedIdentity(t, db, second.Id, site.Id)
		var mapCount int64
		require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id IN ?", []int{first.Id, second.Id}).Count(&mapCount).Error)
		assert.Equal(t, int64(2), mapCount)
		requireReconciledWallet(t, first.Id, site.Id, "s10-first-order", 150_000, 20_000, 0)

		_, concurrent := openActiveSite(t, db, "s10con")
		one := siteCustomer(t, db, concurrent, "s10-con-a")
		two := siteCustomer(t, db, concurrent, "s10-con-b")
		seedInheritedWallet(t, one, concurrent.Id, "s10-con-a-order", 110_000, 0, 0)
		seedInheritedWallet(t, two, concurrent.Id, "s10-con-b-order", 120_000, 0, 0)
		require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", concurrent.Id).Updates(map[string]any{
			"status": model.AdtySiteStatusClosing, "close_execute_at": 1, "close_transfer_to": 0,
		}).Error)
		var start sync.WaitGroup
		start.Add(2)
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			i := i
			go func() {
				defer start.Done()
				errs[i] = ExecuteAdtySiteClose(concurrent.Id, 10)
			}()
		}
		start.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		requireInheritedIdentity(t, db, one.Id, concurrent.Id)
		requireInheritedIdentity(t, db, two.Id, concurrent.Id)
		var concurrentMaps int64
		require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id IN ?", []int{one.Id, two.Id}).Count(&concurrentMaps).Error)
		assert.Equal(t, int64(2), concurrentMaps)
		requireReconciledWallet(t, one.Id, concurrent.Id, "s10-con-a-order", 110_000, 0, 0)
		requireReconciledWallet(t, two.Id, concurrent.Id, "s10-con-b-order", 120_000, 0, 0)
	})

	t.Run("scene_11_mail_partial_failure_and_resend", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s11mail")
		require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("brand", `{"name":"S11 Site","support_contact":"site-support@example.com"}`).Error)
		site, err = GetAdtySite(site.Id)
		require.NoError(t, err)
		const shared = "shared-s11@example.com"
		mainUser := referralUser(t, db, "s11-main")
		require.NoError(t, db.Model(mainUser).Updates(map[string]any{"email": shared, "quota": 9_999_000, "username": "s11-main-secret"}).Error)
		require.NoError(t, db.Create(&model.AdtyTopupOrder{
			TradeNo: "s11-secret-order", UserId: mainUser.Id, Provider: "manual", Method: "usd",
			FaceUSD: decimal.NewFromInt(19), CreditQuota: 9_999_000, PayAmount: decimal.NewFromInt(19),
			PayCurrency: "USD", SiteId: 0, TopupGroupRatio: decimal.NewFromInt(1),
		}).Error)
		okUser := siteCustomer(t, db, site, "s11-ok")
		failUser := siteCustomer(t, db, site, "s11-fail")
		require.NoError(t, db.Model(okUser).Update("email", shared).Error)
		okUser.Email = shared
		require.NoError(t, db.Model(failUser).Update("email", "s11-fail@example.com").Error)
		failUser.Email = "s11-fail@example.com"
		seedInheritedWallet(t, okUser, site.Id, "s11-ok-order", 250_000, 40_000, 0)
		seedInheritedWallet(t, failUser, site.Id, "s11-fail-order", 260_000, 10_000, 0)
		site.CloseExecuteAt = 1_800_000_000

		var mu sync.Mutex
		sent := map[string]string{}
		previous := AdtyDeliverSiteEmail
		AdtyDeliverSiteEmail = func(_ *model.AdtySite, subject, to, content string) error {
			mu.Lock()
			defer mu.Unlock()
			if to == failUser.Email {
				return errors.New("forced delivery failure")
			}
			sent[to] = subject + "\n" + content
			return nil
		}
		t.Cleanup(func() { AdtyDeliverSiteEmail = previous })

		require.NoError(t, SendAdtySiteCloseMigrationMail(site))
		var batch model.AdtySiteCloseMailBatch
		require.NoError(t, db.Where("site_id = ?", site.Id).First(&batch).Error)
		assert.Equal(t, site.Id, batch.SiteId)
		assert.Equal(t, 2, batch.RecipientCount)
		assert.Equal(t, 1, batch.SuccessCount)
		assert.Equal(t, 1, batch.FailCount)
		assert.Equal(t, model.AdtySiteCloseMailRetryRetryable, batch.RetryStatus)
		assert.NotZero(t, batch.SentAt)
		assert.Contains(t, sent, okUser.Email)
		assert.NotContains(t, sent, failUser.Email)
		okBody := sent[okUser.Email]
		assert.Contains(t, okBody, okUser.Username)
		assert.Contains(t, okBody, "https://adtyai.com")
		assert.Contains(t, okBody, "site-support@example.com")
		assert.NotContains(t, okBody, failUser.Email)
		assert.NotContains(t, okBody, "s11-main-secret")
		assert.NotContains(t, okBody, "s11-secret-order")
		assert.NotContains(t, okBody, "9999000")
		assert.NotContains(t, okBody, failUser.Username)
		var failed []model.AdtySiteCloseMailRecipient
		require.NoError(t, db.Where("batch_id = ? AND status = ?", batch.Id, model.AdtySiteCloseMailFailed).Find(&failed).Error)
		require.Len(t, failed, 1)
		assert.Equal(t, failUser.Id, failed[0].UserId)
		assert.Equal(t, failUser.Email, failed[0].Email)
		assert.Equal(t, failUser.Username, failed[0].Username)

		// Mail failure must not block the official internal migrate.
		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, okUser.Id, site.Id)
		requireInheritedIdentity(t, db, failUser.Id, site.Id)
		requireReconciledWallet(t, okUser.Id, site.Id, "s11-ok-order", 250_000, 40_000, 0)
		requireReconciledWallet(t, failUser.Id, site.Id, "s11-fail-order", 260_000, 10_000, 0)

		AdtyDeliverSiteEmail = func(_ *model.AdtySite, subject, to, content string) error {
			mu.Lock()
			defer mu.Unlock()
			sent[to] = subject + "\n" + content
			return nil
		}
		require.NoError(t, RetryAdtySiteCloseMigrationMail(site))
		require.NoError(t, db.Where("site_id = ?", site.Id).First(&batch).Error)
		assert.Equal(t, 2, batch.RecipientCount)
		assert.Equal(t, 2, batch.SuccessCount)
		assert.Zero(t, batch.FailCount)
		assert.Equal(t, model.AdtySiteCloseMailRetryComplete, batch.RetryStatus)
		require.Contains(t, sent, failUser.Email)
		failBody := sent[failUser.Email]
		assert.Contains(t, failBody, failUser.Username)
		assert.NotContains(t, failBody, okUser.Email)
		assert.NotContains(t, failBody, okUser.Username)
		assert.NotContains(t, failBody, "s11-main-secret")
		assert.NotContains(t, failBody, "s11-secret-order")
		assert.Contains(t, failBody, "https://adtyai.com")
		// A second retry must not create another campaign or duplicate recipients.
		require.NoError(t, RetryAdtySiteCloseMigrationMail(site))
		var batchCount, recipientCount int64
		require.NoError(t, db.Model(&model.AdtySiteCloseMailBatch{}).Where("site_id = ?", site.Id).Count(&batchCount).Error)
		require.NoError(t, db.Model(&model.AdtySiteCloseMailRecipient{}).Where("site_id = ?", site.Id).Count(&recipientCount).Error)
		assert.Equal(t, int64(1), batchCount)
		assert.Equal(t, int64(2), recipientCount)
	})

	t.Run("scene_12_old_domain_session_and_key_rejected", func(t *testing.T) {
		_, site := openActiveSite(t, db, "s12deny")
		customer := siteCustomer(t, db, site, "s12-customer")
		seedInheritedWallet(t, customer, site.Id, "s12-order", 130_000, 20_000, 0)
		tokenID := seedUserAPIKey(t, customer.Id, "sk-s12-old")
		sid := seedLoginSession(t, customer.Id, int64(customer.AuthVersion))
		oldHost := AdtySiteDisplayHost(site)
		identity := AuthIdentity{UserID: customer.Id, SessionID: sid, UserAuthVersion: int64(customer.AuthVersion), SessionVersion: 1}
		_, _, err := ValidateLoginSession(identity)
		require.NoError(t, err)
		_, err = model.ValidateUserToken("sk-s12-old")
		require.NoError(t, err)
		require.True(t, AdtySiteServesCustomers(site))
		assert.Zero(t, AdtyRejectSessionAPIStatus(oldHost))

		closeSiteNow(t, site)
		requireInheritedIdentity(t, db, customer.Id, site.Id)
		requireReconciledWallet(t, customer.Id, site.Id, "s12-order", 130_000, 20_000, 0)
		requireKeyRevokedButRetained(t, tokenID, "sk-s12-old")

		closed, err := GetAdtySite(site.Id)
		require.NoError(t, err)
		assert.False(t, AdtySiteServesCustomers(closed))
		assert.Equal(t, http.StatusForbidden, AdtyRejectSessionAPIStatus(oldHost))
		assert.Equal(t, closed.Id, AdtySiteForHost(oldHost).Id)
		assert.Equal(t, 0, mustUser(t, customer.Id).SiteId, "new business belongs to main")
		_, _, err = ValidateLoginSession(identity)
		assert.ErrorIs(t, err, ErrLoginSessionRevoked)
		session, err := model.GetUserSessionBySID(sid)
		require.NoError(t, err)
		assert.Equal(t, model.UserSessionStatusActive, session.Status, "the session row is retained; the bumped auth version rejects it")
		assert.NotEqual(t, session.UserAuthVersion, int64(mustUser(t, customer.Id).AuthVersion))
	})
}

func closeSiteNow(t *testing.T, site *model.AdtySite) {
	t.Helper()
	closed, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{Immediate: true, Reason: "A.16.7 matrix"})
	require.NoError(t, err)
	require.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.Zero(t, closed.CloseTransferTo)
	InvalidateAdtySiteCache()
}

func seedInheritedWallet(t *testing.T, user *model.User, siteId int, tradeNo string, principal, bonus, frozen int64) {
	t.Helper()
	available := int(principal + bonus - frozen)
	require.GreaterOrEqual(t, available, 0)
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"quota": available, "quota_pending": int(frozen)}).Error)
	user.Quota = available
	user.QuotaPending = int(frozen)
	require.NoError(t, model.CreateAdtyFundBatchesTx(model.DB, user.Id, tradeNo, principal, bonus))
	require.NoError(t, model.InsertAdtyLedgerTx(model.DB, &model.AdtyBalanceLedger{
		UserId: user.Id, DeltaQuota: principal + bonus, BalanceBefore: 0, BalanceAfter: principal + bonus,
		Type: "topup", RefType: "order", RefId: tradeNo, Reason: "A.16.7 seed",
	}))
	require.NoError(t, model.DB.Create(&model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: user.Id, Provider: "manual", Method: "usd",
		FaceUSD: decimal.NewFromInt(principal / 500000), CreditQuota: principal + bonus,
		PayAmount: decimal.NewFromInt(principal / 500000), PayCurrency: "USD",
		PaidAmount: decimal.NewFromInt(principal / 500000), PaidCurrency: "USD", PaidSource: "manual",
		SiteId: siteId, TopupGroupRatio: decimal.NewFromInt(1),
	}).Error)
}

func seedUserAPIKey(t *testing.T, userId int, key string) int {
	t.Helper()
	token := &model.Token{UserId: userId, Key: key, Name: key, Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
	require.NoError(t, model.DB.Create(token).Error)
	return token.Id
}

func seedVoucher(t *testing.T, code, purpose, status string, siteId, issuerId int) *model.AdtyVoucher {
	t.Helper()
	sum := sha256.Sum256([]byte(code))
	row := &model.AdtyVoucher{
		CodeHash: hex.EncodeToString(sum[:]), CodeTail: code[len(code)-4:], Purpose: purpose, Status: status,
		AmountUSD: decimal.NewFromInt(1), Quota: 500_000, SourceKind: model.AdtyVoucherSourcePlatform,
		SourceSiteId: siteId, TargetSiteId: siteId, IssuerId: issuerId,
	}
	require.NoError(t, model.DB.Create(row).Error)
	return row
}

func seedLoginSession(t *testing.T, userId int, authVersion int64) string {
	t.Helper()
	now := time.Now().Unix()
	sid := fmt.Sprintf("sid-s12-%d", userId)
	session := &model.UserSession{
		SID: sid, UserID: userId, Version: 1, UserAuthVersion: authVersion,
		Status: model.UserSessionStatusActive, RefreshHash: fmt.Sprintf("%064d", userId),
		LoginMethod: "password", CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 86400,
	}
	require.NoError(t, model.CreateUserSession(session))
	return sid
}

func mustUser(t *testing.T, id int) *model.User {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, id).Error)
	return &user
}

func requireInheritedIdentity(t *testing.T, db *gorm.DB, userId, sourceSiteId int) {
	t.Helper()
	requireArchivedCloseIdentity(t, db, userId, sourceSiteId)
	stored := mustUser(t, userId)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, sourceSiteId, stored.InheritedFromSiteId)
	assert.Equal(t, userId, stored.Id)
}

func requireReconciledWallet(t *testing.T, userId, originalSiteId int, tradeNo string, principal, bonus, frozen int64) {
	t.Helper()
	stored := mustUser(t, userId)
	assert.Equal(t, int(principal+bonus-frozen), stored.Quota)
	assert.Equal(t, int(frozen), stored.QuotaPending)
	var lots []model.AdtyFundBatch
	require.NoError(t, model.DB.Where("user_id = ? AND source = ?", userId, tradeNo).Order("kind").Find(&lots).Error)
	got := map[string]int64{}
	for _, lot := range lots {
		got[lot.Kind] = lot.Remaining
		assert.Equal(t, userId, lot.UserId)
		if lot.Kind == "principal" {
			assert.Equal(t, principal, lot.Original)
		}
		if lot.Kind == "bonus" {
			assert.Equal(t, bonus, lot.Original)
		}
	}
	assert.Equal(t, principal, got["principal"])
	if bonus > 0 {
		assert.Equal(t, bonus, got["bonus"])
	} else {
		assert.Zero(t, got["bonus"])
	}
	var ledger model.AdtyBalanceLedger
	require.NoError(t, model.DB.Where("user_id = ? AND ref_id = ?", userId, tradeNo).First(&ledger).Error)
	assert.Equal(t, principal+bonus, ledger.DeltaQuota)
	assert.Equal(t, principal+bonus, ledger.BalanceAfter)
	var order model.AdtyTopupOrder
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	assert.Equal(t, userId, order.UserId)
	assert.Equal(t, originalSiteId, order.SiteId)
	assert.Equal(t, principal+bonus, order.CreditQuota)
}

func requireKeyRevokedButRetained(t *testing.T, tokenID int, secret string) {
	t.Helper()
	_, err := model.ValidateUserToken(secret)
	assert.Error(t, err, "old API key secret must be unusable after close")
	var live model.Token
	assert.Error(t, model.DB.First(&live, tokenID).Error)
	var historical model.Token
	require.NoError(t, model.DB.Unscoped().First(&historical, tokenID).Error)
	assert.Equal(t, tokenID, historical.Id)
	assert.Equal(t, secret, historical.Key)
	assert.True(t, historical.DeletedAt.Valid, "historical key id is retained by soft delete")
}

func seedPlatformVoucherLiability(t *testing.T, frozen decimal.Decimal) {
	t.Helper()
	var row model.AdtyVoucherLiability
	lookup := model.DB.Where("scope = ?", "main").Limit(1).Find(&row)
	require.NoError(t, lookup.Error)
	if lookup.RowsAffected == 0 {
		require.NoError(t, model.DB.Create(&model.AdtyVoucherLiability{Scope: "main", LimitUSD: frozen.Add(decimal.NewFromInt(100)), FrozenUSD: frozen}).Error)
		return
	}
	require.NoError(t, model.DB.Model(&row).Updates(map[string]any{
		"limit_usd":  decimal.Max(row.LimitUSD, row.FrozenUSD.Add(row.SpentUSD).Add(frozen).Add(decimal.NewFromInt(1))),
		"frozen_usd": row.FrozenUSD.Add(frozen),
	}).Error)
}
func installCloseMigrationInterruptTrigger(t *testing.T, name string, userID int) func() {
	t.Helper()
	fn := name + "_fn"
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$
BEGIN
	RAISE EXCEPTION 'forced close-migration transaction interrupt';
END;
$$ LANGUAGE plpgsql`, fn)).Error)
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON users FOR EACH ROW WHEN (OLD.id = %d AND NEW.site_id IS DISTINCT FROM OLD.site_id) EXECUTE FUNCTION %s()`, name, userID, fn)).Error)
	dropped := false
	drop := func() {
		if dropped {
			return
		}
		dropped = true
		_ = model.DB.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON users`, name)).Error
		_ = model.DB.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn)).Error
	}
	t.Cleanup(drop)
	return drop
}
