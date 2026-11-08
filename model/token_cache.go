package model

import (
	"context"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

func getTokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func getTokenCacheFenceKey(key string) string {
	return fmt.Sprintf("token:fence:%s", common.GenerateHMAC(key))
}

func getTokenCacheGenerationKey(key string) string {
	return fmt.Sprintf("token:generation:%s", common.GenerateHMAC(key))
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
// under a mutation's new generation. An existing hash only gets its TTL refreshed.
// Amount fields are a compatibility projection; every reader uses SQL balances.
// Redis generations only guard in-flight publication. GetTokenByKey verifies
// authorization against SQL even on hits because Redis recovery can restore an
// old hash and its generation together.
// 返回值：0=快照已失效或被旧 fence 拦截，1=完成初始化，2=哈希已存在，仅刷新 TTL。
func cacheInitToken(token Token) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	const script = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
if (redis.call('GET', KEYS[3]) or '') ~= ARGV[19] then
  return 0
end
if redis.call('EXISTS', KEYS[1]) == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[18])
  return 2
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'AccessedTime', ARGV[6], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'RemainQuota', ARGV[15], 'UsedQuota', ARGV[16],
  'AllowUserQuotaQuery', ARGV[17])
redis.call('EXPIRE', KEYS[1], ARGV[18])
return 1`

	return common.RDB.Eval(context.Background(), script, []string{
		getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key), getTokenCacheGenerationKey(token.Key),
	},
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota,
		strconv.FormatBool(token.AllowUserQuotaQuery),
		tokenCacheTTLSeconds(),
		token.cacheGeneration,
	).Int()
}

// cacheGetTokenByKey 只读取缓存快照；授权调用方必须经 GetTokenByKey 核对数据库。
// 不完整的哈希（如仅有配额字段）会被拒绝。
// 变更隔离标记还在时不返回哈希，避免删除失败后继续使用变更前的状态。
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
	return &token, nil
}
