package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCollectAdtySidebarBadgesConfirmedKindsAndScopes(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, db.AutoMigrate(model.ReferralTables()...))
	require.NoError(t, model.MigrateAdtyTables())

	root := badgeTestUser(t, db, "badge-root", common.RoleRootUser, 0)
	admin := badgeTestUser(t, db, "badge-admin", common.RoleAdminUser, 0)
	ownerA := badgeTestUser(t, db, "badge-owner-a", common.RoleCommonUser, 0)
	ownerB := badgeTestUser(t, db, "badge-owner-b", common.RoleCommonUser, 0)
	siteA := model.AdtySite{OwnerUserId: ownerA.Id, Subdomain: "badge-site-a", Status: model.AdtySiteStatusActive}
	siteB := model.AdtySite{OwnerUserId: ownerB.Id, Subdomain: "badge-site-b", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)
	require.NoError(t, db.Model(ownerA).Update("site_id", siteA.Id).Error)
	require.NoError(t, db.Model(ownerB).Update("site_id", siteB.Id).Error)
	ownerA.SiteId, ownerB.SiteId = siteA.Id, siteB.Id

	staffA := badgeTestUser(t, db, "badge-staff-a", common.RoleCommonUser, siteA.Id)
	staffB := badgeTestUser(t, db, "badge-staff-b", common.RoleCommonUser, siteB.Id)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: siteA.Id, OwnerUserId: ownerA.Id, UserId: staffA.Id, Role: "support", Status: "enabled", Permissions: `["customers.read"]`}).Error)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: siteB.Id, OwnerUserId: ownerB.Id, UserId: staffB.Id, Role: "support", Status: "enabled", Permissions: `["customers.read"]`}).Error)

	customerA := badgeTestUser(t, db, "badge-customer-a", common.RoleCommonUser, siteA.Id)
	customerB := badgeTestUser(t, db, "badge-customer-b", common.RoleCommonUser, siteB.Id)
	mainUser := badgeTestUser(t, db, "badge-main-user", common.RoleCommonUser, 0)
	leader := badgeTestUser(t, db, "badge-leader", common.RoleCommonUser, siteA.Id)
	member := badgeTestUser(t, db, "badge-member", common.RoleCommonUser, siteA.Id)
	team := model.AdtyTeam{SiteId: siteA.Id, LeaderUserId: leader.Id, Name: "badge-team", Status: model.AdtyTeamStatusActive, Discount: decimal.NewFromInt(1)}
	require.NoError(t, db.Create(&team).Error)
	require.NoError(t, db.Create(&model.AdtyTeamMember{TeamId: team.Id, UserId: leader.Id, Role: model.AdtyTeamRoleLeader, Status: model.AdtyTeamMemberActive}).Error)
	require.NoError(t, db.Create(&model.AdtyTeamMember{TeamId: team.Id, UserId: member.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}).Error)

	pubVisible := model.AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: root.Id, PublisherName: "root", Audience: "all_users",
		Kind: "system", Title: "Notice A", Content: "Hello", Status: "published", Revision: 1,
		ReadKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PublishedAt: 1, UpdatedAt: 1, UpdatedBy: root.Id,
	}
	pubOther := model.AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: root.Id, PublisherName: "root", Audience: "all_users",
		Kind: "system", Title: "Notice B", Content: "Later", Status: "published", Revision: 1,
		ReadKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", PublishedAt: 2, UpdatedAt: 2, UpdatedBy: root.Id,
	}
	require.NoError(t, db.Create(&pubVisible).Error)
	require.NoError(t, db.Create(&pubOther).Error)

	require.NoError(t, db.Create(&model.ReferralAffiliate{UserId: customerA.Id, InviteCode: "aff-a", PartnerStatus: model.ReferralPartnerStatusPending, Status: model.ReferralAffiliateStatusApproved}).Error)
	require.NoError(t, db.Create(&model.ReferralAffiliate{UserId: mainUser.Id, InviteCode: "aff-main", PartnerStatus: model.ReferralPartnerStatusPending, Status: model.ReferralAffiliateStatusApproved}).Error)
	require.NoError(t, db.Create(&model.ReferralWithdrawal{UserId: customerA.Id, Amount: 1, NetAmount: 1, Status: model.ReferralWithdrawalStatusPending, IdempotencyKey: strPtr("rw-a")}).Error)
	require.NoError(t, db.Create(&model.ReferralWithdrawal{UserId: mainUser.Id, Amount: 1, NetAmount: 1, Status: model.ReferralWithdrawalStatusApproved, IdempotencyKey: strPtr("rw-main")}).Error)
	require.NoError(t, db.Create(&model.ReferralCommission{AffiliateUserId: customerA.Id, InviteeUserId: customerB.Id, SourceTradeNo: "c-a", Status: model.ReferralCommissionStatusFrozen, CommissionAmount: 1}).Error)
	require.NoError(t, db.Create(&model.ReferralCommission{AffiliateUserId: mainUser.Id, InviteeUserId: admin.Id, SourceTradeNo: "c-main", Status: model.ReferralCommissionStatusFrozen, CommissionAmount: 1}).Error)

	require.NoError(t, db.Create(&model.AdtySiteWithdrawal{SiteId: siteA.Id, OwnerUserId: ownerA.Id, AmountUSD: decimal.NewFromInt(3), FeeUSD: decimal.Zero, NetUSD: decimal.NewFromInt(3), Status: model.AdtySiteWithdrawPending, IdempotencyKey: "sw-a"}).Error)
	require.NoError(t, db.Create(&model.AdtySiteWithdrawal{SiteId: siteB.Id, OwnerUserId: ownerB.Id, AmountUSD: decimal.NewFromInt(1), FeeUSD: decimal.Zero, NetUSD: decimal.NewFromInt(1), Status: model.AdtySiteWithdrawPaid, IdempotencyKey: "sw-b-paid"}).Error)
	require.NoError(t, db.Create(&model.AdtySiteInviteWithdrawal{SiteId: siteA.Id, UserId: customerA.Id, AmountUSD: decimal.NewFromInt(2), NetUSD: decimal.NewFromInt(2), Status: model.AdtySiteInviteWithdrawPending, IdempotencyKey: "iw-a"}).Error)
	require.NoError(t, db.Create(&model.AdtySiteInviteWithdrawal{SiteId: siteB.Id, UserId: customerB.Id, AmountUSD: decimal.NewFromInt(2), NetUSD: decimal.NewFromInt(2), Status: model.AdtySiteInviteWithdrawPending, IdempotencyKey: "iw-b"}).Error)
	require.NoError(t, db.Create(&model.AdtySiteInviteCommission{SiteId: siteA.Id, InviterId: customerA.Id, InviteeId: member.Id, TradeNo: "cov-a", AmountUSD: decimal.NewFromInt(1), Status: model.AdtySiteCommissionPendingCoverage}).Error)
	require.NoError(t, db.Create(&model.AdtySiteInviteCommission{SiteId: siteB.Id, InviterId: customerB.Id, InviteeId: ownerB.Id, TradeNo: "cov-b", AmountUSD: decimal.NewFromInt(1), Status: model.AdtySiteCommissionCovered}).Error)

	require.NoError(t, db.Create(&model.AdtyAnomalyDisposition{Kind: "balance_ledger_gap", Ref: "gap-a", Status: "open", SiteId: siteA.Id}).Error)
	require.NoError(t, db.Create(&model.AdtyAnomalyDisposition{Kind: "margin_anomaly", Ref: "gap-b", Status: "reviewing", SiteId: siteB.Id}).Error)
	require.NoError(t, db.Create(&model.AdtyAnomalyDisposition{Kind: "duplicate_credit", Ref: "gap-closed", Status: "closed", SiteId: siteA.Id}).Error)

	reqA := model.AdtyStaffBalanceRequest{SiteId: siteA.Id, StaffUserId: staffA.Id, CustomerUserId: customerA.Id, Direction: model.AdtyStaffBalanceIncrease, AmountUSD: decimal.NewFromInt(1), Status: model.AdtyStaffBalancePending, IdempotencyKey: strPtr("bal-a")}
	reqB := model.AdtyStaffBalanceRequest{SiteId: siteB.Id, StaffUserId: staffB.Id, CustomerUserId: customerB.Id, Direction: model.AdtyStaffBalanceIncrease, AmountUSD: decimal.NewFromInt(1), Status: model.AdtyStaffBalancePending, IdempotencyKey: strPtr("bal-b")}
	require.NoError(t, db.Create(&reqA).Error)
	require.NoError(t, db.Create(&reqB).Error)

	ticketPending, err := CreateAdtyTicket(customerA.Id, AdtyTicketInput{Type: "billing", Title: "Invoice A", Body: "please check", Idempotency: "t-pending"})
	require.NoError(t, err)
	require.NoError(t, SetAdtyTicketStatus(staffA.Id, ticketPending.Id, model.AdtyTicketPendingAssignment, "", 0))
	ticketWaitSelf, err := CreateAdtyTicket(member.Id, AdtyTicketInput{Type: "api", Title: "Member wait", Body: "need reply", Idempotency: "t-member"})
	require.NoError(t, err)
	require.NoError(t, db.Model(ticketWaitSelf).Updates(map[string]any{"team_id": team.Id, "status": model.AdtyTicketWaitingUser}).Error)
	ticketWaitLeader, err := CreateAdtyTicket(leader.Id, AdtyTicketInput{Type: "account", Title: "Leader wait", Body: "need reply", Idempotency: "t-leader"})
	require.NoError(t, err)
	require.NoError(t, db.Model(ticketWaitLeader).Updates(map[string]any{"team_id": team.Id, "status": model.AdtyTicketWaitingUser}).Error)
	ticketOtherSite, err := CreateAdtyTicket(customerB.Id, AdtyTicketInput{Type: "billing", Title: "Invoice B", Body: "other site", Idempotency: "t-b"})
	require.NoError(t, err)
	require.NoError(t, SetAdtyTicketStatus(staffB.Id, ticketOtherSite.Id, model.AdtyTicketPendingAssignment, "", 0))

	require.NoError(t, db.Create(&model.AdtyTopupOrder{UserId: mainUser.Id, TradeNo: "main-orphan-order", SiteId: 0, Provider: "epay"}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{UserId: customerA.Id, TradeNo: "site-a-orphan-order", SiteId: siteA.Id, Provider: "epay"}).Error)
	require.NoError(t, db.Create(&model.PaymentOrphanEvent{Provider: "epay", EventID: "orphan-main", EventType: "callback", ReferenceID: "main-orphan-order", Status: model.PaymentOrphanStatusPendingReview}).Error)
	require.NoError(t, db.Create(&model.PaymentOrphanEvent{Provider: "epay", EventID: "orphan-site-a", EventType: "callback", ReferenceID: "site-a-orphan-order", Status: model.PaymentOrphanStatusPendingReview}).Error)
	require.NoError(t, db.Create(&model.PaymentOrphanEvent{Provider: "epay", EventID: "orphan-unmatched", EventType: "callback", ReferenceID: "no-such-order", Status: model.PaymentOrphanStatusPendingReview}).Error)
	require.NoError(t, db.Create(&model.PaymentOrphanEvent{Provider: "epay", EventID: "orphan-done", EventType: "callback", ReferenceID: "done", Status: model.PaymentOrphanStatusDismissed}).Error)

	assertConfirmedOnly := func(t *testing.T, counts map[string]int64) {
		t.Helper()
		allowed := map[string]struct{}{}
		for _, kind := range AdtySidebarBadgeConfirmedKinds() {
			allowed[kind] = struct{}{}
		}
		for kind := range counts {
			_, ok := allowed[kind]
			assert.Truef(t, ok, "invented or unconfirmed kind %s", kind)
		}
		for _, kind := range AdtySidebarBadgeUnconfirmedKinds() {
			_, present := counts[kind]
			assert.Falsef(t, present, "unconfirmed kind %s was returned", kind)
		}
		_, hasTopup := counts["topup_orders_pending"]
		assert.False(t, hasTopup)
	}

	rootCounts, err := CollectAdtySidebarBadges(root.Id, nil)
	require.NoError(t, err)
	assertConfirmedOnly(t, rootCounts)
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeNotificationUnread])
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeReferralPartners])
	assert.EqualValues(t, 1, rootCounts[AdtyBadgeReferralWithdrawalsPending])
	assert.EqualValues(t, 1, rootCounts[AdtyBadgeReferralWithdrawalsApproved])
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeReferralCommissionsFrozen])
	assert.EqualValues(t, 1, rootCounts[AdtyBadgeSiteWithdrawalsPending])
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeFundAnomaliesOpen])
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeStaffBalancePending])
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeTicketsPending])
	assert.EqualValues(t, 2, rootCounts[AdtyBadgeTicketsWaitingUser])
	assert.EqualValues(t, 3, rootCounts[AdtyBadgePaymentOrphansPendingReview])
	_, ownerKind := rootCounts[AdtyBadgeInviteWithdrawalsPending]
	assert.False(t, ownerKind, "Root is not the official invite-withdrawal reviewer")
	_, coverKind := rootCounts[AdtyBadgeSiteInviteCommissionsPendingCoverage]
	assert.False(t, coverKind, "Root is not the official cover actor")

	adminCounts, err := CollectAdtySidebarBadges(admin.Id, nil)
	require.NoError(t, err)
	assertConfirmedOnly(t, adminCounts)
	assert.EqualValues(t, 1, adminCounts[AdtyBadgeReferralPartners], "non-root admin is limited to main-site users")
	assert.EqualValues(t, 0, adminCounts[AdtyBadgeReferralWithdrawalsPending])
	assert.EqualValues(t, 1, adminCounts[AdtyBadgeReferralWithdrawalsApproved])
	assert.EqualValues(t, 1, adminCounts[AdtyBadgeReferralCommissionsFrozen])
	assert.EqualValues(t, 1, adminCounts[AdtyBadgePaymentOrphansPendingReview], "non-root admin sees main-site orphans only, not unmatched or other sites")
	_, rootOnly := adminCounts[AdtyBadgeSiteWithdrawalsPending]
	assert.False(t, rootOnly)
	_, anomalies := adminCounts[AdtyBadgeFundAnomaliesOpen]
	assert.False(t, anomalies)

	ownerACounts, err := CollectAdtySidebarBadges(ownerA.Id, nil)
	require.NoError(t, err)
	assertConfirmedOnly(t, ownerACounts)
	_, hasReferral := ownerACounts[AdtyBadgeReferralPartners]
	assert.False(t, hasReferral, "site owner must not receive platform referral admin counts")
	_, hasOrphan := ownerACounts[AdtyBadgePaymentOrphansPendingReview]
	assert.False(t, hasOrphan, "site owner must not learn payment-orphan counts")
	_, hasAnomaly := ownerACounts[AdtyBadgeFundAnomaliesOpen]
	assert.False(t, hasAnomaly)
	_, hasSiteWithdraw := ownerACounts[AdtyBadgeSiteWithdrawalsPending]
	assert.False(t, hasSiteWithdraw)
	assert.EqualValues(t, 1, ownerACounts[AdtyBadgeInviteWithdrawalsPending])
	assert.EqualValues(t, 1, ownerACounts[AdtyBadgeSiteInviteCommissionsPendingCoverage])
	assert.EqualValues(t, 1, ownerACounts[AdtyBadgeStaffBalancePending])
	assert.EqualValues(t, 1, ownerACounts[AdtyBadgeTicketsPending], "owner A does not see site B tickets")
	assert.EqualValues(t, 2, ownerACounts[AdtyBadgeTicketsWaitingUser])

	ownerBCounts, err := CollectAdtySidebarBadges(ownerB.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, ownerBCounts[AdtyBadgeInviteWithdrawalsPending])
	assert.EqualValues(t, 0, ownerBCounts[AdtyBadgeSiteInviteCommissionsPendingCoverage])
	assert.EqualValues(t, 1, ownerBCounts[AdtyBadgeStaffBalancePending])
	assert.EqualValues(t, 1, ownerBCounts[AdtyBadgeTicketsPending])
	assert.EqualValues(t, 0, ownerBCounts[AdtyBadgeTicketsWaitingUser])

	staffACounts, err := CollectAdtySidebarBadges(staffA.Id, nil)
	require.NoError(t, err)
	assertConfirmedOnly(t, staffACounts)
	_, hasFinance := staffACounts[AdtyBadgePaymentOrphansPendingReview]
	assert.False(t, hasFinance)
	_, hasInvite := staffACounts[AdtyBadgeInviteWithdrawalsPending]
	assert.False(t, hasInvite)
	assert.EqualValues(t, 1, staffACounts[AdtyBadgeStaffBalancePending], "staff sees only their own pending request")
	assert.EqualValues(t, 1, staffACounts[AdtyBadgeTicketsPending])
	assert.EqualValues(t, 2, staffACounts[AdtyBadgeTicketsWaitingUser])

	staffBCounts, err := CollectAdtySidebarBadges(staffB.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, staffBCounts[AdtyBadgeStaffBalancePending])
	assert.EqualValues(t, 1, staffBCounts[AdtyBadgeTicketsPending])
	assert.EqualValues(t, 0, staffBCounts[AdtyBadgeTicketsWaitingUser], "staff B must not see site A tickets")

	memberCounts, err := CollectAdtySidebarBadges(member.Id, nil)
	require.NoError(t, err)
	assertConfirmedOnly(t, memberCounts)
	_, memberPending := memberCounts[AdtyBadgeTicketsPending]
	assert.False(t, memberPending, "team members are not ticket handlers")
	assert.EqualValues(t, 1, memberCounts[AdtyBadgeTicketsWaitingUser], "member sees only their own waiting ticket")
	_, memberStaff := memberCounts[AdtyBadgeStaffBalancePending]
	assert.False(t, memberStaff)

	leaderCounts, err := CollectAdtySidebarBadges(leader.Id, nil)
	require.NoError(t, err)
	_, leaderPending := leaderCounts[AdtyBadgeTicketsPending]
	assert.False(t, leaderPending)
	assert.EqualValues(t, 2, leaderCounts[AdtyBadgeTicketsWaitingUser], "team leader scope includes team_id")

	customerACounts, err := CollectAdtySidebarBadges(customerA.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 2, customerACounts[AdtyBadgeNotificationUnread])
	_, customerPending := customerACounts[AdtyBadgeTicketsPending]
	assert.False(t, customerPending)
	assert.EqualValues(t, 0, customerACounts[AdtyBadgeTicketsWaitingUser])
	_, customerOrphan := customerACounts[AdtyBadgePaymentOrphansPendingReview]
	assert.False(t, customerOrphan)

	// Marking publications read clears only the notification badge.
	require.NoError(t, MarkAdtyNotificationInboxRead(customerA.Id, nil, []string{pubVisible.ReadKey, pubOther.ReadKey}))
	afterRead, err := CollectAdtySidebarBadges(customerA.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 0, afterRead[AdtyBadgeNotificationUnread])
	rootAfterRead, err := CollectAdtySidebarBadges(root.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 2, rootAfterRead[AdtyBadgeNotificationUnread], "another user's mark-read does not clear Root's unread")
	assert.EqualValues(t, 3, rootAfterRead[AdtyBadgePaymentOrphansPendingReview], "mark-read must not clear finance badges")
	assert.EqualValues(t, 2, rootAfterRead[AdtyBadgeTicketsPending])

	// Opening / listing badges is a read and must not clear todos.
	again, err := CollectAdtySidebarBadges(root.Id, nil)
	require.NoError(t, err)
	assert.Equal(t, rootAfterRead[AdtyBadgeTicketsPending], again[AdtyBadgeTicketsPending])
	assert.Equal(t, rootAfterRead[AdtyBadgeStaffBalancePending], again[AdtyBadgeStaffBalancePending])
	assert.Equal(t, rootAfterRead[AdtyBadgeNotificationUnread], again[AdtyBadgeNotificationUnread])

	// A terminal staff-balance review clears only that badge.
	require.NoError(t, db.Model(&reqA).Update("status", model.AdtyStaffBalanceApproved).Error)
	rootAfterApprove, err := CollectAdtySidebarBadges(root.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, rootAfterApprove[AdtyBadgeStaffBalancePending])
	assert.EqualValues(t, 2, rootAfterApprove[AdtyBadgeTicketsPending])
	assert.EqualValues(t, 3, rootAfterApprove[AdtyBadgePaymentOrphansPendingReview])
	ownerAAfter, err := CollectAdtySidebarBadges(ownerA.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 0, ownerAAfter[AdtyBadgeStaffBalancePending])
	assert.EqualValues(t, 1, ownerAAfter[AdtyBadgeInviteWithdrawalsPending])

	// Official pending-assignment -> in_progress leaves the 待处理 queue.
	require.NoError(t, SetAdtyTicketStatus(staffA.Id, ticketPending.Id, "", "", staffA.Id))
	rootAfterTicket, err := CollectAdtySidebarBadges(root.Id, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, rootAfterTicket[AdtyBadgeTicketsPending], "only the remaining site B pending ticket")
	assert.EqualValues(t, 1, rootAfterTicket[AdtyBadgeStaffBalancePending], "ticket status must not clear staff-balance badges")

	referral, err := CollectAdtyReferralAdminBadges(root.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 2, referral.Partners)
	adminReferral, err := CollectAdtyReferralAdminBadges(admin.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, adminReferral.Partners)
	_, err = CollectAdtyReferralAdminBadges(ownerA.Id)
	require.ErrorIs(t, err, ErrAdtyTicketForbidden)

	_, err = CollectAdtySidebarBadges(0, nil)
	require.Error(t, err)
}

func TestCollectAdtySidebarBadgesFailsClosedOnQueryError(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := badgeTestUser(t, db, "badge-fail", common.RoleRootUser, 0)
	_, err := CollectAdtySidebarBadges(user.Id, nil)
	require.Error(t, err, "missing badge source tables must fail, not return a successful zero map")
}

func TestCollectAdtyReferralAdminBadgesFailsClosedOnQueryError(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	root := badgeTestUser(t, db, "badge-ref-fail", common.RoleRootUser, 0)
	_, err := CollectAdtyReferralAdminBadges(root.Id)
	require.Error(t, err)
}

func badgeTestUser(t *testing.T, db *gorm.DB, name string, role, siteID int) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com", SiteId: siteID}
	require.NoError(t, db.Create(user).Error)
	return user
}

func strPtr(v string) *string { return &v }
