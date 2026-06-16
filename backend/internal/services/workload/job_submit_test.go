package workload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
)

const (
	testSchedulerQueuesResource     = "scheduler-quota-service:queues"
	testSchedulerPlansResource      = "scheduler-quota-service:plans"
	testSchedulerAdmissionsResource = "scheduler-quota-service:submit_admissions"
	testOrgProjectsResource         = "org-project-service:projects"
	testOrgProjectMembersResource   = "org-project-service:project_members"
)

func TestSubmitJobCallsAdmissionAndPersistsSubmittedJob(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})

	rec := serveSubmitJob(t, app, `{
		"project_id":"P1",
		"user_id":"U1",
		"queue_name":"default-batch",
		"required_gpu":1,
		"required_cpu":1,
		"required_memory":1024,
		"resources":[{"name":"train","kind":"Pod","json_data":"{\"apiVersion\":\"v1\",\"kind\":\"Pod\",\"spec\":{\"containers\":[{\"name\":\"main\",\"resources\":{\"limits\":{\"nvidia.com/gpu\":\"1\",\"cpu\":\"1\",\"memory\":\"1Gi\"}}}]}}"}]
	}`, "U1", http.StatusCreated)
	job := responseRecordData(t, rec)

	if job["status"] != "submitted" || job["queue_name"] != "default-batch" || job["required_gpu"] != float64(1) {
		t.Fatalf("submitted job = %#v, want submitted job with normalized admission fields", job)
	}
	if len(app.Store.List(context.Background(), jobsResource)) != 1 {
		t.Fatal("submitted job was not persisted in workload-service:jobs")
	}
	if len(app.Store.List(context.Background(), testSchedulerAdmissionsResource)) != 1 {
		t.Fatal("scheduler admission review was not recorded")
	}
	if len(app.Events.Outbox()) == 0 {
		t.Fatal("job submit did not publish a domain event")
	}
}

func TestSubmitJobDeniedAdmissionDoesNotCreateJob(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{"max_queued_jobs_per_user": 1})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":           "J-existing",
		"project_id":   "P1",
		"user_id":      "U1",
		"status":       "submitted",
		"required_cpu": 1,
	})

	before := len(app.Store.List(context.Background(), jobsResource))
	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusConflict)
	data := responseEnvelopeData(t, rec)

	if !strings.Contains(data["reason"].(string), "max queued jobs exceeded") {
		t.Fatalf("denial = %#v, want queued-limit reason", data)
	}
	if after := len(app.Store.List(context.Background(), jobsResource)); after != before {
		t.Fatalf("job count = %d, want unchanged %d after denied admission", after, before)
	}
}

func TestSubmitJobDerivesProjectFromConfigCommit(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "train.yaml"})
	createWorkloadRecord(t, app, versionsResource, map[string]any{"id": "ver1", "config_id": "cfg1"})

	rec := serveSubmitJob(t, app, `{"config_commit_id":"ver1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusCreated)
	job := responseRecordData(t, rec)

	if job["project_id"] != "P1" || job["config_id"] != "cfg1" || job["config_commit_id"] != "ver1" {
		t.Fatalf("job config context = %#v, want project/config derived from commit", job)
	}
}

func TestSubmitJobRejectsInvalidConfigContextWithoutPartialWrites(t *testing.T) {
	tests := []struct {
		name    string
		seed    func(*testing.T, *platform.App)
		body    string
		want    int
		message string
	}{
		{
			name:    "missing config commit",
			body:    `{"config_commit_id":"missing","user_id":"U1"}`,
			want:    http.StatusNotFound,
			message: "config commit not found",
		},
		{
			name: "missing config file",
			seed: func(t *testing.T, app *platform.App) {
				t.Helper()
				createWorkloadRecord(t, app, versionsResource, map[string]any{"id": "ver1", "config_id": "missing"})
			},
			body:    `{"config_commit_id":"ver1","user_id":"U1"}`,
			want:    http.StatusNotFound,
			message: "config file not found",
		},
		{
			name: "project mismatch",
			seed: func(t *testing.T, app *platform.App) {
				t.Helper()
				createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "train.yaml"})
			},
			body:    `{"project_id":"P2","config_id":"cfg1","user_id":"U1"}`,
			want:    http.StatusBadRequest,
			message: "project_id does not match config file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newJobSubmitTestApp()
			if tt.seed != nil {
				tt.seed(t, app)
			}
			rec := serveSubmitJob(t, app, tt.body, "U1", tt.want)
			data := responseEnvelopeData(t, rec)
			if data["message"] != tt.message {
				t.Fatalf("response data = %#v, want message %q", data, tt.message)
			}
			if got := len(app.Store.List(context.Background(), jobsResource)); got != 0 {
				t.Fatalf("job count = %d, want 0", got)
			}
			if got := len(app.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != 0 {
				t.Fatalf("admission count = %d, want 0", got)
			}
		})
	}
}

func TestSubmitJobRejectsMalformedJSONWithoutWrites(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})

	serveSubmitJob(t, app, `{`, "U1", http.StatusBadRequest)

	if got := len(app.Store.List(context.Background(), jobsResource)); got != 0 {
		t.Fatalf("job count = %d, want 0", got)
	}
	if got := len(app.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != 0 {
		t.Fatalf("admission count = %d, want 0", got)
	}
}

func TestSubmitJobUsesRemoteSchedulerAdmissionWhenIsolated(t *testing.T) {
	serviceKey := "service-secret"
	schedulerApp := platform.NewApp(platform.Config{
		ServiceName:      schedulerServiceName,
		HTTPAddr:         ":0",
		RequireAuth:      true,
		APIKeys:          map[string]bool{serviceKey: true},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{serviceKey: {ID: serviceName, Role: "service"}},
	})
	registerSchedulerAdmissionRoute(schedulerApp)
	schedulerquota.Register(schedulerApp)
	seedJobAdmissionProject(t, schedulerApp, map[string]any{})
	var received http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Clone()
		schedulerApp.ServeHTTP(w, r)
	}))
	defer server.Close()

	app := platform.NewApp(platform.Config{
		ServiceName:    serviceName,
		HTTPAddr:       ":0",
		ServiceURLs:    map[string]string{schedulerServiceName: server.URL},
		ServiceAPIKey:  serviceKey,
		AdapterTimeout: time.Second,
	})
	registerWorkloadJobRoute(app)
	Register(app)

	rec := serveSubmitJobWithHeaders(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "", map[string]string{
		"Authorization":   "Bearer caller-token",
		"Cookie":          "token=caller-cookie",
		"Idempotency-Key": "submit-idempotency",
		"Traceparent":     "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"X-API-Key":       "caller-api-key",
		"X-Request-ID":    "req-submit",
		"X-Service-Key":   "caller-service-key",
		"X-Trace-ID":      "trace-submit",
	}, http.StatusCreated)
	job := responseRecordData(t, rec)

	if job["project_id"] != "P1" || len(app.Store.List(context.Background(), jobsResource)) != 1 {
		t.Fatalf("remote-admitted job = %#v, want local persisted job", job)
	}
	if got := len(schedulerApp.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != 1 {
		t.Fatalf("remote scheduler admissions = %d, want 1", got)
	}
	if received.Get("X-Service-Key") != serviceKey || received.Get("X-API-Key") != serviceKey {
		t.Fatalf("remote scheduler service auth headers = X-Service-Key:%q X-API-Key:%q, want service key", received.Get("X-Service-Key"), received.Get("X-API-Key"))
	}
	traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if received.Get("X-Request-ID") != "req-submit" ||
		received.Get("X-Trace-ID") != traceparent ||
		received.Get("Traceparent") != traceparent ||
		received.Get("Idempotency-Key") != "submit-idempotency" {
		t.Fatalf("remote scheduler correlation headers = %#v, want caller request/trace/idempotency context", received)
	}
	for _, key := range []string{"Authorization", "Cookie"} {
		if got := received.Get(key); got != "" {
			t.Fatalf("remote scheduler received caller credential %s=%q", key, got)
		}
	}
}

func newJobSubmitTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	registerWorkloadJobRoute(app)
	registerSchedulerAdmissionRoute(app)
	schedulerquota.Register(app)
	Register(app)
	return app
}

func registerWorkloadJobRoute(app *platform.App) {
	app.RegisterService(platform.ServiceSpec{
		Name: serviceName,
		Routes: []platform.RouteSpec{{
			Method:       http.MethodPost,
			Pattern:      "/api/v1/jobs",
			Resource:     "jobs",
			Action:       "command",
			AuthRequired: true,
		}},
	})
}

func registerSchedulerAdmissionRoute(app *platform.App) {
	app.RegisterService(platform.ServiceSpec{
		Name: schedulerServiceName,
		Routes: []platform.RouteSpec{{
			Method:       http.MethodPost,
			Pattern:      schedulerAdmissionPath,
			Resource:     "submit_admissions",
			Action:       "review",
			AuthRequired: true,
		}},
	})
}

func seedJobAdmissionProject(t *testing.T, app *platform.App, projectOverrides map[string]any) {
	t.Helper()
	createWorkloadRecord(t, app, testSchedulerQueuesResource, map[string]any{"id": "q1", "name": "default-batch"})
	createWorkloadRecord(t, app, testSchedulerPlansResource, map[string]any{
		"id":                 "plan-1",
		"name":               "default",
		"gpu_limit":          4.0,
		"cpu_limit_cores":    8.0,
		"memory_limit_gb":    16.0,
		"queue_ids":          []string{"q1"},
		"allowed_gpu_models": []string{"gpu.nvidia.com"},
	})
	project := map[string]any{
		"id":                           "P1",
		"plan_id":                      "plan-1",
		"max_concurrent_jobs_per_user": 3,
		"max_queued_jobs_per_user":     5,
	}
	for key, value := range projectOverrides {
		project[key] = value
	}
	createWorkloadRecord(t, app, testOrgProjectsResource, project)
	createWorkloadRecord(t, app, testOrgProjectMembersResource, map[string]any{
		"id":         "P1/U1",
		"project_id": "P1",
		"user_id":    "U1",
		"role":       "user",
	})
}

func serveSubmitJob(t *testing.T, app http.Handler, body, userID string, want int) *httptest.ResponseRecorder {
	t.Helper()
	return serveSubmitJobWithHeaders(t, app, body, userID, nil, want)
}

func serveSubmitJobWithHeaders(t *testing.T, app http.Handler, body, userID string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", "test-job-submit")
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
		req.Header.Set("X-Username", userID)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("POST /api/v1/jobs returned %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	return rec
}

func responseRecordData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	record := responseEnvelopeData(t, rec)
	data, ok := record["data"].(map[string]any)
	if !ok {
		t.Fatalf("record data = %#v, want object", record["data"])
	}
	return data
}

func responseEnvelopeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}
