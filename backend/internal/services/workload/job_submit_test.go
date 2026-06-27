package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/orgproject"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testSchedulerQueuesResource      = "scheduler-quota-service:queues"
	testSchedulerPlansResource       = "scheduler-quota-service:plans"
	testSchedulerAdmissionsResource  = "scheduler-quota-service:submit_admissions"
	testSchedulerPreemptionsResource = "scheduler-quota-service:preemption_records"
	testOrgProjectsResource          = "org-project-service:projects"
	testOrgProjectMembersResource    = "org-project-service:project_members"
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

func TestSubmitJobPersistsAdmittedStreamingFields(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})

	rec := serveSubmitJob(t, app, `{
		"project_id":"P1",
		"user_id":"U1",
		"queue_name":"default-batch",
		"required_cpu":1,
		"required_memory":1024,
		"streaming_session":true
	}`, "U1", http.StatusCreated)
	job := responseRecordData(t, rec)

	if job["streaming_session"] != true || job["stream_max_bitrate_kbps"] != float64(12000) {
		t.Fatalf("stream fields = %#v, want admitted streaming session with default bitrate", job)
	}
}

func TestJobAdmissionPayloadIncludesAcceleratorFields(t *testing.T) {
	job := map[string]any{"job_id": "J1", "project_id": "P1", "user_id": "U1"}
	payload := map[string]any{
		"accelerator_profile": "nvidia-gpu-default",
		"sm_percentage":       50,
		"pinned_memory_limit": "8Gi",
	}

	admission := jobAdmissionPayload(job, payload)

	if admission["accelerator_profile"] != "nvidia-gpu-default" || admission["sm_percentage"] != 50 || admission["pinned_memory_limit"] != "8Gi" {
		t.Fatalf("admission payload = %#v, want accelerator fields", admission)
	}
}

func TestApplyAdmissionReviewPersistsAcceleratorFields(t *testing.T) {
	job := map[string]any{"job_id": "J1"}
	review := map[string]any{
		"accelerator_profile":       "nvidia-gpu-default",
		"accelerator_node_selector": map[string]any{"nexuspaas.io/gpu": "true"},
		"accelerator_labels":        map[string]any{"nexuspaas.io/accelerator-profile": "nvidia-gpu-default"},
		"sm_percentage":             50,
		"pinned_memory_limit":       "8Gi",
	}

	applyAdmissionReview(job, review)

	if job["accelerator_profile"] != "nvidia-gpu-default" || job["sm_percentage"] != 50 || job["pinned_memory_limit"] != "8Gi" {
		t.Fatalf("job = %#v, want accelerator fields", job)
	}
	if job["accelerator_node_selector"].(map[string]any)["nexuspaas.io/gpu"] != "true" {
		t.Fatalf("job selector = %#v, want copied selector", job["accelerator_node_selector"])
	}
}

func TestSubmitStreamingJobRejectedWithoutSidecarImage(t *testing.T) {
	app := newJobSubmitTestApp()
	app.Config.StreamSidecarImage = ""
	seedJobAdmissionProject(t, app, map[string]any{})

	serveSubmitJob(t, app, `{
		"project_id":"P1",
		"user_id":"U1",
		"queue_name":"default-batch",
		"required_cpu":1,
		"required_memory":1024,
		"streaming_session":true
	}`, "U1", http.StatusBadRequest)
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

func TestSubmitJobAutoPreemptsQuotaVictimAndPersistsRequesterAfterRetry(t *testing.T) {
	ctx := context.Background()
	cl := cluster.New(fake.NewSimpleClientset(workloadPreemptionPod("proj-p1", "victim-pod", "victim")), "proj")
	app := newJobSubmitTestAppWithCluster(cl)
	seedJobAdmissionProject(t, app, map[string]any{})
	app.Store.Update(ctx, testSchedulerQueuesResource, "q1", map[string]any{
		"priority_value": 10000,
		"is_preemptible": false,
	})
	app.Store.Update(ctx, testSchedulerPlansResource, "plan-1", map[string]any{
		"cpu_limit_cores": 1.5,
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":             "victim",
		"job_id":         "victim",
		"project_id":     "P1",
		"user_id":        "U2",
		"status":         "running",
		"namespace":      "proj-p1",
		"queue_name":     "default-batch",
		"priority_value": 1000,
		"preemptible":    true,
		"required_cpu":   1.0,
	})

	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusCreated)
	job := responseRecordData(t, rec)

	if job["status"] != "submitted" || job["admission_preemption_status"] != "completed" || job["priority_value"] != float64(10000) {
		t.Fatalf("submitted job = %#v, want admitted requester with preemption metadata", job)
	}
	victim, _ := app.Store.Get(ctx, jobsResource, "victim")
	if victim.Data["status"] != "preempted" || victim.Data["preemption_record_id"] == "" {
		t.Fatalf("victim = %#v, want preempted before requester persisted", victim.Data)
	}
	pods, err := cl.Clientset().CoreV1().Pods("proj-p1").List(ctx, metav1.ListOptions{})
	if err != nil || len(pods.Items) != 0 {
		t.Fatalf("victim pods after preemption = %d err=%v, want deleted", len(pods.Items), err)
	}
	if got := len(app.Store.List(ctx, jobsResource)); got != 2 {
		t.Fatalf("jobs after auto preemption = %d, want victim plus requester", got)
	}
}

func TestSubmitJobAutoPreemptionFailureDoesNotPersistRequester(t *testing.T) {
	ctx := context.Background()
	app := newJobSubmitTestAppWithCluster(cluster.New(fake.NewSimpleClientset(), "proj"))
	seedJobAdmissionProject(t, app, map[string]any{})
	app.Store.Update(ctx, testSchedulerQueuesResource, "q1", map[string]any{
		"priority_value": 10000,
		"is_preemptible": false,
	})
	app.Store.Update(ctx, testSchedulerPlansResource, "plan-1", map[string]any{
		"cpu_limit_cores": 1.5,
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "active", "job_id": "active", "project_id": "P1", "user_id": "U2", "status": "running", "required_cpu": 1.0,
		"namespace": "proj-p1", "queue_name": "default-batch", "priority_value": 9000, "preemptible": false,
	})

	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusConflict)
	data := responseEnvelopeData(t, rec)

	if data["auto_preemption"] == nil || !strings.Contains(data["reason"].(string), "CPU quota exceeded") {
		t.Fatalf("denial = %#v, want quota denial with auto_preemption result", data)
	}
	if got := len(app.Store.List(ctx, jobsResource)); got != 1 {
		t.Fatalf("jobs after failed auto preemption = %d, want only original active job", got)
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

func TestSubmitJobIdempotencyKeyReplaysSameRequest(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})
	key := "workload-submit-idempotency-key"
	body := `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`

	first := serveSubmitJobWithHeaders(t, app, body, "U1", map[string]string{"Idempotency-Key": key}, http.StatusCreated)
	firstJob := responseRecordData(t, first)
	jobID := firstJob["job_id"]
	if jobID == "" || firstJob["status"] != "submitted" {
		t.Fatalf("first submit job = %#v, want generated submitted job", firstJob)
	}
	keyHash, fingerprintHash := storedSubmitIdempotencyHashes(t, app, fmt.Sprint(jobID))
	assertNoWorkloadSubmitIdempotencyMaterial(t, firstJob, key, keyHash, fingerprintHash)
	assertWorkloadSubmitSideEffects(t, app, 1, 1, 0, 1)

	replay := serveSubmitJobWithHeaders(t, app, body, "U1", map[string]string{"Idempotency-Key": key}, http.StatusCreated)
	replayJob := responseRecordData(t, replay)
	if replayJob["job_id"] != jobID || replayJob["id"] != firstJob["id"] || replayJob["status"] != "submitted" {
		t.Fatalf("replay job = %#v, want original job id/status %#v", replayJob, firstJob)
	}
	assertNoWorkloadSubmitIdempotencyMaterial(t, replayJob, key, keyHash, fingerprintHash)
	assertWorkloadSubmitSideEffects(t, app, 1, 1, 0, 1)

	code, data, _ := listJobs(app, workloadRequest(http.MethodGet, "/api/v1/jobs", ""), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	assertNoWorkloadSubmitIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	getReq := workloadRequest(http.MethodGet, "/api/v1/jobs/"+fmt.Sprint(jobID), "")
	getReq.SetPathValue("id", fmt.Sprint(jobID))
	code, data, _ = getJob(app, getReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	assertNoWorkloadSubmitIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	events := workloadEventsByName(app, "JobSubmitted")
	if events[0].IdempotencyKey != key {
		t.Fatalf("JobSubmitted IdempotencyKey = %q, want synthetic test key", events[0].IdempotencyKey)
	}
	assertNoWorkloadSubmitIdempotencyMaterial(t, events[0].Data, key, keyHash, fingerprintHash)
}

func TestSubmitJobIdempotencyKeyRejectsDifferentPayload(t *testing.T) {
	app := newJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})
	key := "workload-submit-conflict-key"

	first := serveSubmitJobWithHeaders(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", map[string]string{"Idempotency-Key": key}, http.StatusCreated)
	firstJob := responseRecordData(t, first)
	keyHash, fingerprintHash := storedSubmitIdempotencyHashes(t, app, fmt.Sprint(firstJob["job_id"]))
	assertWorkloadSubmitSideEffects(t, app, 1, 1, 0, 1)

	conflict := serveSubmitJobWithHeaders(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":2,"required_memory":1024}`, "U1", map[string]string{"Idempotency-Key": key}, http.StatusConflict)
	conflictData := responseEnvelopeData(t, conflict)
	assertNoWorkloadSubmitIdempotencyMaterial(t, conflictData, key, keyHash, fingerprintHash)
	assertWorkloadSubmitSideEffects(t, app, 1, 1, 0, 1)

	record, found := app.Store.Get(context.Background(), jobsResource, fmt.Sprint(firstJob["job_id"]))
	if !found {
		t.Fatalf("first submitted job record missing")
	}
	if record.Data["status"] != "submitted" {
		t.Fatalf("first submitted job status = %#v, want submitted", record.Data["status"])
	}
}

func TestSubmitJobUsesRemoteSchedulerAdmissionWhenIsolated(t *testing.T) {
	serviceKey := "service-secret"
	ownerApp := platform.NewApp(platform.Config{
		ServiceName:   "all",
		HTTPAddr:      ":0",
		ServiceAPIKey: serviceKey,
	})
	orgproject.Register(ownerApp)
	Register(ownerApp)
	seedJobAdmissionOwnerData(t, ownerApp, map[string]any{})
	ownerServer := httptest.NewServer(ownerApp)
	defer ownerServer.Close()

	schedulerApp := platform.NewApp(platform.Config{
		ServiceName:      schedulerServiceName,
		HTTPAddr:         ":0",
		RequireAuth:      true,
		APIKeys:          map[string]bool{serviceKey: true},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{serviceKey: {ID: serviceName, Role: "service"}},
		ServiceURLs: map[string]string{
			"org-project-service": ownerServer.URL,
			serviceName:           ownerServer.URL,
		},
		ServiceAPIKey: serviceKey,
	})
	registerSchedulerAdmissionRoute(schedulerApp)
	schedulerquota.Register(schedulerApp)
	seedSchedulerAdmissionPolicy(t, schedulerApp)
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

func TestSubmitJobRequiresProjectMembershipWhenAuthRequired(t *testing.T) {
	app := newAuthJobSubmitTestApp()
	seedJobAdmissionProject(t, app, map[string]any{})

	deniedReq := workloadAuthRequest(http.MethodPost, "/api/v1/jobs", `{"project_id":"P1","user_id":"U2","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U2", "user")
	code, data, _ := submitJob(app, deniedReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)
	if got := len(app.Store.List(context.Background(), jobsResource)); got != 0 {
		t.Fatalf("jobs after denied submit = %d, want 0", got)
	}
	if got := len(app.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != 0 {
		t.Fatalf("scheduler admissions after denied submit = %d, want 0", got)
	}

	allowedReq := workloadAuthRequest(http.MethodPost, "/api/v1/jobs", `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", "user")
	code, data, _ = submitJob(app, allowedReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)
	if got := len(app.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != 1 {
		t.Fatalf("scheduler admissions after allowed submit = %d, want 1", got)
	}
}

func TestJobAccessHandlersFilterAndGuardProjectRoutes(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProject(t, app, "P2")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "job-one", "project_id": "P1", "user_id": "U1", "status": "running"})
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j2", "job_id": "job-two", "project_id": "P2", "user_id": "U2", "status": "running"})
	createWorkloadRecord(t, app, jobLogsResource, map[string]any{"id": "log1", "job_id": "job-one", "line": "hello"})

	code, data, _ := listJobs(app, workloadAuthRequest(http.MethodGet, "/api/v1/jobs", "", "U1", "user"), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if records := data.([]contracts.Record[map[string]any]); len(records) != 1 || records[0].ID != "j1" {
		t.Fatalf("member job list = %#v, want j1", data)
	}

	getDenied := workloadAuthRequest(http.MethodGet, "/api/v1/jobs/j2", "", "U1", "user")
	getDenied.SetPathValue("id", "j2")
	code, data, _ = getJob(app, getDenied, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	cancelReq := workloadAuthRequest(http.MethodPost, "/api/v1/jobs/job-one/cancel", `{}`, "U1", "user")
	cancelReq.SetPathValue("id", "job-one")
	code, data, _ = cancelJob(app, cancelReq, platform.RouteSpec{OperationID: "job_cancel"})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)
	if commands := app.Store.List(context.Background(), jobCommandsResource); len(commands) != 1 {
		t.Fatalf("job cancel commands = %#v, want one command", commands)
	}

	logsReq := workloadAuthRequest(http.MethodGet, "/api/v1/jobs/job-one/logs", "", "U1", "user")
	logsReq.SetPathValue("id", "job-one")
	code, data, _ = listJobLogs(app, logsReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if logs := data.([]contracts.Record[map[string]any]); len(logs) != 1 || logs[0].ID != "log1" {
		t.Fatalf("job logs = %#v, want log1", logs)
	}
}

func TestJobAccessHandlersReadVariantsAndFailures(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "job-one", "project_id": "P1", "user_id": "U1", "status": "running"})
	createWorkloadRecord(t, app, jobGPUUsageResource, map[string]any{"id": "gpu1", "job_record_id": "j1", "gpu_utilization": 42})

	getReq := workloadAuthRequest(http.MethodGet, "/api/v1/jobs/j1", "", "U1", "user")
	getReq.SetPathValue("id", "j1")
	code, data, _ := getJob(app, getReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)

	gpuReq := workloadAuthRequest(http.MethodGet, "/api/v1/jobs/j1/gpu-summary", "", "U1", "user")
	gpuReq.SetPathValue("id", "j1")
	code, data, _ = listJobGPUUsage(app, gpuReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if usage := data.([]contracts.Record[map[string]any]); len(usage) != 1 || usage[0].ID != "gpu1" {
		t.Fatalf("gpu usage = %#v, want gpu1", usage)
	}

	noUserReq := workloadRequest(http.MethodGet, "/api/v1/jobs", "")
	code, data, _ = listJobs(app, noUserReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	missingReq := workloadAuthRequest(http.MethodGet, "/api/v1/jobs/missing", "", "U1", "user")
	missingReq.SetPathValue("id", "missing")
	code, data, _ = getJob(app, missingReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusNotFound)

	cancelBadBody := workloadAuthRequest(http.MethodPost, "/api/v1/jobs/j1/cancel", `{`, "U1", "user")
	cancelBadBody.SetPathValue("id", "j1")
	code, data, _ = cancelJob(app, cancelBadBody, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusBadRequest)
	if commands := app.Store.List(context.Background(), jobCommandsResource); len(commands) != 0 {
		t.Fatalf("job cancel commands = %#v, want none after malformed body", commands)
	}
}

func newJobSubmitTestApp() *platform.App {
	return newJobSubmitTestAppWithCluster(nil)
}

func newJobSubmitTestAppWithCluster(cl *cluster.Client) *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key", StreamSidecarImage: "registry.example.com/nexuspaas/selkies-gl-desktop:24.04"}, platform.WithCluster(cl))
	registerWorkloadJobRoute(app)
	registerWorkloadPreemptionRoutes(app)
	registerSchedulerAdmissionRoute(app)
	registerSchedulerPreemptionRoute(app)
	schedulerquota.Register(app)
	Register(app)
	return app
}

func newAuthJobSubmitTestApp() *platform.App {
	serviceKey := "service-secret"
	app := platform.NewApp(platform.Config{
		ServiceName:   "all",
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceAPIKey: serviceKey,
		APIKeys:       map[string]bool{serviceKey: true},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			serviceKey: {ID: serviceName, Role: "service"},
		},
	})
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

func registerWorkloadPreemptionRoutes(app *platform.App) {
	app.RegisterService(platform.ServiceSpec{
		Name: serviceName,
		Routes: []platform.RouteSpec{
			{Method: http.MethodGet, Pattern: "/internal/workload/preemption-context", Resource: "preemption_context", Action: "internal_read", AuthRequired: false},
			{Method: http.MethodPost, Pattern: "/internal/workload/jobs/{id}/preempt", Resource: "jobs", Action: "preempt", AuthRequired: false},
		},
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

func registerSchedulerPreemptionRoute(app *platform.App) {
	app.RegisterService(platform.ServiceSpec{
		Name: schedulerServiceName,
		Routes: []platform.RouteSpec{{
			Method:       http.MethodPost,
			Pattern:      schedulerPreemptionsPath,
			Resource:     "preemptions",
			Action:       "command",
			AuthRequired: true,
		}},
	})
}

func seedJobAdmissionProject(t *testing.T, app *platform.App, projectOverrides map[string]any) {
	t.Helper()
	seedSchedulerAdmissionPolicy(t, app)
	seedJobAdmissionOwnerData(t, app, projectOverrides)
}

func seedSchedulerAdmissionPolicy(t *testing.T, app *platform.App) {
	t.Helper()
	createWorkloadRecord(t, app, testSchedulerQueuesResource, map[string]any{"id": "q1", "name": "default-batch", "priority_value": 1000, "is_preemptible": true, "max_runtime_seconds": 3600})
	createWorkloadRecord(t, app, testSchedulerPlansResource, map[string]any{
		"id":                 "plan-1",
		"name":               "default",
		"gpu_limit":          4.0,
		"cpu_limit_cores":    8.0,
		"memory_limit_gb":    16.0,
		"queue_ids":          []string{"q1"},
		"allowed_gpu_models": []string{"gpu.nvidia.com"},
	})
}

func workloadPreemptionPod(namespace, name, jobID string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: namespace,
		Name:      name,
		Labels: map[string]string{
			cluster.LabelJobID: jobID,
		},
	}}
}

func seedJobAdmissionOwnerData(t *testing.T, app *platform.App, projectOverrides map[string]any) {
	t.Helper()
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

func storedSubmitIdempotencyHashes(t *testing.T, app *platform.App, jobID string) (string, string) {
	t.Helper()
	record, found := app.Store.Get(context.Background(), jobsResource, jobID)
	if !found {
		t.Fatalf("submitted job %s not found", jobID)
	}
	keyHash, _ := record.Data[internalSubmitIdempotencyKeyHash].(string)
	fingerprintHash, _ := record.Data[internalSubmitIdempotencyFingerprintHash].(string)
	if keyHash == "" || fingerprintHash == "" {
		t.Fatalf("stored submit idempotency hashes missing from internal job record")
	}
	return keyHash, fingerprintHash
}

func workloadEventsByName(app *platform.App, name string) []contracts.Event {
	events := []contracts.Event{}
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			events = append(events, event)
		}
	}
	return events
}

func assertWorkloadSubmitSideEffects(t *testing.T, app *platform.App, jobs, admissions, preemptions, events int) {
	t.Helper()
	if got := len(app.Store.List(context.Background(), jobsResource)); got != jobs {
		t.Fatalf("workload jobs = %d, want %d", got, jobs)
	}
	if got := len(app.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != admissions {
		t.Fatalf("scheduler admissions = %d, want %d", got, admissions)
	}
	if got := len(app.Store.List(context.Background(), testSchedulerPreemptionsResource)); got != preemptions {
		t.Fatalf("scheduler preemptions = %d, want %d", got, preemptions)
	}
	if got := len(workloadEventsByName(app, "JobSubmitted")); got != events {
		t.Fatalf("JobSubmitted events = %d, want %d", got, events)
	}
}

func assertNoWorkloadSubmitIdempotencyMaterial(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value for leak check: %v", err)
	}
	text := string(raw)
	for _, token := range append(forbidden,
		internalSubmitIdempotencyKeyHash,
		internalSubmitIdempotencyFingerprintHash,
		"idempotency_key_hash",
		"fingerprint_hash",
	) {
		if token != "" && strings.Contains(text, token) {
			t.Fatalf("submit idempotency material leaked")
		}
	}
}
