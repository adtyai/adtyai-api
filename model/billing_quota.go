package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// checkedQuotaSum rejects overflow instead of changing a durable amount.
func checkedQuotaSum(base, delta int) (int, error) {
	if (delta > 0 && base > math.MaxInt-delta) || (delta < 0 && base < math.MinInt-delta) {
		return 0, ErrWalletQuotaLimitExceeded
	}
	return base + delta, nil
}

func checkedQuotaDifference(base, delta int) (int, error) {
	if (delta > 0 && base < math.MinInt+delta) || (delta < 0 && base > math.MaxInt+delta) {
		return 0, ErrWalletQuotaLimitExceeded
	}
	return base - delta, nil
}

func userWalletBalance(raw, pending int) (int, error) {
	balance, err := checkedQuotaSum(raw, pending)
	if err != nil || balance > common.MaxWalletQuota {
		return 0, ErrWalletQuotaLimitExceeded
	}
	return balance, nil
}

// applyUserQuotaDeltaTx requires the caller's user row lock. Positive pending
// credits may leave the raw row negative; only the logical balance is used for
// admission. Check-in gifts follow actual consumption before any later credit.
func applyUserQuotaDeltaTx(tx *gorm.DB, user *User, delta int, pending bool) error {
	balance, err := userWalletBalance(user.Quota, user.QuotaPending)
	if err != nil {
		return err
	}
	after, err := userWalletBalance(balance, delta)
	if err != nil {
		return err
	}
	field, current := "quota", user.Quota
	if pending {
		field, current = "quota_pending", user.QuotaPending
	}
	updated, err := checkedQuotaSum(current, delta)
	if err != nil {
		return err
	}
	if delta == 0 {
		return nil
	}
	result := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		field: updated, "checkin_quota": max(0, min(user.CheckinQuota, balance, after)),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func pendingBatchDelta(kind int, id int) int {
	if kind < 0 || kind >= len(batchUpdateStores) || id == 0 {
		return 0
	}
	batchUpdateLocks[kind].Lock()
	defer batchUpdateLocks[kind].Unlock()
	return pendingBatchDeltaLocked(kind, id)
}

// pendingBatchDeltaLocked requires the corresponding batchUpdateLocks[kind]
// to already be held by the caller.
func pendingBatchDeltaLocked(kind int, id int) int {
	if kind < 0 || kind >= len(batchUpdateStores) || id == 0 {
		return 0
	}
	return batchUpdateStores[kind][id]
}

func saturatingAddInt(base int, delta int) int {
	if delta > 0 && base > math.MaxInt-delta {
		return math.MaxInt
	}
	if delta < 0 && base < math.MinInt-delta {
		return math.MinInt
	}
	return base + delta
}

func tokenQuotaBalance(token *Token) (remain, used int, err error) {
	remain, err = checkedQuotaSum(token.RemainQuota, token.QuotaPending)
	if err != nil {
		return 0, 0, err
	}
	used, err = checkedQuotaDifference(token.UsedQuota, token.QuotaPending)
	return remain, used, err
}

// projectTokenQuota only changes a returned snapshot. SQL rows retain their
// pending amount until the locked flush or an explicit limit replacement.
func projectTokenQuota(token *Token) error {
	remain, used, err := tokenQuotaBalance(token)
	if err != nil {
		return err
	}
	token.RemainQuota, token.UsedQuota, token.QuotaPending = remain, used, 0
	return nil
}

// TokenQuotaForBilling includes durable deltas, never process-local hints.
func TokenQuotaForBilling(token *Token) (remain int, used int) {
	if token == nil {
		return 0, 0
	}
	remain, used, err := tokenQuotaBalance(token)
	if err != nil {
		common.SysError(common.LogText("invalid durable token balance: token=%d", token.Id))
		return 0, 0
	}
	return remain, used
}

// UserQuotaForBilling returns the durable logical wallet and usage statistics.
func UserQuotaForBilling(id int) (remain int, used int, err error) {
	// Preserve the authentication fence for the billing endpoint while wallet
	// amounts themselves continue to come from the durable database balance.
	if common.RedisEnabled {
		if _, err = GetUserCache(id); err != nil {
			return 0, 0, err
		}
	}
	remain, err = GetUserQuota(id, false)
	if err != nil {
		return 0, 0, err
	}
	used, err = GetUserUsedQuota(id)
	if err != nil {
		return 0, 0, err
	}
	if common.BatchUpdateEnabled {
		used = saturatingAddInt(used, pendingBatchDelta(BatchUpdateTypeUsedQuota, id))
	}
	return remain, used, nil
}
