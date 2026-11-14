package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Cancellation is refused until the named category is finished or formally
// terminated through an existing official flow. These errors do not settle funds.
var (
	ErrUserDeleteUnfinishedPayment     = errors.New("Finish unfinished payments before deleting this account")
	ErrUserDeleteInFlightBilling       = errors.New("Finish in-flight API billing before deleting this account")
	ErrUserDeleteOpenBillingHold       = errors.New("Resolve open shortfall holds before deleting this account")
	ErrUserDeleteUnfinishedRefund      = errors.New("Finish unfinished refunds before deleting this account")
	ErrUserDeleteUnfinishedWithdraw    = errors.New("Finish unfinished withdrawals before deleting this account")
	ErrUserDeletePendingBalanceReq     = errors.New("Finish pending staff balance-approval requests before deleting this account")
	ErrUserDeleteActiveStaff           = errors.New("Revoke the staff identity before deleting this account")
	ErrUserDeleteUnfinishedTeamFinance = errors.New("Finish unfinished team finance before deleting this account")
	ErrUserDeleteUnfinishedVoucher     = errors.New("Finish unredeemed or frozen vouchers before deleting this account")
	ErrUserDeleteUnsettledSiteEarning  = errors.New("Finish unsettled site earnings before deleting this account")
)

// rejectUserDeletePreconditions blocks soft-delete and hard-delete when the
// account still has an active staff binding or unfinished financial work.
// It does not refund, pay out, forfeit, or rewrite ledgers.
func rejectUserDeletePreconditions(tx *gorm.DB, userId int) error {
	if tx == nil || userId <= 0 {
		return nil
	}
	if err := rejectActiveStaffIdentityForDelete(tx, userId); err != nil {
		return err
	}
	if err := rejectUnfinishedTeamFinanceForDelete(tx, userId); err != nil {
		return err
	}
	if blocked, err := userDeleteHasRows(tx, &TopUp{}, "user_id = ? AND status = ?", userId, common.TopUpStatusPending); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteUnfinishedPayment
	}
	if blocked, err := userDeleteHasRows(tx, &BillingOperation{}, "user_id = ? AND state IN ?", userId, []string{
		BillingReserved, BillingSettlePending, BillingRefundPending,
	}); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteInFlightBilling
	}
	if blocked, err := userDeleteHasRows(tx, &AdtyBillingHold{}, "user_id = ? AND status = ?", userId, AdtyBillingHoldOpen); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteOpenBillingHold
	}
	if blocked, err := userDeleteHasRows(tx, &AdtyManualRefund{},
		"user_id = ? AND (status IN ? OR (payout_status = ? AND status NOT IN ?))",
		userId,
		[]string{AdtyManualRefundStatusSubmitted, AdtyManualRefundStatusApproved},
		AdtyManualRefundPayoutUnknown,
		[]string{AdtyManualRefundStatusCompleted, AdtyManualRefundStatusTerminated},
	); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteUnfinishedRefund
	}
	if blocked, err := userHasUnfinishedWithdrawals(tx, userId); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteUnfinishedWithdraw
	}
	if blocked, err := userDeleteHasRows(tx, &AdtyStaffBalanceRequest{},
		"status = ? AND (staff_user_id = ? OR customer_user_id = ?)",
		AdtyStaffBalancePending, userId, userId,
	); err != nil {
		return err
	} else if blocked {
		return ErrUserDeletePendingBalanceReq
	}
	if err := rejectUnfinishedVouchersForDelete(tx, userId); err != nil {
		return err
	}
	if err := rejectUnsettledSiteEarningsForDelete(tx, userId); err != nil {
		return err
	}
	return rejectAttributablePaymentOrphansForDelete(tx, userId)
}

func rejectActiveStaffIdentityForDelete(tx *gorm.DB, userId int) error {
	var staffSiteId int
	if err := tx.Model(&User{}).Where("id = ?", userId).Select("staff_site_id").Scan(&staffSiteId).Error; err != nil {
		return err
	}
	if !tx.Migrator().HasTable(&AdtySiteStaff{}) {
		if staffSiteId != 0 {
			return ErrUserDeleteActiveStaff
		}
		return nil
	}
	var active int64
	if err := tx.Model(&AdtySiteStaff{}).Where("user_id = ? AND status <> ?", userId, "deleted").Count(&active).Error; err != nil {
		return err
	}
	if active > 0 {
		return ErrUserDeleteActiveStaff
	}
	if staffSiteId == 0 {
		return nil
	}
	// Official staff delete leaves StaffSiteId set and a deleted membership
	// tombstone. That is already revoked. A leftover binding without a
	// tombstone is still treated as an active staff identity.
	var recorded int64
	if err := tx.Model(&AdtySiteStaff{}).Where("user_id = ?", userId).Count(&recorded).Error; err != nil {
		return err
	}
	if recorded == 0 {
		return ErrUserDeleteActiveStaff
	}
	return nil
}

func rejectUnfinishedTeamFinanceForDelete(tx *gorm.DB, userId int) error {
	if !tx.Migrator().HasTable(&AdtyTeam{}) {
		return nil
	}
	var team AdtyTeam
	lookup := tx.Where("leader_user_id = ? AND status = ?", userId, AdtyTeamStatusActive).Limit(1).Find(&team)
	if lookup.Error != nil {
		return lookup.Error
	}
	if lookup.RowsAffected == 0 {
		return nil
	}
	if !tx.Migrator().HasTable(&AdtyTopupOrder{}) || !tx.Migrator().HasTable(&TopUp{}) {
		return nil
	}
	var count int64
	err := tx.Model(&TopUp{}).
		Joins("JOIN adty_topup_orders ON adty_topup_orders.trade_no = top_ups.trade_no").
		Where("adty_topup_orders.team_id = ? AND top_ups.status = ?", team.Id, common.TopUpStatusPending).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrUserDeleteUnfinishedTeamFinance
	}
	return nil
}

func userHasUnfinishedWithdrawals(tx *gorm.DB, userId int) (bool, error) {
	blocked, err := userDeleteHasRows(tx, &AdtySiteWithdrawal{},
		"owner_user_id = ? AND status IN ?", userId, []string{AdtySiteWithdrawPending, AdtySiteWithdrawPaying})
	if err != nil || blocked {
		return blocked, err
	}
	blocked, err = userDeleteHasRows(tx, &AdtySiteInviteWithdrawal{},
		"user_id = ? AND status IN ?", userId, []string{AdtySiteInviteWithdrawPending, AdtySiteInviteWithdrawApproved})
	if err != nil || blocked {
		return blocked, err
	}
	return userDeleteHasRows(tx, &ReferralWithdrawal{},
		"user_id = ? AND status IN ?", userId, []string{ReferralWithdrawalStatusPending, ReferralWithdrawalStatusApproved})
}

// rejectUnfinishedVouchersForDelete refuses cancellation when the user is the
// issuer of a pending or frozen code, or the owner of a site (including a
// closed site) that still has a bound unredeemed owner/margin code. It does
// not cancel or redeem the codes; G.8.15 remains the official cancel path.
func rejectUnfinishedVouchersForDelete(tx *gorm.DB, userId int) error {
	if blocked, err := userDeleteHasRows(tx, &AdtyVoucher{},
		"issuer_id = ? AND status IN ?", userId, []string{AdtyVoucherStatusPending, AdtyVoucherStatusFrozen},
	); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteUnfinishedVoucher
	}
	if !tx.Migrator().HasTable(&AdtySite{}) || !tx.Migrator().HasTable(&AdtyVoucher{}) {
		return nil
	}
	var siteIDs []int
	if err := tx.Model(&AdtySite{}).Where("owner_user_id = ?", userId).Pluck("id", &siteIDs).Error; err != nil {
		return err
	}
	if len(siteIDs) == 0 {
		return nil
	}
	blocked, err := userDeleteHasRows(tx, &AdtyVoucher{},
		"target_site_id IN ? AND purpose IN ? AND status IN ?",
		siteIDs,
		[]string{AdtyVoucherPurposeSiteOwner, AdtyVoucherPurposeSiteMargin},
		[]string{AdtyVoucherStatusPending, AdtyVoucherStatusFrozen},
	)
	if err != nil {
		return err
	}
	if blocked {
		return ErrUserDeleteUnfinishedVoucher
	}
	return nil
}

// rejectUnsettledSiteEarningsForDelete refuses cancellation when daily site
// earnings for this owner are still pending settlement, including leftovers
// after the site has already closed. It does not book, void, or pay out.
func rejectUnsettledSiteEarningsForDelete(tx *gorm.DB, userId int) error {
	blocked, err := userDeleteHasRows(tx, &AdtySiteEarningDaily{},
		"site_owner_user_id = ? AND status = ?", userId, AdtySiteEarningPending,
	)
	if err != nil {
		return err
	}
	if blocked {
		return ErrUserDeleteUnsettledSiteEarning
	}
	return nil
}

// userDeleteUnfinishedTopUpStatuses are the local order states that official
// payment recovery can still credit. A completed order is historical, not pending.
var userDeleteUnfinishedTopUpStatuses = []string{
	common.TopUpStatusPending, common.TopUpStatusFailed, common.TopUpStatusExpired, TopUpStatusCanceled,
}

// rejectAttributablePaymentOrphansForDelete refuses cancellation when a
// pending-review PaymentOrphanEvent can be tied to this account's unfinished
// order through existing association fields only. It does not credit, refund,
// dismiss, or guess a user_id on the event.
func rejectAttributablePaymentOrphansForDelete(tx *gorm.DB, userId int) error {
	if blocked, err := userHasReferenceAttributedPaymentOrphan(tx, userId); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteUnfinishedPayment
	}
	if blocked, err := userHasSessionAttributedPaymentOrphan(tx, userId); err != nil {
		return err
	} else if blocked {
		return ErrUserDeleteUnfinishedPayment
	}
	return nil
}

// userHasReferenceAttributedPaymentOrphan is true when a pending-review
// event's ReferenceID is an existing trade_no of this user's unfinished order.
// ReferenceID is never treated as a user_id, even when it happens to be numeric.
func userHasReferenceAttributedPaymentOrphan(tx *gorm.DB, userId int) (bool, error) {
	if !tx.Migrator().HasTable(&PaymentOrphanEvent{}) || !tx.Migrator().HasTable(&TopUp{}) {
		return false, nil
	}
	var count int64
	err := tx.Model(&PaymentOrphanEvent{}).
		Joins("JOIN top_ups ON top_ups.trade_no = adty_payment_orphan_events.reference_id").
		Where("adty_payment_orphan_events.status = ? AND adty_payment_orphan_events.reference_id <> ? AND top_ups.user_id = ? AND top_ups.status IN ?",
			PaymentOrphanStatusPendingReview, "", userId, userDeleteUnfinishedTopUpStatuses,
		).Limit(1).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// userHasSessionAttributedPaymentOrphan is true only when a pending-review
// event has no ReferenceID, and its SessionID uniquely matches one of this
// user's unfinished orders the same way official reversal recovery does:
// provider plus paid_tx_hash or gateway_trade_id, and exactly one row.
// Zero or multiple matches, a missing provider, or an order owned by someone
// else are not safe attribution.
func userHasSessionAttributedPaymentOrphan(tx *gorm.DB, userId int) (bool, error) {
	if !tx.Migrator().HasTable(&PaymentOrphanEvent{}) || !tx.Migrator().HasTable(&AdtyTopupOrder{}) || !tx.Migrator().HasTable(&TopUp{}) {
		return false, nil
	}
	var orders []AdtyTopupOrder
	if err := tx.Model(&AdtyTopupOrder{}).
		Select("trade_no", "user_id", "provider", "gateway_trade_id", "paid_tx_hash").
		Where("user_id = ? AND (NULLIF(gateway_trade_id, '') IS NOT NULL OR NULLIF(paid_tx_hash, '') IS NOT NULL)", userId).
		Find(&orders).Error; err != nil {
		return false, err
	}
	if len(orders) == 0 {
		return false, nil
	}
	tradeNos := make([]string, 0, len(orders))
	for _, order := range orders {
		tradeNos = append(tradeNos, order.TradeNo)
	}
	var topUps []TopUp
	if err := tx.Model(&TopUp{}).Select("trade_no", "status").
		Where("trade_no IN ? AND user_id = ? AND status IN ?", tradeNos, userId, userDeleteUnfinishedTopUpStatuses).
		Find(&topUps).Error; err != nil {
		return false, err
	}
	unfinished := make(map[string]struct{}, len(topUps))
	for _, topUp := range topUps {
		unfinished[topUp.TradeNo] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, order := range orders {
		if _, ok := unfinished[order.TradeNo]; !ok {
			continue
		}
		provider := strings.TrimSpace(order.Provider)
		if provider == "" {
			continue
		}
		var vouchers []string
		if voucher := strings.TrimSpace(order.GatewayTradeId); voucher != "" {
			vouchers = append(vouchers, voucher)
		}
		if voucher := strings.TrimSpace(order.PaidTxHash); voucher != "" && (len(vouchers) == 0 || voucher != vouchers[0]) {
			vouchers = append(vouchers, voucher)
		}
		for _, voucher := range vouchers {
			key := provider + "\x00" + voucher
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			var matches int64
			if err := tx.Model(&AdtyTopupOrder{}).
				Where("provider = ? AND (paid_tx_hash = ? OR gateway_trade_id = ?)", provider, voucher, voucher).
				Limit(2).Count(&matches).Error; err != nil {
				return false, err
			}
			if matches != 1 {
				continue
			}
			var count int64
			err := tx.Model(&PaymentOrphanEvent{}).
				Where("status = ? AND (reference_id IS NULL OR reference_id = ?) AND session_id = ? AND provider = ?",
					PaymentOrphanStatusPendingReview, "", voucher, provider,
				).Limit(1).Count(&count).Error
			if err != nil {
				return false, err
			}
			if count > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

func userDeleteHasRows(tx *gorm.DB, model any, query string, args ...any) (bool, error) {
	if !tx.Migrator().HasTable(model) {
		return false, nil
	}
	var count int64
	if err := tx.Model(model).Where(query, args...).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func revokeLiveAPIKeysWithTx(tx *gorm.DB, userId int) error {
	if tx == nil || userId <= 0 || !tx.Migrator().HasTable(&Token{}) {
		return nil
	}
	return tx.Model(&Token{}).Where("user_id = ? AND status = ?", userId, common.TokenStatusEnabled).
		Update("status", common.TokenStatusDisabled).Error
}
