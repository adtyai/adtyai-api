package adtylimit

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisStore 把计数存在 Redis 里，多实例共享。
// 不用 common.RedisIncr：键不存在时它什么都不做（common/redis.go）。
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore 用现有的 Redis 连接。
func NewRedisStore(client *redis.Client) *RedisStore { return &RedisStore{client: client} }

// INCR 之后第一次才设过期，原子完成。
var incrWithTTL = redis.NewScript(`
local value = redis.call('INCRBY', KEYS[1], ARGV[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return value`)

// 清掉过期成员，没满就登记，返回 1 表示登记成功。
var acquireScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[3], ARGV[4])
redis.call('EXPIRE', KEYS[1], ARGV[5])
return 1`)

const minuteKeyTTLSeconds = 70

func (s *RedisStore) IncrRPM(userID int, minute int64) (int64, error) {
	return incrWithTTL.Run(context.Background(), s.client, []string{rpmKey(userID, minute)}, 1, minuteKeyTTLSeconds).Int64()
}

func (s *RedisStore) TPM(userID int, minute int64) (int64, error) {
	value, err := s.client.Get(context.Background(), tpmKey(userID, minute)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return value, err
}

func (s *RedisStore) AddTPM(userID int, minute int64, tokens int64) error {
	return incrWithTTL.Run(context.Background(), s.client, []string{tpmKey(userID, minute)}, tokens, minuteKeyTTLSeconds).Err()
}

func (s *RedisStore) Acquire(userID int, member string, limit int, now time.Time, ttl time.Duration) (bool, error) {
	result, err := acquireScript.Run(context.Background(), s.client, []string{concurrencyKey(userID)},
		strconv.FormatInt(now.UnixMilli(), 10), limit, now.Add(ttl).UnixMilli(), member, int64(ttl/time.Second)+60).Int()
	return result == 1, err
}

func (s *RedisStore) Release(userID int, member string) error {
	return s.client.ZRem(context.Background(), concurrencyKey(userID), member).Err()
}
