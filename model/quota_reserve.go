package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// persistUserQuotaDelta commits the amount before returning. The local queue
// only hints which durable pending rows should be folded into the raw balance.
func persistUserQuotaDelta(id int, delta int) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, id).Error; err != nil {
			return err
		}
		return applyUserQuotaDeltaTx(tx, &user, delta, common.BatchUpdateEnabled)
	})
	if err != nil {
		return err
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, 0)
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysError(common.LogText("failed to invalidate committed wallet projection: user=%d error=%s", id, err.Error()))
	}
	return nil
}

func persistTokenQuotaDelta(id int, delta int) error {
	if delta == 0 {
		return nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).First(&token, id).Error; err != nil {
			return err
		}
		return applyTokenQuotaDeltaTx(tx, &token, delta, common.BatchUpdateEnabled)
	})
	if err != nil {
		return err
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, 0)
	}
	return nil
}

// applyTokenQuotaDeltaTx requires the token row lock. Unlimited tokens and
// settlement deltas retain the existing signed remaining-quota semantics.
func applyTokenQuotaDeltaTx(tx *gorm.DB, token *Token, delta int, pending bool) error {
	remain, used, err := tokenQuotaBalance(token)
	if err != nil {
		return err
	}
	if _, err := checkedQuotaSum(remain, delta); err != nil {
		return err
	}
	if _, err := checkedQuotaDifference(used, delta); err != nil {
		return err
	}
	updates := map[string]any{"accessed_time": common.GetTimestamp()}
	if pending {
		amount, err := checkedQuotaSum(token.QuotaPending, delta)
		if err != nil {
			return err
		}
		updates["quota_pending"] = amount
	} else {
		rawRemain, err := checkedQuotaSum(token.RemainQuota, delta)
		if err != nil {
			return err
		}
		rawUsed, err := checkedQuotaDifference(token.UsedQuota, delta)
		if err != nil {
			return err
		}
		updates["remain_quota"], updates["used_quota"] = rawRemain, rawUsed
	}
	result := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func reserveUserQuotaDB(id int, quota int) (bool, error) {
	reserved := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, id).Error; err != nil {
			return err
		}
		balance, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		if balance < quota {
			return nil
		}
		if err := applyUserQuotaDeltaTx(tx, &user, -quota, common.BatchUpdateEnabled && common.RedisEnabled); err != nil {
			return err
		}
		reserved = true
		return nil
	})
	return reserved, err
}

func reserveTokenQuotaDB(id int, quota int) (bool, error) {
	reserved := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).First(&token, id).Error; err != nil {
			return err
		}
		remain, _, err := tokenQuotaBalance(&token)
		if err != nil {
			return err
		}
		if remain < quota {
			return nil
		}
		if err := applyTokenQuotaDeltaTx(tx, &token, -quota, common.BatchUpdateEnabled && common.RedisEnabled); err != nil {
			return err
		}
		reserved = true
		return nil
	})
	return reserved, err
}

func flushTokenQuota(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx.Unscoped()).First(&token, id).Error; err != nil {
			return err
		}
		if token.QuotaPending == 0 {
			return nil
		}
		remain, used, err := tokenQuotaBalance(&token)
		if err != nil {
			return err
		}
		return tx.Unscoped().Model(&Token{}).Where("id = ?", id).Updates(map[string]any{
			"remain_quota": remain, "used_quota": used, "quota_pending": 0,
		}).Error
	})
}

// TryReserveUserQuota checks the shared durable logical balance under the user
// row lock. A cache miss, outage, or a different process cannot authorize the
// same unflushed amount again.
func TryReserveUserQuota(id int, quota int) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return true, nil
	}
	reserved, err := reserveUserQuotaDB(id, quota)
	if err != nil || !reserved {
		return false, err
	}
	if common.BatchUpdateEnabled && common.RedisEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, 0)
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysError(common.LogText("failed to invalidate committed wallet projection: user=%d error=%s", id, err.Error()))
	}
	return true, nil
}

// chargeUserQuotaUpToDB 在事务里锁行读余额，按 min(余额, quota) 扣减，余额已 ≤ 0
// 时不改库。SQLite 没有 FOR UPDATE，但单写者模型下事务内读写同样不会并发交错。
// Redis 关闭时即使开了批量更新也直写数据库（不进批量队列），和 reserveUserQuotaDB 一致，
// 否则落后于队列的数据库余额会让并发结算扣穿下限。
func chargeUserQuotaUpToDB(id int, quota int) (int, error) {
	charged := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, id).Error; err != nil {
			return err
		}
		balance, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		charged = max(min(balance, quota), 0)
		if charged == 0 {
			return nil
		}
		if err := ConsumeAdtyFundBatchesTx(tx, id, int64(charged), int64(balance)); err != nil {
			return err
		}
		return applyUserQuotaDeltaTx(tx, &user, -charged, common.BatchUpdateEnabled && common.RedisEnabled)
	})
	if err != nil {
		return 0, err
	}
	return charged, nil
}

// ChargeUserQuotaUpTo 原子扣减钱包余额，最多扣到 0（余额下限，不透支），返回实扣额。
// It shares the same durable admission path as reservations. The caller treats
// any shortfall as a platform cost; Redis never authorizes a charge.
func ChargeUserQuotaUpTo(id int, quota int) (int, error) {
	if quota < 0 {
		return 0, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return 0, nil
	}
	charged, err := chargeUserQuotaUpToDB(id, quota)
	if err != nil || charged == 0 {
		return charged, err
	}
	if common.BatchUpdateEnabled && common.RedisEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, 0)
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysError(common.LogText("failed to invalidate committed wallet projection: user=%d error=%s", id, err.Error()))
	}
	return charged, nil
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
		err := DecreaseTokenQuota(id, key, quota)
		return err == nil, err
	}
	reserved, err := reserveTokenQuotaDB(id, quota)
	if err == nil && reserved && common.BatchUpdateEnabled && common.RedisEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, 0)
	}
	return reserved, err
}
