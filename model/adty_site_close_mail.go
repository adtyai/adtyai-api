package model

// Official close-migration batch mail inventory (A.16.2).
// One campaign is recorded per closing site. Recipient rows are the failure
// list and retryable state; a later retry must not create a second campaign
// or re-send already successful mail.
const (
	AdtySiteCloseMailPending        = "pending"
	AdtySiteCloseMailSuccess        = "success"
	AdtySiteCloseMailFailed         = "failed"
	AdtySiteCloseMailRetryNone      = "none"
	AdtySiteCloseMailRetryRetryable = "retryable"
	AdtySiteCloseMailRetryComplete  = "complete"
)

// AdtySiteCloseMailBatch records one official close-migration mail campaign.
// It stores the target site, recipient count, success/fail counts, send time,
// and retry status required by A.16.2.
type AdtySiteCloseMailBatch struct {
	Id             int64  `json:"id"`
	SiteId         int    `json:"site_id" gorm:"uniqueIndex"`
	RecipientCount int    `json:"recipient_count"`
	SuccessCount   int    `json:"success_count"`
	FailCount      int    `json:"fail_count"`
	SentAt         int64  `json:"sent_at"`
	RetryStatus    string `json:"retry_status" gorm:"type:varchar(16);not null;default:none"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

func (AdtySiteCloseMailBatch) TableName() string { return "adty_site_close_mail_batches" }

// AdtySiteCloseMailRecipient is one recipient in a close-migration batch.
// Failed rows remain retryable. The stored email is for the operator failure
// list only and must never be placed in another recipient's message.
type AdtySiteCloseMailRecipient struct {
	Id            int64  `json:"id"`
	BatchId       int64  `json:"batch_id" gorm:"uniqueIndex:ux_adty_site_close_mail_recipient;index"`
	SiteId        int    `json:"site_id" gorm:"index"`
	UserId        int    `json:"user_id" gorm:"uniqueIndex:ux_adty_site_close_mail_recipient;index"`
	Username      string `json:"username" gorm:"type:varchar(64)"`
	Email         string `json:"email" gorm:"type:varchar(255)"`
	Status        string `json:"status" gorm:"type:varchar(16);index;not null;default:pending"`
	LastError     string `json:"last_error" gorm:"type:text"`
	AttemptCount  int    `json:"attempt_count"`
	LastAttemptAt int64  `json:"last_attempt_at"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

func (AdtySiteCloseMailRecipient) TableName() string { return "adty_site_close_mail_recipients" }
