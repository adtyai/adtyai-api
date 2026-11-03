package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdtyTriggerSendingLeaseSeconds 是 sending 占位的有效期。超过这个时间还没写成 sent 或 failed，就当作中断，下一轮可以重试。
const AdtyTriggerSendingLeaseSeconds int64 = 15 * 60

// 自动触发邮件（任务 12.3）的发送记录：同一用户同一规则只发一次，靠主键保证。

const (
	AdtyTriggerUnpaid     = "unpaid"      // 调用过但没充值
	AdtyTriggerLowBalance = "low_balance" // 付费用户余额快用完
	AdtyTriggerWinback    = "winback"     // 付费用户召回
)

// AdtyMarketingTriggerSend 是一条规则发给一个用户的记录。Status 是 sending、sent 或 failed；
// 先写 sending 占住，发完改 sent；失败的下一轮重试（Resend 的 Idempotency-Key 防止重复发出）。
type AdtyMarketingTriggerSend struct {
	Rule      string `json:"rule" gorm:"primaryKey;type:varchar(32)"`
	UserId    int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	Status    string `json:"status" gorm:"type:varchar(16);not null"`
	EmailId   string `json:"email_id" gorm:"type:varchar(64)"`
	CreatedAt int64  `json:"created_at" gorm:"not null;index"`
}

func (AdtyMarketingTriggerSend) TableName() string { return "adty_marketing_trigger_sends" }

func init() {
	adtyMarketingTables = append(adtyMarketingTables, &AdtyMarketingTriggerSend{})
}

// AdtyTriggerCandidate 是规则选出的一个人。
type AdtyTriggerCandidate struct {
	UserId   int
	Username string
	Email    string
	Lang     string
	SiteId   int
	Quota    int64
	Setting  string
}

// AdtyTriggerCandidateFilter 是规则的条件。时间是秒级时间戳。
type AdtyTriggerCandidateFilter struct {
	Rule              string
	RegisteredBefore  int64 // unpaid：注册早于这个时间
	BalanceBelowQuota int64 // low_balance：余额低于这个额度
	IdleSince         int64 // winback：这个时间以后没有调用
	Now               int64 // 这一轮的当前时间，用来判断 sending 占位是否已经过期
	Limit             int
}

// AdtyTriggerCandidates 选出符合规则、可以发营销邮件、还没发过这条规则的人。
func AdtyTriggerCandidates(f AdtyTriggerCandidateFilter) ([]AdtyTriggerCandidate, error) {
	consumed := "EXISTS (SELECT 1 FROM logs WHERE logs.user_id = users.id AND logs.type = ?)"
	paid := "EXISTS (SELECT 1 FROM top_ups WHERE top_ups.user_id = users.id AND top_ups.status = ?)"
	query := DB.Table("adty_marketing_contacts").
		Joins("JOIN users ON users.id = adty_marketing_contacts.user_id AND users.deleted_at IS NULL").
		Where("users.status = ?", common.UserStatusEnabled).
		Scopes(AdtyMarketingSendableScope).
		Where("NOT EXISTS (SELECT 1 FROM adty_marketing_trigger_sends s WHERE s.user_id = users.id AND s.rule = ? AND (s.status = ? OR (s.status = ? AND s.created_at > ?)))",
			f.Rule, "sent", "sending", f.Now-AdtyTriggerSendingLeaseSeconds)
	switch f.Rule {
	case AdtyTriggerUnpaid:
		query = query.Where(consumed, LogTypeConsume).Where("NOT "+paid, common.TopUpStatusSuccess).
			Where("users.created_at <= ?", f.RegisteredBefore)
	case AdtyTriggerLowBalance:
		query = query.Where(paid, common.TopUpStatusSuccess).Where("users.quota < ?", f.BalanceBelowQuota)
	case AdtyTriggerWinback:
		query = query.Where(paid, common.TopUpStatusSuccess).Where(consumed, LogTypeConsume).
			Where("NOT EXISTS (SELECT 1 FROM logs WHERE logs.user_id = users.id AND logs.type = ? AND logs.created_at >= ?)", LogTypeConsume, f.IdleSince)
	default:
		return nil, nil
	}
	var rows []AdtyTriggerCandidate
	err := query.Select("users.id AS user_id, users.username, adty_marketing_contacts.email, adty_marketing_contacts.lang, users.site_id, users.quota, users.setting").
		Order("users.id").Limit(f.Limit).Scan(&rows).Error
	return rows, err
}

// ClaimAdtyTriggerSend 占住一条发送：没有记录或上次失败时返回 true。
func ClaimAdtyTriggerSend(rule string, userId int, now int64) (bool, error) {
	var claimed bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&AdtyMarketingTriggerSend{Rule: rule, UserId: userId, Status: "sending", CreatedAt: now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			claimed = true
			return nil
		}
		retry := tx.Model(&AdtyMarketingTriggerSend{}).
			Where("rule = ? AND user_id = ? AND (status = ? OR (status = ? AND created_at <= ?))", rule, userId, "failed", "sending", now-AdtyTriggerSendingLeaseSeconds).
			Updates(map[string]any{"status": "sending", "created_at": now, "email_id": ""})
		claimed = retry.RowsAffected > 0
		return retry.Error
	})
	return claimed, err
}

// FinishAdtyTriggerSend 记下发送结果。
func FinishAdtyTriggerSend(rule string, userId int, emailId string, ok bool) error {
	status := "sent"
	if !ok {
		status = "failed"
	}
	return DB.Model(&AdtyMarketingTriggerSend{}).Where("rule = ? AND user_id = ?", rule, userId).
		Updates(map[string]any{"status": status, "email_id": emailId}).Error
}
