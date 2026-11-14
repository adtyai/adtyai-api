package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

func (token Token) cacheMAC() (string, error) {
	key := getTokenCacheKey(token.Key)
	quotaPending := token.QuotaPending
	token.Key = ""
	token.RemainQuota = 0
	token.UsedQuota = 0
	token.AccessedTime = 0
	token.QuotaPending = 0
	if token.AllowIps != nil && *token.AllowIps == "" {
		token.AllowIps = nil
	}
	// QuotaPending is a 二开 durable field (json:"-") and must be in the MAC
	// payload. RemainQuota/UsedQuota stay unsigned counters, as upstream does.
	return authCacheMAC(key, struct {
		Token          Token
		AutoGroups     string
		QuotaPending   int
		CacheExpiresAt int64
	}{token, token.AutoGroups, quotaPending, token.CacheExpiresAt})
}

func getTokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

// The fence and generation keys use the cache key as their hash tag so
// cacheInitToken and invalidateTokenCacheForMutation stay in one slot on
// sharded Redis. The cache hash name itself is unchanged.
func getTokenCacheFenceKey(key string) string {
	return fmt.Sprintf("token:fence:{%s}", getTokenCacheKey(key))
}

func getTokenCacheGenerationKey(key string) string {
	return fmt.Sprintf("token:generation:{%s}", getTokenCacheKey(key))
}

func tokenCacheGeneration(key string) (string, error) {
	generation, err := common.RDB.Get(context.Background(), getTokenCacheGenerationKey(key)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return generation, err
}

func tokenCacheTTLSeconds() int {
	ttl := common.RedisKeyCacheSeconds()
	if ttl <= 0 {
		return 60
	}
	return ttl
}

// Legacy fence keys remain recognized, but this protocol requires stopping
// writes, draining old work, and switching every node together. Mixed-version
// rolling upgrades are not supported. New mutations do not use a timed fence.
const tokenCacheFenceSeconds = 10

// invalidateTokenCacheForMutation is called before a token metadata mutation
// writes to the database, while holding the token row lock. Advancing the
// generation and removing the hash is atomic. With a continuous Redis history,
// delayed snapshots cannot publish after a mutation, regardless of the pause.
// Restored Redis data still requires the SQL authorization check.
func invalidateTokenCacheForMutation(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	ctx := context.Background()
	const script = `
redis.call('INCR', KEYS[2])
redis.call('DEL', KEYS[1])
return 1`
	return common.RDB.Eval(ctx, script, []string{getTokenCacheKey(key), getTokenCacheGenerationKey(key)}).Err()
}

// cacheInitToken publishes only if the generation captured before the database
// read is still current and no legacy fence is active. The reader holds the
// token row lock through publication, so it cannot read uncommitted metadata
// under a mutation's new generation. An existing hash only gets its TTL refreshed
// unless the stored MAC is missing or no longer matches, in which case metadata
// is repaired in place and integer counters are kept.
// Amount fields are a compatibility projection; every reader uses SQL balances.
// Redis generations only guard in-flight publication. GetTokenByKey verifies
// authorization against SQL even on hits because Redis recovery can restore an
// old hash and its generation together.
// 返回值：0=快照已失效或被旧 fence 拦截，1=完成初始化，2=哈希已存在，仅刷新 TTL 或完成修复。
func cacheInitToken(token Token) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	cacheKey := getTokenCacheKey(token.Key)
	var existing Token
	readErr := common.RedisHGetObj(cacheKey, &existing)
	if readErr != nil {
		// Decoding stops at the first unreadable field, before CacheMAC.
		existing.CacheMAC, _ = common.RDB.HGet(context.Background(), cacheKey, "CacheMAC").Result()
	}
	existing.Key = token.Key
	existingMAC, macErr := existing.cacheMAC()
	repair := readErr != nil || macErr != nil || !validAuthCacheMAC(existingMAC, existing.CacheMAC, existing.CacheExpiresAt)
	token.CacheExpiresAt = time.Now().Add(time.Duration(tokenCacheTTLSeconds()) * time.Second).UnixMilli()
	mac, err := token.cacheMAC()
	if err != nil {
		return 0, err
	}
	const script = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
if (redis.call('GET', KEYS[3]) or '') ~= ARGV[24] then
  return 0
end
local exists = redis.call('EXISTS', KEYS[1])
if exists == 1 and (ARGV[21] ~= 'true' or (redis.call('HGET', KEYS[1], 'CacheMAC') or '') ~= ARGV[22]) then
  redis.call('EXPIRE', KEYS[1], ARGV[18])
  return 2
end
redis.call('HDEL', KEYS[1], 'DeletedAt')
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'AllowUserQuotaQuery', ARGV[17],
  'QuotaPending', ARGV[23],
  'CacheExpiresAt', ARGV[19], 'CacheMAC', ARGV[20])
for _, counter in ipairs({{'AccessedTime', ARGV[6]}, {'RemainQuota', ARGV[15]}, {'UsedQuota', ARGV[16]}}) do
  if not string.match(redis.call('HGET', KEYS[1], counter[1]) or '', '^-?%d+$') then
    redis.call('HSET', KEYS[1], counter[1], counter[2])
  end
end
redis.call('EXPIRE', KEYS[1], ARGV[18])
if exists == 1 then
  return 2
end
return 1`

	return common.RDB.Eval(context.Background(), script, []string{
		cacheKey, getTokenCacheFenceKey(token.Key), getTokenCacheGenerationKey(token.Key),
	},
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota,
		strconv.FormatBool(token.AllowUserQuotaQuery),
		tokenCacheTTLSeconds(),
		token.CacheExpiresAt, mac, strconv.FormatBool(repair), existing.CacheMAC,
		token.QuotaPending,
		token.cacheGeneration,
	).Int()
}

// cacheGetTokenByKey 只读取缓存快照；授权调用方必须经 GetTokenByKey 核对数据库。
// 不完整的哈希（如仅有配额字段）会被拒绝。
// 变更隔离标记还在时不返回哈希，避免删除失败后继续使用变更前的状态。
// 缺少或无效的 MAC 视为未命中，由调用方回源重建。
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	fenced, err := common.RDB.Exists(context.Background(), getTokenCacheFenceKey(key)).Result()
	if err != nil {
		return nil, err
	}
	if fenced > 0 {
		return nil, fmt.Errorf("token cache is fenced")
	}
	var token Token
	if err := common.RedisHGetObj(getTokenCacheKey(key), &token); err != nil {
		return nil, err
	}
	if token.Id <= 0 {
		return nil, fmt.Errorf("token cache is incomplete")
	}
	token.Key = key
	mac, err := token.cacheMAC()
	if err != nil || !validAuthCacheMAC(mac, token.CacheMAC, token.CacheExpiresAt) {
		return nil, fmt.Errorf("token cache integrity check failed")
	}
	return &token, nil
}
