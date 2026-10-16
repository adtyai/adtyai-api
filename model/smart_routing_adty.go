package model

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- Smart routing tables (adtyai) ----

// ChannelRouteWeight stores per channel×model route_weight; never writes channels.weight.
type ChannelRouteWeight struct {
	ChannelID  int       `json:"channel_id" gorm:"primaryKey"`
	Model      string    `json:"model" gorm:"primaryKey;type:text;size:191"`
	RouteWeight int      `json:"route_weight" gorm:"not null;default:100"`
	Score      float64   `json:"score" gorm:"not null;default:70"`
	State      string    `json:"state" gorm:"type:text;size:32;default:'normal'"` // normal|degraded|broken|halfopen|new|floor
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ChannelRouteWeight) TableName() string { return "channel_route_weights" }

// ChannelScoreSnapshot latest score breakdown per channel×model.
type ChannelScoreSnapshot struct {
	ChannelID  int       `json:"channel_id" gorm:"primaryKey"`
	Model      string    `json:"model" gorm:"primaryKey;type:text;size:191"`
	Score      float64   `json:"score"`
	SConn      float64   `json:"s_conn"`
	SPrice     float64   `json:"s_price"`
	SSpeed     float64   `json:"s_speed"`
	SStab      float64   `json:"s_stab"`
	SAuth      float64   `json:"s_auth"`
	Samples    int       `json:"samples"`
	Insufficient bool    `json:"insufficient"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ChannelScoreSnapshot) TableName() string { return "channel_score_snapshots" }

// ChannelRouteTag: locked | excluded | fallback_only | fake_model
type ChannelRouteTag struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ChannelID int       `json:"channel_id" gorm:"index;not null"`
	Model     string    `json:"model" gorm:"type:text;size:191;default:'*'"`
	Tag       string    `json:"tag" gorm:"type:text;size:32;index;not null"`
	Note      string    `json:"note" gorm:"type:text"`
	CreatedBy int       `json:"created_by"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (ChannelRouteTag) TableName() string { return "channel_route_tags" }

// ChannelCapability hand-filled capability metadata.
type ChannelCapability struct {
	ChannelID           int       `json:"channel_id" gorm:"primaryKey"`
	SupportsTools       *bool     `json:"supports_tools"`
	SupportsVision      *bool     `json:"supports_vision"`
	SupportsJSONSchema  *bool     `json:"supports_json_schema"`
	MaxContext          int       `json:"max_context"`
	RPMHint             int       `json:"rpm_hint"`
	MaxInflight         int       `json:"max_inflight"`
	Region              string    `json:"region" gorm:"type:text;size:64"`
	DataPolicy          string    `json:"data_policy" gorm:"type:text;size:64"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ChannelCapability) TableName() string { return "channel_capabilities" }

// ChannelCostPrice cost entry: default ratio mode (recharge rate + channel ratio).
type ChannelCostPrice struct {
	ChannelID               int       `json:"channel_id" gorm:"primaryKey"`
	Model                   string    `json:"model" gorm:"primaryKey;type:text;size:191"`
	PriceMode               string    `json:"price_mode" gorm:"type:text;size:16;default:'ratio'"` // ratio|absolute
	RechargeCreditsPerCNY   float64   `json:"recharge_credits_per_cny"`
	ChannelRatio            float64   `json:"channel_ratio"` // 0 => use channel model ratio
	ModelRefUSDPerMTok      float64   `json:"model_ref_usd_per_mtok"`
	InPriceUSD              float64   `json:"in_price_usd"`
	OutPriceUSD             float64   `json:"out_price_usd"`
	PriceCurrency           string    `json:"price_currency" gorm:"type:text;size:8;default:'USD'"`
	UpdatedAt               time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	UpdatedBy               int       `json:"updated_by"`
}

func (ChannelCostPrice) TableName() string { return "channel_cost_prices" }

// ChannelCostLedger per-attempt cost line.
type ChannelCostLedger struct {
	ID               int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	RequestID        string    `json:"request_id" gorm:"type:text;size:64;index"`
	LogID            int       `json:"log_id" gorm:"index"`
	ChannelID        int       `json:"channel_id" gorm:"index;not null"`
	Model            string    `json:"model" gorm:"type:text;size:191;index"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	CostCNY          float64   `json:"cost_cny"`
	PriceMode        string    `json:"price_mode" gorm:"type:text;size:16"`
	UsageSource      string    `json:"usage_source" gorm:"type:text;size:16"` // upstream|estimate
	AttemptFailed    bool      `json:"attempt_failed"`
	CacheHit         bool      `json:"cache_hit"`
	CreatedAt        time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (ChannelCostLedger) TableName() string { return "channel_cost_ledger" }

// ChannelCostGuard day cost guardrail state.
type ChannelCostGuard struct {
	ChannelID   int       `json:"channel_id" gorm:"primaryKey"`
	Day         string    `json:"day" gorm:"primaryKey;type:text;size:10"` // YYYY-MM-DD
	CostCNY     float64   `json:"cost_cny"`
	Excluded    bool      `json:"excluded"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ChannelCostGuard) TableName() string { return "channel_cost_guards" }

// ChannelBreakerEvent timeline.
type ChannelBreakerEvent struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ChannelID int       `json:"channel_id" gorm:"index;not null"`
	Model     string    `json:"model" gorm:"type:text;size:191"`
	FromState string    `json:"from_state" gorm:"type:text;size:32"`
	ToState   string    `json:"to_state" gorm:"type:text;size:32"`
	Reason    string    `json:"reason" gorm:"type:text"`
	Score     float64   `json:"score"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (ChannelBreakerEvent) TableName() string { return "channel_breaker_events" }

// ChannelAuthFlag passive authenticity flag.
type ChannelAuthFlag struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ChannelID    int       `json:"channel_id" gorm:"index;not null"`
	Model        string    `json:"model" gorm:"type:text;size:191;index"`
	ReqModel     string    `json:"req_model" gorm:"type:text"`
	RespModel    string    `json:"resp_model" gorm:"type:text"`
	TokLocal     int       `json:"tok_local"`
	TokUpstream  int       `json:"tok_upstream"`
	DevPct       float64   `json:"dev_pct"`
	Verdict      string    `json:"verdict" gorm:"type:text;size:16"` // suspect|fail
	FalsePositive bool     `json:"false_positive"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (ChannelAuthFlag) TableName() string { return "channel_auth_flags" }

// RoutingWeightScheme named weight scheme.
type RoutingWeightScheme struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Name      string    `json:"name" gorm:"type:text;size:64;uniqueIndex;not null"`
	Weights   string    `json:"weights" gorm:"type:text;not null"` // JSON {conn,price,speed,stab,auth}
	Note      string    `json:"note" gorm:"type:text"`
	CreatedBy int       `json:"created_by"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (RoutingWeightScheme) TableName() string { return "routing_weight_schemes" }

// RoutingWeightSnapshot config/weight change audit.
type RoutingWeightSnapshot struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Kind       string    `json:"kind" gorm:"type:text;size:32"` // config|scheme|tag|price|rollback
	OperatorID int       `json:"operator_id"`
	Diff       string    `json:"diff" gorm:"type:text"`
	Snapshot   string    `json:"snapshot" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (RoutingWeightSnapshot) TableName() string { return "routing_weight_snapshots" }

// ReconcileDaily daily two-ledger reconcile.
type ReconcileDaily struct {
	ID            int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Day           string    `json:"day" gorm:"type:text;size:10;uniqueIndex"`
	UserChargeCNY float64   `json:"user_charge_cny"`
	CostCNY       float64   `json:"cost_cny"`
	LogCount      int64     `json:"log_count"`
	LedgerCount   int64     `json:"ledger_count"`
	DiffCount     int64     `json:"diff_count"`
	Detail        string    `json:"detail" gorm:"type:text"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (ReconcileDaily) TableName() string { return "reconcile_daily" }

// ChannelBreakerState persisted breaker/half-open state (channel-level).
type ChannelBreakerState struct {
	ChannelID       int       `json:"channel_id" gorm:"primaryKey"`
	Stage           string    `json:"stage" gorm:"type:text;size:32;default:'closed'"` // closed|open|half_2|half_10|full
	CooldownUntil   time.Time `json:"cooldown_until"`
	CooldownSec     int       `json:"cooldown_sec" gorm:"default:120"`
	LowScoreStreak  int       `json:"low_score_streak"`
	HalfOpenOK      int       `json:"half_open_ok"`
	Stage2Since     time.Time `json:"stage2_since"`
	Reason          string    `json:"reason" gorm:"type:text"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ChannelBreakerState) TableName() string { return "channel_breaker_states" }

// EnsureSmartRoutingSchema migrates smart-routing tables and seeds default scheme.
func EnsureSmartRoutingSchema(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if db == nil {
		return fmt.Errorf("smart_routing: db nil")
	}
	if err := db.AutoMigrate(
		&ChannelRouteWeight{},
		&ChannelScoreSnapshot{},
		&ChannelRouteTag{},
		&ChannelCapability{},
		&ChannelCostPrice{},
		&ChannelCostLedger{},
		&ChannelCostGuard{},
		&ChannelBreakerEvent{},
		&ChannelAuthFlag{},
		&RoutingWeightScheme{},
		&RoutingWeightSnapshot{},
		&ReconcileDaily{},
		&ChannelBreakerState{},
	); err != nil {
		return fmt.Errorf("smart_routing migrate: %w", err)
	}
	return SeedDefaultWeightScheme(db)
}

// DefaultWeightJSON balanced_default 30/25/20/15/10.
const DefaultWeightJSON = `{"conn":30,"price":25,"speed":20,"stab":15,"auth":10}`

func SeedDefaultWeightScheme(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	var n int64
	if err := db.Model(&RoutingWeightScheme{}).Where("name = ?", "balanced_default").Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	row := RoutingWeightScheme{
		Name:      "balanced_default",
		Weights:   DefaultWeightJSON,
		Note:      "default balanced 30/25/20/15/10",
		CreatedBy: 0,
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func UpsertRouteWeight(channelID int, modelName string, rw int, score float64, state string) error {
	row := ChannelRouteWeight{
		ChannelID:   channelID,
		Model:       modelName,
		RouteWeight: rw,
		Score:       score,
		State:       state,
		UpdatedAt:   time.Now(),
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "model"}},
		DoUpdates: clause.AssignmentColumns([]string{"route_weight", "score", "state", "updated_at"}),
	}).Create(&row).Error
}

func UpsertScoreSnapshot(s ChannelScoreSnapshot) error {
	s.UpdatedAt = time.Now()
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "model"}},
		DoUpdates: clause.AssignmentColumns([]string{"score", "s_conn", "s_price", "s_speed", "s_stab", "s_auth", "samples", "insufficient", "updated_at"}),
	}).Create(&s).Error
}

func ListRouteTags(channelID int) ([]ChannelRouteTag, error) {
	var rows []ChannelRouteTag
	q := DB.Model(&ChannelRouteTag{})
	if channelID > 0 {
		q = q.Where("channel_id = ?", channelID)
	}
	err := q.Order("id desc").Find(&rows).Error
	return rows, err
}

func HasRouteTag(channelID int, modelName, tag string) bool {
	var n int64
	q := DB.Model(&ChannelRouteTag{}).Where("channel_id = ? AND tag = ?", channelID, tag)
	if modelName != "" && modelName != "*" {
		q = q.Where("model IN ?", []string{"*", modelName})
	}
	_ = q.Limit(1).Count(&n)
	return n > 0
}

func RecordBreakerEvent(channelID int, modelName, from, to, reason string, score float64) {
	_ = DB.Create(&ChannelBreakerEvent{
		ChannelID: channelID,
		Model:     modelName,
		FromState: from,
		ToState:   to,
		Reason:    reason,
		Score:     score,
	}).Error
}

func SaveWeightSnapshot(kind string, operatorID int, diff, snapshot any) error {
	d, _ := json.Marshal(diff)
	s, _ := json.Marshal(snapshot)
	return DB.Create(&RoutingWeightSnapshot{
		Kind:       kind,
		OperatorID: operatorID,
		Diff:       string(d),
		Snapshot:   string(s),
	}).Error
}

func CountScoreBrokenChannels() int64 {
	var n int64
	_ = DB.Model(&Channel{}).Where("status = ?", 4).Count(&n)
	return n
}

func CountFakeModelTags() int64 {
	var n int64
	_ = DB.Model(&ChannelRouteTag{}).Where("tag = ?", "fake_model").Count(&n)
	return n
}

func CountActiveCostGuards() int64 {
	var n int64
	day := time.Now().Format("2006-01-02")
	_ = DB.Model(&ChannelCostGuard{}).Where("day = ? AND excluded = ?", day, true).Count(&n)
	return n
}

func LatestReconcileDiffCount() int64 {
	var row ReconcileDaily
	if err := DB.Order("id desc").First(&row).Error; err != nil {
		return 0
	}
	return row.DiffCount
}
