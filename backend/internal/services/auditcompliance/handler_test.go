package auditcompliance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesEventFedProjectMemberReadModel(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("audit compliance should use local event-fed project member read model, got isolation error: %v", err)
	}
}

func TestCanReadProjectUsesEventFedProjectMembersWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/report?project_id=P1", nil)
	if canReadProject(app, req, "U1", "P1", false) {
		t.Fatal("canReadProject allowed access before projected membership")
	}
	publishProjectMemberTestEvent(t, app, "project_memberCreated", map[string]any{"project_id": "P1", "user_id": "U1", "role": "member"})
	if !canReadProject(app, req, "U1", "P1", false) {
		t.Fatal("canReadProject denied projected project membership")
	}
	if got := len(app.Store.List(context.Background(), orgProjectMembersResource)); got != 0 {
		t.Fatalf("source project members = %d, want isolated audit compliance to avoid owner store", got)
	}
	if _, ok := app.Store.Get(context.Background(), projectReportMembers, "P1:U1"); !ok {
		t.Fatal("missing projected project member")
	}
	if !canReadProject(app, req, "U1", "P1", false) {
		t.Fatal("canReadProject denied already-consumed projected project membership")
	}
}

func TestProjectMemberProjectionDeletesAndMergesCoHostedSource(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := app.Store.Create(context.Background(), orgProjectMembersResource, map[string]any{"id": "source", "project_id": "P2", "user_id": "U2"}); err != nil {
		t.Fatal(err)
	}
	projectMemberEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "project_memberCreated",
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"project_id": "P1", "user_id": "U1"},
	})
	if records := projectMemberRecords(app, req); len(records) != 2 {
		t.Fatalf("merged project members = %#v, want source and projected records", records)
	}
	projectMemberEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "project_memberDeleted",
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"project_id": "P1", "user_id": "U1", "deleted": true},
	})
	if _, ok := app.Store.Get(context.Background(), projectReportMembers, "P1:U1"); ok {
		t.Fatal("projected project member was not deleted")
	}
	projectMemberEvent(app, req, contracts.Event{Name: "Unrelated", Data: map[string]any{"id": "noop"}})
	deleteProjectMemberReadModel(app, req, map[string]any{"project_id": "P2", "user_id": "U2", "deleted": false})
}

func TestProjectMemberProjectionUpdatesExistingEnvelopeData(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	projectMemberEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "project_memberCreated",
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"project_id": "P1", "user_id": "U1", "role": "member"},
	})
	projectMemberEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "project_memberUpdated",
		Source:        "org-project-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data: map[string]any{
			"new": map[string]any{"project_id": "P1", "user_id": "U1", "role": "admin"},
		},
	})

	record, ok := app.Store.Get(context.Background(), projectReportMembers, "P1:U1")
	if !ok {
		t.Fatal("missing updated projected project member")
	}
	if record.Data["role"] != "admin" {
		t.Fatalf("projected role = %v, want admin", record.Data["role"])
	}
}

func TestProjectMemberProjectionDeadLettersFailedReadModelWrite(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	app.Store = auditProjectionFailStore{
		RecordStore: app.Store,
		createErr:   errors.New("write denied"),
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	publishProjectMemberTestEvent(t, app, "project_memberCreated", map[string]any{"project_id": "P1", "user_id": "U1"})

	syncProjectMemberReadModel(app, req)

	deadLetters := app.Store.List(context.Background(), "platform:dead_letters")
	if len(deadLetters) != 1 {
		t.Fatalf("dead letters = %#v, want one failed project-member projection", deadLetters)
	}
	record := deadLetters[0].Data
	if record["consumer"] != projectMemberConsumer || record["event_name"] != "project_memberCreated" {
		t.Fatalf("dead letter = %#v, want audit project-member consumer and event name", record)
	}
	errText, _ := record["error"].(string)
	if !strings.Contains(errText, "audit project-member projection create failed") {
		t.Fatalf("dead letter error = %q, want projection create failure", record["error"])
	}
}

func TestProjectMemberProjectionGuardsNilRuntime(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	syncProjectMemberReadModel(nil, req)
	if !canReadProject(nil, req, "U1", "P1", true) {
		t.Fatal("admin should be able to read without runtime dependencies")
	}
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	app.Store = nil
	syncProjectMemberReadModel(app, req)
	app = platform.NewApp(platform.Config{ServiceName: serviceName})
	app.Events = nil
	syncProjectMemberReadModel(app, req)
}

type auditProjectionFailStore struct {
	platform.RecordStore
	createErr error
}

func (s auditProjectionFailStore) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	if resource == projectReportMembers {
		return contracts.Record[map[string]any]{}, s.createErr
	}
	return s.RecordStore.Create(ctx, resource, data)
}

func publishProjectMemberTestEvent(t *testing.T, app *platform.App, name string, data map[string]any) {
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
