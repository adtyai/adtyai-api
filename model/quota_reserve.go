package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cacheQuotaResult int

const (
	cacheQuotaInsufficient cacheQuotaResult = iota
	cacheQuotaOK
	cacheQuotaMiss
)

const userQuotaReserveScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
local quota = tonumber(redis.call('HGET', KEYS[1], 'Quota'))
if quota == nil or quota < tonumber(ARGV[1]) then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'Quota', -tonumber(ARGV[1]))
return 1`

const userQuotaDeltaScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'Quota', ARGV[1])
return 1`

const tokenQuotaReserveScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
local remain = tonumber(redis.call('HGET', KEYS[1], 'RemainQuota'))
if remain == nil or remain < tonumber(ARGV[1]) then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', -tonumber(ARGV[1]))
redis.call('HINCRBY', KEYS[1], 'UsedQuota', tonumber(ARGV[1]))
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

const tokenQuotaDeltaScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', tonumber(ARGV[1]))
redis.call('HINCRBY', KEYS[1], 'UsedQuota', -tonumber(ARGV[1]))
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

func quotaResultFromLua(result int, err error) (cacheQuotaResult, error) {
	if err != nil {
		return cacheQuotaMiss, err
	}
	switch result {
	case 1:
		return cacheQuotaOK, nil
	case 0:
		return cacheQuotaInsufficient, nil
	default:
		return cacheQuotaMiss, nil
	}
}

func cacheTryReserveUserQuota(userID int, amount int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), userQuotaReserveScript,
		[]string{getUserCacheKey(userID)}, amount, userID, userCacheSchemaVersion).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyUserQuotaDelta(userID int, delta int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), userQuotaDeltaScript,
		[]string{getUserCacheKey(userID)}, delta, userID, userCacheSchemaVersion).Int()
	return quotaResultFromLua(result, err)
}

func cacheTryReserveTokenQuota(id int, key string, amount int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), tokenQuotaReserveScript,
		[]string{getTokenCacheKey(key)}, amount, id, common.GetTimestamp()).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyTokenQuotaDelta(id int, key string, delta int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), tokenQuotaDeltaScript,
		[]string{getTokenCacheKey(key)}, delta, id, common.GetTimestamp()).Int()
	return quotaResultFromLua(result, err)
}

func persistTokenQuotaDelta(id int, delta int) error {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, delta)
		return nil
	}
	result := DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]any{
			"remain_quota":  gorm.Expr("remain_quota + ?", delta),
			"used_quota":    gorm.Expr("used_quota - ?", delta),
			"accessed_time": common.GetTimestamp(),
		},
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func reserveUserQuotaDB(id int, quota int) (bool, error) {
	_, _, ok, err := tryReserveUserWalletDB(id, quota)
	return ok, err
}

// tryReserveUserWalletDB 事务内先锁赠送再锁实充，先扣赠送再扣实充。
// 返回本次从赠送/实充各扣了多少；合计不足时 ok=false 且不改余额。
func tryReserveUserWalletDB(id int, quota int) (bonus int, paid int, ok bool, err error) {
	if quota < 0 {
		return 0, 0, false, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return 0, 0, true, nil
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := EnsureUserBonusWallet(tx, id); err != nil {
			return err
		}
		var wallet UserBonusWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", id).First(&wallet).Error; err != nil {
			return err
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota").Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		total := int(wallet.Balance) + user.Quota
		if total < quota {
			ok = false
			return nil
		}
		fromBonus := int(wallet.Balance)
		if fromBonus > quota {
			fromBonus = quota
		}
		fromPaid := quota - fromBonus
		if fromBonus > 0 {
			if err := tx.Model(&UserBonusWallet{}).Where("user_id = ? AND balance >= ?", id, fromBonus).
				Update("balance", gorm.Expr("balance - ?", fromBonus)).Error; err != nil {
				return err
			}
		}
		if fromPaid > 0 {
			res := tx.Model(&User{}).Where("id = ? AND quota >= ?", id, fromPaid).
				Update("quota", gorm.Expr("quota - ?", fromPaid))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errors.New("paid quota changed concurrently")
			}
		}
		// B-03：同事务记出资拆分
		if err := recordWalletConsumeTx(tx, id, int64(fromBonus), int64(fromPaid)); err != nil {
			return err
		}
		bonus, paid, ok = fromBonus, fromPaid, true
		return nil
	})
	return bonus, paid, ok, err
}

// TryReserveUserWallet 原子预扣两本账，返回赠送/实充拆分。
func TryReserveUserWallet(id int, quota int) (bonus int, paid int, ok bool, err error) {
	if quota < 0 {
		return 0, 0, false, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return 0, 0, true, nil
	}
	// 无 Redis 或缓存未命中时走 DB 事务拆分；有 Redis 时先扣合计缓存再 DB 落两本账。
	if !common.RedisEnabled {
		return tryReserveUserWalletDB(id, quota)
	}
	result, cacheErr := cacheTryReserveUserQuota(id, int64(quota))
	if cacheErr == nil && result == cacheQuotaMiss {
		if _, hydrateErr := GetUserCache(id); hydrateErr == nil {
			result, cacheErr = cacheTryReserveUserQuota(id, int64(quota))
		}
	}
	if cacheErr != nil || result == cacheQuotaMiss {
		if cacheErr != nil {
			common.SysLog(common.LogText("user quota cache reserve unavailable, falling back to database: %s", cacheErr.Error()))
		}
		return tryReserveUserWalletDB(id, quota)
	}
	if result == cacheQuotaInsufficient {
		return 0, 0, false, nil
	}
	bonus, paid, ok, err = tryReserveUserWalletDB(id, quota)
	if err != nil || !ok {
		compensated, compensateErr := cacheApplyUserQuotaDelta(id, int64(quota))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(common.LogText("failed to compensate reserved user quota: result=%d error=%v", compensated, compensateErr))
		}
		if err != nil {
			return 0, 0, false, err
		}
		return 0, 0, false, nil
	}
	return bonus, paid, true, nil
}

// ErrInsufficientWalletBalance 两本账合计不足以完成扣减（结算补扣等）。
// 整笔事务回滚，users.quota / bonus 均不改；上游可 errors.Is 后映 403。
var ErrInsufficientWalletBalance = errors.New("insufficient wallet balance")

// DecreaseUserWallet 结算补扣：锁行后先扣赠送再扣实充。
// 实充不够补齐时整笔失败回滚，绝不把 users.quota 扣成负数（邀请返佣除外，本函数不涉及）。
func DecreaseUserWallet(id int, quota int) (bonus int, paid int, err error) {
	return decreaseUserWallet(id, quota, false)
}

// DecreaseUserWalletAllowDebt 服务已交付后的结算补扣（与官方 new-api DecreaseUserQuota 一致）：
// 先扣赠送到 0，剩余全部从实充扣，实充允许扣成负数（欠费），绝不整笔免单。
// 欠费后 users.quota<=0，下一次请求在 NewBillingSession / PreConsume 被 403 拦住。
func DecreaseUserWalletAllowDebt(id int, quota int) (bonus int, paid int, err error) {
	return decreaseUserWallet(id, quota, true)
}

func decreaseUserWallet(id int, quota int, allowDebt bool) (bonus int, paid int, err error) {
	if quota < 0 {
		return 0, 0, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return 0, 0, nil
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := EnsureUserBonusWallet(tx, id); err != nil {
			return err
		}
		var wallet UserBonusWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", id).First(&wallet).Error; err != nil {
			return err
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota").Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		fromBonus := int(wallet.Balance)
		if fromBonus > quota {
			fromBonus = quota
		}
		fromPaid := quota - fromBonus
		if fromPaid > user.Quota && !allowDebt {
			// 整笔失败回滚，不静默扣到 0、不扣负（仅用于服务交付前的预留）
			return ErrInsufficientWalletBalance
		}
		if fromBonus > 0 {
			res := tx.Model(&UserBonusWallet{}).Where("user_id = ? AND balance >= ?", id, fromBonus).
				Update("balance", gorm.Expr("balance - ?", fromBonus))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errors.New("bonus balance changed concurrently")
			}
		}
		if fromPaid > 0 {
			q := tx.Model(&User{}).Where("id = ?", id)
			if !allowDebt {
				q = q.Where("quota >= ?", fromPaid)
			}
			res := q.Update("quota", gorm.Expr("quota - ?", fromPaid))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errors.New("paid quota changed concurrently")
			}
			if allowDebt && fromPaid > user.Quota {
				common.SysLog(common.LogText("user %d settled into arrears: charged %d, paid balance before %d", id, fromPaid, user.Quota))
			}
		}
		// B-03：同事务记出资拆分
		if err := recordWalletConsumeTx(tx, id, int64(fromBonus), int64(fromPaid)); err != nil {
			return err
		}
		bonus, paid = fromBonus, fromPaid
		return nil
	})
	// 缓存由 DecreaseUserQuota / WalletFunding 调用方统一维护，此处只改 DB。
	return bonus, paid, err
}

// ReturnReservedUserWallet B-03 H-A：同一请求内的预扣退回（结算多扣退回、请求失败整笔退、预留回滚）。
// 只把额度加回原钱包并冲减出资拆分（同一事务），不写流水：小时汇总按 logs 净消费记账，
// 流水合计 = −净消费 = 余额净变化，对账可平。
func ReturnReservedUserWallet(id int, bonus int, paid int) error {
	return creditUserWallet(id, bonus, paid, "wallet_return", "return pre-consumed wallet", "billing", "", false, func(tx *gorm.DB) error {
		return recordWalletConsumeTx(tx, id, -int64(bonus), -int64(paid))
	})
}

// RefundUserWalletAfterTheFact B-03 H-B/M-A：事后退款（异步任务失败、MJ 失败，logs 另记 LogTypeRefund）。
// 按调用方给定的原拆分加回并写 wallet_refund 流水；不碰出资拆分表（消费小时的 consume 日志不变）。
func RefundUserWalletAfterTheFact(id int, bonus int, paid int, refType, refId string) error {
	return creditUserWallet(id, bonus, paid, "wallet_refund", "refund consumed wallet", refType, refId, true, nil)
}

func reserveTokenQuotaDB(id int, quota int) (bool, error) {
	result := DB.Model(&Token{}).
		Where("id = ? AND remain_quota >= ?", id, quota).
		Updates(map[string]any{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		})
	return result.RowsAffected == 1, result.Error
}

// TryReserveUserQuota atomically checks and deducts a user's wallet quota（两本账合计）。
// 内部先扣赠送再扣实充；缓存存合计。
func TryReserveUserQuota(id int, quota int) (bool, error) {
	_, _, ok, err := TryReserveUserWallet(id, quota)
	return ok, err
}

// TryReserveTokenQuota atomically checks and deducts a token quota. Unlimited
// tokens skip the balance check but still update remain/used accounting.
func TryReserveTokenQuota(id int, key string, quota int, unlimited bool) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return true, nil
	}
	if unlimited {
		return true, DecreaseTokenQuota(id, key, quota)
	}
	if !common.RedisEnabled {
		return reserveTokenQuotaDB(id, quota)
	}

	result, err := cacheTryReserveTokenQuota(id, key, int64(quota))
	if err == nil && result == cacheQuotaMiss {
		if _, hydrateErr := GetTokenByKey(key, true); hydrateErr == nil {
			result, err = cacheTryReserveTokenQuota(id, key, int64(quota))
		}
	}
	if err != nil || result == cacheQuotaMiss {
		if err != nil {
			common.SysLog(common.LogText("token quota cache reserve unavailable, falling back to database: %s", err.Error()))
		}
		return reserveTokenQuotaDB(id, quota)
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistTokenQuotaDelta(id, -quota); err != nil {
		compensated, compensateErr := cacheApplyTokenQuotaDelta(id, key, int64(quota))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(common.LogText("failed to compensate reserved token quota: result=%d error=%v", compensated, compensateErr))
		}
		return false, err
	}
	return true, nil
}
