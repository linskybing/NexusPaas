package platform

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// RedisWorkerLease is a distributed implementation of contracts.WorkerLease so
// worker shards coordinate across replicas instead of per-process (finding 4).
// A shard is held by writing the worker id under a TTL key; the same worker can
// idempotently re-acquire (refreshing the TTL), a different worker is rejected
// until expiry, and release only removes the key when the caller still holds it.
type RedisWorkerLease struct {
	rdb *redis.Client
}

var _ contracts.WorkerLease = (*RedisWorkerLease)(nil)

func NewRedisWorkerLease(rdb *redis.Client) *RedisWorkerLease {
	return &RedisWorkerLease{rdb: rdb}
}

func leaseKey(shard string) string { return "lease:" + shard }

func (l *RedisWorkerLease) Acquire(ctx context.Context, worker, shard string, ttl time.Duration) (bool, error) {
	key := leaseKey(shard)
	ok, err := l.rdb.SetNX(ctx, key, worker, ttl).Result()
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	// Key exists: idempotent re-acquire by the same worker refreshes the TTL.
	current, err := l.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		// Expired between SetNX and Get; try once more.
		return l.rdb.SetNX(ctx, key, worker, ttl).Result()
	}
	if err != nil {
		return false, err
	}
	if current == worker {
		if err := l.rdb.PExpire(ctx, key, ttl).Err(); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// releaseScript deletes the lease key only when it is still held by the caller,
// avoiding the race where a worker deletes a lease another worker just acquired.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`)

func (l *RedisWorkerLease) Release(ctx context.Context, worker, shard string) error {
	return releaseScript.Run(ctx, l.rdb, []string{leaseKey(shard)}, worker).Err()
}
