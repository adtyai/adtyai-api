package model

// 站长发件域名（任务 11.3）：一个站最多一个。只由 service/adty_site_mail.go 写。

// 发件域名的状态。
const (
	AdtySenderDomainPending  = "pending"  // 已在 Resend 建好，等站长加 DNS 记录
	AdtySenderDomainVerified = "verified" // Resend 验证通过，系统邮件从它发
	AdtySenderDomainFailed   = "failed"   // Resend 报验证失败，站长可以删掉重加
	AdtySenderDomainDisabled = "disabled" // 管理员停用或关站，已从 Resend 删除
)

// AdtySiteSenderDomain 是站长绑定的发件域名。删除或停用后行保留，同一个站再加时复用这一行。
type AdtySiteSenderDomain struct {
	SiteId         int    `json:"site_id" gorm:"primaryKey;autoIncrement:false"`
	Domain         string `json:"domain" gorm:"type:varchar(253);index"`
	ResendDomainId string `json:"resend_domain_id" gorm:"type:varchar(64)"`
	Status         string `json:"status" gorm:"type:varchar(16);index"`
	Records        string `json:"records" gorm:"type:text"` // Resend 返回的 DNS 记录（JSON 数组），给站长看
	DisabledBy     int    `json:"disabled_by"`              // 管理员停用时记下操作人
	LastCheckedAt  int64  `json:"last_checked_at"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtySiteSenderDomain) TableName() string { return "adty_site_sender_domains" }
