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

func adtyAdjustFundSource(idemKey string) string {
	return "adjust:" + common.Sha1([]byte(idemKey))
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
		if after != user.Quota {
			updated := tx.Model(&User{}).Where("id = ?", userID).Update("quota", after)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if err := clampCheckinQuotaDB(tx, userID); err != nil {
			return err
		}
		// 流水和另外四类写入方（充值、退款、兑换码、佣金转余额）用同一把尺子：逻辑余额
		// （raw + pending）。只记 raw 时，quota_pending 非零的账号在同一张流水表里会
		// 看到两种口径，相邻两行满足不了 after(n) == before(n+1)，对账无法复算。
		liquid, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		entry := &AdtyBalanceLedger{
			UserId: userID, DeltaQuota: int64(after) - int64(user.Quota),
			// DeltaQuota 保留 raw 差值：pending 在调额事务里不变，raw 差值和逻辑差值恒等。
			// liveAfter 就是 userWalletBalance(after, user.QuotaPending)，上面已按逻辑余额校验过。
			BalanceBefore: int64(liquid), BalanceAfter: int64(liveAfter),
			Type: AdtyLedgerTypeManualAdjust, RefType: "admin", RefId: refID,
			Reason: reason, OperatorId: operatorID,
		}
		if idemKey != "" {
			entry.IdempotencyKey = &idemKey
		}
		if err := InsertAdtyLedgerTx(tx, entry); err != nil {
			return err
		}
		delta := int64(after) - int64(user.Quota)
		if delta > 0 {
			// 调额批次记在客户所属站：按站点算本金负债时要能看见这笔凭空补的额度。
			if err := CreateAdtyFundBatchesWithSiteTx(tx, userID, AdtyFundBatchSiteID(user.SiteId), adtyAdjustFundSource(idemKey), delta, 0); err != nil {
				return err
			}
		} else if delta < 0 {
			// Manual adjust is not a usage charge: keep the customer's lots in
			// sync and release any corresponding site gift-guarantee, but do
			// not fire the principal-spent hook (that hook debits the site
			// operating wallet).
			// 传逻辑余额（raw + pending），不是 raw：批次总量要跟「用户实际能用多少」
			// 对齐，否则余额主要落在 quota_pending 里的账号会被算成「批次够了」或
			// 「批次差很多」两种相反的错误。
			logical, err := userWalletBalance(user.Quota, user.QuotaPending)
			if err != nil {
				return err
			}
			if err := ConsumeAdtyFundBatchesQuietTx(tx, userID, -delta, int64(logical)); err != nil {
				return err
			}
		}
		result = AdtyQuotaAdjustResult{UserQuotaAdjustment: UserQuotaAdjustment{
			UserID: user.Id, Username: user.Username, Before: user.Quota, After: after,
		}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !result.Replayed {
		if err := invalidateUserCache(userID); err != nil {
			common.SysError(common.LogText("failed to invalidate manual quota projection for user %d: %s", userID, err))
		}
	}
	return &result, nil
}
