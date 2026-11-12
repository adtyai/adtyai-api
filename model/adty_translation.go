package model

const (
	AdtyTranslationReady   = "ready"
	AdtyTranslationFailed  = "failed"
	AdtyTranslationPending = "pending"
	AdtyTranslationModeOwn = "independent"
	AdtyTranslationModeSet = "platform"
)

// AdtyTranslationConfig is the Root translator setting. The credential ciphertext is never returned.
type AdtyTranslationConfig struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	Mode         string `json:"mode" gorm:"type:varchar(16)"`
	Protocol     string `json:"protocol" gorm:"type:varchar(16)"`
	BaseURL      string `json:"base_url" gorm:"type:varchar(256)"`
	ModelName    string `json:"model_name" gorm:"type:varchar(80)"`
	BackupModel  string `json:"backup_model" gorm:"type:varchar(80)"`
	TokenCipher  string `json:"-" gorm:"type:text"`
	TokenMask    string `json:"token_mask" gorm:"type:varchar(16)"`
	TimeoutSec   int    `json:"timeout_sec"`
	Concurrency  int    `json:"concurrency"`
	MaxChars     int    `json:"max_chars"`
	DailyQuota   int    `json:"daily_quota"`
	RetryLimit   int    `json:"retry_limit"`
	Enabled      bool   `json:"enabled"`
	SavedAt      int64  `json:"saved_at"`
	TestedAt     int64  `json:"tested_at"`
	TranslatedAt int64  `json:"translated_at"`
}

func (AdtyTranslationConfig) TableName() string { return "adty_translation_configs" }

// AdtyTranslationHead is the current English for one dynamic object. A late job cannot overwrite a newer version.
type AdtyTranslationHead struct {
	Id          int64  `json:"id"`
	ObjectKey   string `json:"object_key" gorm:"uniqueIndex;type:varchar(160)"`
	SiteId      int    `json:"site_id" gorm:"index"`
	Version     int    `json:"version"`
	SourceHash  string `json:"source_hash" gorm:"type:varchar(64)"`
	Source      string `json:"source" gorm:"type:text"`
	English     string `json:"english" gorm:"type:text"`
	Status      string `json:"status" gorm:"type:varchar(16);index"`
	HumanEdited bool   `json:"human_edited"`
	ModelName   string `json:"model_name" gorm:"type:varchar(80)"`
	Notice      string `json:"notice" gorm:"type:varchar(80)"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyTranslationHead) TableName() string { return "adty_translation_heads" }

// AdtyTranslationJob is one attempt. Completing an older version is refused.
type AdtyTranslationJob struct {
	Id         int64  `json:"id"`
	ObjectKey  string `json:"object_key" gorm:"uniqueIndex:idx_adty_tr_job,priority:1;type:varchar(160)"`
	Version    int    `json:"version" gorm:"uniqueIndex:idx_adty_tr_job,priority:2"`
	SourceHash string `json:"source_hash" gorm:"type:varchar(64)"`
	Status     string `json:"status" gorm:"type:varchar(16)"`
	ModelName  string `json:"model_name" gorm:"type:varchar(80)"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
	FinishedAt int64  `json:"finished_at"`
}

func (AdtyTranslationJob) TableName() string { return "adty_translation_jobs" }

// AdtyTranslationUsage counts system-task calls. It is not a customer or site-owner debit.
type AdtyTranslationUsage struct {
	Id        int64  `json:"id"`
	Day       string `json:"day" gorm:"uniqueIndex;type:varchar(10)"`
	Calls     int    `json:"calls"`
	Failures  int    `json:"failures"`
	LatencyMs int64  `json:"latency_ms"`
	CostUSD   string `json:"cost_usd" gorm:"type:varchar(32)"`
}

func (AdtyTranslationUsage) TableName() string { return "adty_translation_usages" }
