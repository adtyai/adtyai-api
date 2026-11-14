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
	"github.com/QuantumNous/new-api/setting/operation_setting"
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
	require.NoError(t, creditAdtyTeamTopup(leader.Id, created.Team.Id, 500000, "topup-1"))
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.Equal(t, "Ops Renamed", team.Name)
	require.EqualValues(t, 500000, team.SharedQuota)
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
	view := ownerEarningView(map[string]any{"model": "gpt", "group": "default", "channel": "secret", "channel_id": 9})
	_, hasChannel := view["channel"]
	_, hasID := view["channel_id"]
	require.False(t, hasChannel)
	require.False(t, hasID)
	require.Equal(t, "gpt", view["model"])
	require.NoError(t, HideAdtyNotificationForUser(4, "notice-hide"))
	filtered, err := hideAdtyNotifications(4, AdtyNotificationInbox{Announcements: []map[string]any{{"read_key": "notice-hide"}, {"read_key": "keep"}}})
	require.NoError(t, err)
	require.Len(t, filtered.Announcements, 1)
	require.Equal(t, "keep", filtered.Announcements[0]["read_key"])
}

func TestSiteReferralTeamFloorAndDuplicateRefund(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, SetSiteReferralRate(4, decimal.RequireFromString("0.10")))
	require.True(t, SiteInviteCommissionUSD(decimal.NewFromInt(80), decimal.RequireFromString("0.10")).Equal(decimal.RequireFromString("8")))
	require.NoError(t, teamDiscountAllowed(decimal.RequireFromString("0.80"), decimal.RequireFromString("0.90"), false))
	require.Error(t, teamDiscountAllowed(decimal.RequireFromString("0.80"), decimal.RequireFromString("0.70"), false))
	require.NoError(t, teamDiscountAllowed(decimal.RequireFromString("0.80"), decimal.RequireFromString("0.70"), true))
	require.Equal(t, []string{"T-1"}, duplicateRefundTrades([]string{"T-1", "T-2", "T-1"}))
	require.NoError(t, recordAdtyAnomalyDisposition("duplicate_refund", "T-1", "reviewing", "second ledger", 1))
	owner := referralUser(t, db, "operating-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "operating-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	require.NoError(t, debitOperatingPrincipal(site.Id, decimal.RequireFromString("3"), "consume-1"))
	var operating model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletOperating).First(&operating).Error)
	require.True(t, operating.Balance.Equal(decimal.RequireFromString("-3")))
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
	require.NoError(t, setMainReferralRate(decimal.NewFromInt(10)))
	require.Equal(t, 10.0, *adty_setting.GetReferralSetting().DefaultRate)
	require.False(t, sensitiveActionNeedsProof(common.RoleRootUser))
	require.Equal(t, []string{"password", "email_code"}, sensitiveProofMethods(common.RoleCommonUser))
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
	require.NoError(t, bindSiteInvite(site.Id, inviter.Id, invitee.Id))
	require.Error(t, bindSiteInvite(site.Id, owner.Id, invitee.Id))
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
	mainTeam, err := CreateAdtyTeam(other.Id, 0, "MainAgree", decimal.NewFromInt(1))
	require.NoError(t, err)
	require.NoError(t, setMainTeamAgreementPrice(mainTeam.Team.Id, decimal.RequireFromString("0.90")))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	require.NoError(t, debitOperatingPrincipal(site.Id, decimal.RequireFromString("2"), "remain-op"))
	ops, _, err := ListAdtySiteWalletLedger(site.Id, model.AdtySiteWalletOperating, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, ops)
	require.NoError(t, applyDailyEarningOnce(site.Id, "2026-11-13", decimal.RequireFromString("4")))
	require.NoError(t, applyDailyEarningOnce(site.Id, "2026-11-13", decimal.RequireFromString("4")))
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("4")))
	order := &model.AdtyTopupOrder{}
	snapshotInviteCommission(order, decimal.RequireFromString("8"))
	require.True(t, order.ReferralRate.Equal(decimal.RequireFromString("8")))
	require.NoError(t, recordCommissionLedger("EPU-comm", 3, decimal.NewFromInt(100), decimal.NewFromInt(10)))
	require.NoError(t, recordCommissionLedger("EPU-comm", 3, decimal.NewFromInt(100), decimal.NewFromInt(10)))
	var commissions int64
	require.NoError(t, db.Model(&model.ReferralCommission{}).Where("source_trade_no = ?", "EPU-comm").Count(&commissions).Error)
	require.EqualValues(t, 1, commissions)
	cny, frozenAt, err := snapshotWithdrawFX(decimal.NewFromInt(20), decimal.RequireFromString("7.25"), 99)
	require.NoError(t, err)
	require.True(t, cny.Equal(decimal.RequireFromString("145.00")))
	require.EqualValues(t, 99, frozenAt)
	require.NoError(t, rejectCrossSite(site.Id, site.Id))
	require.Error(t, rejectCrossSite(site.Id, site.Id+1))
	require.NoError(t, db.Create(&model.AdtyTicket{SiteId: site.Id, UserId: owner.Id, Title: "help", Status: model.AdtyTicketNew}).Error)
	var ticket model.AdtyTicket
	require.NoError(t, db.Where("site_id = ?", site.Id).First(&ticket).Error)
	require.NoError(t, assignAdtyTicket(ticket.Id, owner.Id))
	require.NoError(t, db.First(&ticket, ticket.Id).Error)
	require.Equal(t, model.AdtyTicketInProgress, ticket.Status)
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
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	oldGet := AdtyTelegramGetMe
	AdtyTelegramGetMe = func(string) (int64, string, error) { return 0, "", ErrAdtyTelegramUnverified }
	t.Cleanup(func() { AdtyTelegramGetMe = oldGet })
	savedBot, err := saveSiteTelegramBot(site.Id, "424242:ABCDEFGHIJKLMNOPQRSTUV", true)
	require.NoError(t, err)
	require.NotContains(t, savedBot.View.TokenMask, "ABCDEF")
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
	html, err := os.ReadFile("../web/index.html")
	require.NoError(t, err)
	home, err := os.ReadFile("../web/src/features/adty-home/components/hero.tsx")
	require.NoError(t, err)
	require.True(t, publicPageResponsive(string(html), string(home)))
	item, ok := AnnouncementDetail([]map[string]any{{"read_key": "notice-1", "title": "维护"}}, "notice-1")
	require.True(t, ok)
	require.Equal(t, "维护", item["title"])
}

func TestShelfOrdersLedgerAndHealth(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, saveModelAlias("shelf", "gpt", 8000, true))
	require.NoError(t, setModelShelf("shelf", false))
	var alias model.AdtyModelAlias
	require.NoError(t, db.Where("alias = ?", "shelf").First(&alias).Error)
	require.False(t, alias.Enabled)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: "EPU-global"}).Error)
	global, err := listGlobalOrders(10)
	require.NoError(t, err)
	require.NotEmpty(t, global)
	require.Len(t, filterOrdersByStatus([]PersonalOrder{{TradeNo: "EPU-global", Status: "success"}, {TradeNo: "EPU-old", Status: "pending"}}, "success"), 1)
	user := referralUser(t, db, "ledger-user")
	require.NoError(t, db.Create(&model.AdtyBalanceLedger{UserId: user.Id, DeltaQuota: 10, Type: "topup", RefId: "EPU-global"}).Error)
	ledger, err := listUserLedger(user.Id, 10)
	require.NoError(t, err)
	require.Len(t, ledger, 1)
	require.NoError(t, storeDynamicTranslation("announcement:1", "hash-1", "Hello"))
	require.Error(t, changeOwnPassword(false, "long-enough"))
	require.NoError(t, changeOwnPassword(true, "long-enough"))
	require.True(t, accountLoginAllowed(common.UserStatusEnabled))
	require.False(t, accountLoginAllowed(common.UserStatusDisabled))
	require.True(t, serviceHealthy(nil))
	require.False(t, serviceHealthy(common.NewMessage("db down")))
}

func TestModelsGroupsOrdersAndLimits(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, setKeyRateLimit(8, 30))
	require.NoError(t, saveModelAlias("fast", "gpt", 128000, true))
	require.NoError(t, saveGroupConfig("vip", 0, decimal.RequireFromString("1.5"), "gpt,claude"))
	require.Equal(t, "tools", requestModality(true, true))
	require.Equal(t, "multimodal", requestModality(true, false))
	require.Equal(t, "retry", relayTerminal(false, 1, 3))
	require.Equal(t, "failed", relayTerminal(false, 3, 3))
	owner := referralUser(t, db, "orders-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "orders-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: "EPU-site", SiteId: site.Id, Provider: "epay", Method: "alipay"}).Error)
	rows, err := listSiteOrders(site.Id)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "epay:alipay", orderFundSource(rows[0].Provider, rows[0].Method))
	require.NoError(t, db.Create(&model.AdtyStaffBalanceRequest{SiteId: site.Id, StaffUserId: owner.Id, CustomerUserId: owner.Id, Direction: model.AdtyStaffBalanceIncrease, AmountUSD: decimal.NewFromInt(1), Status: model.AdtyStaffBalancePending}).Error)
	requests, err := ListStaffBalanceRequests(site.Id)
	require.NoError(t, err)
	require.Len(t, requests, 1)
}

func TestCatalogSiteOrdersAndSecrets(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	user := referralUser(t, db, "catalog-user")
	token := model.Token{UserId: user.Id, Key: "catalog-key", Name: "k", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, setKeyGroup(user.Id, token.Id, "vip"))
	require.NoError(t, setKeyAllowIps(user.Id, token.Id, "1.1.1.1, 2.2.2.2"))
	require.NoError(t, db.First(&token, token.Id).Error)
	require.Equal(t, "vip", token.Group)
	require.NotNil(t, token.AllowIps)
	require.Equal(t, "1.1.1.1,2.2.2.2", *token.AllowIps)
	modelName, group, err := parseRequestTarget("gpt", "vip")
	require.NoError(t, err)
	require.Equal(t, "gpt", modelName)
	require.Equal(t, "vip", group)
	site := model.AdtySite{Subdomain: "assign-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, assignSiteOwner(site.Id, user.Id))
	require.Error(t, assignSiteOwner(site.Id, user.Id+1))
	require.NoError(t, setSiteTier(site.Id, "gold"))
	customer := referralUser(t, db, "catalog-customer")
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	ids, err := listSiteCustomerIDs(site.Id)
	require.NoError(t, err)
	require.Equal(t, []int{customer.Id}, ids)
	require.Equal(t, "refund", classifyOrderKind("refund"))
	require.True(t, financeExportAllowed("finance"))
	require.False(t, financeExportAllowed("support"))
	text, ok := reuseTranslation("abc", "abc", "hello")
	require.True(t, ok)
	require.Equal(t, "hello", text)
	require.Equal(t, "****cret", maskSecret("secret"))
}

func TestKeysBrandOrdersAndTemplates(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	user := referralUser(t, db, "key-owner")
	token := model.Token{UserId: user.Id, Key: "key-secret", Name: "main", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, setKeyEnabled(user.Id, token.Id, false))
	require.NoError(t, db.First(&token, token.Id).Error)
	require.Equal(t, 2, token.Status)
	owner := referralUser(t, db, "brand-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "brand-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, remainingSetSiteBrand(site.Id, "North"))
	require.ErrorIs(t, setSiteEnabled(site.Id, false), ErrAdtySitePauseRetired)
	require.NoError(t, db.First(&site, site.Id).Error)
	require.Equal(t, "North", site.Brand)
	require.Equal(t, model.AdtySiteStatusActive, site.Status)
	detail, ok := orderDetail([]PersonalOrder{{TradeNo: "EPU-9", Kind: "topup", Status: "success"}}, "EPU-9")
	require.True(t, ok)
	require.Equal(t, "success", detail.Status)
	require.Error(t, testTranslationConnectivity("http://example.com"))
	require.NoError(t, testTranslationConnectivity("https://example.com/v1"))
	require.NoError(t, saveBilingualTemplate("payment_ok", "支付成功", "Payment received"))
}

func TestAccountOrdersTranslationAndHealth(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	parts := WalletUSDPartsFromQuota(500000, 250000)
	require.True(t, parts.Principal.Equal(decimal.NewFromInt(1)))
	require.True(t, parts.Bonus.Equal(decimal.RequireFromString("0.5")))
	rows := []PersonalOrder{{TradeNo: "EPU-1", Kind: "topup"}, {TradeNo: "EPU-2", Kind: "consume"}}
	require.Len(t, filterPersonalOrders(rows, "EPU-1", "", 1, 10), 1)
	require.True(t, notificationVisible("site", 4, 0, 0, 4, 0, 9))
	require.False(t, notificationVisible("user", 4, 0, 8, 4, 0, 9))
	require.True(t, translationPlaceholdersMatch("pay {{amount}}", "付款 {{amount}}"))
	require.False(t, translationPlaceholdersMatch("pay {{amount}}", "付款"))
	require.Equal(t, "no_sample", HealthSampleStatus(0, 0, 100, 60))
	require.Equal(t, "stale", HealthSampleStatus(3, 10, 100, 60))
	require.Equal(t, "ok", HealthSampleStatus(3, 90, 100, 60))
	user := referralUser(t, db, "account-gap")
	require.NoError(t, requestEmailChange(user.Id, "next@example.com"))
	require.NoError(t, requestAccountClosure(user.Id))
	require.Equal(t, "gold", evaluateSiteTier(12, decimal.NewFromInt(100), []SiteTierRule{{Code: "silver", MinUsers: 5, MinTopup: decimal.NewFromInt(10)}, {Code: "gold", MinUsers: 10, MinTopup: decimal.NewFromInt(50)}}))
	require.NoError(t, saveTranslationChoice("gpt", "https://example.com/v1"))
	require.True(t, translationRetryAllowed(model.AdtyTranslationFailed, 1, 3))
	require.False(t, translationRetryAllowed(model.AdtyTranslationFailed, 3, 3))
}

func TestRequestPricePrechargeAndExchangeRate(t *testing.T) {
	old := operation_setting.Price
	t.Cleanup(func() { operation_setting.Price = old })
	require.Error(t, setUSDToCNYRate(decimal.Zero))
	require.NoError(t, setUSDToCNYRate(decimal.RequireFromString("7.2")))
	require.Equal(t, 7.2, operation_setting.Price)
	snapshot := freezeRequestPrice("gpt", "default", decimal.RequireFromString("0.02"), 100)
	require.True(t, estimateRequestUSD(snapshot, decimal.NewFromInt(3)).Equal(decimal.RequireFromString("0.06")))
	require.Equal(t, 400, unusedPrechargeRelease(1000, 600))
	require.Equal(t, 0, unusedPrechargeRelease(1000, 1000))
	require.Equal(t, 0, failureFeeQuota(false, 1000))
	require.Equal(t, 1000, failureFeeQuota(true, 1000))
}

func TestPriceRevisionAndBalanceGap(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, recordAdtyPriceRevision(3, 1, "supply", "0.80", "0.75"))
	var rows []model.AdtyPriceRevision
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.EqualValues(t, 5, adtyBalanceLedgerGap(100, []int64{80, 15}))
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
	return owner, staffUser, customer, site
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
