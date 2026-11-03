package model

// 分站的自定义域名（任务 11.8）。只由 service/adty_cloudflare_saas.go 写。

// 自定义域名的状态。
const (
	AdtySiteDomainPending = "pending" // 已在 Cloudflare 建好，等站长加 CNAME、等证书
	AdtySiteDomainActive  = "active"  // status 和 ssl.status 都是 active，参与站点识别
	AdtySiteDomainFailed  = "failed"  // 72 小时还没通，站长可以重试
	AdtySiteDomainRemoved = "removed" // 已从 Cloudflare 和 Turnstile 删除，保留存档
)

// AdtySiteDomain 是站长绑定的一个自定义域名。主机名全局唯一（删除后行保留为 removed，
// 同一个主机名再加时复用这一行）。
type AdtySiteDomain struct {
	Id              int    `json:"id"`
	SiteId          int    `json:"site_id" gorm:"index"`
	Hostname        string `json:"hostname" gorm:"uniqueIndex;type:varchar(253)"`
	OwnershipToken  string `json:"-" gorm:"type:varchar(64)"` // TXT 归属令牌，通过后才在 Cloudflare 建主机名
	CFHostnameId    string `json:"cf_hostname_id" gorm:"type:varchar(64)"`
	Status          string `json:"status" gorm:"type:varchar(16);index"`
	SSLStatus       string `json:"ssl_status" gorm:"type:varchar(32)"`
	VerifyRecords   string `json:"verify_records" gorm:"type:text"` // 给站长看的 DNS 记录（JSON 数组）
	TurnstileWidget string `json:"turnstile_widget" gorm:"type:varchar(64)"`
	LastError       string `json:"last_error" gorm:"type:text"` // 最近一次 Cloudflare 报错，给站长和后台看
	LastCheckedAt   int64  `json:"last_checked_at"`
	CreatedAt       int64  `json:"created_at"` // 这次（或重试）开始验证的时间，72 小时超时从这里算
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtySiteDomain) TableName() string { return "adty_site_domains" }
