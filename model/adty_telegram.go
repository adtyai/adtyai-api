package model

const (
	AdtyTelegramPurposeUser  = "user"
	AdtyTelegramPurposeAdmin = "admin"
	AdtyTelegramBindActive   = "active"
	AdtyTelegramBindConfirm  = "needs_reconfirm"
	AdtyTelegramDeliveryWait = "pending"
	AdtyTelegramDeliverySent = "sent"
	AdtyTelegramDeliveryFail = "failed"
	AdtyTelegramDeliveryDead = "dead"
	AdtyTelegramDeliverySkip = "skipped"
)

// AdtyTelegramBot is one of the two notification bots. The token ciphertext is never returned.
type AdtyTelegramBot struct {
	Id               int64  `json:"id"`
	Purpose          string `json:"purpose" gorm:"uniqueIndex;type:varchar(16);not null"`
	BotId            int64  `json:"bot_id"`
	Username         string `json:"username" gorm:"type:varchar(64)"`
	TokenCipher      string `json:"-" gorm:"type:text"`
	TokenMask        string `json:"token_mask" gorm:"type:varchar(16)"`
	WebhookHash      string `json:"-" gorm:"type:varchar(64)"`
	IdentityHash     string `json:"-" gorm:"type:varchar(64)"`
	Enabled          bool   `json:"enabled"`
	IdentityVerified bool   `json:"identity_verified"`
	SavedAt          int64  `json:"saved_at"`
	TestedAt         int64  `json:"tested_at"`
}

func (AdtyTelegramBot) TableName() string { return "adty_telegram_bots" }

// AdtyTelegramChallenge is a one-time website bind credential. The plaintext is not stored.
type AdtyTelegramChallenge struct {
	Id               int64  `json:"id"`
	UserId           int    `json:"user_id" gorm:"index"`
	SiteId           int    `json:"site_id"`
	Purpose          string `json:"purpose" gorm:"type:varchar(16);index"`
	CodeHash         string `json:"-" gorm:"uniqueIndex;type:varchar(64);not null"`
	BotIdentity      string `json:"-" gorm:"type:varchar(64)"`
	TelegramUserId   int64  `json:"telegram_user_id"`
	TelegramUsername string `json:"telegram_username" gorm:"type:varchar(64)"`
	ExpiresAt        int64  `json:"expires_at" gorm:"index"`
	SeenAt           int64  `json:"seen_at"`
	UsedAt           int64  `json:"used_at"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTelegramChallenge) TableName() string { return "adty_telegram_challenges" }

// AdtyTelegramBinding stores a notification relationship. It is not a login identity.
type AdtyTelegramBinding struct {
	Id             int64  `json:"id"`
	UserId         int    `json:"user_id" gorm:"uniqueIndex:idx_adty_tg_bind,priority:1"`
	Purpose        string `json:"purpose" gorm:"uniqueIndex:idx_adty_tg_bind,priority:2;type:varchar(16)"`
	SiteId         int    `json:"site_id" gorm:"index"`
	TelegramUserId int64  `json:"telegram_user_id" gorm:"index"`
	BotIdentity    string `json:"-" gorm:"type:varchar(64)"`
	Status         string `json:"status" gorm:"type:varchar(24);index"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTelegramBinding) TableName() string { return "adty_telegram_bindings" }

// AdtyTelegramDelivery is one outbound notice. A retry updates this row and does not create business side effects.
type AdtyTelegramDelivery struct {
	Id        int64  `json:"id"`
	EventKey  string `json:"event_key" gorm:"uniqueIndex;type:varchar(128)"`
	UserId    int    `json:"user_id" gorm:"index"`
	Purpose   string `json:"purpose" gorm:"type:varchar(16)"`
	SiteId    int    `json:"site_id"`
	BindingId int64  `json:"binding_id"`
	Status    string `json:"status" gorm:"type:varchar(16);index"`
	Attempts  int    `json:"attempts"`
	Body      string `json:"body" gorm:"type:text"`
	LastError string `json:"last_error" gorm:"type:varchar(128)"`
	Public    bool   `json:"public"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTelegramDelivery) TableName() string { return "adty_telegram_deliveries" }
