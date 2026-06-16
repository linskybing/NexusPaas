package platform

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisWorkerLeaseAcquireRefreshAndRelease(t *testing.T) {
	server, client := newTestRedisClient(t)
	lease := NewRedisWorkerLease(client)
	ctx := context.Background()

	ok, err := lease.Acquire(ctx, "worker-a", "shard-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
	}
	ok, err = lease.Acquire(ctx, "worker-b", "shard-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("different worker acquired an active lease")
	}
	ok, err = lease.Acquire(ctx, "worker-a", "shard-1", 2*time.Minute)
	if err != nil || !ok {
		t.Fatalf("same worker reacquire ok=%v err=%v", ok, err)
	}
	if ttl := server.TTL(leaseKey("shard-1")); ttl <= time.Minute {
		t.Fatalf("lease TTL = %v, want refreshed above 1m", ttl)
	}
	if err := lease.Release(ctx, "worker-b", "shard-1"); err != nil {
		t.Fatal(err)
	}
	if !server.Exists(leaseKey("shard-1")) {
		t.Fatal("non-owner release removed the lease")
	}
	if err := lease.Release(ctx, "worker-a", "shard-1"); err != nil {
		t.Fatal(err)
	}
	if server.Exists(leaseKey("shard-1")) {
		t.Fatal("owner release left the lease key")
	}
}

func TestRedisWorkerLeaseAcquireAfterExpiry(t *testing.T) {
	server, client := newTestRedisClient(t)
	lease := NewRedisWorkerLease(client)
	ctx := context.Background()

	if ok, err := lease.Acquire(ctx, "worker-a", "shard-1", time.Second); err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
	}
	server.FastForward(2 * time.Second)
	ok, err := lease.Acquire(ctx, "worker-b", "shard-1", time.Second)
	if err != nil || !ok {
		t.Fatalf("acquire after expiry ok=%v err=%v", ok, err)
	}
}

func TestRedisLimiterSharesCountersAndExpiresWindow(t *testing.T) {
	server, client := newTestRedisClient(t)
	limiter := NewRedisLimiter(client, 2, time.Minute)

	firstAllowed := limiter.Allow("user:1")
	secondAllowed := limiter.Allow("user:1")
	if !firstAllowed || !secondAllowed {
		t.Fatalf("firstAllowed=%v secondAllowed=%v, want both true", firstAllowed, secondAllowed)
	}
	if limiter.Allow("user:1") {
		t.Fatal("third request should be limited")
	}
	if ttl := server.TTL("rate:user:1"); ttl <= 0 {
		t.Fatalf("rate key TTL = %v, want positive TTL", ttl)
	}
	server.FastForward(time.Minute + time.Second)
	if !limiter.Allow("user:1") {
		t.Fatal("request after window expiry should be allowed")
	}
}

func TestRedisLimiterFailsOpenWhenRedisUnavailable(t *testing.T) {
	server, client := newTestRedisClient(t)
	limiter := NewRedisLimiter(client, 1, time.Minute)
	server.Close()

	if !limiter.Allow("user:1") {
		t.Fatal("limiter should fail open when Redis is unavailable")
	}
}

func TestNewBackingResourcesInjectsRedisPorts(t *testing.T) {
	_, client := newTestRedisClient(t)
	backing, err := NewBackingResources(context.Background(), Config{
		RedisURL: client.Options().Addr,
	})
	if err == nil {
		backing.Close()
		t.Fatal("bare address should be rejected; RedisURL must be redis://")
	}

	server, _ := miniredis.Run()
	t.Cleanup(server.Close)
	backing, err = NewBackingResources(context.Background(), Config{
		RedisURL: "redis://" + server.Addr(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backing.Close()

	app := NewApp(Config{ServiceName: "all"}, backing.Options...)
	if _, ok := app.Leases.(*RedisWorkerLease); !ok {
		t.Fatalf("leases = %T, want *RedisWorkerLease", app.Leases)
	}
	if _, ok := app.Rate.(*RedisLimiter); !ok {
		t.Fatalf("rate limiter = %T, want *RedisLimiter", app.Rate)
	}
}

func newTestRedisClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		server.Close()
	})
	return server, client
}
