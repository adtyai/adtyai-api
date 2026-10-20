package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserBonusWallet 赠送余额账（与 users.quota 实充分开）。
// 扣费顺序：先赠送后实充；余额检查看合计。
type UserBonusWallet struct {
	UserId    int       `json:"user_id" gorm:"primaryKey;column:user_id"`
	Balance   int64     `json:"balance" gorm:"type:bigint;not null;default:0;check:balance>=0"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (UserBonusWallet) TableName() string {
	return "user_bonus_wallets"
}

// EnsureUserBonusWallet 确保用户有赠送钱包行（余额 0）。
func EnsureUserBonusWallet(tx *gorm.DB, userId int) error {
	if tx == nil {
		tx = DB
	}
	w := UserBonusWallet{UserId: userId, Balance: 0}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&w).Error
}

// GetUserBonusBalance 读取赠送余额；无行视为 0。
func GetUserBonusBalance(userId int) (int64, error) {
	var bal int64
	err := DB.Model(&UserBonusWallet{}).Where("user_id = ?", userId).Select("balance").Scan(&bal).Error
	if err != nil {
		return 0, err
	}
	return bal, nil
}

// GetUserPaidQuota 只读实充（users.quota），不含赠送。
func GetUserPaidQuota(userId int) (int, error) {
	var quota int
	err := DB.Model(&User{}).Where("id = ?", userId).Select("quota").Scan(&quota).Error
	return quota, err
}

// WalletSplit 一次扣费/退款在赠送与实充上的拆分。
type WalletSplit struct {
	Bonus int
	Paid  int
}

// CreditUserWallet 向赠送/实充加回额度（退款、调增），并写流水。
// 加款前对对应行 FOR UPDATE，避免并发加赠送丢更新。
func CreditUserWallet(userId int, bonus int, paid int, kind, reason, refType, refId string) error {
	return creditUserWallet(userId, bonus, paid, kind, reason, refType, refId, true, nil)
}

// creditUserWallet writeLedger=false 时只改余额不写流水（B-03 请求内预扣退回）；
// extra 非空时在同一事务内最后执行（B-03 冲减出资拆分）。
func creditUserWallet(userId int, bonus int, paid int, kind, reason, refType, refId string, writeLedger bool, extra func(tx *gorm.DB) error) error {
	if bonus < 0 || paid < 0 {
		return errors.New("credit amounts cannot be negative")
	}
	if bonus == 0 && paid == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return creditUserWalletTx(tx, userId, bonus, paid, kind, reason, refType, refId, writeLedger, extra)
	})
}

func creditUserWalletTx(tx *gorm.DB, userId int, bonus int, paid int, kind, reason, refType, refId string, writeLedger bool, extra func(tx *gorm.DB) error) error {
	if bonus < 0 || paid < 0 {
		return errors.New("credit amounts cannot be negative")
	}
	if bonus == 0 && paid == 0 {
		return nil
	}
	if err := EnsureUserBonusWallet(tx, userId); err != nil {
		return err
	}
	if bonus > 0 {
		var wallet UserBonusWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userId).First(&wallet).Error; err != nil {
			return err
		}
		if err := tx.Model(&UserBonusWallet{}).Where("user_id = ?", userId).
			Update("balance", gorm.Expr("balance + ?", bonus)).Error; err != nil {
			return err
		}
		var after int64
		if err := tx.Model(&UserBonusWallet{}).Where("user_id = ?", userId).Select("balance").Scan(&after).Error; err != nil {
			return err
		}
		if writeLedger {
			if err := writeLedgerEntryTx(tx, userId, "bonus", kind, int64(bonus), after, reason, refType, refId, 0); err != nil {
				return err
			}
		}
	}
	if paid > 0 {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		if err := increaseUserQuotaTx(tx, userId, paid); err != nil {
			return err
		}
		var after int
		if err := tx.Model(&User{}).Where("id = ?", userId).Select("quota").Scan(&after).Error; err != nil {
			return err
		}
		if writeLedger {
			if err := writeLedgerEntryTx(tx, userId, "paid", kind, int64(paid), int64(after), reason, refType, refId, 0); err != nil {
				return err
			}
		}
	}
	if extra != nil {
		if err := extra(tx); err != nil {
			return err
		}
	}
	// 使缓存失效，下次读取从 DB 水合合计（实充+赠送）
	_ = invalidateUserCache(userId)
	return nil
}

// IncreaseUserBonusQuota 增加赠送余额（充值赠送、兑换码、调额等）。
func IncreaseUserBonusQuota(userId int, amount int, kind, reason, refType, refId string) error {
	if amount < 0 {
		return errors.New("bonus amount cannot be negative")
	}
	if amount == 0 {
		return nil
	}
	return CreditUserWallet(userId, amount, 0, kind, reason, refType, refId)
}


func increaseUserQuotaTx(tx *gorm.DB, id int, quota int) error {
	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", id, common.MaxWalletQuota-quota).
		Update("quota", gorm.Expr("quota + ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var count int64
	if err := tx.Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrWalletQuotaLimitExceeded
}

// ApplyUserQuotaCacheDelta 调整缓存中的合计余额（Redis 未开则空操作）。
func ApplyUserQuotaCacheDelta(userId int, delta int64) {
	if delta == 0 {
		return
	}
	if err := cacheIncrUserQuota(userId, delta); err != nil {
		common.SysLog(common.LogText("failed to apply user quota cache delta: %s", err.Error()))
	}
}
