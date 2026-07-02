package platform

import (
	"context"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// fakeTxStore is an in-memory Store that also satisfies transactionalRecordStore,
// so transactionalStoreFor detects it and App routes the event into the write.
type fakeTxStore struct {
	*Store
	createWithEvent bool
	updateWithEvent bool
	upsertWithEvent bool
	deleteWithEvent bool
	ranInTx         bool
	committed       []contracts.Event
}

// RunInTx makes fakeTxStore a txRunner: it buffers Emit-ed events
// and only "commits" them (records them) when the callback succeeds, mirroring the
// Postgres adapter's commit-or-rollback of the outbox inserts.
func (f *fakeTxStore) RunInTx(ctx context.Context, fn func(tx StoreTx) error) error {
	f.ranInTx = true
	adapter := &fallbackStoreTx{store: f.Store}
	if err := fn(adapter); err != nil {
		return err
	}
	f.committed = append(f.committed, adapter.events...)
	return nil
}

func (f *fakeTxStore) CreateWithEvent(ctx context.Context, resource string, data map[string]any, _ recordEventBuilder) (contracts.Record[map[string]any], error) {
	f.createWithEvent = true
	return f.Store.Create(ctx, resource, data)
}

func (f *fakeTxStore) UpdateWithEvent(ctx context.Context, resource, id string, data map[string]any, _ recordEventBuilder) (contracts.Record[map[string]any], bool, error) {
	f.updateWithEvent = true
	rec, ok := f.Store.Update(ctx, resource, id, data)
	return rec, ok, nil
}

func (f *fakeTxStore) UpsertWithEvent(ctx context.Context, resource, id string, data map[string]any, _ recordEventBuilder) (contracts.Record[map[string]any], error) {
	f.upsertWithEvent = true
	payload := cloneMap(data)
	payload["id"] = id
	if rec, ok := f.Store.Update(ctx, resource, id, payload); ok {
		return rec, nil
	}
	return f.Store.Create(ctx, resource, payload)
}

func (f *fakeTxStore) DeleteWithEvent(ctx context.Context, resource, id string, _ deleteEventBuilder) (bool, error) {
	f.deleteWithEvent = true
	return f.Store.Delete(ctx, resource, id), nil
}

func eventBuilder(rec contracts.Record[map[string]any]) contracts.Event {
	ev := sampleEvent("evt-" + rec.ID)
	ev.Data = rec.Data
	return ev
}

// A transactional store must receive the event in-tx and NOT also see a separate
// Events.Publish (that would be the dual-write this fix closes).
func TestAppCreateRecordWithEventUsesTransactionalStore(t *testing.T) {
	store := &fakeTxStore{Store: NewStore()}
	bus := NewEventBus()
	app := &App{Store: store, Events: bus}

	rec, err := app.CreateRecordWithEvent(context.Background(), "svc:records", map[string]any{"id": "r1"}, eventBuilder)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "r1" || !store.createWithEvent {
		t.Fatalf("record=%#v createWithEvent=%v, want routed to CreateWithEvent", rec, store.createWithEvent)
	}
	if got := len(bus.Outbox()); got != 0 {
		t.Fatalf("Events.Publish called %d times, want 0 (event is committed in-tx)", got)
	}
}

// A non-transactional store falls back to Create + Events.Publish so the event is
// still emitted (behaviour preserved for the in-memory store used in tests).
func TestAppCreateRecordWithEventFallsBackToPublish(t *testing.T) {
	bus := NewEventBus()
	app := &App{Store: NewStore(), Events: bus}

	if _, err := app.CreateRecordWithEvent(context.Background(), "svc:records", map[string]any{"id": "r1"}, eventBuilder); err != nil {
		t.Fatal(err)
	}
	out := bus.Outbox()
	if len(out) != 1 || out[0].Data["id"] != "r1" {
		t.Fatalf("outbox=%#v, want one published event for r1", out)
	}
}

func TestAppUpsertRecordWithEventUsesTransactionalStore(t *testing.T) {
	store := &fakeTxStore{Store: NewStore()}
	bus := NewEventBus()
	app := &App{Store: store, Events: bus}

	rec, err := app.UpsertRecordWithEvent(context.Background(), "svc:records", "r1", map[string]any{"name": "created"}, eventBuilder)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "r1" || !store.upsertWithEvent {
		t.Fatalf("record=%#v upsertWithEvent=%v, want routed to UpsertWithEvent", rec, store.upsertWithEvent)
	}
	if got := len(bus.Outbox()); got != 0 {
		t.Fatalf("Events.Publish called %d times, want 0 (event is committed in-tx)", got)
	}
}

func TestAppUpsertRecordWithEventFallsBackToPublishForCreateAndUpdate(t *testing.T) {
	bus := NewEventBus()
	app := &App{Store: NewStore(), Events: bus}

	created, err := app.UpsertRecordWithEvent(context.Background(), "svc:records", "r1", map[string]any{"name": "created"}, eventBuilder)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := app.UpsertRecordWithEvent(context.Background(), "svc:records", "r1", map[string]any{"name": "updated"}, eventBuilder)
	if err != nil {
		t.Fatal(err)
	}
	out := bus.Outbox()
	if created.ID != "r1" || updated.Data["name"] != "updated" || len(out) != 2 {
		t.Fatalf("created=%#v updated=%#v outbox=%#v, want create/update events", created, updated, out)
	}
}

func TestAppUpsertRecordWithEventConflictFallbackUpdatesAndPublishesOnce(t *testing.T) {
	store := &conflictFallbackStore{record: contracts.Record[map[string]any]{ID: "r1", Data: map[string]any{"id": "r1", "name": "updated"}}}
	bus := NewEventBus()
	app := &App{Store: store, Events: bus}

	record, err := app.UpsertRecordWithEvent(context.Background(), "svc:records", "r1", map[string]any{"name": "updated"}, eventBuilder)
	if err != nil {
		t.Fatal(err)
	}
	if record.Data["name"] != "updated" || store.createCalls != 1 || store.updateCalls != 2 {
		t.Fatalf("record=%#v createCalls=%d updateCalls=%d, want conflict fallback update", record, store.createCalls, store.updateCalls)
	}
	if out := bus.Outbox(); len(out) != 1 || out[0].Data["name"] != "updated" {
		t.Fatalf("outbox=%#v, want one update event", out)
	}
}

func TestAppUpsertRecordWithEventDoesNotPublishWhenOwnerWriteFails(t *testing.T) {
	store := &failingUpsertStore{}
	bus := NewEventBus()
	app := &App{Store: store, Events: bus}

	if _, err := app.UpsertRecordWithEvent(context.Background(), "svc:records", "r1", map[string]any{"name": "failed"}, eventBuilder); err == nil {
		t.Fatal("UpsertRecordWithEvent err = nil, want owner write failure")
	}
	if out := bus.Outbox(); len(out) != 0 {
		t.Fatalf("outbox=%#v, want no event after failed owner write", out)
	}
}

// WithTx on a transactional store routes multi-record writes + the event through
// RunInTx (committed together) and does NOT separately publish to the bus.
func TestAppWithTxUsesScopedStore(t *testing.T) {
	store := &fakeTxStore{Store: NewStore()}
	bus := NewEventBus()
	app := &App{Store: store, Events: bus}

	err := app.WithTx(context.Background(), func(tx StoreTx) error {
		if _, err := tx.Create(context.Background(), "svc:records", map[string]any{"id": "a"}); err != nil {
			return err
		}
		if _, err := tx.Create(context.Background(), "svc:records", map[string]any{"id": "b"}); err != nil {
			return err
		}
		tx.Emit(sampleEvent("evt-cascade"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.ranInTx || len(store.committed) != 1 {
		t.Fatalf("ranInTx=%v committed=%d, want one event committed in-tx", store.ranInTx, len(store.committed))
	}
	if got := len(bus.Outbox()); got != 0 {
		t.Fatalf("Events.Publish called %d times, want 0 (event committed in-tx)", got)
	}
}

// A callback error aborts the transaction: no event is committed.
func TestAppWithTxRollsBackOnError(t *testing.T) {
	store := &fakeTxStore{Store: NewStore()}
	app := &App{Store: store, Events: NewEventBus()}

	wantErr := context.Canceled
	if err := app.WithTx(context.Background(), func(tx StoreTx) error {
		tx.Emit(sampleEvent("evt-doomed"))
		return wantErr
	}); err != wantErr {
		t.Fatalf("WithTx err = %v, want %v", err, wantErr)
	}
	if len(store.committed) != 0 {
		t.Fatalf("committed = %d, want 0 (rolled back)", len(store.committed))
	}
}

// Off Postgres, WithTx applies writes directly and publishes the events after fn.
func TestAppWithTxFallsBackToPublish(t *testing.T) {
	bus := NewEventBus()
	app := &App{Store: NewStore(), Events: bus}

	if err := app.WithTx(context.Background(), func(tx StoreTx) error {
		if _, err := tx.Create(context.Background(), "svc:records", map[string]any{"id": "a"}); err != nil {
			return err
		}
		tx.Emit(sampleEvent("evt-fallback"))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := len(bus.Outbox()); got != 1 {
		t.Fatalf("published %d events, want 1 after fallback fn", got)
	}
}

type conflictFallbackStore struct {
	record      contracts.Record[map[string]any]
	createCalls int
	updateCalls int
}

func (s *conflictFallbackStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, CreateConflictError{Resource: "svc:records", ID: "r1"}
}

func (s *conflictFallbackStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *conflictFallbackStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *conflictFallbackStore) Update(context.Context, string, string, map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	if s.updateCalls == 1 {
		return contracts.Record[map[string]any]{}, false
	}
	return s.record, true
}

func (s *conflictFallbackStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *conflictFallbackStore) NextID(string, string, int, int) string {
	return ""
}

type failingUpsertStore struct{}

func (s *failingUpsertStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	return contracts.Record[map[string]any]{}, context.Canceled
}

func (s *failingUpsertStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *failingUpsertStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *failingUpsertStore) Update(context.Context, string, string, map[string]any) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *failingUpsertStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *failingUpsertStore) NextID(string, string, int, int) string {
	return ""
}
