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

func reserveUserQuotaDB(id int, quota int, operationID string) (bool, error) {
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
		// A.12.1: record which batches back this pre-charge so a release or a
		// refund can return the same shares (A.12.2.6). The batch rows themselves
		// are consumed at settlement, not here.
		if err := AllocateAdtyFundAllocationsTx(tx, id, operationID, int64(quota), int64(balance)); err != nil {
			return err
		}
		reserved = true
		return nil
	})
	return reserved, err
}

// RefundUserQuotaForOperation 把一次预扣整笔退回，并在**同一个事务里**释放
// 这次操作登记的分摊。
//
// 为什么必须同一事务：`IncreaseUserQuota` 只加 `users.quota`，预扣时登记的分摊
// （held）留在库里，而 `AllocateAdtyFundAllocationsTx` 算可用额度用的是
// `batch.Remaining - heldByBatch`，所以那笔占用会**永久**占住同一批次的容量。
// 实测：余额 100000、预扣 50000、退款后 quota 回到 100000，残留 held 仍是 50000，
// 下一次预扣 100000 只能登记 50000。失败退款每来一次，可用额度就少一块。
//
// operationKey 为空表示这次预扣没有登记过分摊（升级前的调用面），退钱即可。
func RefundUserQuotaForOperation(id int, quota int, operationKey string) error {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if quota == 0 {
		// 仍然要释放：预扣为 0 也可能因为后续补扣登记过分摊。
		if operationKey == "" {
			return nil
		}
	} else if err := common.ValidateWalletQuota(quota); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if quota > 0 {
			var user User
			if err := lockForUpdate(tx).First(&user, id).Error; err != nil {
				return err
			}
			// 退款必须直写数据库：这里已经在事务里，走批量队列会让「钱回来了」
			// 和「占用释放了」不在同一个提交边界上。
			if err := applyUserQuotaDeltaTx(tx, &user, quota, false); err != nil {
				return err
			}
		}
		return ReleaseAdtyFundAllocationsTx(tx, id, operationKey)
	})
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
// same unflushed amount again. It carries no operation identity, so it records
// no fund shares; the relay billing path must use
// TryReserveUserQuotaForOperation instead.
func TryReserveUserQuota(id int, quota int) (bool, error) {
	return tryReserveUserQuota(id, quota, "")
}

// TryReserveUserQuotaForOperation is the relay billing reservation. The
// operation key makes the pre-charge reversible: A.12.2.6 requires releases and
// refunds to hand back the original shares instead of a merged balance.
func TryReserveUserQuotaForOperation(id int, quota int, operationID string) (bool, error) {
	return tryReserveUserQuota(id, quota, operationID)
}

func tryReserveUserQuota(id int, quota int, operationID string) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota cannot be negative")
	}
	if quota == 0 {
		return true, nil
	}
	reserved, err := reserveUserQuotaDB(id, quota, operationID)
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
