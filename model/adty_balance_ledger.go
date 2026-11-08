package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AdtyLedgerTypeRefund             = "refund"
	AdtyLedgerTypeManualAdjust       = "manual_adjust"
	AdtyLedgerTypeCommissionTransfer = "commission_transfer"
	AdtyLedgerTypeSiteTrial          = "site_trial" // 站长送的试用额度（P11）
)

var ErrAdtyIdempotencyConflict = errors.New("idempotency key reused for a different operation")

// AdtyBalanceLedger 是余额流水：退款、人工调额、佣金转余额（任务 3.5、7.2、6.4）。
// Reason 只给后台看，不写进用户自己的日志。
type AdtyBalanceLedger struct {
	Id             int64   `json:"id"`
	UserId         int     `json:"user_id" gorm:"index"`
	DeltaQuota     int64   `json:"delta_quota"`
	BalanceBefore  int64   `json:"balance_before"`
	BalanceAfter   int64   `json:"balance_after"`
	Type           string  `json:"type" gorm:"type:varchar(32);index"`
	RefType        string  `json:"ref_type" gorm:"type:varchar(32)"`
	RefId          string  `json:"ref_id" gorm:"type:varchar(128);index"`
	Reason         string  `json:"reason" gorm:"type:text"`
	OperatorId     int     `json:"operator_id"`
	IdempotencyKey *string `json:"-" gorm:"uniqueIndex;type:varchar(64)"`
	CreatedAt      int64   `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyBalanceLedger) TableName() string {
	return "adty_balance_ledgers"
}

// InsertAdtyLedgerTx 写一条流水。
func InsertAdtyLedgerTx(tx *gorm.DB, entry *AdtyBalanceLedger) error {
	if entry.IdempotencyKey != nil && strings.TrimSpace(*entry.IdempotencyKey) == "" {
		entry.IdempotencyKey = nil
	}
	return tx.Create(entry).Error
}

// FindAdtyLedgerByIdemKeyTx 按幂等键查流水；没有时返回 nil。先查后写，不靠唯一键冲突判断：
// PostgreSQL 冲突后整个事务作废（审阅重点 10）。
func FindAdtyLedgerByIdemKeyTx(tx *gorm.DB, key string) (*AdtyBalanceLedger, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	var rows []AdtyBalanceLedger
	if err := tx.Where("idempotency_key = ?", key).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// ListAdtyLedgers 分页列出一个用户的余额流水。
func ListAdtyLedgers(userId int, pageInfo *common.PageInfo) ([]AdtyBalanceLedger, int64, error) {
	query := DB.Model(&AdtyBalanceLedger{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AdtyBalanceLedger
	err := query.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error
	return rows, total, err
}

// lockUserQuotaTx 锁用户行，返回当前额度（入账、退款、调额都最后才锁用户行）。
func lockUserQuotaTx(tx *gorm.DB, userId int) (int64, error) {
	balance, _, err := lockUserQuotaForRefundTx(tx, userId)
	return balance, err
}

// lockUserQuotaForRefundTx 锁用户行，返回余额和还在钱包里的签到赠送。
func lockUserQuotaForRefundTx(tx *gorm.DB, userId int) (balance, checkin int64, err error) {
	var users []User
	if err := lockForUpdate(tx).Select("id", "quota", "quota_pending", "checkin_quota").Where("id = ?", userId).Limit(1).Find(&users).Error; err != nil {
		return 0, 0, err
	}
	if len(users) == 0 {
		return 0, 0, gorm.ErrRecordNotFound
	}
	balanceInt, err := userWalletBalance(users[0].Quota, users[0].QuotaPending)
	if err != nil {
		return 0, 0, err
	}
	return int64(balanceInt), int64(max(0, min(users[0].CheckinQuota, balanceInt))), nil
}
