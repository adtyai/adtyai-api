package smartrouting

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const OptionKey = "SmartRouting.Config"

// WeightParts five-dimension weights.
type WeightParts struct {
	Conn  float64 `json:"conn"`
	Price float64 `json:"price"`
	Speed float64 `json:"speed"`
	Stab  float64 `json:"stab"`
	Auth  float64 `json:"auth"`
}

func (w WeightParts) Sum() float64 {
	return w.Conn + w.Price + w.Speed + w.Stab + w.Auth
}

func DefaultWeights() WeightParts {
	return WeightParts{Conn: 30, Price: 25, Speed: 20, Stab: 15, Auth: 10}
}

// Config is the live smart-routing configuration (options JSON).
type Config struct {
	Enabled                 bool        `json:"enabled"`
	Mode                    string      `json:"mode"` // off | observe | active
	ActiveWeightScheme      string      `json:"active_weight_scheme"`
	Weights                 WeightParts `json:"weights"`
	ScoreRefreshSec         int         `json:"score_refresh_sec"`
	FlushSec                int         `json:"flush_sec"`
	ScoreStaleSec           int         `json:"score_stale_sec"`
	EWMAShortMin            int         `json:"ewma_short_min"`
	EWMALongMin             int         `json:"ewma_long_min"`
	EWMAShortWeight         float64     `json:"ewma_short_weight"`
	MinSamples              int         `json:"min_samples"`
	FloorShare              float64     `json:"floor_share"`
	FloorTotalMax           float64     `json:"floor_total_max"`
	FloorDecayCycles        int         `json:"floor_decay_cycles"`
	HalfOpenShareStage      []float64   `json:"halfopen_share_stage"`
	HalfOpenSuccessNeeded   int         `json:"halfopen_success_needed"`
	HalfOpenStage2Stable    int         `json:"halfopen_stage2_stable_cycles"`
	HalfOpenMaxInflight     int         `json:"halfopen_max_inflight"`
	HalfOpenPreferNonStream bool        `json:"halfopen_prefer_nonstream"`
	EpsilonGreedyEnabled    bool        `json:"epsilon_greedy_enabled"`
	Epsilon                 float64     `json:"epsilon"`
	PeerWorseBreakEnabled   bool        `json:"peer_worse_break_enabled"`
	PeerWorseGap            float64     `json:"peer_worse_gap"`
	StickySoftEnabled       bool        `json:"sticky_soft_enabled"`
	TelegramAlertEnabled    bool        `json:"telegram_alert_enabled"`
	RegionFilterEnabled     bool        `json:"region_filter_enabled"`
	ScoreTrafficPct         int         `json:"score_traffic_pct"`
	RetryAfterMaxWaitMs     int         `json:"retry_after_max_wait_ms"`
	RateLimitCooldownSec    int         `json:"rate_limit_cooldown_sec"`
	SampleCapPerToken       float64     `json:"sample_cap_per_token"`
	UserFailMeltCap         float64     `json:"user_fail_melt_cap"`
	RetryBudgetPct          int         `json:"retry_budget_pct"`
	DailyCostGuardMult      float64     `json:"daily_cost_guard_mult"`
	TokenSkewSuspect        float64     `json:"token_skew_suspect"`
	TokenSkewFail           float64     `json:"token_skew_fail"`
	EstimateWeight          float64     `json:"estimate_weight"`
	MinPriceSamples         float64     `json:"min_price_samples"`
	PriceScoreSkipNonToken  bool        `json:"price_score_skip_non_token"`
	HistoryBackfillEnabled  bool        `json:"history_backfill_enabled"`
	BalancePatrolEnabled    bool        `json:"balance_patrol_enabled"`
	BalanceLowUSD           float64     `json:"balance_low_usd"`
	CooldownInitialSec      int         `json:"cooldown_initial_sec"`
	CooldownMaxSec          int         `json:"cooldown_max_sec"`
	ConsecutiveFailBreak    int         `json:"consecutive_fail_break"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:                 true,
		Mode:                    "active",
		ActiveWeightScheme:      "balanced_default",
		Weights:                 DefaultWeights(),
		ScoreRefreshSec:         300,
		FlushSec:                60,
		ScoreStaleSec:           600,
		EWMAShortMin:            5,
		EWMALongMin:             60,
		EWMAShortWeight:         0.6,
		MinSamples:              20,
		FloorShare:              0.03,
		FloorTotalMax:           0.30,
		FloorDecayCycles:        3,
		HalfOpenShareStage:      []float64{0.02, 0.10, 1.0},
		HalfOpenSuccessNeeded:   5,
		HalfOpenStage2Stable:    1,
		HalfOpenMaxInflight:     2,
		HalfOpenPreferNonStream: true,
		EpsilonGreedyEnabled:    false,
		Epsilon:                 0.01,
		PeerWorseBreakEnabled:   false,
		PeerWorseGap:            25,
		StickySoftEnabled:       false,
		TelegramAlertEnabled:    false,
		RegionFilterEnabled:     false,
		ScoreTrafficPct:         100,
		RetryAfterMaxWaitMs:     2000,
		RateLimitCooldownSec:    45,
		SampleCapPerToken:       0.20,
		UserFailMeltCap:         0.40,
		RetryBudgetPct:          20,
		DailyCostGuardMult:      3,
		TokenSkewSuspect:        0.15,
		TokenSkewFail:           0.40,
		EstimateWeight:          0.3,
		MinPriceSamples:         10,
		PriceScoreSkipNonToken:  true,
		HistoryBackfillEnabled:  false,
		BalancePatrolEnabled:    true,
		BalanceLowUSD:           5,
		CooldownInitialSec:      120,
		CooldownMaxSec:          3600,
		ConsecutiveFailBreak:    8,
	}
}

var (
	cfgAtomic               atomic.Value // Config
	cfgMu                   sync.Mutex
	configLoadedFromOptions atomic.Bool
)

func init() {
	cfgAtomic.Store(DefaultConfig())
}

func GetConfig() Config {
	v, _ := cfgAtomic.Load().(Config)
	return v
}

func Enabled() bool {
	return GetConfig().Mode == "active"
}

// Collects reports whether samples and score refresh should run.
// observe records without changing dispatch; off does neither.
func Collects() bool {
	m := GetConfig().Mode
	return m == "observe" || m == "active"
}

func SetConfig(c Config) {
	normalizeConfig(&c)
	cfgAtomic.Store(c)
}

func normalizeConfig(c *Config) {
	switch c.Mode {
	case "off", "observe", "active":
	default:
		if c.Enabled {
			c.Mode = "active"
		} else {
			c.Mode = "off"
		}
	}
	c.Enabled = c.Mode == "active"
	d := DefaultConfig()
	if c.ActiveWeightScheme == "" {
		c.ActiveWeightScheme = d.ActiveWeightScheme
	}
	if c.Weights.Sum() <= 0 {
		c.Weights = d.Weights
	}
	if c.ScoreRefreshSec <= 0 {
		c.ScoreRefreshSec = d.ScoreRefreshSec
	}
	if c.FlushSec <= 0 {
		c.FlushSec = d.FlushSec
	}
	if c.ScoreStaleSec <= 0 {
		c.ScoreStaleSec = d.ScoreStaleSec
	}
	if c.EWMAShortMin <= 0 {
		c.EWMAShortMin = d.EWMAShortMin
	}
	if c.EWMALongMin <= 0 {
		c.EWMALongMin = d.EWMALongMin
	}
	if c.EWMAShortWeight <= 0 || c.EWMAShortWeight >= 1 {
		c.EWMAShortWeight = d.EWMAShortWeight
	}
	if c.MinSamples <= 0 {
		c.MinSamples = d.MinSamples
	}
	if c.FloorShare <= 0 {
		c.FloorShare = d.FloorShare
	}
	if c.FloorTotalMax <= 0 {
		c.FloorTotalMax = d.FloorTotalMax
	}
	if c.FloorDecayCycles <= 0 {
		c.FloorDecayCycles = d.FloorDecayCycles
	}
	if len(c.HalfOpenShareStage) < 3 {
		c.HalfOpenShareStage = d.HalfOpenShareStage
	}
	if c.HalfOpenSuccessNeeded <= 0 {
		c.HalfOpenSuccessNeeded = d.HalfOpenSuccessNeeded
	}
	if c.HalfOpenMaxInflight <= 0 {
		c.HalfOpenMaxInflight = d.HalfOpenMaxInflight
	}
	if c.Epsilon <= 0 {
		c.Epsilon = d.Epsilon
	}
	if c.ScoreTrafficPct <= 0 || c.ScoreTrafficPct > 100 {
		c.ScoreTrafficPct = 100
	}
	if c.RetryAfterMaxWaitMs <= 0 {
		c.RetryAfterMaxWaitMs = d.RetryAfterMaxWaitMs
	}
	if c.RateLimitCooldownSec <= 0 {
		c.RateLimitCooldownSec = d.RateLimitCooldownSec
	}
	if c.SampleCapPerToken <= 0 {
		c.SampleCapPerToken = d.SampleCapPerToken
	}
	if c.UserFailMeltCap <= 0 {
		c.UserFailMeltCap = d.UserFailMeltCap
	}
	if c.RetryBudgetPct <= 0 {
		c.RetryBudgetPct = d.RetryBudgetPct
	}
	if c.DailyCostGuardMult <= 0 {
		c.DailyCostGuardMult = d.DailyCostGuardMult
	}
	if c.EstimateWeight <= 0 {
		c.EstimateWeight = d.EstimateWeight
	}
	if c.MinPriceSamples <= 0 {
		c.MinPriceSamples = d.MinPriceSamples
	}
	if c.CooldownInitialSec <= 0 {
		c.CooldownInitialSec = d.CooldownInitialSec
	}
	if c.CooldownMaxSec <= 0 {
		c.CooldownMaxSec = d.CooldownMaxSec
	}
	if c.ConsecutiveFailBreak <= 0 {
		c.ConsecutiveFailBreak = d.ConsecutiveFailBreak
	}
	if c.PeerWorseGap <= 0 {
		c.PeerWorseGap = d.PeerWorseGap
	}
	if c.BalanceLowUSD <= 0 {
		c.BalanceLowUSD = d.BalanceLowUSD
	}
}

// LoadConfigFromOptions reads SmartRouting.Config from OptionMap / DB.
// A missing key stays at the install default only before the first successful load.
// Bad JSON before that load uses the active default and must not force off.
// After a load, bad JSON or a deleted key keeps the last good config.
func LoadConfigFromOptions() {
	common.OptionMapRWMutex.RLock()
	raw, ok := common.OptionMap[OptionKey]
	common.OptionMapRWMutex.RUnlock()
	if !ok || raw == "" {
		if !configLoadedFromOptions.Load() {
			SetConfig(DefaultConfig())
		} else {
			common.SysError("smartrouting: config key missing; keeping the last loaded config")
		}
		return
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawMap); err != nil {
		common.SysError(common.LogText("smartrouting: bad config json: %v", err))
		keepOrDisableConfig()
		configLoadedFromOptions.Store(true)
		return
	}
	var c Config
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		common.SysError(common.LogText("smartrouting: bad config json: %v", err))
		keepOrDisableConfig()
		configLoadedFromOptions.Store(true)
		return
	}
	applyOmittedDefaults(&c, rawMap)
	SetConfig(c)
	configLoadedFromOptions.Store(true)
}

func keepOrDisableConfig() {
	if configLoadedFromOptions.Load() {
		return
	}
	SetConfig(DefaultConfig())
}

func applyOmittedDefaults(c *Config, raw map[string]json.RawMessage) {
	d := DefaultConfig()
	if _, ok := raw["halfopen_prefer_nonstream"]; !ok {
		c.HalfOpenPreferNonStream = d.HalfOpenPreferNonStream
	}
	if _, ok := raw["price_score_skip_non_token"]; !ok {
		c.PriceScoreSkipNonToken = d.PriceScoreSkipNonToken
	}
	if _, ok := raw["balance_patrol_enabled"]; !ok {
		c.BalancePatrolEnabled = d.BalancePatrolEnabled
	}
}

// SaveConfig persists config to options and memory.
func SaveConfig(c Config, operatorID int) error {
	normalizeConfig(&c)
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if err := model.UpdateOption(OptionKey, string(b)); err != nil {
		return err
	}
	SetConfig(c)
	configLoadedFromOptions.Store(true)
	_ = model.SaveWeightSnapshot("config", operatorID, map[string]any{"op": "save_config"}, c)
	return nil
}

// Init wires hooks and loads config. Safe to call multiple times.
func Init() {
	LoadConfigFromOptions()
	loadBreakerStages()
	RepairScoreBrokenStatus()
	model.RefreshRouteState()
	model.RegisterSmartWeightHook(SmartWeights)
	model.RegisterOnChannelSelectedHook(OnChannelSelected)
	model.RegisterOnHalfOpenProbeHook(ReserveHalfOpenProbe)
	model.RegisterHalfOpenFallbackHook(HalfOpenFallback)
	model.RegisterOnChannelManuallyEnabledHook(ResetBreakerOnManualEnable)
	loadPublishedScores()
	StartHalfOpenLoop()
	EnsureDefaultSchemeInDB()
}

// OnChannelSelected accepts a normal weighted pick. Probe slots are reserved only by ReserveHalfOpenProbe.
func OnChannelSelected(channelID int) bool {
	return true
}

// ReserveHalfOpenProbe takes one probe slot. A full quota rejects the probe instead of a normal pick.
func ReserveHalfOpenProbe(channelID int) bool {
	return TryIncHalfOpenInflight(channelID, GetConfig().HalfOpenMaxInflight)
}

func EnsureDefaultSchemeInDB() {
	_ = model.SeedDefaultWeightScheme(model.DB)
}
