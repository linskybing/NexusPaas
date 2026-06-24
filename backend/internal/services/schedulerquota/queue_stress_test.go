package schedulerquota

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestQueueStressCoversPendingAdmittedPreemptedRejectedWorkloads(t *testing.T) {
	app := newQueueStressTestApp(t)

	assertQueueStressPendingAndAdmitted(t, app)
	assertQueueStressPreempted(t, app)
	assertQueueStressRejected(t, app)
}

func newQueueStressTestApp(t *testing.T) *platform.App {
	t.Helper()

	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(
		preemptionPod("proj-p1", "stress-victim-pod", "stress-victim"),
	), "proj"))
	seedAdmissionProject(t, app, admissionFixture{})
	createSchedulerRecord(t, app, projectMembersResource, map[string]any{
		"id":         "P1/UReject",
		"project_id": "P1",
		"user_id":    "UReject",
		"role":       "user",
	})
	return app
}

func assertQueueStressPendingAndAdmitted(t *testing.T, app *platform.App) {
	t.Helper()

	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id":              "stress-pending",
		"job_id":          "stress-pending",
		"project_id":      "P1",
		"user_id":         "U1",
		"status":          "submitted",
		"namespace":       "proj-p1",
		"queue_name":      "default-batch",
		"priority_value":  1000,
		"preemptible":     true,
		"required_cpu":    1.0,
		"required_memory": 1024,
	})

	admittedCode, admittedData, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"job_id":          "stress-admitted",
		"project_id":      "P1",
		"user_id":         "U1",
		"queue_name":      "default-batch",
		"required_cpu":    1,
		"required_memory": 1024,
		"resources":       []any{podAdmissionResource(t, "stress-admitted", "0", "1", "1Gi")},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, admittedCode, admittedData, http.StatusOK)
	admitted := admittedData.(map[string]any)
	if admitted["allowed"] != true || admitted["queue_name"] != "default-batch" {
		t.Fatalf("admitted review = %#v, want allowed default-batch", admitted)
	}
	if admitted["priority_value"] != 1000 || admitted["preemptible"] != true || admitted["runtime_limit_seconds"] != 3600 {
		t.Fatalf("admitted review = %#v, want trusted queue metadata", admitted)
	}
	usage := admitted["usage"].(map[string]any)
	if usage["user_queued_jobs"] != 1 || usage["project_cpu"] != 1.0 || usage["user_cpu"] != 1.0 {
		t.Fatalf("admission usage = %#v, want pending submitted workload counted as queued and active", usage)
	}
	pending, found := app.Store.Get(context.Background(), workloadJobsResource, "stress-pending")
	if !found || pending.Data["status"] != "submitted" {
		t.Fatalf("pending workload = %#v found=%v, want submitted row", pending.Data, found)
	}
	if _, found := app.Store.Get(context.Background(), submitAdmissionsResource, "P1/U1/default-batch"); !found {
		t.Fatal("allowed admission review was not persisted")
	}
	event := requireSchedulerEvent(t, app, "SubmitAdmissionReviewed", "allowed")
	if event.Data["project_id"] != "P1" || event.Data["queue_name"] != "default-batch" {
		t.Fatalf("SubmitAdmissionReviewed event = %#v, want local admission evidence", event.Data)
	}
}

func assertQueueStressPreempted(t *testing.T, app *platform.App) {
	t.Helper()

	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id":             "stress-requester",
		"job_id":         "stress-requester",
		"project_id":     "P1",
		"user_id":        "U1",
		"status":         "submitted",
		"namespace":      "proj-p1",
		"queue_name":     "default-batch",
		"priority_value": 10000,
		"preemptible":    false,
		"required_cpu":   2.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id":             "stress-victim",
		"job_id":         "stress-victim",
		"project_id":     "P1",
		"user_id":        "U2",
		"status":         "running",
		"namespace":      "proj-p1",
		"queue_name":     "batch-low",
		"priority_value": 1000,
		"preemptible":    true,
		"required_cpu":   2.0,
		"created_at":     "2026-06-23T00:00:00Z",
	})

	preemptRec := postPreemption(t, app, "queue-stress-cpu-only", `{"requester_job_id":"stress-requester"}`, http.StatusOK)
	preempted := preemptionResponseData(t, preemptRec)
	if preempted["status"] != "completed" || preempted["accepted"] != true {
		t.Fatalf("preemption response = %#v, want completed accepted", preempted)
	}
	victim, found := app.Store.Get(context.Background(), workloadJobsResource, "stress-victim")
	if !found || victim.Data["status"] != "preempted" || victim.Data["preemption_record_id"] == "" {
		t.Fatalf("victim after preemption = %#v found=%v, want preempted with record id", victim.Data, found)
	}
	pods, err := app.Cluster.Clientset().CoreV1().Pods("proj-p1").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(pods.Items) != 0 {
		t.Fatalf("victim pods after preemption = %d err=%v, want deleted", len(pods.Items), err)
	}
	if countEvents(app, "JobPreempted") != 1 {
		t.Fatalf("JobPreempted events = %d, want 1", countEvents(app, "JobPreempted"))
	}
	preemptEvent := requireSchedulerEvent(t, app, "JobPreempted", "preempted")
	if preemptEvent.Data["victim_job_id"] != "stress-victim" {
		t.Fatalf("JobPreempted event = %#v, want stress-victim evidence", preemptEvent.Data)
	}
}

func assertQueueStressRejected(t *testing.T, app *platform.App) {
	t.Helper()

	rejectedCode, rejectedData, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"job_id":          "stress-rejected",
		"project_id":      "P1",
		"user_id":         "UReject",
		"queue_name":      "default-batch",
		"required_cpu":    99,
		"required_memory": 1024,
		"resources":       []any{podAdmissionResource(t, "stress-rejected", "0", "99", "1Gi")},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, rejectedCode, rejectedData, http.StatusConflict)
	rejected := rejectedData.(map[string]any)
	if rejected["allowed"] != false || !strings.Contains(rejected["reason"].(string), "CPU quota exceeded") {
		t.Fatalf("rejected review = %#v, want deterministic CPU quota rejection", rejected)
	}
	if _, found := app.Store.Get(context.Background(), submitAdmissionsResource, "P1/UReject/default-batch"); found {
		t.Fatal("rejected admission unexpectedly created a successful admission record")
	}
}
