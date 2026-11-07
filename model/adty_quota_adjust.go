package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// 人工调额留痕（任务 7.2）：原因必填，带幂等键，同一个事务里写余额流水（含调额前后余额）。

var (
	ErrAdtyAdjustReasonRequired = errors.New("adjustment reason required")
	ErrAdtyAdjustReasonTooLong  = errors.New("adjustment reason too long")
	ErrAdtyAdjustKeyTooLong     = errors.New("idempotency key too long")
	ErrAdtyAdjustKeyRequired    = errors.New("idempotency key required")
)

const (
	adtyAdjustReasonMaxChars = 200
	adtyAdjustKeyMaxLen      = 64
)

// AdtyQuotaAdjustResult 是调额结果；Replayed 说明这是重复提交，返回的是第一次的结果。
type AdtyQuotaAdjustResult struct {
	UserQuotaAdjustment
	Replayed bool
}

// AdjustUserQuotaWithLedger 和 AdjustUserQuota 的规则一样，另外要求原因、按幂等键去重、写余额流水。
// 幂等判断在锁住用户行之后先查后写，不靠唯一键冲突（PostgreSQL 冲突后整个事务作废）。
func AdjustUserQuotaWithLedger(userID, operatorID, operatorRole int, mode string, value int, reason, idemKey string) (*AdtyQuotaAdjustResult, error) {
	reason = strings.TrimSpace(reason)
	idemKey = strings.TrimSpace(idemKey)
	if reason == "" {
		return nil, ErrAdtyAdjustReasonRequired
	}
	if utf8.RuneCountInString(reason) > adtyAdjustReasonMaxChars {
		return nil, ErrAdtyAdjustReasonTooLong
	}
	if idemKey == "" || len(idemKey) > adtyAdjustKeyMaxLen {
		return nil, ErrAdtyAdjustKeyRequired
	}
	if userID <= 0 || (mode != "add" && mode != "subtract" && mode != "override") {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if mode != "override" && value <= 0 {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if value > common.MaxWalletQuota || value < -common.MaxWalletQuota {
		return nil, ErrWalletQuotaLimitExceeded
	}
	// RefId 记下这次调额的方式和数值，重放时用来核对是不是同一个操作。
	refID := fmt.Sprintf("%s:%d", mode, value)

	var result AdtyQuotaAdjustResult
	var cacheApplied bool
	unlockBatchQueue := lockUserQuotaBatchQueue()
	defer unlockBatchQueue()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if operatorRole != common.RoleRootUser && operatorRole <= user.Role {
			return ErrUserQuotaPermission
		}
		existing, err := FindAdtyLedgerByIdemKeyTx(tx, idemKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Type != AdtyLedgerTypeManualAdjust || existing.UserId != userID || existing.RefId != refID {
				return ErrAdtyIdempotencyConflict
			}
			result = AdtyQuotaAdjustResult{
				UserQuotaAdjustment: UserQuotaAdjustment{UserID: user.Id, Username: user.Username,
					Before: int(existing.BalanceBefore), After: int(existing.BalanceAfter)},
				Replayed: true,
			}
			return nil
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
		if after != user.Quota {
			updated := tx.Model(&User{}).Where("id = ?", userID).Update("quota", after)
			if updated.Error != nil {
				return fail(updated.Error)
			}
			if updated.RowsAffected != 1 {
				return fail(gorm.ErrRecordNotFound)
			}
		}
		entry := &AdtyBalanceLedger{
			UserId: userID, DeltaQuota: int64(after) - int64(user.Quota),
			BalanceBefore: int64(user.Quota), BalanceAfter: int64(after),
			Type: AdtyLedgerTypeManualAdjust, RefType: "admin", RefId: refID,
			Reason: reason, OperatorId: operatorID,
		}
		if idemKey != "" {
			entry.IdempotencyKey = &idemKey
		}
		if err := InsertAdtyLedgerTx(tx, entry); err != nil {
			return fail(err)
		}
		result = AdtyQuotaAdjustResult{UserQuotaAdjustment: UserQuotaAdjustment{
			UserID: user.Id, Username: user.Username, Before: user.Quota, After: after,
		}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !cacheApplied && !result.Replayed {
		if delta := int64(result.After) - int64(result.Before); delta != 0 {
			if err := cacheIncrUserQuota(userID, delta); err != nil {
				common.SysError(common.LogText("failed to sync manual quota adjustment for user %d: %s", userID, err))
				recordAdjustCacheDivergence(userID, delta, err)
			}
		}
	}
	return &result, nil
}
