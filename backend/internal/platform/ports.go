package platform

import (
	"context"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

// RecordStore is the persistence port the App depends on. The in-memory *Store is
// the default implementation; an alternative (for example a Postgres-backed
// store) can be injected via WithStore without editing the platform core, which
// is what the Dependency Inversion Principle asks for. The interface is kept to
// exactly the operations App needs (Interface Segregation).
type RecordStore interface {
	Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error)
	Get(ctx context.Context, resource, id string) (contracts.Record[map[string]any], bool)
	List(ctx context.Context, resource string) []contracts.Record[map[string]any]
	Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool)
	Delete(ctx context.Context, resource, id string) bool
	// NextID atomically allocates a collision-free identifier for resource using
	// the given prefix, starting at base and zero-padded to width digits when
	// width>0. Implementations must make the allocation safe against concurrent
	// callers and must not reuse a previously allocated id.
	NextID(resource, prefix string, base, width int) string
}

// EventStream is the eventing port. It extends the shared contracts.EventBus with
// the checkpoint/lag operations the rollback gate relies on, so callers depend on
// an abstraction rather than the concrete in-memory bus.
type EventStream interface {
	contracts.EventBus
	Checkpoint(consumer string)
	Lag(consumer string) int
	// ResetConsumer clears a consumer's idempotency/checkpoint state so its events
	// are re-delivered on the next consume pass (projection replay).
	ResetConsumer(consumer string)
}

// Compile-time assertions that the in-memory defaults satisfy the ports.
var (
	_ RecordStore               = (*Store)(nil)
	_ EventStream               = (*EventBus)(nil)
	_ contracts.WorkerLease     = (*WorkerLeases)(nil)
	_ contracts.ExternalAdapter = (*ExternalAdapter)(nil)
	_ contracts.ProxyAdapter    = (*ExternalAdapter)(nil)
)

// Option configures an App at construction time. Options let the composition
// root inject alternative implementations of the runtime ports without modifying
// NewApp (Open/Closed).
type Option func(*App)

// WithStore injects the persistence implementation.
func WithStore(store RecordStore) Option {
	return func(a *App) {
		if store != nil {
			a.Store = store
		}
	}
}

// WithCluster injects the Kubernetes cluster client used by reconcilers and
// reaper workers. A nil client leaves the App in degraded mode, where cluster
// operations are no-ops, exactly as the reference treats an unset Clientset.
func WithCluster(c *cluster.Client) Option {
	return func(a *App) {
		if c != nil {
			a.Cluster = c
		}
	}
}

// WithEventBus injects the event stream implementation.
func WithEventBus(events EventStream) Option {
	return func(a *App) {
		if events != nil {
			a.Events = events
		}
	}
}

// WithLeases injects the worker-lease implementation.
func WithLeases(leases contracts.WorkerLease) Option {
	return func(a *App) {
		if leases != nil {
			a.Leases = leases
		}
	}
}

// WithRevocations injects the distributed credential revocation store. When unset
// the App uses an in-process denylist suitable for single-process/local runs.
func WithRevocations(store RevocationStore) Option {
	return func(a *App) {
		if store != nil {
			a.Revocations = store
		}
	}
}

// WithObjectStore injects the binary-blob object store (MinIO/S3). When unset
// the App leaves ObjectStore nil and blob-handling services fall back to
// RecordStore-inline storage for local no-dependency runs.
func WithObjectStore(store ObjectStore) Option {
	return func(a *App) {
		if store != nil {
			a.ObjectStore = store
		}
	}
}

// WithPDP injects the policy decision point.
func WithPDP(pdp contracts.PolicyDecisionPoint) Option {
	return func(a *App) {
		if pdp != nil {
			a.PDP = pdp
		}
	}
}

// WithBackingChecker injects production backing-service health checks used by
// readiness. Tests and alternate runtimes can provide protocol-specific checks.
func WithBackingChecker(checker BackingChecker) Option {
	return func(a *App) {
		if checker != nil {
			a.BackingChecker = checker
		}
	}
}

// WithAdapters injects external-service adapters by name, overriding the
// default URL-driven construction. Entries supplied here are preserved; any
// well-known adapter not provided still gets a no-op default in NewApp.
func WithAdapters(adapters map[string]contracts.ExternalAdapter) Option {
	return func(a *App) {
		for name, adapter := range adapters {
			if adapter != nil {
				a.Adapters[name] = adapter
			}
		}
	}
}

// WithMetrics injects the metrics sink.
func WithMetrics(metrics *Metrics) Option {
	return func(a *App) {
		if metrics != nil {
			a.Metrics = metrics
		}
	}
}

// WithRateLimiter injects the rate limiter.
func WithRateLimiter(limiter Limiter) Option {
	return func(a *App) {
		if limiter != nil {
			a.Rate = limiter
		}
	}
}

// WithSwitches injects the route-switch (rollback/traffic) controller.
func WithSwitches(switches *RouteSwitches) Option {
	return func(a *App) {
		if switches != nil {
			a.Switches = switches
		}
	}
}
