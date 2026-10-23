package smartrouting

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/go-redis/redis/v8"
)

type HalfOpenInfo struct {
	ChannelID int
	Model     string
	Stage     string // half_2 | half_10
}

var (
	rateLimitUntil        = map[string]time.Time{}
	rateLimitMu           sync.Mutex
	consecFails           = map[int]int{}
	consecFailMu          sync.Mutex
	halfOpenInflightLocal = map[int]int{}
	halfOpenInflightMu    sync.Mutex
	halfOpenProgressMu    sync.Mutex
)

func halfOpenOKKey(channelID int) string {
	return fmt.Sprintf("sr:halfopen:%d:ok", channelID)
}
func halfOpenInflightKey(channelID int) string {
	return fmt.Sprintf("sr:halfopen:%d:inflight", channelID)
}

// EvaluateBreaker marks one low or peer-worse model. It does not open the channel breaker.
func EvaluateBreaker(channelID int, modelName string, ent *ScoreEntry, cfg Config) {
	if ent == nil || ent.Insufficient || !scoreBreakEligible(channelID) {
		return
	}
	if modelName != "" && ent.Model == "" {
		ent.Model = modelName
	}
	if ent.ChannelID == 0 {
		ent.ChannelID = channelID
	}
	noteModelScore(ent, cfg)
}

func scoreBreakEligible(channelID int) bool {
	if ch, ok := model.CacheGetChannelMemory(channelID); ok {
		return ch.Status != common.ChannelStatusManuallyDisabled && ch.Status != common.ChannelStatusAutoDisabled
	}
	// Unit tests without a database or memory cache have no status to reject.
	// A live process must not query the database from this path.
	if !common.MemoryCacheEnabled && model.DB == nil {
		return true
	}
	return false
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

// TripImmediate for banned/balance/consec fails.
func TripImmediate(channelID int, reason string) bool {
	cfg := GetConfig()
	ch, ok := model.CacheGetChannelMemory(channelID)
	if !ok || ch == nil || !ch.GetAutoBan() {
		return false
	}
	if ch.Status == common.ChannelStatusManuallyDisabled || ch.Status == common.ChannelStatusAutoDisabled {
		return false
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
	model.RecordBreakerEvent(channelID, "", from, "open", reason, 0)
	InvalidateHalfOpenCache()
	clearHalfOpenCounters(channelID)
	return true
}

func loadBreakerState(channelID int) *model.ChannelBreakerState {
	if model.DB == nil {
		stage := breakerStageOf(channelID)
		if stage == "" {
			stage = "closed"
		}
		return &model.ChannelBreakerState{ChannelID: channelID, Stage: stage, CooldownSec: GetConfig().CooldownInitialSec}
	}
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
	noteBreakerStage(st.ChannelID, st.Stage)
	InvalidateHalfOpenCache()
}

// breakerOpen is the in-process stage. A miss is not an exclusion.
func breakerOpen(channelID int) bool {
	stage := breakerStageOf(channelID)
	return stage != "" && stage != "closed"
}

// ListHalfOpenReady returns channels whose cooldown elapsed (stage open→half_2 or already half_*).
// 审查 N-P1-2 / N-P2-3：状态推进与查库由后台 AdvanceHalfOpen 定时完成，热路径只读内存快照。
func ListHalfOpenReady() []HalfOpenInfo {
	halfOpenCacheMu.RLock()
	list, at := halfOpenCache, halfOpenCacheAt
	halfOpenCacheMu.RUnlock()
	if time.Since(at) > halfOpenCacheMaxAge {
		if !halfOpenLoopRunning.Load() {
			// 后台循环没跑起来（测试/单机启动早期）时兜底同步刷新一次
			return AdvanceHalfOpen()
		}
		// 审查 R-P2-6：热路径不同步查库，只唤醒后台循环刷新，本次继续读旧快照
		kickHalfOpenRefresh()
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
	halfOpenLoopRunning atomic.Bool
	halfOpenKick        = make(chan struct{}, 1)
	breakerStageMu      sync.Mutex
	breakerStages       = map[int]string{}
	modelBreakMu        sync.Mutex
	modelBreaks         = map[string]*modelBreakState{}
	tripBalanceFn       = TripImmediate
)

type modelBreakState struct {
	streak int
	broken bool
}

func noteBreakerStage(channelID int, stage string) {
	breakerStageMu.Lock()
	if stage == "" || stage == "closed" {
		delete(breakerStages, channelID)
	} else {
		breakerStages[channelID] = stage
	}
	breakerStageMu.Unlock()
	notifyRoutingAlert(channelID, stage)
}

func notifyRoutingAlert(channelID int, stage string) {
	if stage == "" || stage == "closed" || !GetConfig().TelegramAlertEnabled || model.OpsTelegramSender == nil {
		return
	}
	model.OpsTelegramSender("warning", "smart_routing", fmt.Sprintf("channel %d routing stage %s", channelID, stage))
}

func breakerStageOf(channelID int) string {
	breakerStageMu.Lock()
	defer breakerStageMu.Unlock()
	return breakerStages[channelID]
}

var breakerStageQuery = queryBreakerStages

func queryBreakerStages() ([]model.ChannelBreakerState, error) {
	if model.DB == nil {
		return nil, errors.New("database is not configured")
	}
	var rows []model.ChannelBreakerState
	err := model.DB.Where("stage <> ? AND stage <> ?", "closed", "").Find(&rows).Error
	return rows, err
}

func loadBreakerStages() {
	rows, err := breakerStageQuery()
	if err != nil {
		common.SysError(common.LogText("smartrouting: breaker stage reload failed: %v", err))
		return
	}
	next := map[int]string{}
	for _, r := range rows {
		if r.Stage == "" || r.Stage == "closed" {
			continue
		}
		next[r.ChannelID] = r.Stage
	}
	breakerStageMu.Lock()
	breakerStages = next
	breakerStageMu.Unlock()
}

func knownScoreBreakerReason(reason string) bool {
	switch reason {
	case "peer_worse", "score_low", "banned", "balance", "consec_fail", "halfopen_fail":
		return true
	default:
		return false
	}
}

// RepairScoreBrokenStatus re-enables only status-4 rows that this package wrote.
// Unknown reasons stay disabled and are logged. The event is the rollback record.
func RepairScoreBrokenStatus() {
	if model.DB == nil {
		return
	}
	var channels []model.Channel
	if err := model.DB.Where("status = ?", common.ChannelStatusScoreBroken).Find(&channels).Error; err != nil {
		return
	}
	for i := range channels {
		ch := &channels[i]
		reason, _ := ch.GetOtherInfo()["status_reason"].(string)
		var st model.ChannelBreakerState
		hasBreaker := model.DB.First(&st, "channel_id = ?", ch.Id).Error == nil
		if !hasBreaker || !knownScoreBreakerReason(reason) {
			model.RecordBreakerEvent(ch.Id, "", "status4", "left", "repair_skipped:"+reason, 0)
			continue
		}
		model.RecordBreakerEvent(ch.Id, "", "status4", "enabled", "repair:"+reason, 0)
		model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusEnabled, "")
	}
}

func kickHalfOpenRefresh() {
	select {
	case halfOpenKick <- struct{}{}:
	default:
	}
}

// AdvanceHalfOpen 把冷却到期的 open 推进到 half_2，并刷新内存快照。
func AdvanceHalfOpen() []HalfOpenInfo {
	if model.DB == nil {
		halfOpenCacheMu.Lock()
		halfOpenCache, halfOpenCacheAt = nil, time.Now()
		halfOpenCacheMu.Unlock()
		return nil
	}
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

// InvalidateHalfOpenCache 熔断状态变更后标脏：后台循环在运行时由它异步刷新（审查 R-P2-6），
// 否则下次读取时同步重载。
func InvalidateHalfOpenCache() {
	halfOpenCacheMu.Lock()
	halfOpenCacheAt = time.Time{}
	halfOpenCacheMu.Unlock()
	if halfOpenLoopRunning.Load() {
		kickHalfOpenRefresh()
	}
}

// StartHalfOpenLoop 后台每 5s（或被标脏时立即）推进一次半开状态（幂等）。
func StartHalfOpenLoop() {
	halfOpenLoopOnce.Do(func() {
		halfOpenLoopRunning.Store(true)
		go func() {
			defer halfOpenLoopRunning.Store(false)
			defer func() {
				if r := recover(); r != nil {
					common.SysError(common.LogText("half-open loop panic: %v", r))
				}
			}()
			t := time.NewTicker(5 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-t.C:
				case <-halfOpenKick:
				}
				halfOpenTick()
			}
		}()
	})
}

// HalfOpenFallback 审查 N-P1-2：候选为空时挑一个到期半开、服务该模型与分组、inflight 未满的渠道。
func HalfOpenFallback(modelName, group string, caps model.RouteCaps, accept func(*model.Channel) bool) *model.Channel {
	cfg := GetConfig()
	for _, h := range ListHalfOpenReady() {
		ch, ok := model.CacheGetChannelMemory(h.ChannelID)
		if !ok {
			continue
		}
		if ch.Status != common.ChannelStatusEnabled && ch.Status != common.ChannelStatusScoreBroken {
			continue
		}
		if hardExclude(ch, modelName, caps) != "" {
			continue
		}
		if ok, _ := model.ChannelSatisfiesFilters(ch, modelName, caps.Filters); !ok {
			continue
		}
		if !channelServesModel(ch, modelName) || !channelInGroup(ch, group) {
			continue
		}
		if accept != nil && !accept(ch) {
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
		if err == nil || errors.Is(err, redis.Nil) {
			return n
		}
		// Redis error is fail-closed. Do not fall back to the process-local counter.
		return int(^uint(0) >> 1)
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
			return false
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

var loadBreakerStateFor = loadBreakerState

var saveBreakerStateFor = saveBreakerState

var recordBreakerEventFor = model.RecordBreakerEvent

var saveHalfOpenProgress = persistHalfOpenOK

func persistHalfOpenOK(st *model.ChannelBreakerState) {
	if st == nil || model.DB == nil {
		return
	}
	st.UpdatedAt = time.Now()
	_ = model.DB.Model(&model.ChannelBreakerState{}).
		Where("channel_id = ?", st.ChannelID).
		Updates(map[string]any{
			"half_open_ok": st.HalfOpenOK,
			"updated_at":   st.UpdatedAt,
		}).Error
}

// OnHalfOpenResult H6 atomic success counting + ramp 2%→10%→100%.
func OnHalfOpenResult(channelID int, success bool) {
	halfOpenProgressMu.Lock()
	defer halfOpenProgressMu.Unlock()
	cfg := GetConfig()
	st := loadBreakerStateFor(channelID)
	if st.Stage != "half_2" && st.Stage != "half_10" && st.Stage != "full" && st.Stage != "open" {
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
		saveBreakerStateFor(st)
		clearHalfOpenOK(channelID)
		recordBreakerEventFor(channelID, "", "halfopen", "open", "halfopen_fail", 0)
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
		saveHalfOpenProgress(st)
	}
	if st.Stage == "half_2" && int(ok) >= cfg.HalfOpenSuccessNeeded {
		st.Stage = "half_10"
		st.Stage2Since = time.Now()
		st.HalfOpenOK = int(ok)
		saveBreakerStateFor(st)
		clearHalfOpenOK(channelID)
		recordBreakerEventFor(channelID, "", "half_2", "half_10", "ramp", 0)
		return
	}
	if st.Stage == "half_10" {
		need := time.Duration(cfg.ScoreRefreshSec*cfg.HalfOpenStage2Stable) * time.Second
		if st.Stage2Since.IsZero() {
			st.Stage2Since = time.Now()
			saveBreakerStateFor(st)
			return
		}
		if time.Since(st.Stage2Since) >= need {
			// hysteresis: full recovery requires Score≥65 (P1-7)
			recoverHalfOpen(channelID, st, cfg)
		}
		return
	}
	if st.Stage == "full" {
		recoverHalfOpen(channelID, st, cfg)
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

type sharedCounter interface {
	Incr(key string, ttl time.Duration) (int, bool)
	Del(key string) bool
	SetExpiry(key string, ttl time.Duration) bool
	Alive(key string) (bool, bool)
}

type redisSharedCounter struct{}

func (redisSharedCounter) Incr(key string, ttl time.Duration) (int, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, false
	}
	n, err := common.RDB.Incr(context.Background(), key).Result()
	if err != nil {
		return 0, false
	}
	if ttl > 0 {
		_ = common.RDB.Expire(context.Background(), key, ttl).Err()
	}
	return int(n), true
}

func (redisSharedCounter) Del(key string) bool {
	if !common.RedisEnabled || common.RDB == nil {
		return false
	}
	return common.RDB.Del(context.Background(), key).Err() == nil
}

func (redisSharedCounter) SetExpiry(key string, ttl time.Duration) bool {
	if !common.RedisEnabled || common.RDB == nil || ttl <= 0 {
		return false
	}
	return common.RDB.Set(context.Background(), key, "1", ttl).Err() == nil
}

func (redisSharedCounter) Alive(key string) (bool, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return false, false
	}
	n, err := common.RDB.Exists(context.Background(), key).Result()
	if err != nil {
		return false, false
	}
	return n > 0, true
}

var routingCounter sharedCounter = redisSharedCounter{}

var tripImmediateAsync = func(channelID int, reason string) {
	go func() {
		if !TripImmediate(channelID, reason) {
			noteBreakerStage(channelID, "closed")
		}
	}()
}

var reloadBreakerStagesFn = loadBreakerStages

func halfOpenTick() {
	reloadBreakerStagesFn()
	if model.DB != nil {
		AdvanceHalfOpen()
	}
}

func consecKey(channelID int) string {
	return "sr:consec:" + strconv.Itoa(channelID)
}

func rateLimitKey(channelID int, modelName string) string {
	return "sr:rl:" + strconv.Itoa(channelID) + ":" + modelName
}

func noteConsecFail(channelID int) int {
	if n, ok := routingCounter.Incr(consecKey(channelID), 10*time.Minute); ok {
		return n
	}
	consecFailMu.Lock()
	defer consecFailMu.Unlock()
	consecFails[channelID]++
	return consecFails[channelID]
}

func clearConsecFail(channelID int) {
	routingCounter.Del(consecKey(channelID))
	consecFailMu.Lock()
	delete(consecFails, channelID)
	consecFailMu.Unlock()
}

func MarkRateLimitCooldown(channelID int, modelName string) {
	cfg := GetConfig()
	ttl := time.Duration(cfg.RateLimitCooldownSec) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	key := rateLimitKey(channelID, modelName)
	if routingCounter != nil {
		routingCounter.SetExpiry(key, ttl)
	}
	rateLimitMu.Lock()
	rateLimitUntil[key] = time.Now().Add(ttl)
	rateLimitMu.Unlock()
}

func InRateLimitCooldown(channelID int, modelName string) bool {
	key := rateLimitKey(channelID, modelName)
	if routingCounter != nil {
		if alive, ok := routingCounter.Alive(key); ok {
			return alive
		}
	}
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	t, ok := rateLimitUntil[key]
	return ok && time.Now().Before(t)
}

func NoteAttemptResult(channelID int, modelName string, success bool, class ErrClass) {
	if class == ClassBanned || class == ClassBalance {
		TripImmediate(channelID, string(class))
		clearConsecFail(channelID)
		return
	}
	if class == ClassRateLimit {
		MarkRateLimitCooldown(channelID, modelName)
		// never status=4 on single 429
		return
	}
	if success || FailWeight(class) <= 0 {
		if success {
			clearConsecFail(channelID)
		}
		return
	}
	n := noteConsecFail(channelID)
	if n >= GetConfig().ConsecutiveFailBreak {
		clearConsecFail(channelID)
		noteBreakerStage(channelID, "open")
		tripImmediateAsync(channelID, "consec_fail")
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

// ResetBreakerOnManualEnable 审查 N-P2-7：手动启用后熔断状态回到 closed，避免 status=1 而 stage=open 不一致。
func ResetBreakerOnManualEnable(channelID int) {
	var st model.ChannelBreakerState
	if err := model.DB.First(&st, "channel_id = ?", channelID).Error; err != nil {
		return // 没有熔断记录
	}
	if st.Stage == "closed" {
		return
	}
	from := st.Stage
	st.Stage = "closed"
	st.CooldownSec = GetConfig().CooldownInitialSec
	st.CooldownUntil = time.Time{}
	st.HalfOpenOK = 0
	st.Reason = "manual_enable"
	saveBreakerState(&st)
	clearHalfOpenCounters(channelID)
	model.RecordBreakerEvent(channelID, "", from, "closed", "manual_enable", 0)
}

func recoverHalfOpen(channelID int, st *model.ChannelBreakerState, cfg Config) {
	worst, has := worstSufficientScore(channelID)
	if !has || st == nil {
		return
	}
	from := st.Stage
	if worst >= 65 {
		st.Stage = "closed"
		st.CooldownSec = cfg.CooldownInitialSec
		st.HalfOpenOK = 0
		st.Reason = ""
		saveBreakerStateFor(st)
		clearHalfOpenCounters(channelID)
		recordBreakerEventFor(channelID, "", from, "closed", "recovered", worst)
		return
	}
	if from == "half_10" && worst >= 50 {
		st.Stage = "full"
		st.Reason = "full"
		saveBreakerStateFor(st)
		recordBreakerEventFor(channelID, "", "half_10", "full", "full", worst)
	}
}

func worstSufficientScore(channelID int) (float64, bool) {
	worst := 0.0
	has := false
	for _, s := range GetAllScores() {
		if s == nil || s.ChannelID != channelID || s.Insufficient {
			continue
		}
		if !has || s.Score < worst {
			worst = s.Score
			has = true
		}
	}
	return worst, has
}

func channelBreakerActive(channelID int) bool {
	switch breakerStageOf(channelID) {
	case "open", "half_2", "half_10", "full":
		return true
	default:
		return false
	}
}

func resetModelBreaks() {
	modelBreakMu.Lock()
	modelBreaks = map[string]*modelBreakState{}
	modelBreakMu.Unlock()
}

func noteModelScore(ent *ScoreEntry, cfg Config) {
	if ent == nil || ent.Insufficient || ent.ChannelID == 0 || ent.Model == "" || channelBreakerActive(ent.ChannelID) {
		return
	}
	key := scoreKey(ent.ChannelID, ent.Model)
	var publish *ScoreEntry
	modelBreakMu.Lock()
	st := modelBreaks[key]
	if st == nil {
		st = &modelBreakState{}
		modelBreaks[key] = st
	}
	switch {
	case ent.Score >= 65:
		st.streak = 0
		if st.broken {
			st.broken = false
			ent.State = "normal"
			publish = ent
		}
	case st.broken:
		ent.State = "broken"
	case ent.Score < 40:
		st.streak++
		if st.streak >= 2 || peerWorseBreak(ent, cfg) {
			st.broken = true
			ent.State = "broken"
			publish = ent
		}
	default:
		st.streak = 0
	}
	modelBreakMu.Unlock()
	if publish != nil {
		publishModelState(publish)
	}
}

func peerWorseBreak(ent *ScoreEntry, cfg Config) bool {
	return cfg.PeerWorseBreakEnabled && ent != nil && ent.Samples >= cfg.MinSamples && peerMedianGap(ent.Model, ent.Score) >= cfg.PeerWorseGap
}

func publishModelState(ent *ScoreEntry) {
	if ent == nil || model.DB == nil {
		return
	}
	rw := ent.RouteWeight
	if rw < 1 {
		rw = 1
	}
	_ = model.UpsertRouteWeight(ent.ChannelID, ent.Model, rw, ent.Score, ent.State)
}

func PatrolLowBalance(channels []model.Channel) {
	cfg := GetConfig()
	if !cfg.BalancePatrolEnabled || model.DB == nil {
		return
	}
	for i := range channels {
		ch := &channels[i]
		if ch.BalanceUpdatedTime <= 0 || ch.Balance >= cfg.BalanceLowUSD || channelBreakerActive(ch.Id) {
			continue
		}
		if ch.GetAutoBan() {
			tripBalanceFn(ch.Id, "balance")
			continue
		}
		notifyRoutingAlert(ch.Id, "balance_low")
	}
}
