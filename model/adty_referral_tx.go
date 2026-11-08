package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

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
	if quota <= 0 || quota > int64(common.MaxWalletQuota) {
		return 0, errors.New("credit quota must be positive")
	}
	var user User
	if err := lockForUpdate(tx).First(&user, userId).Error; err != nil {
		return 0, err
	}
	balance, err := userWalletBalance(user.Quota, user.QuotaPending)
	if err != nil {
		return 0, err
	}
	return int64(balance), applyUserQuotaDeltaTx(tx, &user, int(quota), false)
}

// AdtySyncCreditUserQuotaCache 在加额度的事务提交后同步缓存。
func AdtySyncCreditUserQuotaCache(userId int, quota int64, operation string) {
	if quota <= 0 {
		return
	}
	if err := invalidateUserCache(userId); err != nil {
		common.SysError(common.LogText("failed to sync %s credit to user quota cache: %s", operation, err.Error()))
		recordWalletCacheReview(userId, quota, operation, err)
		return
	}
}

func recordWalletCacheReview(userID int, delta int64, operation string, cause error) {
	if DB == nil || userID <= 0 {
		return
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	event := &PaymentOrphanEvent{
		Provider:    "wallet",
		EventID:     fmt.Sprintf("wallet-cache-%d-%d", userID, time.Now().UnixNano()),
		EventType:   "wallet_cache_sync",
		ReferenceID: strconv.Itoa(userID),
		Status:      PaymentOrphanStatusPendingReview,
		Reason:      operation + " cache update failed",
		Error:       message,
		Payload:     fmt.Sprintf(`{"user_id":%d,"delta":%d}`, userID, delta),
		CreateTime:  common.GetTimestamp(),
		UpdateTime:  common.GetTimestamp(),
	}
	if err := DB.Create(event).Error; err != nil {
		common.SysError(common.LogText("failed to record wallet cache divergence for user %d: %s", userID, err.Error()))
	}
}

// AdtyRecordUserLog 给用户写一条充值类日志（佣金转余额用）。
func AdtyRecordUserLog(userId int, content *common.Message, callerIp string) {
	RecordTopupLog(userId, content, callerIp, "referral", "referral")
}
