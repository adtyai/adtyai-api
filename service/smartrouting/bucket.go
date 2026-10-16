package smartrouting

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const bucketTTL = 2 * time.Hour

// redisMinuteKey uses Redis TIME for minute boundary (H7).
func redisMinuteKey(channelID int, modelName string, minute time.Time) string {
	return fmt.Sprintf("sr:b:%d:%s:%s", channelID, modelName, minute.UTC().Format("200601021504"))
}

func RedisTIME(ctx context.Context) (time.Time, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return time.Now().UTC(), nil
	}
	t, err := common.RDB.Time(ctx).Result()
	if err != nil {
		return time.Now().UTC(), err
	}
	return t.UTC(), nil
}

func minuteFloor(t time.Time) time.Time {
	return t.UTC().Truncate(time.Minute)
}

var (
	localBuckets   = map[string]*MinuteBucket{}
	localBucketsMu sync.Mutex
)

func bucketKeyLocal(channelID int, modelName string, minute time.Time) string {
	return fmt.Sprintf("%d|%s|%s", channelID, modelName, minute.UTC().Format("200601021504"))
}

// RecordSample writes into Redis minute bucket (or local fallback).
func RecordSample(ev AttemptEvent) {
	cfg := GetConfig()
	if !cfg.Enabled {
		return
	}
	if ev.CacheHit {
		return // H10: cache hit not counted by default
	}
	ctx := context.Background()
	now, _ := RedisTIME(ctx)
	minute := minuteFloor(now)
	fw := FailWeight(ev.Class)
	okW := 0.0
	if ev.Success {
		okW = 1.0
		fw = 0
	}
	// sample cap per token (H8) applied at score time; here we still record with token id.
	if common.RedisEnabled && common.RDB != nil {
		key := redisMinuteKey(ev.ChannelID, ev.Model, minute)
		pipe := common.RDB.Pipeline()
		if okW > 0 {
			pipe.HIncrByFloat(ctx, key, "ok", okW)
			if ev.LatencyMs > 0 {
				pipe.HIncrByFloat(ctx, key, "lat_sum", float64(ev.LatencyMs))
				pipe.HIncrBy(ctx, key, "lat_n", 1)
			}
		}
		if fw > 0 {
			pipe.HIncrByFloat(ctx, key, "fail", fw)
		}
		tokField := fmt.Sprintf("tok:%d", ev.TokenID)
		pipe.HIncrByFloat(ctx, key, tokField, okW+fw)
		if fw > 0 && ev.UserID > 0 {
			pipe.HIncrByFloat(ctx, key, fmt.Sprintf("ufail:%d", ev.UserID), fw)
			pipe.HIncrByFloat(ctx, key, fmt.Sprintf("utot:%d", ev.UserID), fw)
		} else if ev.UserID > 0 {
			pipe.HIncrByFloat(ctx, key, fmt.Sprintf("utot:%d", ev.UserID), okW+fw)
		}
		pipe.Expire(ctx, key, bucketTTL)
		_, _ = pipe.Exec(ctx)
		return
	}
	// local fallback
	localBucketsMu.Lock()
	defer localBucketsMu.Unlock()
	k := bucketKeyLocal(ev.ChannelID, ev.Model, minute)
	b := localBuckets[k]
	if b == nil {
		b = &MinuteBucket{TokenSamples: map[int]float64{}, UserFail: map[int]float64{}, UserTotal: map[int]float64{}}
		localBuckets[k] = b
	}
	b.OK += okW
	b.Fail += fw
	if okW > 0 && ev.LatencyMs > 0 {
		b.LatencySumMs += float64(ev.LatencyMs)
		b.LatencyN++
	}
	b.TokenSamples[ev.TokenID] += okW + fw
	if ev.UserID > 0 {
		b.UserTotal[ev.UserID] += okW + fw
		if fw > 0 {
			b.UserFail[ev.UserID] += fw
		}
	}
}

// ReadWindow aggregates buckets for last n minutes using Redis TIME.
func ReadWindow(channelID int, modelName string, minutes int) (ok, fail, latSum float64, latN int, tokenShare map[int]float64, userFailRatio map[int]float64, userFailAbs map[int]float64) {
	ctx := context.Background()
	now, _ := RedisTIME(ctx)
	tokenShare = map[int]float64{}
	userFail := map[int]float64{}
	userTot := map[int]float64{}
	userFailAbs = map[int]float64{}
	for i := 0; i < minutes; i++ {
		m := minuteFloor(now.Add(-time.Duration(i) * time.Minute))
		if common.RedisEnabled && common.RDB != nil {
			key := redisMinuteKey(channelID, modelName, m)
			vals, err := common.RDB.HGetAll(ctx, key).Result()
			if err != nil || len(vals) == 0 {
				continue
			}
			ok += parseF(vals["ok"])
			fail += parseF(vals["fail"])
			latSum += parseF(vals["lat_sum"])
			latN += int(parseF(vals["lat_n"]))
			for k, v := range vals {
				if len(k) > 4 && k[:4] == "tok:" {
					id, _ := strconv.Atoi(k[4:])
					tokenShare[id] += parseF(v)
				} else if len(k) > 6 && k[:6] == "ufail:" {
					id, _ := strconv.Atoi(k[6:])
					userFail[id] += parseF(v)
				} else if len(k) > 5 && k[:5] == "utot:" {
					id, _ := strconv.Atoi(k[5:])
					userTot[id] += parseF(v)
				}
			}
			continue
		}
		localBucketsMu.Lock()
		b := localBuckets[bucketKeyLocal(channelID, modelName, m)]
		localBucketsMu.Unlock()
		if b == nil {
			continue
		}
		ok += b.OK
		fail += b.Fail
		latSum += b.LatencySumMs
		latN += b.LatencyN
		for id, v := range b.TokenSamples {
			tokenShare[id] += v
		}
		for id, v := range b.UserFail {
			userFail[id] += v
		}
		for id, v := range b.UserTotal {
			userTot[id] += v
		}
	}
	userFailRatio = map[int]float64{}
	for id, tot := range userTot {
		if tot > 0 {
			userFailRatio[id] = userFail[id] / tot
		}
	}
	for id, v := range userFail {
		userFailAbs[id] = v
	}
	return
}

// ApplyUserFailMeltCap drops excess fail weight when one user dominates failures (P1-3).
// Cap is max share of total fail any single user may contribute toward melt/score.
func ApplyUserFailMeltCap(fail float64, userFail map[int]float64, cap float64) float64 {
	if fail <= 0 || cap <= 0 || len(userFail) == 0 {
		return fail
	}
	var excess float64
	for _, v := range userFail {
		if v/fail > cap {
			excess += v - fail*cap
		}
	}
	out := fail - excess
	if out < 0 {
		return 0
	}
	return out
}

func parseF(s string) float64 {
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ApplySampleCap reduces effective ok/fail by capping any single token's share (H8).
func ApplySampleCap(ok, fail float64, tokenShare map[int]float64, cap float64) (float64, float64, bool) {
	total := ok + fail
	if total <= 0 || cap <= 0 || len(tokenShare) == 0 {
		return ok, fail, false
	}
	capped := false
	var excess float64
	for _, v := range tokenShare {
		if v/total > cap {
			excess += v - total*cap
			capped = true
		}
	}
	if !capped || excess <= 0 {
		return ok, fail, false
	}
	// drop excess proportionally from ok and fail
	scale := (total - excess) / total
	if scale < 0 {
		scale = 0
	}
	return ok * scale, fail * scale, true
}
