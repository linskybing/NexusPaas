package workload

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
)

type workloadTxStore struct {
	*platform.Store
	createWithEvent int
	updateWithEvent int
	deleteWithEvent int
	runInTx         int
	txEvents        []contracts.Event
}

func (s *workloadTxStore) CreateWithEvent(ctx context.Context, resource string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], error) {
	s.createWithEvent++
	record, err := s.Store.Create(ctx, resource, data)
	if err == nil && build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, err
}

func (s *workloadTxStore) UpdateWithEvent(ctx context.Context, resource, id string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], bool, error) {
	s.updateWithEvent++
	record, ok := s.Store.Update(ctx, resource, id, data)
	if ok && build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, ok, nil
}

func (s *workloadTxStore) DeleteWithEvent(ctx context.Context, resource, id string, build platform.DeleteEventBuilder) (bool, error) {
	s.deleteWithEvent++
	deleted := s.Store.Delete(ctx, resource, id)
	if deleted && build != nil {
		s.txEvents = append(s.txEvents, build(deleted))
	}
	return deleted, nil
}

func (s *workloadTxStore) RunInTx(ctx context.Context, fn func(platform.StoreTx) error) error {
	s.runInTx++
	tx := &workloadRecordingTx{store: s.Store}
	if err := fn(tx); err != nil {
		return err
	}
	s.txEvents = append(s.txEvents, tx.events...)
	return nil
}

func (s *workloadTxStore) resetTx() {
	s.createWithEvent = 0
	s.updateWithEvent = 0
	s.deleteWithEvent = 0
	s.runInTx = 0
	s.txEvents = nil
}

type workloadRecordingTx struct {
	store  *platform.Store
	events []contracts.Event
}

func (tx *workloadRecordingTx) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	return tx.store.Create(ctx, resource, data)
}

func (tx *workloadRecordingTx) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	record, ok := tx.store.Update(ctx, resource, id, data)
	return record, ok, nil
}

func (tx *workloadRecordingTx) Delete(ctx context.Context, resource, id string) (bool, error) {
	return tx.store.Delete(ctx, resource, id), nil
}

func (tx *workloadRecordingTx) Emit(event contracts.Event) {
	tx.events = append(tx.events, event)
}

func TestWorkloadJobAndConfigMutationsUseTransactionalEvents(t *testing.T) {
	app, store := newWorkloadTxTestApp(t)
	seedJobAdmissionProject(t, app, map[string]any{})

	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusCreated)
	job := responseRecordData(t, rec)
	jobID := job["id"].(string)
	if store.createWithEvent != 1 {
		t.Fatalf("submit createWithEvent=%d, want 1", store.createWithEvent)
	}
	assertWorkloadTxEvents(t, app, store, "JobSubmitted")

	store.resetTx()
	cancelReq := workloadAuthRequest(http.MethodPost, "/api/v1/jobs/"+jobID+"/cancel", `{}`, "U1", "user")
	cancelReq.SetPathValue("id", jobID)
	code, data, _ := cancelJob(app, cancelReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)
	if store.createWithEvent != 1 {
		t.Fatalf("cancel createWithEvent=%d, want 1", store.createWithEvent)
	}
	assertWorkloadTxEvents(t, app, store, "JobCancelRequested")

	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "train.yaml"})
	store.resetTx()
	commitReq := workloadRequest(http.MethodPost, "/api/v1/configfiles/cfg1/versions", `{"content":"kind: Job","message":"manual"}`)
	commitReq.SetPathValue("id", "cfg1")
	code, data, _ = commitConfigFileVersion(app, commitReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)
	if store.createWithEvent != 1 {
		t.Fatalf("commit createWithEvent=%d, want 1", store.createWithEvent)
	}
	assertWorkloadTxEvents(t, app, store, "ConfigCommitted")

	store.resetTx()
	startReq := workloadRequest(http.MethodPost, "/api/v1/configfiles/cfg1/instance", `{"namespace":"proj-p1"}`)
	startReq.SetPathValue("id", "cfg1")
	code, data, _ = startConfigInstance(app, startReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)
	if store.createWithEvent != 1 {
		t.Fatalf("instance command createWithEvent=%d, want 1", store.createWithEvent)
	}
	assertWorkloadTxEvents(t, app, store, "ConfigInstanceCommanded")
}

func newWorkloadTxTestApp(t *testing.T) (*platform.App, *workloadTxStore) {
	t.Helper()
	store := &workloadTxStore{Store: platform.NewStore()}
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"}, platform.WithStore(store))
	registerWorkloadJobRoute(app)
	registerWorkloadPreemptionRoutes(app)
	registerSchedulerAdmissionRoute(app)
	registerSchedulerPreemptionRoute(app)
	schedulerquota.Register(app)
	Register(app)
	return app, store
}

func assertWorkloadTxEvents(t *testing.T, app *platform.App, store *workloadTxStore, names ...string) {
	t.Helper()
	if len(store.txEvents) != len(names) {
		t.Fatalf("tx events = %#v, want names %v", store.txEvents, names)
	}
	for i, name := range names {
		if store.txEvents[i].Name != name {
			t.Fatalf("event[%d].Name = %q, want %q; events=%#v", i, store.txEvents[i].Name, name, store.txEvents)
		}
	}
	assertNoDirectWorkloadPublish(t, app, names...)
}

func assertNoDirectWorkloadPublish(t *testing.T, app *platform.App, names ...string) {
	t.Helper()
	blocked := map[string]bool{}
	for _, name := range names {
		blocked[name] = true
	}
	for _, event := range app.Events.Outbox() {
		if blocked[event.Name] {
			t.Fatalf("event %q was directly published to app.Events; outbox=%#v", event.Name, app.Events.Outbox())
		}
	}
}
