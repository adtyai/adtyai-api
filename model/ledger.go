package model

import (
	"time"

	"gorm.io/gorm"
)

// LedgerEntry 账本流水。主键 (id, created_at)，按 created_at 月分区；只增不改。
// API 扣费不逐条记；由 hourly_rollup 按小时汇总写入（hour_bucket=整点，created_at=hour_bucket）。
// account: paid | bonus | aff 等。
type LedgerEntry struct {
	Id           int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId       int        `json:"user_id" gorm:"index;not null"`
	Account      string     `json:"account" gorm:"type:varchar(16);not null"` // paid | bonus | aff
	Kind         string     `json:"kind" gorm:"type:varchar(64);not null"`
	Delta        int64      `json:"delta" gorm:"type:bigint;not null"`
	BalanceAfter int64      `json:"balance_after" gorm:"type:bigint;not null"`
	Reason       string     `json:"reason" gorm:"type:text"`
	RefType      string     `json:"ref_type" gorm:"type:varchar(64)"`
	RefId        string     `json:"ref_id" gorm:"type:varchar(128)"`
	HourBucket   *time.Time `json:"hour_bucket" gorm:"index"`
	OperatorId   int        `json:"operator_id" gorm:"default:0"`
	CreatedAt    time.Time  `json:"created_at" gorm:"primaryKey;autoCreateTime;index"`
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

// LedgerKindAPIConsumeHourly 小时汇总扣费 kind。
const LedgerKindAPIConsumeHourly = "api_consume_hourly"

// LedgerIdempotency 流水幂等键（不分区）。
type LedgerIdempotency struct {
	Key       string    `json:"key" gorm:"primaryKey;type:varchar(191)"`
	EntryId   int64     `json:"entry_id"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (LedgerIdempotency) TableName() string { return "ledger_idempotency" }

func createLedgerEntryTx(tx *gorm.DB, userId int, account, kind string, delta, balanceAfter int64, reason, refType, refId string, operatorId int) (*LedgerEntry, error) {
	entry := LedgerEntry{
		UserId:       userId,
		Account:      account,
		Kind:         kind,
		Delta:        delta,
		BalanceAfter: balanceAfter,
		Reason:       reason,
		RefType:      refType,
		RefId:        refId,
		OperatorId:   operatorId,
		CreatedAt:    time.Now().UTC(),
	}
	if err := tx.Create(&entry).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

func writeLedgerEntryTx(tx *gorm.DB, userId int, account, kind string, delta, balanceAfter int64, reason, refType, refId string, operatorId int) error {
	_, err := createLedgerEntryTx(tx, userId, account, kind, delta, balanceAfter, reason, refType, refId, operatorId)
	return err
}

// createHourlyRollupLedgerTx 写入小时汇总行：created_at = hour_bucket；依赖唯一键防重。
func createHourlyRollupLedgerTx(tx *gorm.DB, userId int, account string, delta, balanceAfter int64, hourBucket time.Time) (*LedgerEntry, error) {
	hb := hourBucket.UTC().Truncate(time.Hour)
	entry := LedgerEntry{
		UserId:       userId,
		Account:      account,
		Kind:         LedgerKindAPIConsumeHourly,
		Delta:        delta,
		BalanceAfter: balanceAfter,
		Reason:       "hourly api consume rollup",
		RefType:      "hourly_rollup",
		RefId:        hb.Format(time.RFC3339),
		HourBucket:   &hb,
		OperatorId:   0,
		CreatedAt:    hb,
	}
	if err := tx.Create(&entry).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}
