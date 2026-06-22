package platform

import (
	"context"
	"log/slog"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// WithTx runs fn inside one transaction when the store supports it (PostgresStore):
// every StoreTx write and Emit-ed event commits together or rolls back, so a
// multi-record operation (a cascade, or a record plus its sibling rows) stays
// atomic with its outbox event. For the in-memory store it falls back to applying
// the writes directly and publishing the events after fn returns — no atomicity
// off Postgres, matching the single-record *WithEvent fallback.
func (a *App) WithTx(ctx context.Context, fn func(tx StoreTx) error) error {
	if scoped, ok := scopedStoreFor(a.Store); ok {
		return scoped.RunInTx(ctx, fn)
	}
	adapter := &fallbackStoreTx{store: a.Store}
	if err := fn(adapter); err != nil {
		return err
	}
	for _, event := range adapter.events {
		if err := a.Events.Publish(ctx, event); err != nil {
			slog.Error(eventPublishFailedLogMsg, "error", err)
			return err
		}
	}
	return nil
}

func scopedStoreFor(store RecordStore) (transactionalScopedStore, bool) {
	if wrapped, ok := store.(*crossServiceStore); ok {
		return scopedStoreFor(wrapped.local)
	}
	scoped, ok := store.(transactionalScopedStore)
	return scoped, ok
}

// fallbackStoreTx applies StoreTx operations directly to a non-transactional store
// (the in-memory *Store) and buffers events for publication after the callback.
type fallbackStoreTx struct {
	store  RecordStore
	events []contracts.Event
}

func (t *fallbackStoreTx) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	return t.store.Create(ctx, resource, data)
}

func (t *fallbackStoreTx) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	record, ok := t.store.Update(ctx, resource, id, data)
	return record, ok, nil
}

func (t *fallbackStoreTx) Delete(ctx context.Context, resource, id string) (bool, error) {
	return t.store.Delete(ctx, resource, id), nil
}

func (t *fallbackStoreTx) Emit(event contracts.Event) {
	t.events = append(t.events, event)
}
