package platform

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RevocationStore is a distributed denylist for credentials that must be rejected
// before their natural expiry (logout, admin revoke). It gives multi-process
// coordination: a revoke on one replica is immediately visible to every replica
// sharing the backend, including for otherwise-stateless bearer credentials
// (finding 1). Entries are keyed by kind ("session", "api_token") and id and are
// stored with a TTL so the denylist self-prunes once the credential would expire.
type RevocationStore interface {
	Revoke(ctx context.Context, kind, id string, ttl time.Duration) error
	IsRevoked(ctx context.Context, kind, id string) (bool, error)
}

func revocationKey(kind, id string) string {
	return "revocation:" + kind + ":" + id
}

// MemoryRevocations is the in-process default used for local no-dependency runs
// and tests. It is safe for concurrent use and prunes expired entries lazily.
type MemoryRevocations struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

// NewMemoryRevocations returns an empty in-memory revocation store.
func NewMemoryRevocations() *MemoryRevocations {
	return &MemoryRevocations{entries: map[string]time.Time{}}
}

func (m *MemoryRevocations) Revoke(_ context.Context, kind, id string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Minute
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[revocationKey(kind, id)] = time.Now().Add(ttl)
	return nil
}

func (m *MemoryRevocations) IsRevoked(_ context.Context, kind, id string) (bool, error) {
	key := revocationKey(kind, id)
	m.mu.Lock()
	defer m.mu.Unlock()
	expiry, ok := m.entries[key]
	if !ok {
		return false, nil
	}
	if time.Now().After(expiry) {
		delete(m.entries, key)
		return false, nil
	}
	return true, nil
}

// RedisRevocations is the cross-replica revocation store. A revoked credential is
// a Redis key with a TTL so the denylist is shared and self-expiring.
type RedisRevocations struct {
	client *redis.Client
}

// NewRedisRevocations builds a Redis-backed revocation store.
func NewRedisRevocations(client *redis.Client) *RedisRevocations {
	return &RedisRevocations{client: client}
}

func (r *RedisRevocations) Revoke(ctx context.Context, kind, id string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return r.client.Set(ctx, revocationKey(kind, id), "1", ttl).Err()
}

func (r *RedisRevocations) IsRevoked(ctx context.Context, kind, id string) (bool, error) {
	n, err := r.client.Exists(ctx, revocationKey(kind, id)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

var (
	_ RevocationStore = (*MemoryRevocations)(nil)
	_ RevocationStore = (*RedisRevocations)(nil)
)
