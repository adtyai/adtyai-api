package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrInvalidUserQuotaAdjustment = errors.New("invalid user quota adjustment")
	ErrUserQuotaPermission        = errors.New("cannot adjust quota for this user role")
	// ErrAdtyQuotaFloor rejects an admin subtract that would go below 0 and an
	// override that would write a negative balance.
	ErrAdtyQuotaFloor = common.NewMessage("Adjustment cannot make the balance negative")
)

// UserQuotaAdjustment is the immutable database snapshot of a committed manual
// adjustment. Pending relay deductions in the quota cache are not part of it.
type UserQuotaAdjustment struct {
	UserID   int
	Username string
	Before   int
	After    int
}

func AdjustUserQuota(userID, operatorRole int, mode string, value int) (*UserQuotaAdjustment, error) {
	if userID <= 0 || (mode != "add" && mode != "subtract" && mode != "override") {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if mode != "override" && value <= 0 {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if value > common.MaxWalletQuota || value < -common.MaxWalletQuota {
		return nil, ErrWalletQuotaLimitExceeded
	}

	var adjustment UserQuotaAdjustment
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if operatorRole != common.RoleRootUser && operatorRole <= user.Role {
			return ErrUserQuotaPermission
		}
		if user.Quota > common.MaxWalletQuota || user.Quota < -common.MaxWalletQuota {
			return ErrWalletQuotaLimitExceeded
		}
		after, err := quotaAfterManualAdjust(user.Quota, value, mode)
		if err != nil {
			return err
		}
		// An unchanged override is a successful operation, including on MySQL
		// configurations that count only changed rows in RowsAffected.
		if after != user.Quota {
			result := tx.Model(&User{}).Where("id = ?", userID).Update("quota", after)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		adjustment = UserQuotaAdjustment{UserID: user.Id, Username: user.Username, Before: user.Quota, After: after}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Apply only the committed difference, preserving outstanding reservations.
	// Both balances are bounded above, so their difference fits in int64.
	delta := int64(adjustment.After) - int64(adjustment.Before)
	if delta != 0 {
		if err := cacheIncrUserQuota(userID, delta); err != nil {
			common.SysError(common.LogText("failed to sync manual quota adjustment for user %d: %s", userID, err))
		}
	}
	return &adjustment, nil
}

// quotaAfterManualAdjust applies the confirmed admin floor: subtract stops at 0,
// and override cannot write a negative balance. Add is unchanged.
func quotaAfterManualAdjust(current, value int, mode string) (int, error) {
	if mode == "override" && value < 0 {
		return 0, ErrAdtyQuotaFloor
	}
	quota := decimal.NewFromInt(int64(value))
	switch mode {
	case "add":
		quota = decimal.NewFromInt(int64(current)).Add(quota)
	case "subtract":
		quota = decimal.NewFromInt(int64(current)).Sub(quota)
	}
	after, err := common.WalletQuotaFromDecimalStrict(quota)
	if err != nil {
		return 0, ErrWalletQuotaLimitExceeded
	}
	if mode == "subtract" && after < 0 {
		return 0, ErrAdtyQuotaFloor
	}
	return after, nil
}
