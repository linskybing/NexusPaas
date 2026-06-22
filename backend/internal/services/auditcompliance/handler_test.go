package auditcompliance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const testServiceKey = "service-key"

func TestRegisterUsesEventFedProjectMemberReadModel(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("audit compliance should use local event-fed project member read model, got isolation error: %v", err)
	}
}

func TestAuditServiceSpecKeepsAuditRecordsAppendOnly(t *testing.T) {
	assertAuditGroupReadModel(t)
	assertAuditRoutesAppendOnly(t)
	assertAuditRetentionRoute(t)
}

func assertAuditGroupReadModel(t *testing.T) {
	t.Helper()
	for _, table := range Spec().Tables {
		if table == "group_report_members" {
			return
		}
	}
	t.Fatal("audit service spec is missing group_report_members read model table")
}

func assertAuditRoutesAppendOnly(t *testing.T) {
	t.Helper()
	for _, route := range Spec().Routes {
		if route.Resource != "audit_logs" && route.Resource != "audit_reports" {
			continue
		}
		if route.Method != http.MethodGet || route.StateChanging {
			t.Fatalf("audit read resource route %#v must not be mutable through normal APIs", route)
		}
		if route.Resource == "audit_logs" && (route.Admin || route.PolicyBypass) {
			t.Fatalf("audit log route %#v must use handler-scoped RBAC after PDP, not route admin or bypass", route)
		}
	}
}

func assertAuditRetentionRoute(t *testing.T) {
	t.Helper()
	for _, route := range Spec().Routes {
		if route.Resource != "audit_retention" {
			continue
		}
		if !route.Admin || !route.ServiceAuthRequired || !route.PolicyBypass {
			t.Fatalf("audit retention route %#v must stay admin-only and service-internal", route)
		}
		return
	}
	t.Fatal("audit retention route is missing from service spec")
}

func TestAuditLogsRouteStillHonorsPDPDenial(t *testing.T) {
	app := platform.NewApp(
		platform.Config{
			ServiceName: serviceName,
			RequireAuth: true,
			APIKeys:     map[string]bool{"audit-key": true},
			APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
				"audit-key": {ID: "AUDITOR", Username: "auditor", Role: "platform_auditor"},
			},
		},
		platform.WithPDP(platform.StaticPDP{Allowed: false, Reason: "denied by test"}),
	)
	app.RegisterService(Spec())
	Register(app)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs", nil)
	req.Header.Set("X-API-Key", "audit-key")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("audit logs PDP denial status=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
}

func TestAuditRetentionCleanupRouteRequiresServiceAuth(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
	}{
		{name: "missing service key"},
		{name: "invalid service key", key: "wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newAuditRetentionTestApp(30)
			seedAuditLog(t, app, "old", time.Now().UTC().AddDate(0, 0, -90))

			rec := callAuditRetentionCleanup(app, tc.key)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s, want 401", rec.Code, rec.Body.String())
			}
			if got := len(app.Store.List(context.Background(), auditLogResource)); got != 1 {
				t.Fatalf("audit logs after denied cleanup = %d, want 1", got)
			}
		})
	}
}

func TestAuditRetentionCleanupRouteDeletesOnlyExpiredLogs(t *testing.T) {
	app := newAuditRetentionTestApp(30)
	now := time.Now().UTC()
	seedAuditLog(t, app, "old", now.AddDate(0, 0, -90))
	seedAuditLog(t, app, "fresh", now.AddDate(0, 0, -5))

	rec := callAuditRetentionCleanup(app, testServiceKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	data := decodeAuditRetentionCleanupData(t, rec)
	if data["removed"] != float64(1) || data["retention_days"] != float64(30) {
		t.Fatalf("cleanup data = %#v, want removed=1 retention_days=30", data)
	}
	records := app.Store.List(context.Background(), auditLogResource)
	if len(records) != 1 || records[0].ID != "fresh" {
		t.Fatalf("remaining audit logs = %#v, want only fresh", records)
	}
}

func TestAuditRetentionCleanupRouteReportsDefaultRetention(t *testing.T) {
	app := newAuditRetentionTestApp(0)
	now := time.Now().UTC()
	seedAuditLog(t, app, "old", now.AddDate(0, 0, -45))
	seedAuditLog(t, app, "fresh", now.AddDate(0, 0, -5))

	rec := callAuditRetentionCleanup(app, testServiceKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	data := decodeAuditRetentionCleanupData(t, rec)
	if data["removed"] != float64(1) || data["retention_days"] != float64(defaultRetentionDays) {
		t.Fatalf("cleanup data = %#v, want removed=1 retention_days=%d", data, defaultRetentionDays)
	}
	records := app.Store.List(context.Background(), auditLogResource)
	if len(records) != 1 || records[0].ID != "fresh" {
		t.Fatalf("remaining audit logs = %#v, want only fresh", records)
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

func TestCanQueryGroupUsesEventFedGroupMembersWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs?group_id=G1", nil)
	if _, err := app.Store.Create(context.Background(), orgUserGroupsResource, map[string]any{"id": "source", "group_id": "G1", "user_id": "U1", "role": "admin"}); err != nil {
		t.Fatal(err)
	}
	if canQueryGroupAuditLogs(app, req, "U1", "G1") {
		t.Fatal("canQueryGroupAuditLogs read owner user_groups in isolated mode before projection")
	}
	publishGroupMemberTestEvent(t, app, map[string]any{"group_id": "G1", "user_id": "U1", "role": "admin", "action": "create"})
	if !canQueryGroupAuditLogs(app, req, "U1", "G1") {
		t.Fatal("canQueryGroupAuditLogs denied projected group admin membership")
	}
	if got := len(app.Store.List(context.Background(), orgUserGroupsResource)); got != 1 {
		t.Fatalf("source user_groups = %d, want isolated audit compliance to avoid owner store writes", got)
	}
	if _, ok := app.Store.Get(context.Background(), groupReportMembers, "G1:U1"); !ok {
		t.Fatal("missing projected group member")
	}
}

func TestGroupMemberProjectionCreatesUpdatesAndDeletes(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	publishGroupMemberTestEvent(t, app, map[string]any{"group_id": "G1", "user_id": "U1", "role": "member", "action": "create"})
	syncGroupMemberReadModel(app, req)

	record, ok := app.Store.Get(context.Background(), groupReportMembers, "G1:U1")
	if !ok || record.Data["role"] != "member" {
		t.Fatalf("created projected group member = %#v ok=%v, want member", record.Data, ok)
	}
	publishGroupMemberTestEvent(t, app, map[string]any{
		"old": map[string]any{"group_id": "G1", "user_id": "U1", "role": "member"},
		"new": map[string]any{"group_id": "G1", "user_id": "U1", "role": "admin"},
	})
	syncGroupMemberReadModel(app, req)
	record, ok = app.Store.Get(context.Background(), groupReportMembers, "G1:U1")
	if !ok || record.Data["role"] != "admin" {
		t.Fatalf("updated projected group member = %#v ok=%v, want admin", record.Data, ok)
	}
	publishGroupMemberTestEvent(t, app, map[string]any{"group_id": "G1", "user_id": "U1", "action": "delete"})
	syncGroupMemberReadModel(app, req)
	if _, ok := app.Store.Get(context.Background(), groupReportMembers, "G1:U1"); ok {
		t.Fatal("projected group member was not deleted")
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

func TestGroupMemberProjectionDeadLettersFailedReadModelWrite(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	app.Store = auditGroupProjectionFailStore{
		RecordStore: app.Store,
		createErr:   errors.New("write denied"),
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	publishGroupMemberTestEvent(t, app, map[string]any{"group_id": "G1", "user_id": "U1", "role": "admin", "action": "create"})

	syncGroupMemberReadModel(app, req)

	deadLetters := app.Store.List(context.Background(), "platform:dead_letters")
	if len(deadLetters) != 1 {
		t.Fatalf("dead letters = %#v, want one failed group-member projection", deadLetters)
	}
	record := deadLetters[0].Data
	if record["consumer"] != groupMemberConsumer || record["event_name"] != "GroupMembershipChanged" {
		t.Fatalf("dead letter = %#v, want audit group-member consumer and event name", record)
	}
	errText, _ := record["error"].(string)
	if !strings.Contains(errText, "audit group-member projection create failed") {
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

func newAuditRetentionTestApp(retentionDays int) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:        serviceName,
		ServiceAPIKey:      testServiceKey,
		AuditRetentionDays: retentionDays,
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func seedAuditLog(t *testing.T, app *platform.App, id string, createdAt time.Time) {
	t.Helper()
	row := map[string]any{
		"id":         id,
		"action":     "login",
		"created_at": createdAt.Format(time.RFC3339),
	}
	if _, err := app.Store.Create(context.Background(), auditLogResource, row); err != nil {
		t.Fatalf("seed audit log %s: %v", id, err)
	}
}

func callAuditRetentionCleanup(app *platform.App, serviceKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/audit/cleanup", nil)
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func decodeAuditRetentionCleanupData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode cleanup response: %v body=%s", err, rec.Body.String())
	}
	if !envelope.Success {
		t.Fatalf("cleanup response success=false body=%s", rec.Body.String())
	}
	return envelope.Data
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

type auditGroupProjectionFailStore struct {
	platform.RecordStore
	createErr error
}

func (s auditGroupProjectionFailStore) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	if resource == groupReportMembers {
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
