package model

import (
	"errors"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAdjustQuotaZeroDelta      = errors.New("adjust quota delta must be non-zero")
	ErrAdjustQuotaBadAccount     = errors.New("account must be quota or bonus")
	ErrAdjustQuotaInsufficient   = errors.New("insufficient account balance")
	ErrAdjustQuotaUserNotFound   = errors.New("user not found")
	ErrAdjustQuotaInjectedFail   = errors.New("adjust quota simulated failure")
	ErrAdjustQuotaIdempotentMiss = errors.New("idempotent replay entry missing")
)

// AdjustQuotaFailInject 验收用：为 true 时 AdjustQuota 在写流水前失败，事务回滚。
var AdjustQuotaFailInject atomic.Bool

// AdjustQuotaResult 统一调额结果。
type AdjustQuotaResult struct {
	Entry         *LedgerEntry `json:"entry"`
	BalanceBefore int64        `json:"balance_before"`
	BalanceAfter  int64        `json:"balance_after"`
	Replay        bool         `json:"replay"`
	LedgerAccount string       `json:"ledger_account"` // paid | bonus
}

// NormalizeAdjustAccount 将 API account（quota|bonus）规范为 ledger account（paid|bonus）。
func NormalizeAdjustAccount(account string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(account)) {
	case "quota", "paid":
		return "paid", nil
	case "bonus":
		return "bonus", nil
	default:
		return "", ErrAdjustQuotaBadAccount
	}
}

// AdjustQuota 所有改余额的唯一入口：写对应账户 + ledger_entries；可选幂等键。
// account: quota|paid|bonus；delta 正加负减；tx 非空则加入调用方事务。
// kindOpt 可选：非空时作为 ledger kind；空则保持现行为（kind=refType，再空则 balance_adjust）。
// 锁序：user_bonus_wallets → users（与扣费/退款一致）。
func AdjustQuota(tx *gorm.DB, userId int, account string, delta int64, reason, refType, refId, idemKey string, operatorId int, kindOpt ...string) (*AdjustQuotaResult, error) {
	return adjustQuota(tx, userId, account, delta, reason, refType, refId, idemKey, operatorId, true, kindOpt...)
}

// adjustQuotaKeepUserCache writes the balance without deleting the quota hash.
// The caller must add the same positive delta after commit. Deleting the hash
// inside the credit transaction lets a concurrent reader store the pre-commit total.
func adjustQuotaKeepUserCache(tx *gorm.DB, userId int, account string, delta int64, reason, refType, refId, idemKey string, operatorId int, kindOpt ...string) (*AdjustQuotaResult, error) {
	return adjustQuota(tx, userId, account, delta, reason, refType, refId, idemKey, operatorId, false, kindOpt...)
}

func adjustQuota(tx *gorm.DB, userId int, account string, delta int64, reason, refType, refId, idemKey string, operatorId int, invalidateCache bool, kindOpt ...string) (*AdjustQuotaResult, error) {
	kindOverride := ""
	if len(kindOpt) > 0 {
		kindOverride = strings.TrimSpace(kindOpt[0])
	}
	if delta == 0 {
		return nil, ErrAdjustQuotaZeroDelta
	}
	ledgerAccount, err := NormalizeAdjustAccount(account)
	if err != nil {
		return nil, err
	}
	idemKey = strings.TrimSpace(idemKey)
	reason = strings.TrimSpace(reason)

	run := func(tx *gorm.DB) (*AdjustQuotaResult, error) {
		if idemKey != "" {
			var exist LedgerIdempotency
			if err := tx.Where("key = ?", idemKey).First(&exist).Error; err == nil {
				var entry LedgerEntry
				if err := tx.Where("id = ?", exist.EntryId).First(&entry).Error; err != nil {
					return nil, ErrAdjustQuotaIdempotentMiss
				}
				return &AdjustQuotaResult{
					Entry:         &entry,
					BalanceBefore: entry.BalanceAfter - entry.Delta,
					BalanceAfter:  entry.BalanceAfter,
					Replay:        true,
					LedgerAccount: entry.Account,
				}, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}

		if AdjustQuotaFailInject.Load() {
			return nil, ErrAdjustQuotaInjectedFail
		}

		if err := EnsureUserBonusWallet(tx, userId); err != nil {
			return nil, err
		}

		// 锁序：bonus → users
		var wallet UserBonusWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userId).First(&wallet).Error; err != nil {
			return nil, err
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota").Where("id = ?", userId).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrAdjustQuotaUserNotFound
			}
			return nil, err
		}

		var before, after int64
		kind := "balance_adjust"
		if kindOverride != "" {
			kind = kindOverride
		} else if refType != "" {
			kind = refType
		}

		switch ledgerAccount {
		case "paid":
			before = int64(user.Quota)
			after = before + delta
			if after < 0 {
				return nil, ErrAdjustQuotaInsufficient
			}
			if delta > 0 {
				if err := increaseUserQuotaTx(tx, userId, int(delta)); err != nil {
					return nil, err
				}
			} else {
				need := -delta
				res := tx.Model(&User{}).Where("id = ? AND quota >= ?", userId, need).
					Update("quota", gorm.Expr("quota - ?", need))
				if res.Error != nil {
					return nil, res.Error
				}
				if res.RowsAffected != 1 {
					return nil, ErrAdjustQuotaInsufficient
				}
			}
			var q int
			if err := tx.Model(&User{}).Where("id = ?", userId).Select("quota").Scan(&q).Error; err != nil {
				return nil, err
			}
			after = int64(q)
		case "bonus":
			before = wallet.Balance
			after = before + delta
			if after < 0 {
				return nil, ErrAdjustQuotaInsufficient
			}
			if delta > 0 {
				if err := tx.Model(&UserBonusWallet{}).Where("user_id = ?", userId).
					Update("balance", gorm.Expr("balance + ?", delta)).Error; err != nil {
					return nil, err
				}
			} else {
				need := -delta
				res := tx.Model(&UserBonusWallet{}).Where("user_id = ? AND balance >= ?", userId, need).
					Update("balance", gorm.Expr("balance - ?", need))
				if res.Error != nil {
					return nil, res.Error
				}
				if res.RowsAffected != 1 {
					return nil, ErrAdjustQuotaInsufficient
				}
			}
			if err := tx.Model(&UserBonusWallet{}).Where("user_id = ?", userId).Select("balance").Scan(&after).Error; err != nil {
				return nil, err
			}
		}

		entry, err := createLedgerEntryTx(tx, userId, ledgerAccount, kind, delta, after, reason, refType, refId, operatorId)
		if err != nil {
			return nil, err
		}
		if idemKey != "" {
			if err := tx.Create(&LedgerIdempotency{Key: idemKey, EntryId: entry.Id}).Error; err != nil {
				if isUniqueViolation(err) {
					// 并发抢到同一幂等键：按重放处理
					var exist LedgerIdempotency
					if e2 := tx.Where("key = ?", idemKey).First(&exist).Error; e2 != nil {
						return nil, err
					}
					var replay LedgerEntry
					if e2 := tx.Where("id = ?", exist.EntryId).First(&replay).Error; e2 != nil {
						return nil, ErrAdjustQuotaIdempotentMiss
					}
					return &AdjustQuotaResult{
						Entry:         &replay,
						BalanceBefore: replay.BalanceAfter - replay.Delta,
						BalanceAfter:  replay.BalanceAfter,
						Replay:        true,
						LedgerAccount: replay.Account,
					}, nil
				}
				return nil, err
			}
		}
		if invalidateCache {
			_ = invalidateUserCache(userId)
		}
		return &AdjustQuotaResult{
			Entry:         entry,
			BalanceBefore: before,
			BalanceAfter:  after,
			Replay:        false,
			LedgerAccount: ledgerAccount,
		}, nil
	}

	if tx != nil {
		return run(tx)
	}
	var out *AdjustQuotaResult
	err = DB.Transaction(func(inner *gorm.DB) error {
		r, e := run(inner)
		if e != nil {
			return e
		}
		out = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BalanceAdjustMaxQuota 返回 BalanceAdjustMaxAmount（美元）换算的额度上限。
func BalanceAdjustMaxQuota() (int64, error) {
	maxUSD := GetBalanceAdjustMaxAmount()
	if maxUSD <= 0 {
		maxUSD = 500
	}
	q, err := DollarsToQuota(maxUSD)
	if err != nil {
		return 0, err
	}
	return int64(q), nil
}

// CountEnabledAdmins 实时统计未禁用管理员（role>=Admin，含 Root）。
func CountEnabledAdmins(tx *gorm.DB) (int64, error) {
	db := DB
	if tx != nil {
		db = tx
	}
	var n int64
	err := db.Model(&User{}).
		Where("role >= ? AND status = ?", common.RoleAdminUser, common.UserStatusEnabled).
		Count(&n).Error
	return n, err
}
