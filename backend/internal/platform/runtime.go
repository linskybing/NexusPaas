package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

const (
	defaultRateLimit  = 600
	defaultRateWindow = time.Minute
)

// BackingResources holds the runtime options that wire real backing services
// (Postgres store, Redis leases/rate/sessions, Redis-Streams events) selected by
// config, plus the closers to release them on shutdown. When a backing URL is
// unset the corresponding in-memory default is left in place, so tests and local
// no-dependency runs are unaffected.
type BackingResources struct {
	Options          []Option
	closers          []func()
	postgresEventBus *PostgresEventBus
}

// NewBackingResources connects to the configured backing services and returns
// the options to pass to NewApp. The caller must invoke Close on shutdown.
func NewBackingResources(ctx context.Context, cfg Config) (*BackingResources, error) {
	br := &BackingResources{}
	// Protocol-level readiness probes built from the live clients (finding 13).
	checks := map[string]func(context.Context) error{}

	setupSteps := []func(context.Context, Config, map[string]func(context.Context) error) error{
		br.addDatabaseBacking,
		br.addRedisBacking,
		br.addEventBusBacking,
		br.addObjectStoreBacking,
		br.addClusterBacking,
	}
	for _, setup := range setupSteps {
		if err := setup(ctx, cfg, checks); err != nil {
			br.Close()
			return nil, err
		}
	}

	br.addBackingChecker(cfg, checks)
	return br, nil
}

func (b *BackingResources) addDatabaseBacking(ctx context.Context, cfg Config, checks map[string]func(context.Context) error) error {
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		b.postgresEventBus = NewPostgresEventBus(pool)
		b.Options = append(b.Options,
			WithStore(NewPostgresStore(pool)),
			WithEventBus(b.postgresEventBus),
		)
		b.closers = append(b.closers, pool.Close)
		checks[envDatabaseURL] = func(ctx context.Context) error { return pool.Ping(ctx) }
	}
	return nil
}

func (b *BackingResources) addRedisBacking(_ context.Context, cfg Config, checks map[string]func(context.Context) error) error {
	if cfg.RedisURL != "" {
		rdb, err := newRedisClient(cfg.RedisURL)
		if err != nil {
			return err
		}
		b.Options = append(b.Options,
			WithLeases(NewRedisWorkerLease(rdb)),
			WithRateLimiter(NewRedisLimiter(rdb, defaultRateLimit, defaultRateWindow)),
			WithRevocations(NewRedisRevocations(rdb)),
		)
		b.closers = append(b.closers, func() { _ = rdb.Close() })
		checks[envRedisURL] = func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
	}
	return nil
}

func (b *BackingResources) addEventBusBacking(_ context.Context, cfg Config, checks map[string]func(context.Context) error) error {
	if cfg.EventBusURL != "" {
		rdb, err := newRedisClient(cfg.EventBusURL)
		if err != nil {
			return fmt.Errorf("connect event bus: %w", err)
		}
		redisBus := NewRedisEventBus(rdb)
		if b.postgresEventBus != nil {
			b.postgresEventBus.WithRelaySink(redisBus)
			b.Options = append(b.Options, WithEventRelay(b.postgresEventBus, cfg.EffectiveEventRelayBatchSize()))
		} else {
			b.Options = append(b.Options, WithEventBus(redisBus))
		}
		b.closers = append(b.closers, func() { _ = rdb.Close() })
		checks[envEventBusURL] = func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
	}
	return nil
}

func (b *BackingResources) addObjectStoreBacking(ctx context.Context, cfg Config, checks map[string]func(context.Context) error) error {
	if cfg.RequiresObjectStore() && cfg.ObjectStoreURL != "" {
		store, err := NewMinioObjectStore(ctx, cfg.ObjectStoreURL, cfg.ObjectStoreAccessKey, cfg.ObjectStoreSecretKey, cfg.ObjectStoreBucket)
		if err != nil {
			return fmt.Errorf("connect object store: %w", err)
		}
		b.Options = append(b.Options, WithObjectStore(store))
		checks[envObjectStoreURL] = func(ctx context.Context) error { return store.HealthCheck(ctx) }
	}
	return nil
}

func (b *BackingResources) addClusterBacking(_ context.Context, cfg Config, _ map[string]func(context.Context) error) error {
	// Build the real Kubernetes cluster client from the ambient config
	// (in-cluster service account, or KUBECONFIG). When neither is present the
	// client is nil and reaper/reconciler workers run in degraded (no-op) mode.
	clusterClient, err := cluster.NewFromEnv(cfg.K8sNamespacePrefix)
	if err != nil {
		return fmt.Errorf("connect kubernetes cluster: %w", err)
	}
	if clusterClient != nil {
		b.Options = append(b.Options, WithCluster(clusterClient))
	}
	return nil
}

func (b *BackingResources) addBackingChecker(cfg Config, checks map[string]func(context.Context) error) {
	if len(checks) > 0 {
		b.Options = append(b.Options, WithBackingChecker(compositeBackingChecker{
			checks:   checks,
			fallback: TCPBackingChecker{Timeout: cfg.AdapterTimeout},
		}))
	}
}

// Close releases every backing-service connection.
func (b *BackingResources) Close() {
	for i := len(b.closers) - 1; i >= 0; i-- {
		b.closers[i]()
	}
}
