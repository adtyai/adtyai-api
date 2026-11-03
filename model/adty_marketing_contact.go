package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdtyMarketingContact 是一个用户的营销邮件状态（任务 12.1），一个用户一行。
// 只有 Consent 且没退订、没退信、没投诉的人同步到 Resend。退订、退信、投诉一旦标记就不再清除，以后永不再发；
// 退订的人在个人设置里重新勾选同意可以再收。系统邮件（验证码、找回密码、充值到账、额度不足）不看这张表。
// UpdatedAt 是本地状态最后一次变化的毫秒时间，SyncedAt 是最后同步到 Resend 时的 UpdatedAt；UpdatedAt > SyncedAt 就要再同步。
type AdtyMarketingContact struct {
	UserId          int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	Email           string `json:"email" gorm:"type:varchar(255);index"`
	Consent         bool   `json:"consent" gorm:"not null"`
	ConsentAt       int64  `json:"consent_at" gorm:"not null"`
	ConsentIpHash   string `json:"-" gorm:"type:varchar(64)"`
	ConsentLang     string `json:"consent_lang" gorm:"type:varchar(16)"`
	ConsentVersion  string `json:"consent_version" gorm:"type:varchar(32)"`
	ConsentSource   string `json:"consent_source" gorm:"type:varchar(16)"` // register | profile
	Unsubscribed    bool   `json:"unsubscribed" gorm:"not null"`
	UnsubscribedAt  int64  `json:"unsubscribed_at" gorm:"not null"`
	Bounced         bool   `json:"bounced" gorm:"not null"`
	Complained      bool   `json:"complained" gorm:"not null"`
	ResendContactId string `json:"-" gorm:"type:varchar(64);index"`
	SyncedEmail     string `json:"-" gorm:"type:varchar(255)"` // Resend 上这个联系人的邮箱；本地邮箱变了要删掉重建
	Lang            string `json:"lang" gorm:"type:varchar(16)"`
	SiteId          int    `json:"site_id" gorm:"index;not null"`
	SyncedAt        int64  `json:"synced_at" gorm:"not null"`
	UpdatedAt       int64  `json:"updated_at" gorm:"index;not null;autoUpdateTime:false"`
}

func (AdtyMarketingContact) TableName() string { return "adty_marketing_contacts" }

// Sendable：同意了、没退订、没退信、没投诉，邮箱不为空。
func (c AdtyMarketingContact) Sendable() bool {
	return c.Consent && !c.Unsubscribed && !c.Bounced && !c.Complained && strings.TrimSpace(c.Email) != ""
}

// AdtyMarketingSendableScope 是 Sendable 的 SQL 版本，筛收件人时用。
func AdtyMarketingSendableScope(db *gorm.DB) *gorm.DB {
	return db.Where("adty_marketing_contacts.consent = ? AND adty_marketing_contacts.unsubscribed = ? AND adty_marketing_contacts.bounced = ? AND adty_marketing_contacts.complained = ? AND adty_marketing_contacts.email <> ''",
		true, false, false, false)
}

// adtyMarketingStamp 返回新的变化时间：毫秒，并且一定大于旧值，同一毫秒里的两次变化也分得开。
func adtyMarketingStamp(previous int64) int64 {
	return max(time.Now().UnixMilli(), previous+1)
}

// AdtyMarketingWebhookEvent 记下处理过的 webhook（按 svix-id 去重，Resend 重试时 id 不变）。
// 群发统计按 BroadcastId 和 Type 数这张表（任务 12.2）。
type AdtyMarketingWebhookEvent struct {
	Id          string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	Type        string `json:"type" gorm:"type:varchar(48);index:idx_adty_mkt_event_broadcast,priority:2"`
	BroadcastId string `json:"broadcast_id" gorm:"type:varchar(64);index:idx_adty_mkt_event_broadcast,priority:1"`
	EmailId     string `json:"email_id" gorm:"type:varchar(64)"`
	Email       string `json:"email" gorm:"type:varchar(255)"`
	CreatedAt   int64  `json:"created_at" gorm:"index"`
}

func (AdtyMarketingWebhookEvent) TableName() string { return "adty_marketing_webhook_events" }

// adtyMarketingTables 是 P12 的表，后面的任务往里加。
var adtyMarketingTables = []any{&AdtyMarketingContact{}, &AdtyMarketingWebhookEvent{}}

// migrateAdtyMarketingTables 建 P12 的表（MigrateAdtyTables 调用）。
func migrateAdtyMarketingTables() error {
	return DB.AutoMigrate(adtyMarketingTables...)
}

// AdtyMarketingConsentInput 是写同意状态的参数。
type AdtyMarketingConsentInput struct {
	UserId  int
	Email   string
	Consent bool
	IpHash  string
	Lang    string
	Version string
	Source  string
	SiteId  int
}

// SaveAdtyMarketingConsent 写同意状态（有就改）。改成同意时重记时间、IP 哈希、语言和文案版本；
// 重新同意会解除退订（用户自己的明确操作），退信、投诉的标记不动。
func SaveAdtyMarketingConsent(in AdtyMarketingConsentInput) (*AdtyMarketingContact, error) {
	now := time.Now().Unix()
	var saved AdtyMarketingContact
	err := DB.Transaction(func(tx *gorm.DB) error {
		var rows []AdtyMarketingContact
		if err := lockForUpdate(tx).Where("user_id = ?", in.UserId).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			saved = AdtyMarketingContact{UserId: in.UserId, SiteId: in.SiteId}
		} else {
			saved = rows[0]
		}
		saved.Email = strings.ToLower(strings.TrimSpace(in.Email))
		if in.Lang != "" {
			saved.Lang = in.Lang
		}
		if in.Consent && (!saved.Consent || saved.Unsubscribed) {
			saved.ConsentAt, saved.ConsentIpHash, saved.ConsentLang, saved.ConsentVersion, saved.ConsentSource = now, in.IpHash, in.Lang, in.Version, in.Source
			saved.Unsubscribed, saved.UnsubscribedAt = false, 0
		}
		if !in.Consent && saved.Consent {
			saved.ConsentAt, saved.ConsentSource = now, in.Source
		}
		saved.Consent = in.Consent
		saved.UpdatedAt = adtyMarketingStamp(saved.UpdatedAt)
		if len(rows) == 0 {
			return tx.Create(&saved).Error
		}
		return tx.Save(&saved).Error
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// GetAdtyMarketingContact 取一个用户的营销状态，没有时返回 nil。
func GetAdtyMarketingContact(userId int) (*AdtyMarketingContact, error) {
	var rows []AdtyMarketingContact
	if err := DB.Where("user_id = ?", userId).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// AdtyMarketingFlag 是 webhook 能打的永久标记。
type AdtyMarketingFlag string

const (
	AdtyMarketingFlagUnsubscribed AdtyMarketingFlag = "unsubscribed"
	AdtyMarketingFlagBounced      AdtyMarketingFlag = "bounced"
	AdtyMarketingFlagComplained   AdtyMarketingFlag = "complained"
)

// MarkAdtyMarketingByEmailTx 在调用方的事务里按邮箱打永久标记（不分大小写），返回改到的行数。
func MarkAdtyMarketingByEmailTx(tx *gorm.DB, email string, flag AdtyMarketingFlag) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, nil
	}
	switch flag {
	case AdtyMarketingFlagUnsubscribed, AdtyMarketingFlagBounced, AdtyMarketingFlagComplained:
	default:
		return 0, errors.New("unknown marketing flag")
	}
	var rows []AdtyMarketingContact
	if err := lockForUpdate(tx).Where("email = ? AND "+string(flag)+" = ?", email, false).Find(&rows).Error; err != nil {
		return 0, err
	}
	for _, row := range rows {
		updates := map[string]any{string(flag): true, "updated_at": adtyMarketingStamp(row.UpdatedAt)}
		if flag == AdtyMarketingFlagUnsubscribed {
			updates["unsubscribed_at"] = time.Now().Unix()
		}
		if err := tx.Model(&AdtyMarketingContact{}).Where("user_id = ?", row.UserId).Updates(updates).Error; err != nil {
			return 0, err
		}
	}
	return int64(len(rows)), nil
}

// MarkAdtyMarketingByEmail 是不带事务的 MarkAdtyMarketingByEmailTx。
func MarkAdtyMarketingByEmail(email string, flag AdtyMarketingFlag) (int64, error) {
	var affected int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		affected, err = MarkAdtyMarketingByEmailTx(tx, email, flag)
		return err
	})
	return affected, err
}

// MarkAdtyMarketingUserUnsubscribed 只把这一个用户标成退订。
// 退订令牌签的是用户 id。联系人表里的邮箱可能还是改绑前的旧值，按邮箱打标会把后来占用这个邮箱的人一起退订。
func MarkAdtyMarketingUserUnsubscribed(userId int) error {
	if userId <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var rows []AdtyMarketingContact
		if err := lockForUpdate(tx).Where("user_id = ?", userId).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 || rows[0].Unsubscribed {
			return nil
		}
		return tx.Model(&AdtyMarketingContact{}).Where("user_id = ?", userId).Updates(map[string]any{
			"unsubscribed":    true,
			"unsubscribed_at": time.Now().Unix(),
			"updated_at":      adtyMarketingStamp(rows[0].UpdatedAt),
		}).Error
	})
}

// RecordAdtyMarketingWebhookEventTx 记下一个 webhook；已经处理过（同一个 svix-id）时返回 false。
func RecordAdtyMarketingWebhookEventTx(tx *gorm.DB, event AdtyMarketingWebhookEvent) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// AdtyMarketingContactsToSync 取最多 limit 个本地状态变了的联系人。
func AdtyMarketingContactsToSync(limit int) ([]AdtyMarketingContact, error) {
	var rows []AdtyMarketingContact
	err := DB.Where("updated_at > synced_at").Order("updated_at").Limit(limit).Find(&rows).Error
	return rows, err
}

// MarkAdtyMarketingContactSynced 记下同步结果。syncedStamp 是同步时读到的 UpdatedAt，
// 同步期间状态又变了的话 UpdatedAt 更大，下一轮还会同步。resendContactId 为空表示 Resend 上已经没有这个人。
func MarkAdtyMarketingContactSynced(userId int, syncedStamp int64, resendContactId, syncedEmail string) error {
	return DB.Model(&AdtyMarketingContact{}).Where("user_id = ?", userId).Updates(map[string]any{
		"synced_at": syncedStamp, "resend_contact_id": resendContactId, "synced_email": syncedEmail,
	}).Error
}

// RefreshAdtyMarketingContacts 找出用户表那边变了、联系人表还不知道的人，标记待同步，最多 limit 个：
// 改了邮箱的（顺便写进新邮箱）；在 Resend 上还有联系人、但账号已删除或停用的。
func RefreshAdtyMarketingContacts(limit int) error {
	var changed []struct {
		UserId    int
		Email     string
		UpdatedAt int64
	}
	if err := DB.Table("adty_marketing_contacts").
		Select("adty_marketing_contacts.user_id, users.email, adty_marketing_contacts.updated_at").
		Joins("JOIN users ON users.id = adty_marketing_contacts.user_id").
		Where("LOWER(COALESCE(users.email, '')) <> adty_marketing_contacts.email").
		Limit(limit).Scan(&changed).Error; err != nil {
		return err
	}
	for _, row := range changed {
		if err := DB.Model(&AdtyMarketingContact{}).Where("user_id = ?", row.UserId).Updates(map[string]any{
			"email": strings.ToLower(strings.TrimSpace(row.Email)), "updated_at": adtyMarketingStamp(row.UpdatedAt),
		}).Error; err != nil {
			return err
		}
	}
	var gone []AdtyMarketingContact
	if err := DB.Model(&AdtyMarketingContact{}).
		Joins("LEFT JOIN users ON users.id = adty_marketing_contacts.user_id").
		Where("adty_marketing_contacts.resend_contact_id <> '' AND adty_marketing_contacts.updated_at <= adty_marketing_contacts.synced_at").
		Where("users.id IS NULL OR users.deleted_at IS NOT NULL OR users.status <> ?", common.UserStatusEnabled).
		Limit(limit).Find(&gone).Error; err != nil {
		return err
	}
	for _, row := range gone {
		if err := DB.Model(&AdtyMarketingContact{}).Where("user_id = ?", row.UserId).
			Update("updated_at", adtyMarketingStamp(row.UpdatedAt)).Error; err != nil {
			return err
		}
	}
	return nil
}
