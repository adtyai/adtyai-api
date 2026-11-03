package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// 推广返佣在 service 包里要用到的事务工具（任务 6.4）。lockForUpdate 未导出，这里导出一个包装，
// 旧代码在 service 里直接写 clause.Locking 的地方统一改用它。

// AdtyLockForUpdate 给查询加行锁（SQLite 下跳过，和 lockForUpdate 一致）。
func AdtyLockForUpdate(tx *gorm.DB) *gorm.DB {
	return lockForUpdate(tx)
}

// AdtyCreditUserQuotaTx 在调用方的事务里给用户加额度，返回加之前的额度（写余额流水用）。
// 提交后调用方要调 AdtySyncCreditUserQuotaCache。
func AdtyCreditUserQuotaTx(tx *gorm.DB, userId int, quota int64) (before int64, err error) {
	if quota <= 0 {
		return 0, errors.New("credit quota must be positive")
	}
	before, err = lockUserQuotaTx(tx, userId)
	if err != nil {
		return 0, err
	}
	if before+quota > int64(common.MaxWalletQuota) {
		return 0, ErrWalletQuotaLimitExceeded
	}
	err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", quota)).Error
	return before, err
}

// AdtySyncCreditUserQuotaCache 在加额度的事务提交后同步缓存。
func AdtySyncCreditUserQuotaCache(userId int, quota int64, operation string) {
	syncCreditUserQuotaCache(userId, int(quota), operation)
}

// AdtyRecordUserLog 给用户写一条充值类日志（佣金转余额用）。
func AdtyRecordUserLog(userId int, content *common.Message, callerIp string) {
	RecordTopupLog(userId, content, callerIp, "referral", "referral")
}
