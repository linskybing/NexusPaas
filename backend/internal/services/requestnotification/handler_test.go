package requestnotification

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesEventFedProjectAccessReadModels(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("request notification should use local event-fed project access read models, got isolation error: %v", err)
	}
}

func TestPublishFormEventPublishesTraceableFormFacts(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forms", nil)
	req.Header.Set("X-Trace-ID", "trace-123")
	req.Header.Set("Idempotency-Key", "form-create-1")
	projectID := "P1"
	form := Form{
		ID:          "F1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		UserID:      "U1",
		ProjectID:   &projectID,
		Title:       "Need GPU",
		Description: "training job",
		Tag:         "quota",
		Status:      "Pending",
	}

	publishFormEvent(app, req, "FormCreated", form)
	events := app.Events.Outbox()
	if len(events) != 1 {
		t.Fatalf("events = %d, want one form event", len(events))
	}
	event := events[0]
	if event.Name != "FormCreated" || event.Source != serviceName || event.TraceID != "trace-123" || event.IdempotencyKey != "form-create-1" {
		t.Fatalf("event metadata = %#v, want form event with propagated trace/idempotency", event)
	}
	if event.Data["id"] != "F1" || event.Data["project_id"] != "P1" || event.Data["status"] != "Pending" {
		t.Fatalf("event data = %#v, want serialized form", event.Data)
	}
	publishFormEvent(nil, req, "FormCreated", form)
	reqWithoutTrace := httptest.NewRequest(http.MethodPost, "/api/v1/forms", nil)
	publishFormEvent(app, reqWithoutTrace, "FormUpdated", form)
	if app.Events.Outbox()[1].TraceID == "" {
		t.Fatal("fallback trace id was empty")
	}
}

func TestCreateAndTransitionFormPublishEvents(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	service := NewService()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forms", bytes.NewBufferString(`{"title":"Need GPU","description":"training job","tag":"resource"}`))
	createReq.Header.Set("X-User-ID", "U1")
	createReq.Header.Set("X-Username", "alice")
	createReq.Header.Set("X-Trace-ID", "trace-create")

	code, data, degraded := service.createForm(app, createReq, platform.RouteSpec{})
	if degraded != nil || code != http.StatusCreated {
		t.Fatalf("create status=%d degraded=%v data=%#v, want 201", code, degraded, data)
	}
	form := data.(Form)
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/forms/"+form.ID, nil)
	updateReq.Header.Set("X-User-ID", "ADMIN")
	updateReq.Header.Set("X-Username", "admin")
	updateReq.Header.Set("X-User-Role", "admin")
	updateReq.Header.Set("X-Trace-ID", "trace-update")
	code, data, degraded = service.transitionForm(app, updateReq, form.ID, "Processing")
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("transition status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}

	events := app.Events.Outbox()
	if len(events) != 2 {
		t.Fatalf("events = %d, want create and update form events", len(events))
	}
	if events[0].Name != "FormCreated" || events[0].TraceID != "trace-create" {
		t.Fatalf("create event = %#v, want FormCreated with trace", events[0])
	}
	if events[1].Name != "FormUpdated" || events[1].TraceID != "trace-update" || events[1].Data["status"] != "Processing" {
		t.Fatalf("update event = %#v, want FormUpdated Processing with trace", events[1])
	}
}

func TestCreateProjectFormUsesEventFedAccessReadModelsWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	service := NewService()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forms", bytes.NewBufferString(`{"project_id":"P1","title":"Need GPU","description":"training job","tag":"resource"}`))
	req.Header.Set("X-User-ID", "U1")
	req.Header.Set("X-Username", "alice")

	code, _, _ := service.createForm(app, req, platform.RouteSpec{})
	if code != http.StatusForbidden {
		t.Fatalf("create before projection status = %d, want 403", code)
	}
	publishProjectAccessTestEvent(t, app, "ProjectCreated", map[string]any{"id": "P1", "owner_id": "G1"})
	publishProjectAccessTestEvent(t, app, "GroupMembershipChanged", map[string]any{"user_id": "U1", "group_id": "G1", "role": "member", "action": "create"})

	req = httptest.NewRequest(http.MethodPost, "/api/v1/forms", bytes.NewBufferString(`{"project_id":"P1","title":"Need GPU","description":"training job","tag":"resource"}`))
	req.Header.Set("X-User-ID", "U1")
	req.Header.Set("X-Username", "alice")
	code, data, degraded := service.createForm(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusCreated {
		t.Fatalf("create status=%d degraded=%v data=%#v, want 201", code, degraded, data)
	}
	form := data.(Form)
	if form.ProjectID == nil || *form.ProjectID != "P1" {
		t.Fatalf("form project id = %#v, want P1", form.ProjectID)
	}
	if got := len(app.Store.List(context.Background(), orgProjectsResource)); got != 0 {
		t.Fatalf("source org projects = %d, want isolated request notification to avoid owner store", got)
	}
	if _, ok := app.Store.Get(context.Background(), projectAccessProjects, "P1"); !ok {
		t.Fatal("missing projected project")
	}
	if _, ok := app.Store.Get(context.Background(), projectAccessUserGroups, "U1:G1"); !ok {
		t.Fatal("missing projected group membership")
	}
}

func TestProjectAccessProjectionUpdatesAndDeletesMemberships(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	publishProjectAccessTestEvent(t, app, "project_memberCreated", map[string]any{"project_id": "P1", "user_id": "U1", "role": "member"})
	syncProjectAccessReadModels(app, req)
	if _, ok := app.Store.Get(context.Background(), projectAccessMembers, "P1:U1"); !ok {
		t.Fatal("missing projected project membership")
	}
	projectProjectAccessEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "GroupMembershipChanged",
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data: map[string]any{
			"old": map[string]any{"user_id": "U1", "group_id": "G1", "role": "member"},
			"new": map[string]any{"user_id": "U1", "group_id": "G1", "role": "admin"},
		},
	})
	record, ok := app.Store.Get(context.Background(), projectAccessUserGroups, "U1:G1")
	if !ok || record.Data["role"] != "admin" {
		t.Fatalf("projected user group = %#v, want admin role", record.Data)
	}
	projectProjectAccessEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "GroupMembershipChanged",
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"user_id": "U1", "group_id": "G1", "action": "delete"},
	})
	if _, ok := app.Store.Get(context.Background(), projectAccessUserGroups, "U1:G1"); ok {
		t.Fatal("projected user group was not deleted")
	}
	projectProjectAccessEvent(app, req, contracts.Event{Name: "Unrelated", Data: map[string]any{"id": "noop"}})
	deleteProjectAccessReadModel(projectAccessRepo(app), req, projectAccessProjects, map[string]any{"id": "P1", "deleted": false})
}

func TestProjectAccessRecordsMergeProjectionWithCoHostedSource(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := app.Store.Create(context.Background(), orgProjectsResource, map[string]any{"id": "source", "name": "source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), projectAccessProjects, map[string]any{"id": "projected", "name": "projected"}); err != nil {
		t.Fatal(err)
	}

	records := projectAccessRecords(app, req, projectAccessProjects, orgProjectsResource)
	if len(records) != 2 {
		t.Fatalf("merged records = %#v, want source and projected records", records)
	}
}

func publishProjectAccessTestEvent(t *testing.T, app *platform.App, name string, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}
