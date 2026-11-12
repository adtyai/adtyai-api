package model

const (
	AdtyGroupHealthRequest     = "request"
	AdtyGroupHealthProbe       = "probe"
	AdtyGroupHealthSuccess     = "success"
	AdtyGroupHealthUpstream    = "upstream_fail"
	AdtyGroupHealthCustomer    = "customer_refuse"
	AdtyGroupHealthNoData      = "no_data"
	AdtyGroupHealthInterrupted = "interrupted"
	AdtyGroupHealthOK          = "ok"
	AdtyGroupHealthDegraded    = "degraded"
)

// AdtyGroupHealthSample is one finished user request or one probe. It stores no channel or credential.
type AdtyGroupHealthSample struct {
	Id        int64  `json:"id"`
	SiteId    int    `json:"site_id" gorm:"index"`
	GroupName string `json:"group_name" gorm:"type:varchar(64);index"`
	ModelName string `json:"model_name" gorm:"type:varchar(80);index"`
	Kind      string `json:"kind" gorm:"type:varchar(16);index"`
	Outcome   string `json:"outcome" gorm:"type:varchar(24)"`
	CreatedAt int64  `json:"created_at" gorm:"index"`
}

func (AdtyGroupHealthSample) TableName() string { return "adty_group_health_samples" }

// AdtyGroupHealthState is the collector heartbeat and Root threshold. It does not change token groups.
type AdtyGroupHealthState struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	HeartbeatAt  int64  `json:"heartbeat_at"`
	SuccessMin   string `json:"success_min" gorm:"type:varchar(16)"`
	StaleSeconds int    `json:"stale_seconds"`
	UpdatedAt    int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyGroupHealthState) TableName() string { return "adty_group_health_states" }

// AdtyGroupHealthAlert is a notice that a group crossed the threshold. It does not reroute keys.
type AdtyGroupHealthAlert struct {
	Id        int64  `json:"id"`
	GroupName string `json:"group_name" gorm:"type:varchar(64);index"`
	SiteId    int    `json:"site_id" gorm:"index"`
	Kind      string `json:"kind" gorm:"type:varchar(16)"`
	Open      bool   `json:"open" gorm:"index"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyGroupHealthAlert) TableName() string { return "adty_group_health_alerts" }
