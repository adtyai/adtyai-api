package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	FirstTopupStatusClaimed       = "claimed"
	FirstTopupStatusPendingReview = "pending_review"
	FirstTopupStatusRejected      = "rejected"
)

// FirstTopupClaim 首充领取记录（账号唯一；device_fp 部分唯一索引见 EnsureFirstTopupClaimIndexes）。
type FirstTopupClaim struct {
	Id        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int       `json:"user_id" gorm:"uniqueIndex;not null"`
	DeviceFp  *string   `json:"device_fp" gorm:"type:varchar(128)"` // 空/缺省存 nil，不进唯一约束
	IP        string    `json:"ip" gorm:"type:varchar(64)"`
	OrderId   int64     `json:"order_id" gorm:"index"`
	OrderNo   string    `json:"order_no" gorm:"type:varchar(64);index"`
	Status    string    `json:"status" gorm:"type:varchar(32);not null;default:'claimed'"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (FirstTopupClaim) TableName() string { return "first_topup_claims" }

// EnsureFirstTopupClaimIndexes 建部分唯一索引（忽略 null/空串）。仅 PostgreSQL。
func EnsureFirstTopupClaimIndexes(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return nil
	}
	// 幂等：IF NOT EXISTS
	return db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_first_topup_claims_device_fp_partial
ON first_topup_claims (device_fp)
WHERE device_fp IS NOT NULL AND device_fp <> ''
`).Error
}

// UserHasFirstTopupRecord 账号是否已有首充记录（claimed / pending_review 都算，二次充值不再享首充）。
func UserHasFirstTopupRecord(userId int) (bool, error) {
	var n int64
	err := DB.Model(&FirstTopupClaim{}).Where("user_id = ?", userId).Count(&n).Error
	return n > 0, err
}

// UserHasClaimedFirstTopup 账号是否已成功领取首充（claimed）。
func UserHasClaimedFirstTopup(userId int) (bool, error) {
	var n int64
	err := DB.Model(&FirstTopupClaim{}).
		Where("user_id = ? AND status = ?", userId, FirstTopupStatusClaimed).
		Count(&n).Error
	return n > 0, err
}

// DeviceFpClaimed 非空指纹是否已被占用（任意状态记录均算占用，含 pending_review）。
func DeviceFpClaimed(deviceFp string) (bool, error) {
	if deviceFp == "" {
		return false, nil
	}
	var n int64
	err := DB.Model(&FirstTopupClaim{}).
		Where("device_fp = ?", deviceFp).
		Count(&n).Error
	return n > 0, err
}

// IPHasClaimedFirstTopup 是否有已领记录同 IP。
func IPHasClaimedFirstTopup(ip string) (bool, error) {
	if ip == "" {
		return false, nil
	}
	var n int64
	err := DB.Model(&FirstTopupClaim{}).
		Where("ip = ? AND status = ?", ip, FirstTopupStatusClaimed).
		Count(&n).Error
	return n > 0, err
}

// InsertFirstTopupClaimTx 写入领取记录；device_fp 空则存 NULL。
// 唯一冲突（user_id / device_fp）必须返回错误，由调用方事务回滚，禁止 DoNothing 静默成功。
func InsertFirstTopupClaimTx(tx *gorm.DB, userId int, deviceFp, ip, orderNo string, orderId int64, status string) error {
	var fp *string
	if deviceFp != "" {
		fp = &deviceFp
	}
	row := FirstTopupClaim{
		UserId:   userId,
		DeviceFp: fp,
		IP:       ip,
		OrderId:  orderId,
		OrderNo:  orderNo,
		Status:   status,
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	return nil
}

var ErrFirstTopupAlreadyClaimed = errors.New("first topup already claimed")
