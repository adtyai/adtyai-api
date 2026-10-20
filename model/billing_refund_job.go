package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	BillingRefundOpen     = "open"
	BillingRefundDue      = "refund_due"
	BillingRefundSettled  = "settled"
	BillingRefundRefunded = "refunded"

	// BillingHoldDeadAfterSeconds is how long an open hold must sit before a
	// dead node can be refunded. Two missed heartbeats are required.
	BillingHoldDeadAfterSeconds int64 = max(SystemInstanceStaleAfterSeconds*2, 180)
)

// ErrBillingRefundClosed means the hold was already settled or refunded.
var ErrBillingRefundClosed = errors.New("billing refund already closed")

// BillingRefundJob is the durable pre-consume record. A process crash after
// the hold is written can be refunded by the sweeper once the owning node is
// no longer heartbeating. A live node's open hold is never refunded.
type BillingRefundJob struct {
	RequestID             string `json:"request_id" gorm:"primaryKey;type:varchar(64)"`
	Status                string `json:"status" gorm:"type:varchar(16);index;not null"`
	Source                string `json:"source" gorm:"type:varchar(32)"`
	UserID                int    `json:"user_id" gorm:"index"`
	Bonus                 int    `json:"bonus"`
	Paid                  int    `json:"paid"`
	TokenID               int    `json:"token_id"`
	TokenKey              string `json:"-" gorm:"type:varchar(128)"`
	TokenQuota            int    `json:"token_quota"`
	Playground            bool   `json:"playground"`
	SubscriptionID        int    `json:"subscription_id"`
	SubscriptionRequestID string `json:"subscription_request_id" gorm:"type:varchar(64)"`
	ExtraReserved         int    `json:"extra_reserved"`
	NodeName              string `json:"node_name" gorm:"type:varchar(128);index"`
	CreatedAt             int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt             int64  `json:"updated_at" gorm:"bigint;index"`
}

func (BillingRefundJob) TableName() string { return "billing_refund_jobs" }

var (
	billingRefundSchemaMu sync.Mutex
	billingRefundReadyDB  *gorm.DB
)

func ensureBillingRefundSchema() error {
	billingRefundSchemaMu.Lock()
	defer billingRefundSchemaMu.Unlock()
	if DB == nil {
		return errors.New("database is not ready")
	}
	if billingRefundReadyDB == DB {
		return nil
	}
	if err := DB.AutoMigrate(&BillingRefundJob{}); err != nil {
		return err
	}
	billingRefundReadyDB = DB
	return nil
}

func resetBillingRefundSchemaForTest() {
	billingRefundSchemaMu.Lock()
	billingRefundReadyDB = nil
	billingRefundSchemaMu.Unlock()
}

// SaveOpenBillingHold inserts or updates an open hold. It does not reopen a
// hold that is already due, settled, or refunded.
func SaveOpenBillingHold(job *BillingRefundJob) error {
	if job == nil || strings.TrimSpace(job.RequestID) == "" {
		return errors.New("billing refund request id is empty")
	}
	if err := ensureBillingRefundSchema(); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing BillingRefundJob
		err := lockForUpdate(tx).Where("request_id = ?", job.RequestID).First(&existing).Error
		now := common.GetTimestamp()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := *job
			row.Status = BillingRefundOpen
			if row.CreatedAt == 0 {
				row.CreatedAt = now
			}
			row.UpdatedAt = now
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		if existing.Status != BillingRefundOpen {
			return fmt.Errorf("billing hold %s is %s", job.RequestID, existing.Status)
		}
		copyBillingHoldAmounts(&existing, job)
		existing.Status = BillingRefundOpen
		existing.UpdatedAt = now
		return tx.Save(&existing).Error
	})
}

// MarkBillingRefundDue records that this request must be refunded. A settled
// or already refunded hold returns ErrBillingRefundClosed.
func MarkBillingRefundDue(job *BillingRefundJob) error {
	if job == nil || strings.TrimSpace(job.RequestID) == "" {
		return errors.New("billing refund request id is empty")
	}
	if err := ensureBillingRefundSchema(); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing BillingRefundJob
		err := lockForUpdate(tx).Where("request_id = ?", job.RequestID).First(&existing).Error
		now := common.GetTimestamp()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := *job
			row.Status = BillingRefundDue
			row.CreatedAt = now
			row.UpdatedAt = now
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		if existing.Status == BillingRefundSettled || existing.Status == BillingRefundRefunded {
			return ErrBillingRefundClosed
		}
		copyBillingHoldAmounts(&existing, job)
		existing.Status = BillingRefundDue
		existing.UpdatedAt = now
		return tx.Save(&existing).Error
	})
}

// MarkBillingHoldSettled closes an open hold after funding settlement. A
// missing hold is not an error. A hold already marked due or refunded is.
func MarkBillingHoldSettled(requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	if err := ensureBillingRefundSchema(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	res := DB.Model(&BillingRefundJob{}).
		Where("request_id = ? AND status = ?", requestID, BillingRefundOpen).
		Updates(map[string]any{"status": BillingRefundSettled, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var existing BillingRefundJob
	err := DB.Where("request_id = ?", requestID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.Status == BillingRefundSettled {
		return nil
	}
	return ErrBillingRefundClosed
}

// ApplyBillingRefund credits a refund_due job once. Settled and already
// refunded jobs are no-ops. Open jobs are refused so a live request cannot be
// refunded by calling Apply directly.
func ApplyBillingRefund(requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return errors.New("billing refund request id is empty")
	}
	if err := ensureBillingRefundSchema(); err != nil {
		return err
	}
	var missingToken bool
	var userID, tokenID, tokenQuota int
	var tokenKey string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var job BillingRefundJob
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&job).Error; err != nil {
			return err
		}
		if job.Status == BillingRefundRefunded || job.Status == BillingRefundSettled {
			return nil
		}
		if job.Status != BillingRefundDue {
			return fmt.Errorf("billing refund %s is %s", requestID, job.Status)
		}
		if job.UserID > 0 && (job.Bonus > 0 || job.Paid > 0) {
			if err := creditReservedWalletTx(tx, job.UserID, job.Bonus, job.Paid); err != nil {
				return err
			}
			userID = job.UserID
		}
		if strings.TrimSpace(job.SubscriptionRequestID) != "" {
			if err := refundSubscriptionPreConsumeTx(tx, job.SubscriptionRequestID); err != nil {
				return err
			}
		}
		if job.ExtraReserved > 0 && job.SubscriptionID > 0 {
			if err := postConsumeUserSubscriptionDeltaTx(tx, job.SubscriptionID, -int64(job.ExtraReserved)); err != nil {
				return err
			}
		}
		job.Status = BillingRefundRefunded
		job.UpdatedAt = common.GetTimestamp()
		if err := tx.Model(&BillingRefundJob{}).Where("request_id = ?", job.RequestID).
			Updates(map[string]any{"status": job.Status, "updated_at": job.UpdatedAt}).Error; err != nil {
			return err
		}
		// Token update stays last so observers that only see the final statement
		// still observe the quota restore.
		if job.TokenQuota > 0 && !job.Playground && job.TokenID > 0 {
			var count int64
			if err := tx.Model(&Token{}).Where("id = ?", job.TokenID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				missingToken = true
			} else if err := increaseTokenQuotaTx(tx, job.TokenID, job.TokenQuota); err != nil {
				return err
			} else {
				tokenID = job.TokenID
				tokenKey = job.TokenKey
				tokenQuota = job.TokenQuota
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if userID > 0 {
		_ = invalidateUserCache(userID)
	}
	if missingToken {
		common.SysLog(common.LogText("billing refund %s skipped missing token", requestID))
	}
	if tokenQuota > 0 && common.RedisEnabled {
		if _, cacheErr := cacheApplyTokenQuotaDelta(tokenID, tokenKey, int64(tokenQuota)); cacheErr != nil {
			common.SysLog(common.LogText("billing refund token cache update failed: %s", cacheErr.Error()))
		}
	}
	return nil
}

// SweepBillingRefunds applies due jobs, then converts open holds owned by a
// dead node into due jobs and applies those. A live node is left alone.
func SweepBillingRefunds(now int64) (int, error) {
	if err := ensureBillingRefundSchema(); err != nil {
		return 0, err
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}
	applied := 0
	var due []BillingRefundJob
	if err := DB.Where("status = ?", BillingRefundDue).Order("updated_at asc").Limit(100).Find(&due).Error; err != nil {
		return 0, err
	}
	for _, job := range due {
		if err := ApplyBillingRefund(job.RequestID); err != nil {
			common.SysError(common.LogText("billing refund apply failed for %s: %s", job.RequestID, err.Error()))
			continue
		}
		applied++
	}
	cutoff := now - BillingHoldDeadAfterSeconds
	var open []BillingRefundJob
	if err := DB.Where("status = ? AND updated_at < ?", BillingRefundOpen, cutoff).Order("updated_at asc").Limit(100).Find(&open).Error; err != nil {
		return applied, err
	}
	for _, job := range open {
		alive, err := billingHoldOwnerAlive(job.NodeName, now)
		if err != nil {
			return applied, err
		}
		if alive {
			continue
		}
		res := DB.Model(&BillingRefundJob{}).
			Where("request_id = ? AND status = ?", job.RequestID, BillingRefundOpen).
			Updates(map[string]any{"status": BillingRefundDue, "updated_at": now})
		if res.Error != nil {
			common.SysError(common.LogText("billing refund mark due failed for %s: %s", job.RequestID, res.Error.Error()))
			continue
		}
		if res.RowsAffected == 0 {
			continue
		}
		common.SysLog(common.LogText("billing hold %s owned by dead node %q marked refund due", job.RequestID, job.NodeName))
		if err := ApplyBillingRefund(job.RequestID); err != nil {
			common.SysError(common.LogText("billing refund apply failed for %s: %s", job.RequestID, err.Error()))
			continue
		}
		applied++
	}
	return applied, nil
}

func billingHoldOwnerAlive(node string, now int64) (bool, error) {
	node = strings.TrimSpace(node)
	if node == "" {
		return false, nil
	}
	var instance SystemInstance
	err := DB.Where("node_name = ?", node).First(&instance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return now-instance.LastSeenAt <= SystemInstanceStaleAfterSeconds, nil
}

func creditReservedWalletTx(tx *gorm.DB, userID, bonus, paid int) error {
	if bonus == 0 && paid == 0 {
		return nil
	}
	return creditUserWalletTx(tx, userID, bonus, paid, "wallet_return", "return pre-consumed wallet", "billing", "", false, func(tx *gorm.DB) error {
		return recordWalletConsumeTx(tx, userID, -int64(bonus), -int64(paid))
	})
}

func copyBillingHoldAmounts(dst, src *BillingRefundJob) {
	dst.Source = src.Source
	dst.UserID = src.UserID
	dst.Bonus = src.Bonus
	dst.Paid = src.Paid
	dst.TokenID = src.TokenID
	dst.TokenKey = src.TokenKey
	dst.TokenQuota = src.TokenQuota
	dst.Playground = src.Playground
	dst.SubscriptionID = src.SubscriptionID
	dst.SubscriptionRequestID = src.SubscriptionRequestID
	dst.ExtraReserved = src.ExtraReserved
	if strings.TrimSpace(src.NodeName) != "" {
		dst.NodeName = src.NodeName
	}
}
