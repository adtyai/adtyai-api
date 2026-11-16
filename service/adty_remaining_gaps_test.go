package service

import (
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTeamProfileAndTopupDoNotAddBonus(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "gap-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "Ops", decimal.NewFromInt(1))
	require.NoError(t, err)
	require.NoError(t, UpdateAdtyTeamProfile(leader.Id, created.Team.Id, "Ops Renamed"))
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.Equal(t, "Ops Renamed", team.Name)
	member := referralUser(t, db, "gap-member")
	_, err = JoinAdtyTeam(member.Id, created.Team.Id)
	require.NoError(t, err)
	require.ErrorIs(t, UpdateAdtyTeamProfile(member.Id, created.Team.Id, "Stolen"), ErrAdtyTeamForbidden)
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.Equal(t, "Ops Renamed", team.Name)
	require.NoError(t, DisableAdtyTeamMember(leader.Id, created.Team.Id, member.Id))
	require.Error(t, SpendAdtyTeam(member.Id, 1, "left-member"))
}

func TestStaffBalanceRequestNeedsApproval(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "gap-owner")
	staffUser := referralUser(t, db, "gap-staff")
	customer := referralUser(t, db, "gap-customer")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "gap-site", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: site.Id, OwnerUserId: owner.Id, UserId: staffUser.Id, Status: "enabled", Role: "support"}).Error)
	fundStaffBalanceSite(t, db, site.Id, "gap")
	request, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-bal-inc-1")
	require.NoError(t, err)
	require.Equal(t, model.AdtyStaffBalancePending, request.Status)
	var before model.User
	require.NoError(t, db.First(&before, customer.Id).Error)
	require.Error(t, ReviewAdtyStaffBalanceRequest(staffUser.Id, "member", request.Id, true))
	require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true))
	var after model.User
	require.NoError(t, db.First(&after, customer.Id).Error)
	require.EqualValues(t, before.Quota+1_000_000, after.Quota)
	require.Error(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true))
	root := referralUser(t, db, "gap-root")
	second, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceDecrease, decimal.RequireFromString("1"), "staff-bal-dec-1")
	require.NoError(t, err)
	require.NoError(t, ReviewAdtyStaffBalanceRequest(root.Id, "root", second.Id, true))
}

func TestFundChecksWindowsGlossaryAndEarningView(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.True(t, ReconcileOperatingWallet(decimal.RequireFromString("10"), decimal.RequireFromString("10")).IsZero())
	require.Equal(t, []string{"balance_ledger_gap", "open_shortfall_hold", "margin_anomaly"}, AdtyFundAnomalies(2, 1, decimal.RequireFromString("0.5")))
	require.Len(t, AdtyHealthWindowStarts(time.Date(2026, 11, 13, 12, 7, 0, 0, time.UTC)), 96)
	require.NoError(t, SaveAdtyGlossaryTerm("充值", "en", "top-up"))
	var terms int64
	require.NoError(t, db.Model(&model.AdtyGlossaryTerm{}).Count(&terms).Error)
	require.EqualValues(t, 1, terms)
	require.NoError(t, HideAdtyNotificationForUser(4, "notice-hide"))
	filtered, err := hideAdtyNotifications(4, AdtyNotificationInbox{Announcements: []map[string]any{{"read_key": "notice-hide"}, {"read_key": "keep"}}})
	require.NoError(t, err)
	require.Len(t, filtered.Announcements, 1)
	require.Equal(t, "keep", filtered.Announcements[0]["read_key"])
}

func TestSiteReferralTeamFloorAndDuplicateRefund(t *testing.T) {
	prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, SetSiteReferralRate(4, decimal.RequireFromString("0.10")))
	require.True(t, SiteInviteCommissionUSD(decimal.NewFromInt(80), decimal.RequireFromString("0.10")).Equal(decimal.RequireFromString("8")))
	require.NoError(t, recordAdtyAnomalyDisposition("duplicate_refund", "T-1", "reviewing", "second ledger", 1))
}

func TestStaffDeleteRevokesSession(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "staff-del-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "staff-del", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	created, err := CreateAdtyStaff(owner.Id, site.Id, AdtyStaffInput{Username: "staff-del", DisplayName: "Support", Password: "employee-password-42", Role: "support"})
	require.NoError(t, err)
	updated, err := MutateAdtyStaff(owner.Id, site.Id, created.Id, AdtySiteOwnerActionStaffUpdate, AdtyStaffInput{DisplayName: "Finance", Role: "finance", Status: "enabled", ExpectedVersion: created.Version})
	require.NoError(t, err)
	require.Equal(t, "finance", updated.Role)
	var before model.User
	require.NoError(t, db.First(&before, created.UserId).Error)
	_, err = MutateAdtyStaff(owner.Id, site.Id, created.Id, AdtySiteOwnerActionStaffDelete, AdtyStaffInput{ExpectedVersion: updated.Version})
	require.NoError(t, err)
	var staff model.AdtySiteStaff
	require.NoError(t, db.First(&staff, created.Id).Error)
	require.Equal(t, "deleted", staff.Status)
	var after model.User
	require.NoError(t, db.First(&after, created.UserId).Error)
	require.Greater(t, after.AuthVersion, before.AuthVersion)
	require.Equal(t, common.UserStatusDisabled, after.Status)
}

func TestPricingWalletsReferralAndReplayGuards(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.True(t, QuotaToUSD(500000).Equal(decimal.NewFromInt(1)))
	owner := referralUser(t, db, "price-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "price-site", Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&site).Error)
	factor := decimal.RequireFromString("0.75")
	_, err := AdjustAdtySite(site.Id, owner.Id, AdtySiteAdjust{CostFactor: &factor, Reason: "supply"})
	require.NoError(t, err)
	require.NoError(t, SetSiteReferralRate(site.Id, decimal.RequireFromString("0.08")))
	inviter := referralUser(t, db, "site-inviter")
	invitee := referralUser(t, db, "site-invitee")
	require.NoError(t, db.Model(inviter).Update("site_id", site.Id).Error)
	require.NoError(t, db.Model(invitee).Update("site_id", site.Id).Error)
	require.NoError(t, BindSiteInviteFromIP(site.Id, inviter.Id, invitee.Id, ""))
	require.Error(t, BindSiteInviteFromIP(site.Id, owner.Id, invitee.Id, ""))
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(5), AdtySiteWalletChange{Type: "earn", IdempotencyKey: "earn-ledger"})
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, decimal.NewFromInt(7), AdtySiteWalletChange{Type: "margin", IdempotencyKey: "margin-ledger"})
	require.NoError(t, err)
	earnings, _, err := ListAdtySiteWalletLedger(site.Id, model.AdtySiteWalletEarnings, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, earnings)
	margin, _, err := ListAdtySiteWalletLedger(site.Id, model.AdtySiteWalletMargin, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, margin)
}

func TestPaymentCallbackReplayDoesNotCreditTwice(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "replay-pay-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "replay-pay", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	user := siteCustomer(t, db, &site, "replay-pay-user")
	siteTopUp(t, user, &site, "EPU-replay-pay", 20, "0")
	var before model.User
	require.NoError(t, db.First(&before, user.Id).Error)
	already, err := model.CompleteAdtyTopUp("EPU-replay-pay", model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(20), FiatAmount: decimal.NewFromInt(20),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	require.True(t, already)
	var after model.User
	require.NoError(t, db.First(&after, user.Id).Error)
	require.Equal(t, before.Quota, after.Quota)
}

func TestRemainingPriceCommissionAndIsolation(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "remain-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "remain-site", Status: model.AdtySiteStatusActive, MaxMarkup: decimalPtr("2")}
	require.NoError(t, db.Create(&site).Error)
	markup := decimal.RequireFromString("1.2")
	_, err := SaveAdtySitePricing(AdtySitePricingInput{SiteId: site.Id, DefaultMarkup: &markup, Now: 1})
	require.NoError(t, err)
	leader := referralUser(t, db, "remain-leader")
	require.NoError(t, db.Model(leader).Update("site_id", site.Id).Error)
	team, err := CreateAdtyTeam(leader.Id, site.Id, "Remain", decimal.RequireFromString("0.90"))
	require.NoError(t, err)
	require.True(t, team.Team.Discount.Equal(decimal.RequireFromString("0.90")))
	main, err := CreateAdtyTeam(leader.Id, 0, "Main", decimal.NewFromInt(1))
	require.Error(t, err)
	_ = main
	other := referralUser(t, db, "remain-main-leader")
	_, err = CreateAdtyTeam(other.Id, 0, "MainAgree", decimal.NewFromInt(1))
	require.NoError(t, err)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	require.NoError(t, CompleteAdtyTodo(owner.Id, "severe-1"))
}

func TestSiteRegisterDocsAndRollback(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	cfg := adty_setting.GetResellerSetting()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	*cfg = adty_setting.DefaultResellerSetting()
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(5.0), new(10.0)
	owner := referralUser(t, db, "open-owner")
	_, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	site, err := OpenAdtySite(owner.Id, "north", model.AdtySiteOpenedByAdmin, AdtySitePreset{})
	require.NoError(t, err)
	require.Equal(t, owner.Id, site.OwnerUserId)
	// Official site registration is POST /api/user/register with a password.
	// A passwordless RegisterOnSite factory is not a landed product API.
	created := &model.User{
		Username: "north-user", Password: "correct-horse-1", Email: "north-user@example.com",
		SiteId: site.Id, Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(created).Error)
	require.Equal(t, site.Id, created.SiteId)
	require.Error(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.User{Username: "rollback-user", Email: "rollback@example.com"}).Error; err != nil {
			return err
		}
		return common.NewMessage("rollback")
	}))
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "rollback-user").Count(&count).Error)
	require.Zero(t, count)
	for _, path := range []string{
		"../web/src/features/adty-docs/content/zh-CN/roles.md",
		"../web/src/features/adty-docs/content/en/roles.md",
		"../web/src/features/adty-docs/content/en/openai-sdk.md",
		"../web/src/features/adty-docs/content/en/claude-code.md",
		"../web/src/features/adty-docs/content/zh-CN/models.md",
	} {
		body, err := os.ReadFile(path)
		require.NoError(t, err, path)
		require.NotEmpty(t, body, path)
	}
	zh, err := os.ReadFile("../web/src/features/adty-docs/content/zh-CN/roles.md")
	require.NoError(t, err)
	for _, heading := range []string{"普通用户", "团队负责人", "团队成员", "分站站长", "分站员工", "主站管理员"} {
		require.Contains(t, string(zh), heading)
	}
	item, ok := AnnouncementDetail([]map[string]any{{"read_key": "notice-1", "title": "维护"}}, "notice-1")
	require.True(t, ok)
	require.Equal(t, "维护", item["title"])
}

func TestModelsGroupsOrdersAndLimits(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "orders-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "orders-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.AdtyStaffBalanceRequest{SiteId: site.Id, StaffUserId: owner.Id, CustomerUserId: owner.Id, Direction: model.AdtyStaffBalanceIncrease, AmountUSD: decimal.NewFromInt(1), Status: model.AdtyStaffBalancePending}).Error)
	requests, err := ListStaffBalanceRequests(site.Id)
	require.NoError(t, err)
	require.Len(t, requests, 1)
}

func TestAccountOrdersTranslationAndHealth(t *testing.T) {
	prepareReferralDialectDB(t, "sqlite")
	parts := WalletUSDPartsFromQuota(500000, 250000)
	require.True(t, parts.Principal.Equal(decimal.NewFromInt(1)))
	require.True(t, parts.Bonus.Equal(decimal.RequireFromString("0.5")))
	require.Equal(t, "no_sample", HealthSampleStatus(0, 0, 100, 60))
	require.Equal(t, "stale", HealthSampleStatus(3, 10, 100, 60))
	require.Equal(t, "ok", HealthSampleStatus(3, 90, 100, 60))
}

func TestPriceRevisionAndBalanceGap(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, HideAdtyNotificationForUser(9, "notice-1"))
	require.NoError(t, HideAdtyNotificationForUser(9, "notice-1"))
	var hides int64
	require.NoError(t, db.Model(&model.AdtyNotificationHide{}).Where("user_id = ?", 9).Count(&hides).Error)
	require.EqualValues(t, 1, hides)
	var hideAudits int64
	require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("action = ? AND object_id = ?", "adty.notification.hide", "notice-1").Count(&hideAudits).Error)
	require.EqualValues(t, 1, hideAudits)
}

func TestHideAdtyNotificationWritesOfficialAuditOnce(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, model.MigrateAuditLogs())
	user := referralUser(t, db, "notice-hide-actor")
	site := model.AdtySite{OwnerUserId: user.Id, Subdomain: "notice-hide-site", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(user).Update("site_id", site.Id).Error)
	user.SiteId = site.Id
	publication := model.AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: 1, PublisherName: "Root",
		Audience: "all", TargetSiteId: site.Id, TargetSiteName: site.Subdomain,
		Kind: "notice", Title: "Maintenance", Content: "window",
		Business: "ops", Status: "published", Revision: 1,
		ReadKey: "notice-audit-1", PublishedAt: 1, UpdatedAt: 1, UpdatedBy: 1,
	}
	require.NoError(t, db.Create(&publication).Error)

	require.Error(t, HideAdtyNotificationForUser(0, publication.ReadKey))
	require.Error(t, HideAdtyNotificationForUser(user.Id, "  "))
	var beforeAudits int64
	require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("action = ?", "adty.notification.hide").Count(&beforeAudits).Error)
	require.Zero(t, beforeAudits)

	require.NoError(t, HideAdtyNotificationForUser(user.Id, publication.ReadKey))
	require.NoError(t, HideAdtyNotificationForUser(user.Id, publication.ReadKey))

	var hides int64
	require.NoError(t, db.Model(&model.AdtyNotificationHide{}).Where("user_id = ? AND key = ?", user.Id, publication.ReadKey).Count(&hides).Error)
	require.EqualValues(t, 1, hides)

	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ? AND object_id = ?", "adty.notification.hide", publication.ReadKey).Find(&audits).Error)
	require.Len(t, audits, 1)
	row := audits[0]
	require.NotNil(t, row.SiteId)
	require.Equal(t, site.Id, *row.SiteId)
	require.Equal(t, "notification", row.ObjectType)
	require.Equal(t, publication.ReadKey, row.ObjectId)
	require.Equal(t, "adty.notification.hide", row.Action)
	require.Equal(t, model.AuditResultSuccess, row.Result)
	require.True(t, row.Success)
	require.Equal(t, user.Id, row.UserId)
	require.Equal(t, "notice-hide-actor", row.Username)
	require.Equal(t, common.RoleCommonUser, row.ActorRole)

	var still model.AdtyNotificationPublication
	require.NoError(t, db.First(&still, publication.Id).Error)
	require.Equal(t, "published", still.Status)
	require.Equal(t, publication.ReadKey, still.ReadKey)

	requestSite := model.AdtySite{Id: site.Id + 100}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(AdtySiteContextKey, &requestSite)
	require.NoError(t, HideAdtyNotificationForUserFromRequest(ctx, user.Id, "notice-audit-ctx"))
	var fromRequest model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ? AND object_id = ?", "adty.notification.hide", "notice-audit-ctx").Take(&fromRequest).Error)
	require.NotNil(t, fromRequest.SiteId)
	require.Equal(t, requestSite.Id, *fromRequest.SiteId)
}

func staffBalanceBatchRemaining(t *testing.T, db *gorm.DB, userID int) int64 {
	t.Helper()
	var rows []model.AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", userID).Find(&rows).Error)
	var total int64
	for _, row := range rows {
		total += row.Remaining
	}
	return total
}

func staffBalanceLedgers(t *testing.T, db *gorm.DB, userID int) []model.AdtyBalanceLedger {
	t.Helper()
	var rows []model.AdtyBalanceLedger
	require.NoError(t, db.Where("user_id = ?", userID).Order("id").Find(&rows).Error)
	return rows
}

func seedStaffBalanceActors(t *testing.T, db *gorm.DB, prefix string) (owner, staffUser, customer *model.User, site model.AdtySite) {
	t.Helper()
	owner = referralUser(t, db, prefix+"-owner")
	staffUser = referralUser(t, db, prefix+"-staff")
	customer = referralUser(t, db, prefix+"-customer")
	site = model.AdtySite{OwnerUserId: owner.Id, Subdomain: prefix + "-site", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: site.Id, OwnerUserId: owner.Id, UserId: staffUser.Id, Status: "enabled", Role: "support"}).Error)
	fundStaffBalanceSite(t, db, site.Id, prefix)
	return owner, staffUser, customer, site
}

// fundStaffBalanceSite 给站点的运营钱包放进足够的钱。
//
// 员工给客户加款必须有出资（`ISS-R4-0075`）。放进钱让既有用例继续只测
// 「加款账务本身」；「钱包为空必须被拒」由
// `adty_staff_balance_funding_pg_test.go` 单独覆盖。
func fundStaffBalanceSite(t *testing.T, db *gorm.DB, siteID int, tag string) {
	t.Helper()
	require.NoError(t, EnsureAdtySiteWallets(siteID))
	_, err := ApplyAdtySiteWalletChange(siteID, model.AdtySiteWalletOperating, decimal.NewFromInt(100), AdtySiteWalletChange{
		Type: "seed", RefType: "test", RefID: tag, Reason: "seed operating balance for staff balance tests",
		IdempotencyKey: "seed-operating-" + tag,
	})
	require.NoError(t, err)
}

func requireWalletMatchesStaffBatches(t *testing.T, db *gorm.DB, userID int) {
	t.Helper()
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	require.EqualValues(t, user.Quota, staffBalanceBatchRemaining(t, db, userID))
}

func TestStaffBalanceApprovalWritesLedgerAndBatches(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			owner, staffUser, customer, _ := seedStaffBalanceActors(t, db, "staff-book-"+dialect)
			request, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-book-inc")
			require.NoError(t, err)
			require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true))

			var after model.User
			require.NoError(t, db.First(&after, customer.Id).Error)
			require.EqualValues(t, 1_000_000, after.Quota)
			ledgers := staffBalanceLedgers(t, db, customer.Id)
			require.Len(t, ledgers, 1)
			require.Equal(t, model.AdtyLedgerTypeManualAdjust, ledgers[0].Type)
			require.Equal(t, "staff_balance", ledgers[0].RefType)
			require.Equal(t, strconv.FormatInt(request.Id, 10), ledgers[0].RefId)
			require.EqualValues(t, 1_000_000, ledgers[0].DeltaQuota)
			require.EqualValues(t, 0, ledgers[0].BalanceBefore)
			require.EqualValues(t, 1_000_000, ledgers[0].BalanceAfter)
			require.Equal(t, owner.Id, ledgers[0].OperatorId)
			var lots []model.AdtyFundBatch
			require.NoError(t, db.Where("user_id = ? AND source = ?", customer.Id, adtyStaffBalanceLedgerKey(*request)).Find(&lots).Error)
			require.Len(t, lots, 1)
			require.Equal(t, "principal", lots[0].Kind)
			require.EqualValues(t, 1_000_000, lots[0].Original)
			require.EqualValues(t, 1_000_000, lots[0].Remaining)
			requireWalletMatchesStaffBatches(t, db, customer.Id)
		})
	}
}

func TestStaffBalanceDeductRefusesInFlightHolds(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			owner, staffUser, customer, _ := seedStaffBalanceActors(t, db, "staff-hold-"+dialect)
			credit, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-hold-inc")
			require.NoError(t, err)
			require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", credit.Id, true))
			require.NoError(t, db.Model(customer).Update("quota_pending", -600_000).Error)

			debit, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceDecrease, decimal.RequireFromString("1"), "staff-hold-dec")
			require.NoError(t, err)
			err = ReviewAdtyStaffBalanceRequest(owner.Id, "owner", debit.Id, true)
			require.Error(t, err)
			require.Equal(t, "Customer balance is not enough", err.Error())

			var after model.User
			require.NoError(t, db.First(&after, customer.Id).Error)
			require.EqualValues(t, 1_000_000, after.Quota)
			require.EqualValues(t, -600_000, after.QuotaPending)
			require.Len(t, staffBalanceLedgers(t, db, customer.Id), 1)
			require.EqualValues(t, 1_000_000, staffBalanceBatchRemaining(t, db, customer.Id))
			var pending model.AdtyStaffBalanceRequest
			require.NoError(t, db.First(&pending, debit.Id).Error)
			require.Equal(t, model.AdtyStaffBalancePending, pending.Status)
		})
	}
}

func TestStaffBalanceReviewReplayDoesNotDoubleBook(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			owner, staffUser, customer, _ := seedStaffBalanceActors(t, db, "staff-replay-"+dialect)
			first, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-replay-inc")
			require.NoError(t, err)
			again, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-replay-inc")
			require.NoError(t, err)
			require.Equal(t, first.Id, again.Id)
			_, err = SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("3"), "staff-replay-inc")
			require.ErrorIs(t, err, model.ErrAdtyIdempotencyConflict)
			_, err = SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "")
			require.Error(t, err)

			require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", first.Id, true))
			require.Error(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", first.Id, true))
			replayed, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-replay-inc")
			require.NoError(t, err)
			require.Equal(t, first.Id, replayed.Id)
			require.Equal(t, model.AdtyStaffBalanceApproved, replayed.Status)

			var after model.User
			require.NoError(t, db.First(&after, customer.Id).Error)
			require.EqualValues(t, 1_000_000, after.Quota)
			require.Len(t, staffBalanceLedgers(t, db, customer.Id), 1)
			requireWalletMatchesStaffBatches(t, db, customer.Id)
		})
	}
}

func TestStaffBalanceDecreaseConsumesBatches(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			owner, staffUser, customer, _ := seedStaffBalanceActors(t, db, "staff-dec-"+dialect)
			credit, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceIncrease, decimal.RequireFromString("2"), "staff-dec-inc")
			require.NoError(t, err)
			require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", credit.Id, true))
			debit, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceDecrease, decimal.RequireFromString("1"), "staff-dec-dec")
			require.NoError(t, err)
			require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", debit.Id, true))

			var after model.User
			require.NoError(t, db.First(&after, customer.Id).Error)
			require.EqualValues(t, 500_000, after.Quota)
			ledgers := staffBalanceLedgers(t, db, customer.Id)
			require.Len(t, ledgers, 2)
			require.EqualValues(t, -500_000, ledgers[1].DeltaQuota)
			require.EqualValues(t, 1_000_000, ledgers[1].BalanceBefore)
			require.EqualValues(t, 500_000, ledgers[1].BalanceAfter)
			requireWalletMatchesStaffBatches(t, db, customer.Id)
		})
	}
}
