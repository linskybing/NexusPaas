package schedulerquota

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
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/workload"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPreemptionRouteDoesNotCreateGenericCommandRecord(t *testing.T) {
	ctx := context.Background()
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "preemptible": false,
		"required_gpu": 1.0, "required_cpu": 1.0, "device_class_name": "gpu.nvidia.com",
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true,
		"required_gpu": 1.0, "required_cpu": 2.0, "device_class_name": "gpu.nvidia.com",
		"created_at": time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})

	rec := postPreemption(t, app, "preempt-happy", `{"requester_job_id":"requester"}`, http.StatusOK)
	data := preemptionResponseData(t, rec)
	if data["status"] != "completed" || data["accepted"] != true {
		t.Fatalf("preemption response = %#v, want completed accepted", data)
	}
	if got := len(app.Store.List(ctx, serviceName+":preemptions:commands")); got != 0 {
		t.Fatalf("generic preemption command records = %d, want 0", got)
	}
	victim, _ := app.Store.Get(ctx, workloadJobsResource, "victim")
	if victim.Data["status"] != "preempted" || victim.Data["preemption_record_id"] == "" {
		t.Fatalf("victim after preemption = %#v, want terminal preempted with record id", victim.Data)
	}
	if pods, err := app.Cluster.Clientset().CoreV1().Pods("proj-p1").List(ctx, metav1.ListOptions{}); err != nil || len(pods.Items) != 0 {
		t.Fatalf("pods after preemption = %d err=%v, want deleted", len(pods.Items), err)
	}
	if countEvents(app, "JobPreempted") != 1 {
		t.Fatalf("JobPreempted events = %d, want 1", countEvents(app, "JobPreempted"))
	}
}

func TestPreemptionSuccessUsesTransactionalEvent(t *testing.T) {
	ctx := context.Background()
	store := &schedulerTxStore{Store: platform.NewStore()}
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"}, platform.WithStore(store), platform.WithCluster(cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj")))
	registerPreemptionSchedulerRoute(app)
	app.RegisterService(platform.ServiceSpec{Name: workloadServiceName, Routes: []platform.RouteSpec{
		{Method: http.MethodGet, Pattern: workloadPreemptionContextPath, Resource: "preemption_context", Action: "internal_read", AuthRequired: false},
		{Method: http.MethodPost, Pattern: workloadPreemptJobPathTemplate, Resource: "jobs", Action: "preempt", AuthRequired: false},
	}})
	Register(app)
	workload.Register(app)
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})
	store.resetTx()

	postPreemption(t, app, "tx-preempt", `{"requester_job_id":"requester"}`, http.StatusOK)

	for _, event := range app.Events.Outbox() {
		if event.Name == "JobPreempted" {
			t.Fatalf("JobPreempted was directly published to app.Events; outbox=%#v", app.Events.Outbox())
		}
	}
	if len(store.txEvents) != 1 || store.txEvents[0].Name != "JobPreempted" {
		t.Fatalf("tx events = %#v, want one JobPreempted", store.txEvents)
	}
	victim, _ := app.Store.Get(ctx, workloadJobsResource, "victim")
	if victim.Data["status"] != "preempted" {
		t.Fatalf("victim after preemption = %#v, want preempted", victim.Data)
	}
}

func TestPreemptionIdempotencyReplaysExistingDecision(t *testing.T) {
	ctx := context.Background()
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(
		preemptionPod("proj-p1", "victim-pod", "victim"),
		preemptionPod("proj-p1", "victim2-pod", "victim2"),
	), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})

	first := postPreemption(t, app, "same-key", `{"requester_job_id":"requester"}`, http.StatusOK)
	firstData := preemptionResponseData(t, first)
	privateRecordID := preemptionRecordID("same-key")
	stored, keyHash, fingerprintHash := storedPreemptionRecordHashes(t, app, privateRecordID)
	publicID := assertPublicPreemptionID(t, firstData, privateRecordID)
	assertStoredPreemptionRecordPrivateMaterial(t, stored)
	assertNoPreemptionIdempotencyMaterial(t, firstData, "same-key", privateRecordID, keyHash, fingerprintHash)
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim2", "job_id": "victim2", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 500, "preemptible": true, "required_gpu": 1.0,
	})
	beforeEvents := countEvents(app, "JobPreempted")
	beforePods, _ := app.Cluster.Clientset().CoreV1().Pods("proj-p1").List(ctx, metav1.ListOptions{})

	rec := postPreemption(t, app, "same-key", `{"requester_job_id":"requester"}`, http.StatusOK)
	data := preemptionResponseData(t, rec)

	if data["status"] != "completed" {
		t.Fatalf("replayed data = %#v, want completed record", data)
	}
	if data["preemption_id"] != publicID || data["id"] != publicID {
		t.Fatalf("replayed public preemption id changed")
	}
	assertNoPreemptionIdempotencyMaterial(t, data, "same-key", privateRecordID, keyHash, fingerprintHash)
	victim2, _ := app.Store.Get(ctx, workloadJobsResource, "victim2")
	if victim2.Data["status"] != "running" {
		t.Fatalf("victim2 status = %#v, want unchanged running on replay", victim2.Data)
	}
	afterPods, _ := app.Cluster.Clientset().CoreV1().Pods("proj-p1").List(ctx, metav1.ListOptions{})
	if len(afterPods.Items) != len(beforePods.Items) {
		t.Fatalf("pods after replay = %d, want unchanged %d", len(afterPods.Items), len(beforePods.Items))
	}
	if after := countEvents(app, "JobPreempted"); after != beforeEvents {
		t.Fatalf("JobPreempted events after replay = %d, want %d", after, beforeEvents)
	}
	if records := app.Store.List(ctx, preemptionRecordsResource); len(records) != 1 {
		t.Fatalf("preemption records after replay = %d, want 1", len(records))
	}
}

func TestPreemptionMissingWorkloadContractFailsClosed(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:    serviceName,
		HTTPAddr:       ":0",
		ServiceAPIKey:  "svc-key",
		AdapterTimeout: time.Second,
	})
	registerPreemptionSchedulerRoute(app)
	Register(app)

	rec := postPreemption(t, app, "missing-workload", `{"requester_job_id":"requester"}`, http.StatusServiceUnavailable)
	data := preemptionResponseData(t, rec)
	if data["status"] != "preflight_failed" {
		t.Fatalf("preemption data = %#v, want preflight_failed", data)
	}
	if got := len(app.Store.List(context.Background(), serviceName+":preemptions:commands")); got != 0 {
		t.Fatalf("generic command records = %d, want 0", got)
	}
}

func TestPreemptionUsesWorkloadContractInIsolatedRuntime(t *testing.T) {
	ctx := context.Background()
	workloadApp := newWorkloadContractOwnerTestApp(t)
	seedPreemptionJob(t, workloadApp, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, workloadApp, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})
	server := httptest.NewServer(workloadApp)
	defer server.Close()

	schedulerApp := platform.NewApp(platform.Config{
		ServiceName:             serviceName,
		HTTPAddr:                ":0",
		ServiceAPIKey:           "svc-key",
		ServiceURLs:             map[string]string{workloadServiceName: server.URL},
		ServiceFallbackDisabled: true,
		AdapterTimeout:          time.Second,
	}, platform.WithCluster(cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj")))
	registerPreemptionSchedulerRoute(schedulerApp)
	Register(schedulerApp)
	seedPreemptionQueue(t, schedulerApp, "q-low", "batch-low", true, 1000)

	postPreemption(t, schedulerApp, "isolated", `{"requester_job_id":"requester"}`, http.StatusOK)

	victim, _ := workloadApp.Store.Get(ctx, workloadJobsResource, "victim")
	if victim.Data["status"] != "preempted" {
		t.Fatalf("remote workload victim = %#v, want preempted through workload contract", victim.Data)
	}
	if got := len(schedulerApp.Store.List(ctx, workloadJobsResource)); got != 0 {
		t.Fatalf("isolated scheduler local workload jobs = %d, want 0", got)
	}
}

func TestPreemptionPartialCleanupFailureRecordsPartialFailure(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		preemptionPod("proj-p1", "victim-a", "victim"),
		preemptionPod("proj-p1", "victim-b", "victim"),
	)
	deletes := 0
	clientset.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deletes++
		if deletes == 2 {
			return true, nil, errors.New("delete pod failed after partial cleanup")
		}
		return false, nil, nil
	})
	app := newPreemptionTestApp(t, cluster.New(clientset, "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})

	rec := postPreemption(t, app, "partial", `{"requester_job_id":"requester"}`, http.StatusServiceUnavailable)
	data := preemptionResponseData(t, rec)
	if data["status"] != "partial_failure" {
		t.Fatalf("partial cleanup response = %#v, want partial_failure", data)
	}
	victim, _ := app.Store.Get(context.Background(), workloadJobsResource, "victim")
	if victim.Data["status"] != "running" {
		t.Fatalf("victim after partial failure = %#v, want unchanged running", victim.Data)
	}
	if countEvents(app, "JobPreempted") != 0 {
		t.Fatalf("JobPreempted events = %d, want none on partial cleanup failure", countEvents(app, "JobPreempted"))
	}
	postPreemption(t, app, "partial", `{"requester_job_id":"requester"}`, http.StatusOK)
	victim, _ = app.Store.Get(context.Background(), workloadJobsResource, "victim")
	if victim.Data["status"] != "running" {
		t.Fatalf("victim after partial replay = %#v, want still running", victim.Data)
	}
}

func TestPreemptionPreflightFailureDoesNotCleanup(t *testing.T) {
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})

	rec := postPreemption(t, app, "preflight", `{"requester_job_id":"requester"}`, http.StatusConflict)
	data := preemptionResponseData(t, rec)
	if data["status"] != "preflight_failed" {
		t.Fatalf("preflight response = %#v, want preflight_failed", data)
	}
	pods, _ := app.Cluster.Clientset().CoreV1().Pods("proj-p1").List(context.Background(), metav1.ListOptions{})
	if len(pods.Items) != 1 {
		t.Fatalf("pods after preflight failure = %d, want untouched", len(pods.Items))
	}
}

func TestPreemptionZeroCleanupResourcesDoesNotMarkPreempted(t *testing.T) {
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})

	rec := postPreemption(t, app, "zero-cleanup", `{"requester_job_id":"requester"}`, http.StatusConflict)
	data := preemptionResponseData(t, rec)
	if data["status"] != "preflight_failed" {
		t.Fatalf("zero cleanup response = %#v, want preflight_failed", data)
	}
	victim, _ := app.Store.Get(context.Background(), workloadJobsResource, "victim")
	if victim.Data["status"] != "running" {
		t.Fatalf("victim after zero cleanup = %#v, want unchanged running", victim.Data)
	}
	if countEvents(app, "JobPreempted") != 0 {
		t.Fatalf("JobPreempted events = %d, want none", countEvents(app, "JobPreempted"))
	}
}

func TestPreemptionIdempotencyFingerprintMismatchDoesNotSelectVictims(t *testing.T) {
	ctx := context.Background()
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	for _, id := range []string{"requester-a", "requester-b"} {
		seedPreemptionJob(t, app, map[string]any{
			"id": id, "job_id": id, "status": "submitted", "namespace": "proj-p1",
			"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
		})
	}
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})
	postPreemption(t, app, "mismatch", `{"requester_job_id":"requester-a"}`, http.StatusOK)
	privateRecordID := preemptionRecordID("mismatch")
	_, keyHash, fingerprintHash := storedPreemptionRecordHashes(t, app, privateRecordID)
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim2", "job_id": "victim2", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 500, "preemptible": true, "required_gpu": 1.0,
	})
	if _, err := app.Cluster.Clientset().CoreV1().Pods("proj-p1").Create(ctx, preemptionPod("proj-p1", "victim2-pod", "victim2"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	beforeEvents := countEvents(app, "JobPreempted")
	beforeRecords := len(app.Store.List(ctx, preemptionRecordsResource))

	rec := postPreemption(t, app, "mismatch", `{"requester_job_id":"requester-b"}`, http.StatusConflict)
	data := preemptionResponseData(t, rec)
	if !strings.Contains(data["message"].(string), "different preemption request") {
		t.Fatalf("fingerprint mismatch = %#v, want conflict reason", data)
	}
	assertNoPreemptionIdempotencyMaterial(t, data, "mismatch", privateRecordID, keyHash, fingerprintHash)
	if events := countEvents(app, "JobPreempted"); events != beforeEvents {
		t.Fatalf("JobPreempted events after conflict = %d, want %d", events, beforeEvents)
	}
	if records := len(app.Store.List(ctx, preemptionRecordsResource)); records != beforeRecords {
		t.Fatalf("preemption records after conflict = %d, want %d", records, beforeRecords)
	}
	victim2, _ := app.Store.Get(ctx, workloadJobsResource, "victim2")
	if victim2.Data["status"] != "running" {
		t.Fatalf("victim2 status after conflict = %#v, want running", victim2.Data)
	}
	pods, _ := app.Cluster.Clientset().CoreV1().Pods("proj-p1").List(ctx, metav1.ListOptions{})
	if len(pods.Items) != 1 {
		t.Fatalf("pods after conflict = %d, want untouched victim2 pod", len(pods.Items))
	}
}

func TestPreemptionResponseAndEventsDoNotExposeIdempotencyMaterial(t *testing.T) {
	ctx := context.Background()
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "requester", "job_id": "requester", "status": "submitted", "namespace": "proj-p1",
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "status": "running", "namespace": "proj-p1",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})
	key := "material-check"

	rec := postPreemption(t, app, key, `{"requester_job_id":"requester"}`, http.StatusOK)
	data := preemptionResponseData(t, rec)
	privateRecordID := preemptionRecordID(key)
	stored, keyHash, fingerprintHash := storedPreemptionRecordHashes(t, app, privateRecordID)
	publicID := assertPublicPreemptionID(t, data, privateRecordID)
	assertStoredPreemptionRecordPrivateMaterial(t, stored)
	assertNoPreemptionIdempotencyMaterial(t, data, key, privateRecordID, keyHash, fingerprintHash)

	events := preemptionEventsByName(app, "JobPreempted")
	if len(events) != 1 {
		t.Fatalf("JobPreempted events = %d, want 1", len(events))
	}
	if events[0].Data["preemption_id"] != publicID {
		t.Fatalf("JobPreempted event public preemption id mismatch")
	}
	if events[0].IdempotencyKey != "" {
		t.Fatalf("JobPreempted event exposed preemption idempotency key")
	}
	assertNoPreemptionIdempotencyMaterial(t, events[0].Data, key, privateRecordID, keyHash, fingerprintHash)

	victim, _ := app.Store.Get(ctx, workloadJobsResource, "victim")
	if victim.Data["preemption_record_id"] != publicID {
		t.Fatalf("victim preemption_record_id did not use public preemption id")
	}
}

func TestPreemptionRejectsUnauthorizedPriorityOverride(t *testing.T) {
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(), "proj"))

	rec := postPreemption(t, app, "override", `{"priority_value":10000,"required_gpu":1}`, http.StatusForbidden)
	data := preemptionResponseData(t, rec)
	if !strings.Contains(data["message"].(string), "administrator") {
		t.Fatalf("override denial = %#v, want admin reason", data)
	}
}

func TestPreemptionAllowsServicePriorityOverride(t *testing.T) {
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(preemptionPod("proj-p1", "victim-pod", "victim")), "proj"))
	seedPreemptionQueue(t, app, "q-low", "batch-low", true, 1000)
	seedPreemptionJob(t, app, map[string]any{
		"id": "victim", "job_id": "victim", "project_id": "P1", "user_id": "U1", "namespace": "proj-p1", "status": "running",
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0,
	})

	rec := postPreemptionWithHeaders(t, app, "service-override", `{"requester_job_id":"requester-new","project_id":"P1","priority_value":10000,"required_gpu":1}`, map[string]string{
		"X-Service-Key": "svc-key",
	}, http.StatusOK)
	data := preemptionResponseData(t, rec)
	if data["status"] != "completed" || data["accepted"] != true {
		t.Fatalf("service override preemption = %#v, want completed accepted", data)
	}
	victim, _ := app.Store.Get(context.Background(), workloadJobsResource, "victim")
	if victim.Data["status"] != "preempted" {
		t.Fatalf("victim = %#v, want preempted", victim.Data)
	}
}

func TestPreemptionSelectionFiltersGPUModelAndMaxPreemptions(t *testing.T) {
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(), "proj"))
	req := preemptionRequest{
		RequesterJobID: "requester",
		ProjectID:      "P1",
		PriorityValue:  10000,
		RequiredGPU:    2,
		GPUModel:       "a100",
		MaxPreemptions: 1,
	}
	candidates := []workloadJobSnapshot{
		{ID: "v-b", JobID: "v-b", ProjectID: "P1", Status: "running", PriorityValue: 100, Preemptible: true, RequiredGPU: 1, GPUModel: "h100", CreatedAt: "2026-06-15T08:00:00Z"},
		{ID: "v-new", JobID: "v-new", ProjectID: "P1", Status: "running", PriorityValue: 100, Preemptible: true, RequiredGPU: 1, GPUModel: "a100", CreatedAt: "2026-06-15T09:00:00Z"},
		{ID: "v-old", JobID: "v-old", ProjectID: "P1", Status: "running", PriorityValue: 100, Preemptible: true, RequiredGPU: 1, GPUModel: "a100", CreatedAt: "2026-06-15T07:00:00Z"},
		{ID: "v-other-project", JobID: "v-other-project", ProjectID: "P2", Status: "running", PriorityValue: 1, Preemptible: true, RequiredGPU: 2, GPUModel: "a100", CreatedAt: "2026-06-15T10:00:00Z"},
	}
	victims, _, _ := selectPreemptionVictims(app, req, candidates)
	if len(victims) != 0 {
		t.Fatalf("victims with max=1 = %#v, want none because demand cannot be satisfied", victims)
	}
	req.MaxPreemptions = 2
	victims, freedGPU, _ := selectPreemptionVictims(app, req, candidates)
	if len(victims) != 2 || victims[0].JobID != "v-new" || victims[1].JobID != "v-old" || freedGPU != 2 {
		t.Fatalf("victims = %#v freedGPU=%v, want newest/oldest same-project a100 only", victims, freedGPU)
	}
}

func TestPreemptionSelectionMinimizesVictimCount(t *testing.T) {
	app := newPreemptionTestApp(t, cluster.New(fake.NewSimpleClientset(), "proj"))
	req := preemptionRequest{RequesterJobID: "requester", ProjectID: "P1", PriorityValue: 10000, RequiredGPU: 2, MaxPreemptions: 3}
	candidates := []workloadJobSnapshot{
		{ID: "small-a", JobID: "small-a", ProjectID: "P1", Status: "running", PriorityValue: 100, Preemptible: true, RequiredGPU: 1, CreatedAt: "2026-06-15T09:00:00Z"},
		{ID: "small-b", JobID: "small-b", ProjectID: "P1", Status: "running", PriorityValue: 100, Preemptible: true, RequiredGPU: 1, CreatedAt: "2026-06-15T08:00:00Z"},
		{ID: "single", JobID: "single", ProjectID: "P1", Status: "running", PriorityValue: 500, Preemptible: true, RequiredGPU: 2, CreatedAt: "2026-06-15T07:00:00Z"},
	}

	victims, freedGPU, _ := selectPreemptionVictims(app, req, candidates)

	if len(victims) != 1 || victims[0].JobID != "single" || freedGPU != 2 {
		t.Fatalf("victims = %#v freedGPU=%v, want one sufficient victim", victims, freedGPU)
	}
}

func newPreemptionTestApp(t *testing.T, cl *cluster.Client) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"}, platform.WithCluster(cl))
	registerPreemptionSchedulerRoute(app)
	app.RegisterService(platform.ServiceSpec{Name: workloadServiceName, Routes: []platform.RouteSpec{
		{Method: http.MethodGet, Pattern: workloadPreemptionContextPath, Resource: "preemption_context", Action: "internal_read", AuthRequired: false},
		{Method: http.MethodPost, Pattern: workloadPreemptJobPathTemplate, Resource: "jobs", Action: "preempt", AuthRequired: false},
	}})
	Register(app)
	workload.Register(app)
	return app
}

func newWorkloadContractOwnerTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: workloadServiceName, HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	app.RegisterService(platform.ServiceSpec{Name: workloadServiceName, Routes: []platform.RouteSpec{
		{Method: http.MethodGet, Pattern: workloadPreemptionContextPath, Resource: "preemption_context", Action: "internal_read", AuthRequired: false},
		{Method: http.MethodPost, Pattern: workloadPreemptJobPathTemplate, Resource: "jobs", Action: "preempt", AuthRequired: false},
	}})
	workload.Register(app)
	return app
}

func registerPreemptionSchedulerRoute(app *platform.App) {
	app.RegisterService(platform.ServiceSpec{Name: serviceName, Routes: []platform.RouteSpec{{
		Method:       http.MethodPost,
		Pattern:      "/api/v1/internal/scheduler/preemptions",
		Resource:     "preemptions",
		Action:       "command",
		AuthRequired: false,
	}}})
}

func seedPreemptionQueue(t *testing.T, app *platform.App, id, name string, preemptible bool, priority int) {
	t.Helper()
	createSchedulerRecord(t, app, queuesResource, map[string]any{
		"id": id, "name": name, "is_preemptible": preemptible, "priority_value": priority,
	})
}

func seedPreemptionJob(t *testing.T, app *platform.App, data map[string]any) {
	t.Helper()
	createSchedulerRecord(t, app, workloadJobsResource, data)
}

func postPreemption(t *testing.T, app http.Handler, key, body string, want int) *httptest.ResponseRecorder {
	return postPreemptionWithHeaders(t, app, key, body, nil, want)
}

func postPreemptionWithHeaders(t *testing.T, app http.Handler, key, body string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/scheduler/preemptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("POST preemptions returned %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	return rec
}

func preemptionResponseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func preemptionPod(namespace, name, jobID string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				cluster.LabelJobID:        jobID,
				"platform-go/preemptible": "true",
			},
		},
	}
}

func countEvents(app *platform.App, name string) int {
	count := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			count++
		}
	}
	return count
}

func preemptionEventsByName(app *platform.App, name string) []contracts.Event {
	events := []contracts.Event{}
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			events = append(events, event)
		}
	}
	return events
}

func storedPreemptionRecordHashes(t *testing.T, app *platform.App, privateRecordID string) (map[string]any, string, string) {
	t.Helper()
	record, found := app.Store.Get(context.Background(), preemptionRecordsResource, privateRecordID)
	if !found {
		t.Fatalf("stored preemption record missing")
	}
	keyHash, _ := record.Data[internalPreemptionIdempotencyKeyHash].(string)
	fingerprintHash, _ := record.Data[internalPreemptionFingerprintHash].(string)
	if keyHash == "" || fingerprintHash == "" {
		t.Fatalf("stored preemption private hashes missing")
	}
	return record.Data, keyHash, fingerprintHash
}

func assertStoredPreemptionRecordPrivateMaterial(t *testing.T, data map[string]any) {
	t.Helper()
	if _, ok := data["idempotency_key"]; ok {
		t.Fatalf("raw preemption idempotency key stored")
	}
	if _, ok := data["fingerprint"]; ok {
		t.Fatalf("raw preemption fingerprint stored")
	}
	if data[internalPreemptionIdempotencyKeyHash] == "" || data[internalPreemptionFingerprintHash] == "" {
		t.Fatalf("private preemption hashes missing")
	}
	if data["preemption_id"] == "" || data["preemption_id"] == data["id"] {
		t.Fatalf("public preemption id must be generated and distinct from private record id")
	}
}

func assertPublicPreemptionID(t *testing.T, data map[string]any, privateRecordID string) string {
	t.Helper()
	publicID, _ := data["preemption_id"].(string)
	if publicID == "" {
		t.Fatalf("public preemption_id missing")
	}
	if publicID == privateRecordID || data["id"] == privateRecordID {
		t.Fatalf("preemption response exposed private record id")
	}
	if data["id"] != publicID {
		t.Fatalf("preemption response id should match public preemption_id")
	}
	return publicID
}

func assertNoPreemptionIdempotencyMaterial(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal preemption value for leak check: %v", err)
	}
	text := string(raw)
	for _, token := range append(forbidden,
		"idempotency_key",
		"idempotencyKey",
		"fingerprint",
		internalPreemptionIdempotencyKeyHash,
		internalPreemptionFingerprintHash,
		internalPreemptionRecordID,
	) {
		if token != "" && strings.Contains(text, token) {
			t.Fatalf("preemption idempotency material leaked")
		}
	}
}
