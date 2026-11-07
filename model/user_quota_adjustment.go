package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

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
	var cacheApplied bool
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
		if err := rejectIfPendingBatchLeavesNegative(userID, after); err != nil {
			return err
		}
		delta := int64(after) - int64(user.Quota)
		applied, err := applyManualAdjustCache(userID, delta)
		cacheApplied = applied
		if err != nil {
			return err
		}
		fail := func(cause error) error {
			if applied {
				compensateManualAdjustCache(userID, delta)
			}
			return cause
		}
		// An unchanged override is a successful operation, including on MySQL
		// configurations that count only changed rows in RowsAffected.
		if after != user.Quota {
			result := tx.Model(&User{}).Where("id = ?", userID).Update("quota", after)
			if result.Error != nil {
				return fail(result.Error)
			}
			if result.RowsAffected != 1 {
				return fail(gorm.ErrRecordNotFound)
			}
		}
		adjustment = UserQuotaAdjustment{UserID: user.Id, Username: user.Username, Before: user.Quota, After: after}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if !cacheApplied {
		if delta := int64(adjustment.After) - int64(adjustment.Before); delta != 0 {
			if err := cacheIncrUserQuota(userID, delta); err != nil {
				common.SysError(common.LogText("failed to sync manual quota adjustment for user %d: %s", userID, err))
			}
		}
	}
	return &adjustment, nil
}

// applyManualAdjustCache moves the live Redis balance by the same delta as the
// database row. A cache miss is left for the next read to hydrate. A delta
// that would make the live balance negative is rejected before any write.
func applyManualAdjustCache(userID int, delta int64) (bool, error) {
	result, err := cacheApplyManualQuotaDelta(userID, delta)
	if err != nil {
		// A dead cache must not roll back a committed wallet change. The
		// caller still tries the post-commit sync and logs that failure.
		common.SysError(common.LogText("manual quota cache check failed for user %d: %s", userID, err))
		return false, nil
	}
	if result == cacheQuotaInsufficient {
		return false, ErrAdtyQuotaFloor
	}
	return result == cacheQuotaOK, nil
}

// compensateManualQuotaDelta restores a cache write when the database transaction
// rolls back. Tests replace it to force a restore failure.
var compensateManualQuotaDelta = cacheApplyUserQuotaDelta

func compensateManualAdjustCache(userID int, delta int64) {
	if _, err := compensateManualQuotaDelta(userID, -delta); err != nil {
		common.SysError(common.LogText("failed to compensate manual quota cache for user %d: %s", userID, err))
		recordAdjustCacheDivergence(userID, delta, err)
	}
}

func recordAdjustCacheDivergence(userID int, delta int64, cause error) {
	if DB == nil || userID <= 0 {
		return
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	event := &PaymentOrphanEvent{
		Provider:    "wallet",
		EventID:     fmt.Sprintf("adjust-cache-%d-%d", userID, time.Now().UnixNano()),
		EventType:   "adjust_cache_compensate",
		ReferenceID: strconv.Itoa(userID),
		Status:      PaymentOrphanStatusPendingReview,
		Reason:      "manual adjust cache compensate failed",
		Error:       message,
		Payload:     fmt.Sprintf(`{"user_id":%d,"delta":%d}`, userID, delta),
		CreateTime:  common.GetTimestamp(),
		UpdateTime:  common.GetTimestamp(),
	}
	if err := DB.Create(event).Error; err != nil {
		common.SysError(common.LogText("failed to record adjust cache divergence for user %d: %s", userID, err.Error()))
	}
}

// rejectIfPendingBatchLeavesNegative keeps a queued consumption from driving
// the wallet negative after this adjustment is flushed. The database row lags
// that queue, so the row alone is not the live balance.
func rejectIfPendingBatchLeavesNegative(userID, after int) error {
	if !common.BatchUpdateEnabled {
		return nil
	}
	if saturatingAddInt(after, userQuotaBatchDelta(userID)) < 0 {
		return ErrAdtyQuotaFloor
	}
	return nil
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
