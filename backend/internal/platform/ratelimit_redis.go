package platform

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisLimiter is a fixed-window rate limiter shared across replicas via Redis
// (finding 4). It fails open: if Redis is unreachable the request is allowed
// rather than blocking all traffic on a limiter outage, and the error is logged.
type RedisLimiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

var _ Limiter = (*RedisLimiter)(nil)

func NewRedisLimiter(rdb *redis.Client, limit int, window time.Duration) *RedisLimiter {
	return &RedisLimiter{rdb: rdb, limit: limit, window: window}
}

// incrWindowScript increments the counter and sets the window TTL on first hit,
// atomically, so a counter can never linger without expiry.
var incrWindowScript = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n`)

func (l *RedisLimiter) Allow(key string) bool {
	return l.AllowWithin(key, l.limit, l.window)
}

func (l *RedisLimiter) AllowWithin(key string, limit int, window time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	n, err := incrWindowScript.Run(ctx, l.rdb, []string{"rate:" + key}, window.Milliseconds()).Int64()
	if err != nil {
		slog.Warn("redis rate limiter unavailable; allowing request", "error", err)
		return true
	}
	return n <= int64(limit)
}
