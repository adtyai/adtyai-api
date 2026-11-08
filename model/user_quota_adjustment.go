package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
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
		after, err := quotaAfterManualAdjust(user.Quota, value, mode)
		if err != nil {
			return err
		}
		liveAfter, err := userWalletBalance(after, user.QuotaPending)
		if err != nil {
			return err
		}
		if mode != "add" && liveAfter < 0 {
			return ErrAdtyQuotaFloor
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
		if err := clampCheckinQuotaDB(tx, userID); err != nil {
			return err
		}
		adjustment = UserQuotaAdjustment{UserID: user.Id, Username: user.Username, Before: user.Quota, After: after}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := invalidateUserCache(userID); err != nil {
		common.SysError(common.LogText("failed to invalidate manual quota projection for user %d: %s", userID, err))
	}
	return &adjustment, nil
}

// quotaAfterManualAdjust preserves raw before/after audit semantics and checks
// storage arithmetic. Callers enforce the floor/ceiling on raw plus pending.
// An override itself must be nonnegative; adds retain their existing behavior.
func quotaAfterManualAdjust(current, value int, mode string) (int, error) {
	if mode == "override" && value < 0 {
		return 0, ErrAdtyQuotaFloor
	}
	switch mode {
	case "add":
		return checkedQuotaSum(current, value)
	case "subtract":
		return checkedQuotaSum(current, -value)
	}
	if _, err := checkedQuotaDifference(value, current); err != nil {
		return 0, err
	}
	return value, nil
}
