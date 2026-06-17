package schedulerquota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestAdmissionReaderForIsolatedAppUsesOwnerReadContracts(t *testing.T) {
	serviceKey := "scheduler-reader-key"
	server := startAdmissionOwnerReadServer(t, serviceKey)
	t.Cleanup(server.Close)

	app := platform.NewApp(platform.Config{
		ServiceName: serviceName,
		ServiceURLs: map[string]string{
			orgProjectServiceName: server.URL,
			"workload-service":    server.URL,
		},
		ServiceAPIKey: serviceKey,
	})
	if _, err := app.Store.Create(context.Background(), projectsResource, map[string]any{"id": "P1", "name": "local-stale-project"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), plansResource, map[string]any{"id": "plan-1", "name": "local-plan"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), queuesResource, map[string]any{"id": "queue-1", "name": "local-queue"}); err != nil {
		t.Fatal(err)
	}

	reader := newAdmissionReaderForApp(app)
	project, found := reader.Project(context.Background(), "P1")
	assertAdmissionRecordField(t, project, found, "name", "remote-project", "project")
	assertAdmissionListField(t, reader.ListProjects(context.Background()), "name", "remote-project", "projects")
	member, found := reader.ProjectMember(context.Background(), "P1/U1")
	assertAdmissionRecordID(t, member, found, "P1/U1", "project member")
	assertAdmissionListID(t, reader.ListProjectMembers(context.Background()), "P1/U1", "project members")
	quota, found := reader.UserQuota(context.Background(), "P1/U1")
	assertAdmissionRecordID(t, quota, found, "P1/U1", "user quota")
	assertAdmissionListID(t, reader.ListUserQuotas(context.Background()), "P1/U1", "user quotas")
	assertAdmissionListID(t, reader.ListUserGroups(context.Background()), "G1/U1", "groups")
	assertAdmissionListID(t, reader.ListWorkloadJobs(context.Background()), "J1", "jobs")
	plan, found := reader.Plan(context.Background(), "plan-1")
	assertAdmissionRecordField(t, plan, found, "name", "local-plan", "plan")
	queue, found := reader.Queue(context.Background(), "queue-1")
	assertAdmissionRecordField(t, queue, found, "name", "local-queue", "queue")
	assertAdmissionListID(t, reader.ListQueues(context.Background()), "queue-1", "queues")
}

func TestAdmissionReaderForIsolatedAppFailsClosedWithBadServiceKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Key") != "correct-key" {
			platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, []contracts.Record[map[string]any]{
			{ID: "P1", Data: map[string]any{"id": "P1"}},
		})
	}))
	t.Cleanup(server.Close)

	app := platform.NewApp(platform.Config{
		ServiceName:      serviceName,
		ServiceURLs:      map[string]string{orgProjectServiceName: server.URL},
		ServiceAPIKey:    "wrong-key",
		AdapterTimeout:   0,
		AdapterRetries:   0,
		AdapterThreshold: 0,
	})
	reader := newAdmissionReaderForApp(app)

	if project, found := reader.Project(context.Background(), "P1"); found {
		t.Fatalf("project = %#v found=true, want fail-closed miss", project)
	}
	if projects := reader.ListProjects(context.Background()); len(projects) != 0 {
		t.Fatalf("projects = %#v, want fail-closed empty list", projects)
	}
}

func startAdmissionOwnerReadServer(t *testing.T, serviceKey string) *httptest.Server {
	t.Helper()
	routes := map[string]any{
		"/internal/org-project/projects": []contracts.Record[map[string]any]{
			{ID: "P1", Data: map[string]any{"id": "P1", "name": "remote-project"}},
		},
		"/internal/org-project/projects/P1": contracts.Record[map[string]any]{
			ID: "P1", Data: map[string]any{"id": "P1", "name": "remote-project"},
		},
		"/internal/org-project/project-members/P1/U1": contracts.Record[map[string]any]{
			ID: "P1/U1", Data: map[string]any{"id": "P1/U1", "project_id": "P1", "user_id": "U1"},
		},
		"/internal/org-project/project-members": []contracts.Record[map[string]any]{
			{ID: "P1/U1", Data: map[string]any{"id": "P1/U1", "project_id": "P1", "user_id": "U1"}},
		},
		"/internal/org-project/user-quotas/P1/U1": contracts.Record[map[string]any]{
			ID: "P1/U1", Data: map[string]any{"id": "P1/U1", "project_id": "P1", "user_id": "U1"},
		},
		"/internal/org-project/user-quotas": []contracts.Record[map[string]any]{
			{ID: "P1/U1", Data: map[string]any{"id": "P1/U1", "project_id": "P1", "user_id": "U1"}},
		},
		"/internal/org-project/user-groups": []contracts.Record[map[string]any]{
			{ID: "G1/U1", Data: map[string]any{"id": "G1/U1", "group_id": "G1", "user_id": "U1"}},
		},
		"/internal/workload/jobs": []contracts.Record[map[string]any]{
			{ID: "J1", Data: map[string]any{"id": "J1", "project_id": "P1", "user_id": "U1", "status": "running"}},
		},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Key") != serviceKey {
			platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		payload, ok := routes[r.URL.Path]
		if !ok {
			platform.WriteError(w, r, http.StatusNotFound, "not_found", "unexpected owner-read path")
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, payload)
	}))
}

func assertAdmissionRecordID(t *testing.T, record admissionRecord, found bool, want, label string) {
	t.Helper()
	if !found || record.ID != want {
		t.Fatalf("%s = %#v found=%v, want id %q", label, record, found, want)
	}
}

func assertAdmissionRecordField(t *testing.T, record admissionRecord, found bool, field, want, label string) {
	t.Helper()
	if !found || record.Data[field] != want {
		t.Fatalf("%s = %#v found=%v, want %s=%q", label, record, found, field, want)
	}
}

func assertAdmissionListID(t *testing.T, records []admissionRecord, want, label string) {
	t.Helper()
	if len(records) != 1 || records[0].ID != want {
		t.Fatalf("%s = %#v, want one record with id %q", label, records, want)
	}
}

func assertAdmissionListField(t *testing.T, records []admissionRecord, field, want, label string) {
	t.Helper()
	if len(records) != 1 || records[0].Data[field] != want {
		t.Fatalf("%s = %#v, want one record with %s=%q", label, records, field, want)
	}
}
