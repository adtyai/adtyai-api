package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// B-03（选 B）：API 扣费按实际出资钱包记账。
//
// AdtyWalletConsumeHourly 每用户每小时从赠送 / 实充各净扣了多少 quota。
// 写入点（与改余额同一事务）：
//   - tryReserveUserWalletDB（预扣 / 缓存落库）      +bonus,+paid
//   - DecreaseUserWallet（结算补扣）                 +bonus,+paid
//   - ReturnReservedUserWallet（请求内多扣退回 / 整笔退预扣，不写流水） −bonus,−paid
//
// 事后退款（RefundUserWalletAfterTheFact，任务/MJ 失败）只写流水，不改本表。
//
// 小时桶取余额变动发生时刻（UTC 整点）。小时汇总按此表拆 bonus/paid 两行，
// 合计仍以 logs 为准，差额记 paid 并打日志（见 planHourlyRollupSplit）。
type AdtyWalletConsumeHourly struct {
	UserId     int       `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	HourBucket time.Time `gorm:"primaryKey" json:"hour_bucket"`
	Bonus      int64     `gorm:"not null;default:0" json:"bonus"`
	Paid       int64     `gorm:"not null;default:0" json:"paid"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (AdtyWalletConsumeHourly) TableName() string { return "adty_wallet_consume_hourly" }

// adtyWalletConsumeNow 测试 / accept 可替换（决定出资记到哪个小时）。
var adtyWalletConsumeNow = func() time.Time { return time.Now().UTC() }

// SetWalletConsumeNowForTest accept 用：固定出资记账时刻；传 nil 恢复真实时间。
func SetWalletConsumeNowForTest(f func() time.Time) {
	if f == nil {
		f = func() time.Time { return time.Now().UTC() }
	}
	adtyWalletConsumeNow = f
}

// recordWalletConsumeTx 在调用方事务内累加（可为负）。bonus/paid 均为 0 时不写。
func recordWalletConsumeTx(tx *gorm.DB, userId int, bonus, paid int64) error {
	if bonus == 0 && paid == 0 {
		return nil
	}
	hb := TruncateToHourUTC(adtyWalletConsumeNow())
	row := AdtyWalletConsumeHourly{UserId: userId, HourBucket: hb, Bonus: bonus, Paid: paid, UpdatedAt: time.Now().UTC()}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "hour_bucket"}},
		DoUpdates: clause.Assignments(map[string]any{
			"bonus":      gorm.Expr("adty_wallet_consume_hourly.bonus + ?", bonus),
			"paid":       gorm.Expr("adty_wallet_consume_hourly.paid + ?", paid),
			"updated_at": row.UpdatedAt,
		}),
	}).Create(&row).Error
}

// loadWalletConsumeForHour 某小时各用户净拆分。
func loadWalletConsumeForHour(db *gorm.DB, hb time.Time) (map[int]AdtyWalletConsumeHourly, error) {
	var rows []AdtyWalletConsumeHourly
	if err := db.Where("hour_bucket = ?", TruncateToHourUTC(hb)).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int]AdtyWalletConsumeHourly, len(rows))
	for _, r := range rows {
		out[r.UserId] = r
	}
	return out, nil
}

// planHourlyRollupSplit 以 logs 合计 total 为准拆出 bonus/paid（均 ≥0，和 = total）。
// bonus 取拆分表净赠送，截到 [0,total]；其余记 paid。remainder = total −(拆分表 bonus+paid)，
// 非 0 表示拆分表与日志不一致（切换前数据、跨整点结算、批量模式等），调用方需打日志。
func planHourlyRollupSplit(total int64, split AdtyWalletConsumeHourly) (bonus, paid, remainder int64) {
	if total <= 0 {
		return 0, 0, total - (split.Bonus + split.Paid)
	}
	bonus = split.Bonus
	if bonus < 0 {
		bonus = 0
	}
	if bonus > total {
		bonus = total
	}
	paid = total - bonus
	remainder = total - (split.Bonus + split.Paid)
	return
}

// SplitRefundByRecorded 事后退款按记录的原拆分反向退：先退实充（≤recPaid），再退赠送（≤recBonus）。
// 没有记录（切换前的老任务，recBonus=recPaid=0）时 legacy=true，全部退赠送（保守口径：
// 宁可少给可提现本金，不让赠送变现金）。超出记录合计的余数同样退赠送。
func SplitRefundByRecorded(amount, recBonus, recPaid int) (bonus, paid int, legacy bool) {
	if amount <= 0 {
		return 0, 0, false
	}
	if recBonus <= 0 && recPaid <= 0 {
		return amount, 0, true
	}
	paid = amount
	if recPaid < 0 {
		recPaid = 0
	}
	if paid > recPaid {
		paid = recPaid
	}
	bonus = amount - paid
	return bonus, paid, false
}
