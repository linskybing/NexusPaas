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

type RecordEventBuilder func(contracts.Record[map[string]any]) contracts.Event
type DeleteEventBuilder func(deleted bool) contracts.Event

type recordEventBuilder = RecordEventBuilder
type deleteEventBuilder = DeleteEventBuilder

// transactionalRecordStore is an optional extension for stores that can commit a
// durable owner write and its outbox event in one transaction. It is deliberately
// not part of RecordStore so in-memory and HTTP-decorated stores stay small.
type transactionalRecordStore interface {
	CreateWithEvent(ctx context.Context, resource string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], error)
	UpdateWithEvent(ctx context.Context, resource, id string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], bool, error)
	DeleteWithEvent(ctx context.Context, resource, id string, buildEvent deleteEventBuilder) (bool, error)
}

// transactionalUpsertRecordStore is a separate optional extension so stores that
// support create/update/delete transactional event coupling are not forced to add
// upsert unless they can implement it atomically.
type transactionalUpsertRecordStore interface {
	UpsertWithEvent(ctx context.Context, resource, id string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], error)
}

// StoreTx is a transaction scope handed to an App.WithTx callback. Record writes
// and Emit-ed events all commit together (Postgres) so a multi-record operation
// (a cascade, or a parent + children) cannot leave the outbox out of sync with the
// owner write. Emitted events are inserted into the outbox just before commit.
type StoreTx interface {
	Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error)
	Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error)
	Delete(ctx context.Context, resource, id string) (bool, error)
	Emit(event contracts.Event)
}

// transactionalScopedStore is the optional capability behind App.WithTx. Kept
// separate from transactionalRecordStore (Interface Segregation) so the
// single-record fast path and in-memory/decorated stores are unaffected.
type transactionalScopedStore interface {
	RunInTx(ctx context.Context, fn func(tx StoreTx) error) error
}

// EventStream is the eventing port. It extends the shared contracts.EventBus with
// the checkpoint/lag operations the rollback gate relies on, so callers depend on
// an abstraction rather than the concrete in-memory bus.
type EventStream interface {
	contracts.EventBus
	Checkpoint(consumer string)
	Lag(consumer string) int
	// ResetConsumer clears a consumer's idempotency/checkpoint state for full
	// read-model rebuild workflows.
	ResetConsumer(consumer string)
	// ResetConsumerEvents clears idempotency state for specific event IDs without
	// clearing checkpoints. It is used for dead-letter retry so successful events
	// are not replayed.
	ResetConsumerEvents(consumer string, eventIDs []string)
}

type eventRelayResult struct {
	Selected   int
	Published  int
	Retried    int
	DeadLetter int
}

type eventRelay interface {
	RelayPending(ctx context.Context, limit int) (eventRelayResult, error)
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

func WithEventRelay(relay eventRelay, batchSize int) Option {
	return func(a *App) {
		if relay == nil {
			return
		}
		a.RegisterMaintenanceTaskForService("all", "event-outbox-relay", func(ctx context.Context) error {
			_, err := relay.RelayPending(ctx, batchSize)
			return err
		})
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
