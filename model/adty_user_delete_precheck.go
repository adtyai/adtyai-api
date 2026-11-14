package model

import (
	"errors"

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
	return nil
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
