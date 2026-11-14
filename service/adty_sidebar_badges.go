package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"gorm.io/gorm"
)

// Confirmed sidebar badge kinds registered in docs/adty/sidebar-badge-inventory.md
// part A. Unconfirmed kinds in part B must not appear in the API.
const (
	AdtyBadgeNotificationUnread                   = "notification_unread"
	AdtyBadgeReferralPartners                     = "referral_partners"
	AdtyBadgeReferralWithdrawalsPending           = "referral_withdrawals_pending"
	AdtyBadgeReferralWithdrawalsApproved          = "referral_withdrawals_approved"
	AdtyBadgeSiteWithdrawalsPending               = "site_withdrawals_pending"
	AdtyBadgeInviteWithdrawalsPending             = "invite_withdrawals_pending"
	AdtyBadgeSiteInviteCommissionsPendingCoverage = "site_invite_commissions_pending_coverage"
	AdtyBadgeReferralCommissionsFrozen            = "referral_commissions_frozen"
	AdtyBadgeFundAnomaliesOpen                    = "fund_anomalies_open"
	AdtyBadgeStaffBalancePending                  = "staff_balance_pending"
	AdtyBadgeTicketsPending                       = "tickets_pending"
	AdtyBadgeTicketsWaitingUser                   = "tickets_waiting_user"
	AdtyBadgePaymentOrphansPendingReview          = "payment_orphans_pending_review"
)

// AdtySidebarBadgeConfirmedKinds is the closed set of kinds the badge API may
// return. Tests use it to reject invented or still-unconfirmed counts.
func AdtySidebarBadgeConfirmedKinds() []string {
	return []string{
		AdtyBadgeNotificationUnread,
		AdtyBadgeReferralPartners,
		AdtyBadgeReferralWithdrawalsPending,
		AdtyBadgeReferralWithdrawalsApproved,
		AdtyBadgeSiteWithdrawalsPending,
		AdtyBadgeInviteWithdrawalsPending,
		AdtyBadgeSiteInviteCommissionsPendingCoverage,
		AdtyBadgeReferralCommissionsFrozen,
		AdtyBadgeFundAnomaliesOpen,
		AdtyBadgeStaffBalancePending,
		AdtyBadgeTicketsPending,
		AdtyBadgeTicketsWaitingUser,
		AdtyBadgePaymentOrphansPendingReview,
	}
}

// AdtySidebarBadgeUnconfirmedKinds are registered in the inventory part B and
// must not be computed or returned.
func AdtySidebarBadgeUnconfirmedKinds() []string {
	return []string{
		"topup_orders_pending",
		"payment_exceptions_other",
		"team_notification_unread",
		"site_applications_pending",
		"site_invite_commissions_frozen",
		"site_withdrawals_paying",
		"manual_refunds_submitted",
	}
}

type AdtyReferralAdminBadges struct {
	Partners            int64
	PendingWithdrawals  int64
	ApprovedWithdrawals int64
	FrozenCommissions   int64
}

// CollectAdtySidebarBadges returns confirmed badge counts the caller is
// allowed to see. Unauthorized kinds are omitted (not zero) so a caller
// cannot learn that work exists outside their scope (EMP-017). Any query
// failure is returned to the caller; the map must not be treated as a
// successful empty result.
func CollectAdtySidebarBadges(userID int, requestSite *model.AdtySite) (map[string]int64, error) {
	if userID <= 0 {
		return nil, common.NewMessage("Invalid parameters")
	}
	actor, err := loadAdtyTicketActor(userID)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	unread, err := countAdtyNotificationUnread(userID, requestSite)
	if err != nil {
		return nil, err
	}
	counts[AdtyBadgeNotificationUnread] = unread

	if err := collectAdtyReferralAdminBadgeCounts(counts, actor); err != nil {
		return nil, err
	}
	if actor.User.Role >= common.RoleRootUser {
		pending, err := countScoped(model.DB.Model(&model.AdtySiteWithdrawal{}).Where("status = ?", model.AdtySiteWithdrawPending))
		if err != nil {
			return nil, err
		}
		counts[AdtyBadgeSiteWithdrawalsPending] = pending
		anomalies, err := countScoped(model.DB.Model(&model.AdtyAnomalyDisposition{}).Where("status IN ?", []string{"open", "reviewing"}))
		if err != nil {
			return nil, err
		}
		counts[AdtyBadgeFundAnomaliesOpen] = anomalies
	}
	if actor.OwnerSiteId > 0 {
		invites, err := countScoped(model.DB.Model(&model.AdtySiteInviteWithdrawal{}).Where("site_id = ? AND status = ?", actor.OwnerSiteId, model.AdtySiteInviteWithdrawPending))
		if err != nil {
			return nil, err
		}
		counts[AdtyBadgeInviteWithdrawalsPending] = invites
		coverage, err := countScoped(model.DB.Model(&model.AdtySiteInviteCommission{}).Where("site_id = ? AND status = ?", actor.OwnerSiteId, model.AdtySiteCommissionPendingCoverage))
		if err != nil {
			return nil, err
		}
		counts[AdtyBadgeSiteInviteCommissionsPendingCoverage] = coverage
	}
	if err := collectAdtyStaffBalanceBadge(counts, actor); err != nil {
		return nil, err
	}
	if err := collectAdtyTicketBadgesInto(counts, actor); err != nil {
		return nil, err
	}
	if err := collectAdtyPaymentOrphanBadge(counts, actor); err != nil {
		return nil, err
	}
	return counts, nil
}

// CollectAdtyReferralAdminBadges is the scoped replacement for the previous
// unscoped GetReferralAdminBadges sums. Root sees every site; a non-root
// admin is limited to main-site users. Callers without referral-admin
// standing receive an authorization error rather than a global or leaked
// count.
func CollectAdtyReferralAdminBadges(userID int) (AdtyReferralAdminBadges, error) {
	var out AdtyReferralAdminBadges
	if userID <= 0 {
		return out, common.NewMessage("Invalid parameters")
	}
	actor, err := loadAdtyTicketActor(userID)
	if err != nil {
		return out, err
	}
	siteID, ok := referralAdminSiteScope(actor)
	if !ok {
		return out, ErrAdtyTicketForbidden
	}
	return countAdtyReferralAdminBadges(siteID)
}

func collectAdtyReferralAdminBadgeCounts(counts map[string]int64, actor adtyTicketActor) error {
	siteID, ok := referralAdminSiteScope(actor)
	if !ok {
		return nil
	}
	if AdtyAdminPermissionsEnabled() && actor.User.Role < common.RoleRootUser && !AdtyAdminRoleCan(actor.User.Id, actor.User.Role, authz.AdtyReferralRead) {
		return nil
	}
	badges, err := countAdtyReferralAdminBadges(siteID)
	if err != nil {
		return err
	}
	counts[AdtyBadgeReferralPartners] = badges.Partners
	counts[AdtyBadgeReferralWithdrawalsPending] = badges.PendingWithdrawals
	counts[AdtyBadgeReferralWithdrawalsApproved] = badges.ApprovedWithdrawals
	counts[AdtyBadgeReferralCommissionsFrozen] = badges.FrozenCommissions
	return nil
}

func referralAdminSiteScope(actor adtyTicketActor) (*int, bool) {
	if actor.User.Role >= common.RoleRootUser {
		return nil, true
	}
	if actor.User.Role >= common.RoleAdminUser {
		mainSite := 0
		return &mainSite, true
	}
	return nil, false
}

func countAdtyReferralAdminBadges(siteID *int) (AdtyReferralAdminBadges, error) {
	var out AdtyReferralAdminBadges
	var err error
	out.Partners, err = countScoped(scopeReferralUserQuery(model.DB.Model(&model.ReferralAffiliate{}).Where("partner_status = ?", model.ReferralPartnerStatusPending), "user_id", siteID))
	if err != nil {
		return out, err
	}
	out.PendingWithdrawals, err = countScoped(scopeReferralUserQuery(model.DB.Model(&model.ReferralWithdrawal{}).Where("status = ?", model.ReferralWithdrawalStatusPending), "user_id", siteID))
	if err != nil {
		return out, err
	}
	out.ApprovedWithdrawals, err = countScoped(scopeReferralUserQuery(model.DB.Model(&model.ReferralWithdrawal{}).Where("status = ?", model.ReferralWithdrawalStatusApproved), "user_id", siteID))
	if err != nil {
		return out, err
	}
	out.FrozenCommissions, err = countScoped(scopeReferralUserQuery(model.DB.Model(&model.ReferralCommission{}).Where("status = ?", model.ReferralCommissionStatusFrozen), "affiliate_user_id", siteID))
	return out, err
}

func scopeReferralUserQuery(query *gorm.DB, userColumn string, siteID *int) *gorm.DB {
	if siteID == nil {
		return query
	}
	return query.Where(userColumn+" IN (?)", model.DB.Model(&model.User{}).Select("id").Where("site_id = ?", *siteID))
}

func collectAdtyStaffBalanceBadge(counts map[string]int64, actor adtyTicketActor) error {
	query := model.DB.Model(&model.AdtyStaffBalanceRequest{}).Where("status = ?", model.AdtyStaffBalancePending)
	switch {
	case actor.User.Role >= common.RoleRootUser:
	case actor.OwnerSiteId > 0:
		query = query.Where("site_id = ?", actor.OwnerSiteId)
	case actor.Staff != nil && actor.Staff.Status == "enabled":
		query = query.Where("site_id = ? AND staff_user_id = ?", actor.Staff.SiteId, actor.User.Id)
	default:
		return nil
	}
	n, err := countScoped(query)
	if err != nil {
		return err
	}
	counts[AdtyBadgeStaffBalancePending] = n
	return nil
}

func collectAdtyTicketBadgesInto(counts map[string]int64, actor adtyTicketActor) error {
	badges, err := countAdtyTicketBadges(scopedAdtyTickets(actor).Session(&gorm.Session{}))
	if err != nil {
		return err
	}
	if adtyTicketActorCanManage(actor) {
		counts[AdtyBadgeTicketsPending] = badges.Pending
	}
	counts[AdtyBadgeTicketsWaitingUser] = badges.WaitingUser
	return nil
}

func adtyTicketActorCanManage(actor adtyTicketActor) bool {
	if actor.User.Role >= common.RoleAdminUser {
		return true
	}
	if actor.OwnerSiteId > 0 {
		return true
	}
	return actor.Staff != nil && actor.Staff.Status == "enabled"
}

func collectAdtyPaymentOrphanBadge(counts map[string]int64, actor adtyTicketActor) error {
	if actor.User.Role < common.RoleAdminUser {
		return nil
	}
	if AdtyAdminPermissionsEnabled() && actor.User.Role < common.RoleRootUser && !AdtyAdminRoleCan(actor.User.Id, actor.User.Role, authz.AdtyFinanceRead) {
		return nil
	}
	query := model.DB.Model(&model.PaymentOrphanEvent{}).Where("status = ?", model.PaymentOrphanStatusPendingReview)
	if actor.User.Role < common.RoleRootUser {
		query = query.Where("EXISTS (SELECT 1 FROM adty_topup_orders a WHERE a.trade_no = adty_payment_orphan_events.reference_id AND a.site_id = 0)")
	}
	n, err := countScoped(query)
	if err != nil {
		return err
	}
	counts[AdtyBadgePaymentOrphansPendingReview] = n
	return nil
}

func countAdtyNotificationUnread(userID int, site *model.AdtySite) (int64, error) {
	inbox, err := GetAdtyNotificationInbox(userID, site, 0)
	if err != nil {
		return 0, err
	}
	return inbox.UnreadPublications, nil
}

func countScoped(query *gorm.DB) (int64, error) {
	var n int64
	if err := query.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
