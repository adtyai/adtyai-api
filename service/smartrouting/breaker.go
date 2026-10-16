package smartrouting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type HalfOpenInfo struct {
	ChannelID int
	Model     string
	Stage     string // half_2 | half_10
}

var (
	rateLimitUntil        = map[int]time.Time{}
	rateLimitMu           sync.Mutex
	consecFails           = map[int]int{}
	consecFailMu          sync.Mutex
	halfOpenInflightLocal = map[int]int{}
	halfOpenInflightMu    sync.Mutex
)

func halfOpenOKKey(channelID int) string {
	return fmt.Sprintf("sr:halfopen:%d:ok", channelID)
}
func halfOpenInflightKey(channelID int) string {
	return fmt.Sprintf("sr:halfopen:%d:inflight", channelID)
}

// EvaluateBreaker applies score thresholds + peer-worse (optional).
func EvaluateBreaker(channelID int, modelName string, ent *ScoreEntry, cfg Config) {
	if ent == nil || ent.Insufficient {
		return
	}
	ch, err := model.GetChannelById(channelID, true)
	if err != nil || ch == nil {
		return
	}
	// never touch manual/auto-disabled
	if ch.Status == common.ChannelStatusManuallyDisabled || ch.Status == common.ChannelStatusAutoDisabled {
		return
	}
	autoBan := ch.GetAutoBan()

	st := loadBreakerState(channelID)

	// hysteresis: recovery to normal needs score>=65 handled in half-open path
	if ent.Score < 40 {
		st.LowScoreStreak++
	} else {
		st.LowScoreStreak = 0
	}

	if cfg.PeerWorseBreakEnabled && ent.Samples >= cfg.MinSamples {
		if gap := peerMedianGap(modelName, ent.Score); gap >= cfg.PeerWorseGap {
			tripBreaker(channelID, modelName, ch, st, "peer_worse", ent.Score, autoBan, cfg)
			return
		}
	}

	if st.LowScoreStreak >= 2 {
		tripBreaker(channelID, modelName, ch, st, "score_low", ent.Score, autoBan, cfg)
		return
	}
	saveBreakerState(st)
}

func peerMedianGap(modelName string, thisScore float64) float64 {
	var scores []float64
	for _, s := range GetAllScores() {
		if s != nil && s.Model == modelName && !s.Insufficient && s.State != "broken" {
			scores = append(scores, s.Score)
		}
	}
	if len(scores) < 2 {
		return 0
	}
	// simple average as proxy for median
	sum := 0.0
	for _, v := range scores {
		sum += v
	}
	med := sum / float64(len(scores))
	return med - thisScore
}

func tripBreaker(channelID int, modelName string, ch *model.Channel, st *model.ChannelBreakerState, reason string, score float64, autoBan bool, cfg Config) {
	from := st.Stage
	if !autoBan {
		// only degrade weights, no status=4
		st.Reason = reason + "_no_ban"
		saveBreakerState(st)
		model.RecordBreakerEvent(channelID, modelName, from, "degraded", reason+"_no_ban", score)
		return
	}
	cool := st.CooldownSec
	if cool <= 0 {
		cool = cfg.CooldownInitialSec
	}
	st.Stage = "open"
	st.CooldownSec = cool
	st.CooldownUntil = time.Now().Add(time.Duration(cool) * time.Second)
	st.HalfOpenOK = 0
	st.Reason = reason
	saveBreakerState(st)
	model.UpdateChannelStatus(channelID, "", common.ChannelStatusScoreBroken, reason)
	model.RecordBreakerEvent(channelID, modelName, from, "open", reason, score)
	clearHalfOpenCounters(channelID)
	InvalidateHalfOpenCache()
}

// TripImmediate for banned/balance/consec fails.
func TripImmediate(channelID int, reason string) {
	cfg := GetConfig()
	ch, err := model.GetChannelById(channelID, true)
	if err != nil || ch == nil || !ch.GetAutoBan() {
		return
	}
	if ch.Status == common.ChannelStatusManuallyDisabled || ch.Status == common.ChannelStatusAutoDisabled {
		return
	}
	st := loadBreakerState(channelID)
	from := st.Stage // 审查 N-P2-2：首次熔断记 closed→open
	cool := st.CooldownSec
	if cool <= 0 {
		cool = cfg.CooldownInitialSec
	}
	st.Stage = "open"
	st.CooldownUntil = time.Now().Add(time.Duration(cool) * time.Second)
	st.CooldownSec = cool
	st.HalfOpenOK = 0
	st.Reason = reason
	saveBreakerState(st)
	model.UpdateChannelStatus(channelID, "", common.ChannelStatusScoreBroken, reason)
	model.RecordBreakerEvent(channelID, "", from, "open", reason, 0)
	InvalidateHalfOpenCache()
	clearHalfOpenCounters(channelID)
}

func loadBreakerState(channelID int) *model.ChannelBreakerState {
	var st model.ChannelBreakerState
	if err := model.DB.First(&st, "channel_id = ?", channelID).Error; err != nil {
		return &model.ChannelBreakerState{ChannelID: channelID, Stage: "closed", CooldownSec: GetConfig().CooldownInitialSec}
	}
	return &st
}

func saveBreakerState(st *model.ChannelBreakerState) {
	if st == nil {
		return
	}
	st.UpdatedAt = time.Now()
	_ = model.DB.Save(st).Error
	InvalidateHalfOpenCache()
}

// ListHalfOpenReady returns channels whose cooldown elapsed (stage open→half_2 or already half_*).
// 审查 N-P1-2 / N-P2-3：状态推进与查库由后台 AdvanceHalfOpen 定时完成，热路径只读内存快照。
func ListHalfOpenReady() []HalfOpenInfo {
	halfOpenCacheMu.RLock()
	list, at := halfOpenCache, halfOpenCacheAt
	halfOpenCacheMu.RUnlock()
	if time.Since(at) > halfOpenCacheMaxAge {
		// 后台没跑起来（测试/单机启动早期）时兜底同步刷新一次
		return AdvanceHalfOpen()
	}
	out := make([]HalfOpenInfo, len(list))
	copy(out, list)
	return out
}

var (
	halfOpenCacheMu     sync.RWMutex
	halfOpenCache       []HalfOpenInfo
	halfOpenCacheAt     time.Time
	halfOpenCacheMaxAge = 15 * time.Second
	halfOpenLoopOnce    sync.Once
)

// AdvanceHalfOpen 把冷却到期的 open 推进到 half_2，并刷新内存快照。
func AdvanceHalfOpen() []HalfOpenInfo {
	var rows []model.ChannelBreakerState
	now := time.Now()
	_ = model.DB.Where("stage IN ? AND cooldown_until <= ?", []string{"open", "half_2", "half_10"}, now).Find(&rows).Error
	out := make([]HalfOpenInfo, 0, len(rows))
	for _, r := range rows {
		stage := r.Stage
		if stage == "open" {
			stage = "half_2"
			r.Stage = "half_2"
			saveBreakerState(&r)
			model.RecordBreakerEvent(r.ChannelID, "", "open", "half_2", "cooldown_elapsed", 0)
		}
		out = append(out, HalfOpenInfo{ChannelID: r.ChannelID, Stage: stage})
	}
	halfOpenCacheMu.Lock()
	halfOpenCache, halfOpenCacheAt = out, time.Now()
	halfOpenCacheMu.Unlock()
	cp := make([]HalfOpenInfo, len(out))
	copy(cp, out)
	return cp
}

// InvalidateHalfOpenCache 熔断状态变更后让下次读取重新加载。
func InvalidateHalfOpenCache() {
	halfOpenCacheMu.Lock()
	halfOpenCacheAt = time.Time{}
	halfOpenCacheMu.Unlock()
}

// StartHalfOpenLoop 后台每 5s 推进一次半开状态（幂等）。
func StartHalfOpenLoop() {
	halfOpenLoopOnce.Do(func() {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysError(common.LogText("half-open loop panic: %v", r))
				}
			}()
			t := time.NewTicker(5 * time.Second)
			defer t.Stop()
			for range t.C {
				if model.DB != nil {
					AdvanceHalfOpen()
				}
			}
		}()
	})
}

// HalfOpenFallback 审查 N-P1-2：候选为空时挑一个到期半开、服务该模型与分组、inflight 未满的渠道。
func HalfOpenFallback(modelName, group string) *model.Channel {
	cfg := GetConfig()
	for _, h := range ListHalfOpenReady() {
		ch, err := model.GetChannelById(h.ChannelID, true)
		if err != nil || ch == nil || ch.Status != common.ChannelStatusScoreBroken {
			continue
		}
		if !channelServesModel(ch, modelName) || !channelInGroup(ch, group) {
			continue
		}
		if HalfOpenInflightFull(h.ChannelID, cfg.HalfOpenMaxInflight) {
			continue
		}
		return ch
	}
	return nil
}

func halfOpenInflightCount(channelID int) int {
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		n, err := common.RDB.Get(ctx, halfOpenInflightKey(channelID)).Int()
		if err == nil {
			return n
		}
		// Redis miss/error: do NOT treat as empty (would bypass H1); fall back to local.
	}
	halfOpenInflightMu.Lock()
	defer halfOpenInflightMu.Unlock()
	return halfOpenInflightLocal[channelID]
}

func HalfOpenInflightFull(channelID, max int) bool {
	if max <= 0 {
		max = 2
	}
	return halfOpenInflightCount(channelID) >= max
}

func tryIncHalfOpenLocal(channelID, max int) bool {
	halfOpenInflightMu.Lock()
	defer halfOpenInflightMu.Unlock()
	if halfOpenInflightLocal[channelID] >= max {
		return false
	}
	halfOpenInflightLocal[channelID]++
	return true
}

// TryIncHalfOpenInflight atomically reserves one in-flight slot. False when at max (H1).
func TryIncHalfOpenInflight(channelID, max int) bool {
	if max <= 0 {
		max = 2
	}
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		key := halfOpenInflightKey(channelID)
		n, err := common.RDB.Incr(ctx, key).Result()
		if err != nil {
			return tryIncHalfOpenLocal(channelID, max)
		}
		common.RDB.Expire(ctx, key, 2*time.Minute)
		if int(n) > max {
			common.RDB.Decr(ctx, key)
			return false
		}
		halfOpenInflightMu.Lock()
		halfOpenInflightLocal[channelID] = int(n)
		halfOpenInflightMu.Unlock()
		return true
	}
	return tryIncHalfOpenLocal(channelID, max)
}

func IncHalfOpenInflight(channelID int) {
	_ = TryIncHalfOpenInflight(channelID, GetConfig().HalfOpenMaxInflight)
}

func DecHalfOpenInflight(channelID int) {
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		key := halfOpenInflightKey(channelID)
		n, err := common.RDB.Decr(ctx, key).Result()
		if err == nil && n < 0 {
			common.RDB.Set(ctx, key, 0, 2*time.Minute)
			n = 0
		}
		halfOpenInflightMu.Lock()
		if err == nil {
			if n < 0 {
				n = 0
			}
			halfOpenInflightLocal[channelID] = int(n)
		} else if halfOpenInflightLocal[channelID] > 0 {
			halfOpenInflightLocal[channelID]--
		}
		halfOpenInflightMu.Unlock()
		return
	}
	halfOpenInflightMu.Lock()
	if halfOpenInflightLocal[channelID] > 0 {
		halfOpenInflightLocal[channelID]--
	}
	halfOpenInflightMu.Unlock()
}

// ResetHalfOpenInflightForTest clears local counters (unit tests).
func ResetHalfOpenInflightForTest() {
	halfOpenInflightMu.Lock()
	halfOpenInflightLocal = map[int]int{}
	halfOpenInflightMu.Unlock()
}

// OnHalfOpenResult H6 atomic success counting + ramp 2%→10%→100%.
func OnHalfOpenResult(channelID int, success bool) {
	cfg := GetConfig()
	st := loadBreakerState(channelID)
	if st.Stage != "half_2" && st.Stage != "half_10" && st.Stage != "open" {
		return
	}
	if st.Stage == "open" {
		st.Stage = "half_2"
	}
	ctx := context.Background()
	if !success {
		// fail → re-open, cooldown double
		cool := st.CooldownSec * 2
		if cool < cfg.CooldownInitialSec {
			cool = cfg.CooldownInitialSec
		}
		if cool > cfg.CooldownMaxSec {
			cool = cfg.CooldownMaxSec
		}
		st.Stage = "open"
		st.CooldownSec = cool
		st.CooldownUntil = time.Now().Add(time.Duration(cool) * time.Second)
		st.HalfOpenOK = 0
		st.Reason = "halfopen_fail"
		saveBreakerState(st)
		model.UpdateChannelStatus(channelID, "", common.ChannelStatusScoreBroken, "halfopen_fail")
		clearHalfOpenCounters(channelID)
		model.RecordBreakerEvent(channelID, "", "halfopen", "open", "halfopen_fail", 0)
		return
	}
	// success
	var ok int64
	if common.RedisEnabled && common.RDB != nil {
		ok, _ = common.RDB.Incr(ctx, halfOpenOKKey(channelID)).Result()
		common.RDB.Expire(ctx, halfOpenOKKey(channelID), time.Hour)
	} else {
		st.HalfOpenOK++
		ok = int64(st.HalfOpenOK)
	}
	if st.Stage == "half_2" && int(ok) >= cfg.HalfOpenSuccessNeeded {
		st.Stage = "half_10"
		st.Stage2Since = time.Now()
		st.HalfOpenOK = int(ok)
		saveBreakerState(st)
		clearHalfOpenOK(channelID)
		model.RecordBreakerEvent(channelID, "", "half_2", "half_10", "ramp", 0)
		return
	}
	if st.Stage == "half_10" {
		need := time.Duration(cfg.ScoreRefreshSec*cfg.HalfOpenStage2Stable) * time.Second
		if st.Stage2Since.IsZero() {
			st.Stage2Since = time.Now()
			saveBreakerState(st)
			return
		}
		if time.Since(st.Stage2Since) >= need {
			// hysteresis: full recovery requires Score≥65 (P1-7)
			best := 0.0
			has := false
			for _, s := range GetAllScores() {
				if s == nil || s.ChannelID != channelID || s.Insufficient {
					continue
				}
				has = true
				if s.Score > best {
					best = s.Score
				}
			}
			if has && best < 65 {
				return // stay half_10 until score recovers
			}
			st.Stage = "closed"
			st.CooldownSec = cfg.CooldownInitialSec
			st.HalfOpenOK = 0
			st.Reason = ""
			saveBreakerState(st)
			model.UpdateChannelStatus(channelID, "", common.ChannelStatusEnabled, "halfopen_recovered")
			clearHalfOpenCounters(channelID)
			recScore := best
			if !has {
				recScore = 70
			}
			model.RecordBreakerEvent(channelID, "", "half_10", "closed", "recovered", recScore)
		}
	}
}

func clearHalfOpenCounters(channelID int) {
	clearHalfOpenOK(channelID)
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.Del(ctx, halfOpenInflightKey(channelID))
	}
	halfOpenInflightMu.Lock()
	delete(halfOpenInflightLocal, channelID)
	halfOpenInflightMu.Unlock()
}

func clearHalfOpenOK(channelID int) {
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.Del(ctx, halfOpenOKKey(channelID))
	}
}

func MarkRateLimitCooldown(channelID int) {
	cfg := GetConfig()
	rateLimitMu.Lock()
	rateLimitUntil[channelID] = time.Now().Add(time.Duration(cfg.RateLimitCooldownSec) * time.Second)
	rateLimitMu.Unlock()
}

func InRateLimitCooldown(channelID int) bool {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	t, ok := rateLimitUntil[channelID]
	return ok && time.Now().Before(t)
}

func NoteAttemptResult(channelID int, success bool, class ErrClass) {
	if class == ClassBanned || class == ClassBalance {
		TripImmediate(channelID, string(class))
		consecFailMu.Lock()
		consecFails[channelID] = 0
		consecFailMu.Unlock()
		return
	}
	if class == ClassRateLimit {
		MarkRateLimitCooldown(channelID)
		// never status=4 on single 429
		return
	}
	cfg := GetConfig()
	consecFailMu.Lock()
	defer consecFailMu.Unlock()
	if success {
		consecFails[channelID] = 0
		return
	}
	if FailWeight(class) <= 0 {
		return
	}
	consecFails[channelID]++
	if consecFails[channelID] >= cfg.ConsecutiveFailBreak {
		consecFails[channelID] = 0
		go TripImmediate(channelID, "consec_fail")
	}
}

// IsHalfOpenChannel reports whether channel is currently in half-open stage.
func IsHalfOpenChannel(channelID int) bool {
	// 审查 N-P2-3：读内存快照，不在热路径查库
	for _, h := range ListHalfOpenReady() {
		if h.ChannelID == channelID {
			return true
		}
	}
	return false
}

// ShouldEnableChannelRespectScoreBroken keeps official ShouldEnable from reviving status=4.
func ShouldEnableChannelRespectScoreBroken(err *types.NewAPIError, status int) bool {
	if status == common.ChannelStatusScoreBroken {
		return false
	}
	return false // caller still uses service.ShouldEnableChannel
}
