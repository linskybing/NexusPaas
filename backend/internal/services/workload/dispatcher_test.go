package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type fakeStorageMountPlanClient struct {
	plan       storageMountPlan
	err        error
	gotProject string
	got        storageMountPlanRequest
}

func (c *fakeStorageMountPlanClient) Resolve(_ context.Context, projectID string, req storageMountPlanRequest) (storageMountPlan, error) {
	c.gotProject = projectID
	c.got = req
	return c.plan, c.err
}

func TestDispatchSubmittedWorkloadCreatesNativeJobAndMarksRunning(t *testing.T) {
	now := time.Date(2026, 6, 14, 16, 30, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":                    "J2600001",
		"job_id":                "J2600001",
		"project_id":            "P1",
		"user_id":               "U1",
		"status":                "submitted",
		"namespace":             "proj-p1",
		"queue_name":            "default-batch",
		"priority":              10000,
		"runtime_limit_seconds": 60,
		"created_at":            now.Add(-time.Minute).Format(time.RFC3339),
		"retry_count":           3,
		"next_retry_at":         now.Add(-time.Second).Format(time.RFC3339),
		"error_message":         "previous transient failure",
		"status_reason":         "previous transient failure",
		"resources":             []any{nativeJobResource("train")},
		"required_cpu":          1,
		"required_memory":       1024,
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	job, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("native job was not created: %v", err)
	}
	labels := job.Spec.Template.Labels
	if labels[cluster.LabelJobID] != "J2600001" || labels[cluster.LabelProjectID] != "P1" || labels[cluster.LabelUserID] != "U1" {
		t.Fatalf("job template labels = %#v, want platform job/project/user labels", labels)
	}
	if job.Labels[cluster.RuntimeLimitSecondsKey] != "60" || labels[cluster.RuntimeLimitSecondsKey] != "60" {
		t.Fatalf("job runtime labels = object:%#v template:%#v, want runtime limit label", job.Labels, labels)
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 60 {
		t.Fatalf("job activeDeadlineSeconds = %#v, want 60", job.Spec.ActiveDeadlineSeconds)
	}
	if job.Spec.Template.Spec.SchedulerName != defaultDispatcherSchedulerName {
		t.Fatalf("schedulerName = %q, want %q", job.Spec.Template.Spec.SchedulerName, defaultDispatcherSchedulerName)
	}
	assertCorePodSpecAutomountSATokenFalse(t, job.Spec.Template.Spec)
	if job.Spec.Template.Spec.PriorityClassName != "platform-batch-high" {
		t.Fatalf("priorityClassName = %q, want platform-batch-high", job.Spec.Template.Spec.PriorityClassName)
	}
	record, _ := app.Store.Get(ctx, jobsResource, "J2600001")
	if record.Data["status"] != "running" || record.Data["started_at"] == nil || record.Data["next_retry_at"] != nil {
		t.Fatalf("dispatched job record = %#v, want running with retry cleared", record.Data)
	}
	if resources, ok := record.Data["created_resources"].([]map[string]any); !ok || len(resources) != 1 || resources[0]["kind"] != "Job" {
		t.Fatalf("created resources = %#v, want one Job", record.Data["created_resources"])
	}
}

func TestDispatchWaitingInfraRetryTreatsAlreadyExistingJobAsRunning(t *testing.T) {
	now := time.Date(2026, 6, 27, 16, 20, 0, 0, time.UTC)
	ctx := context.Background()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "proj-p1"}},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "retry-train",
				Namespace: "proj-p1",
				Labels: map[string]string{
					cluster.LabelJobID:     "J2600098",
					cluster.LabelProjectID: "P1",
					cluster.LabelUserID:    "U1",
				},
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "main", Image: "busybox"}},
				},
			}},
		},
	)
	cl := cluster.New(clientset, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":              "J2600098",
		"job_id":          "J2600098",
		"project_id":      "P1",
		"user_id":         "U1",
		"status":          jobStatusWaitingInfra,
		"namespace":       "proj-p1",
		"queue_name":      "default-batch",
		"priority":        10000,
		"created_at":      now.Add(-time.Minute).Format(time.RFC3339),
		"retry_count":     1,
		"next_retry_at":   now.Add(-time.Second).Format(time.RFC3339),
		"error_message":   "previous api timeout",
		"status_reason":   "previous api timeout",
		"resources":       []any{nativeJobResource("retry-train")},
		"required_cpu":    1,
		"required_memory": 1024,
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "retry-train", metav1.GetOptions{}); err != nil {
		t.Fatalf("pre-existing retry job was removed or not found: %v", err)
	}
	record, _ := app.Store.Get(ctx, jobsResource, "J2600098")
	if record.Data["status"] != jobStatusRunning || record.Data["started_at"] == nil || record.Data["dispatched_at"] == nil {
		t.Fatalf("retry dispatch record = %#v, want running with dispatch timestamps", record.Data)
	}
	if record.Data["next_retry_at"] != nil || record.Data["error_message"] != "" || record.Data["status_reason"] != "" {
		t.Fatalf("retry dispatch record = %#v, want retry/error fields cleared", record.Data)
	}
	if record.Data["retry_count"] != 1 {
		t.Fatalf("retry_count = %#v, want existing retry count retained for audit", record.Data["retry_count"])
	}
	resources, ok := record.Data["created_resources"].([]map[string]any)
	if !ok || len(resources) != 1 || resources[0]["kind"] != "Job" || resources[0]["namespace"] != "proj-p1" || resources[0]["name"] != "retry-train" {
		t.Fatalf("created resources = %#v, want existing Job recorded once", record.Data["created_resources"])
	}
}

func TestDispatchSubmittedWorkloadsAppliesAtMostBatchLimitPerRun(t *testing.T) {
	now := time.Date(2026, 6, 14, 16, 32, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	total := dispatcherMaxJobsPerRun + 2
	for index := 0; index < total; index++ {
		id := fmt.Sprintf("J26002%02d", index)
		createWorkloadRecord(t, app, jobsResource, map[string]any{
			"id":         id,
			"job_id":     id,
			"project_id": "P1",
			"user_id":    "U1",
			"status":     "submitted",
			"namespace":  "proj-p1",
			"queue_name": "default-batch",
			"priority":   1000,
			"created_at": now.Add(-time.Minute).Format(time.RFC3339),
			"resources":  []any{nativeJobResource(fmt.Sprintf("batch-%02d", index))},
		})
	}

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	assertDispatchedBatchState(t, ctx, app, cl, dispatcherMaxJobsPerRun, total-dispatcherMaxJobsPerRun, dispatcherMaxJobsPerRun)
	for index := dispatcherMaxJobsPerRun; index < total; index++ {
		if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, fmt.Sprintf("batch-%02d", index), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("job batch-%02d lookup err = %v, want not created before second run", index, err)
		}
	}

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	assertDispatchedBatchState(t, ctx, app, cl, total, 0, total)
}

func assertDispatchedBatchState(t *testing.T, ctx context.Context, app *platform.App, cl *cluster.Client, wantRunning, wantSubmitted, wantCreated int) {
	t.Helper()
	running := 0
	submitted := 0
	for _, record := range app.Store.List(ctx, jobsResource) {
		switch record.Data["status"] {
		case jobStatusRunning:
			running++
		case jobStatusSubmitted:
			submitted++
			if record.Data["created_resources"] != nil {
				t.Fatalf("submitted job %s has created_resources = %#v", record.ID, record.Data["created_resources"])
			}
		}
	}
	if running != wantRunning || submitted != wantSubmitted {
		t.Fatalf("job statuses running=%d submitted=%d, want running=%d submitted=%d", running, submitted, wantRunning, wantSubmitted)
	}
	jobs, err := cl.Clientset().BatchV1().Jobs("proj-p1").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list batch jobs: %v", err)
	}
	if len(jobs.Items) != wantCreated {
		t.Fatalf("created batch jobs = %d, want %d", len(jobs.Items), wantCreated)
	}
}

func TestDispatchResourcesRejectsRawSecret(t *testing.T) {
	rawSecret := `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"db-creds"},"stringData":{"password":"super-secret"}}`

	resources, err := dispatchResources(map[string]any{
		"resources": []any{map[string]any{"name": "db-creds", "manifest": rawSecret}},
	})

	if err == nil || !strings.Contains(err.Error(), "raw Kubernetes Secret resources are rejected") {
		t.Fatalf("dispatchResources err = %v resources=%#v, want raw Secret rejection", err, resources)
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("raw Secret rejection leaked plaintext: %v", err)
	}
}

func TestDispatchManifestsDisableUserWorkloadServiceAccountToken(t *testing.T) {
	job := map[string]any{"id": "J-sec016", "job_id": "J-sec016", "project_id": "P1", "user_id": "U1"}
	cases := []struct {
		name      string
		resource  dispatchResource
		specPaths [][]string
	}{
		{
			name:      "pod",
			resource:  dispatchResource{Name: "pod-sa", Kind: "Pod", Raw: []byte(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod-sa"},"spec":{"automountServiceAccountToken":true,"containers":[{"name":"main","image":"busybox"}]}}`)},
			specPaths: [][]string{{"spec"}},
		},
		{
			name:      "job",
			resource:  dispatchResource{Name: "job-sa", Kind: "Job", Raw: []byte(`{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":"job-sa"},"spec":{"template":{"spec":{"automountServiceAccountToken":true,"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}]}}}}`)},
			specPaths: [][]string{{"spec", "template", "spec"}},
		},
		{
			name:      "vcjob",
			resource:  dispatchResource{Name: "vcjob-sa", Kind: "Job", Raw: []byte(`{"apiVersion":"batch.volcano.sh/v1alpha1","kind":"Job","metadata":{"name":"vcjob-sa"},"spec":{"tasks":[{"name":"main","template":{"spec":{"automountServiceAccountToken":true,"containers":[{"name":"main","image":"busybox"}]}}}]}}`)},
			specPaths: [][]string{{"spec", "tasks", "0", "template", "spec"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := prepareDispatchManifest(job, tc.resource, "proj-p1")
			if err != nil {
				t.Fatal(err)
			}
			var obj map[string]any
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatalf("unmarshal prepared manifest: %v", err)
			}
			for _, path := range tc.specPaths {
				assertMapPodSpecAutomountSATokenFalse(t, obj, path...)
			}
		})
	}
}

func TestDispatchManifestsRejectRuntimeSocketHostPath(t *testing.T) {
	job := map[string]any{"id": "J-sec017", "job_id": "J-sec017", "project_id": "P1", "user_id": "U1"}
	cases := []struct {
		name     string
		resource dispatchResource
	}{
		{
			name:     "pod",
			resource: dispatchResource{Name: "pod-socket", Kind: "Pod", Raw: []byte(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod-socket"},"spec":{"containers":[{"name":"main","image":"busybox"}],"volumes":[{"name":"runtime","hostPath":{"path":"/var/run/docker.sock"}}]}}`)},
		},
		{
			name:     "job",
			resource: dispatchResource{Name: "job-socket", Kind: "Job", Raw: []byte(`{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":"job-socket"},"spec":{"template":{"spec":{"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}],"volumes":[{"name":"runtime","hostPath":{"path":"/run/containerd/containerd.sock"}}]}}}}`)},
		},
		{
			name:     "deployment",
			resource: dispatchResource{Name: "deploy-socket", Kind: "Deployment", Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"deploy-socket"},"spec":{"template":{"spec":{"containers":[{"name":"main","image":"busybox"}],"volumes":[{"name":"runtime","hostPath":{"path":"/run/crio/crio.sock"}}]}}}}`)},
		},
		{
			name:     "vcjob",
			resource: dispatchResource{Name: "vcjob-socket", Kind: "Job", Raw: []byte(`{"apiVersion":"batch.volcano.sh/v1alpha1","kind":"Job","metadata":{"name":"vcjob-socket"},"spec":{"tasks":[{"name":"main","template":{"spec":{"containers":[{"name":"main","image":"busybox"}],"volumes":[{"name":"runtime","hostPath":{"path":"/var/run/crio/crio.sock"}}]}}}]}}`)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := prepareDispatchManifest(job, tc.resource, "proj-p1")
			if err == nil || !strings.Contains(err.Error(), "container runtime socket") {
				t.Fatalf("prepareDispatchManifest err = %v, want runtime socket rejection", err)
			}
		})
	}
}

func TestDispatchVolcanoSynthesisRejectsRuntimeSocketHostPath(t *testing.T) {
	job := map[string]any{"id": "J-sec017-vc", "job_id": "J-sec017-vc", "project_id": "P1", "user_id": "U1", "scheduler_name": "volcano"}
	resource := dispatchResource{Name: "train", Kind: "Job", Raw: []byte(`{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":"train"},"spec":{"template":{"spec":{"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}],"volumes":[{"name":"runtime","hostPath":{"path":"/var/run/containerd/containerd.sock"}}]}}}}`)}

	_, err := prepareVolcanoDispatchManifests(job, []dispatchResource{resource}, "proj-p1")

	if err == nil || !strings.Contains(err.Error(), "container runtime socket") {
		t.Fatalf("prepareVolcanoDispatchManifests err = %v, want runtime socket rejection", err)
	}
}

func TestDispatchFailureReleasesReservation(t *testing.T) {
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":             "J-secret",
		"job_id":         "J-secret",
		"project_id":     "P1",
		"user_id":        "U1",
		"status":         jobStatusSubmitted,
		"namespace":      "proj-p1",
		"reservation_id": "res-dispatch",
		"resources": []any{map[string]any{
			"name":     "db-creds",
			"manifest": `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"db-creds"},"stringData":{"password":"super-secret"}}`,
		}},
	})
	released := []string{}
	release := func(_ context.Context, _ http.Header, id string) (schedulerReservationResult, error) {
		released = append(released, id)
		return schedulerReservationResult{}, nil
	}

	if err := dispatchSubmittedWorkloadsWithReservationRelease(ctx, app.Cluster, app.Store, nil, nil, release, now); err != nil {
		t.Fatal(err)
	}

	record, _ := app.Store.Get(ctx, jobsResource, "J-secret")
	if record.Data["status"] != jobStatusFailed {
		t.Fatalf("dispatch failed record = %#v, want failed", record.Data)
	}
	if len(released) != 1 || released[0] != "res-dispatch" {
		t.Fatalf("released reservations = %#v, want res-dispatch", released)
	}
}

func TestDispatchSubmittedWorkloadLabelsDeploymentRuntimeLimit(t *testing.T) {
	now := time.Date(2026, 6, 14, 16, 31, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":                    "J2600005",
		"job_id":                "J2600005",
		"project_id":            "P1",
		"user_id":               "U1",
		"status":                "submitted",
		"namespace":             "proj-p1",
		"queue_name":            "default-batch",
		"priority_value":        1000,
		"runtime_limit_seconds": 120,
		"created_at":            now.Add(-time.Minute).Format(time.RFC3339),
		"resources":             []any{nativeDeploymentResource("trainer")},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	deployment, err := cl.Clientset().AppsV1().Deployments("proj-p1").Get(ctx, "trainer", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment was not created: %v", err)
	}
	if deployment.Labels[cluster.RuntimeLimitSecondsKey] != "120" ||
		deployment.Spec.Template.Labels[cluster.RuntimeLimitSecondsKey] != "120" ||
		deployment.Spec.Template.Labels[cluster.LabelJobID] != "J2600005" {
		t.Fatalf("deployment runtime labels = object:%#v template:%#v, want runtime and platform labels", deployment.Labels, deployment.Spec.Template.Labels)
	}
	if deployment.Spec.Template.Spec.ActiveDeadlineSeconds != nil {
		t.Fatalf("deployment pod template activeDeadlineSeconds = %#v, want nil; controller deletion owns timeout", deployment.Spec.Template.Spec.ActiveDeadlineSeconds)
	}
	record, _ := app.Store.Get(ctx, jobsResource, "J2600005")
	if resources, ok := record.Data["created_resources"].([]map[string]any); !ok || len(resources) != 1 || resources[0]["kind"] != "Deployment" {
		t.Fatalf("created resources = %#v, want one Deployment", record.Data["created_resources"])
	}
}

func TestPrepareDispatchRuntimeLimitKeepsShorterNativeDeadline(t *testing.T) {
	raw, err := prepareDispatchManifest(map[string]any{
		"job_id":                "J-short",
		"project_id":            "P1",
		"runtime_limit_seconds": 60,
	}, nativeJobWithDeadlineResource("short", 30), "proj-p1")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if got := testNestedInt64(obj, "spec", "activeDeadlineSeconds"); got != 30 {
		t.Fatalf("activeDeadlineSeconds = %d, want shorter user deadline 30", got)
	}
}

func TestDispatchSubmittedWorkloadCreatesVolcanoVCJobWithDynamicClient(t *testing.T) {
	now := time.Date(2026, 6, 14, 16, 35, 0, 0, time.UTC)
	vcJobGVR := schema.GroupVersionResource{Group: "batch.volcano.sh", Version: "v1alpha1", Resource: "jobs"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{vcJobGVR: "JobList"},
	)
	cl := cluster.NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":         "J2600099",
		"job_id":     "J2600099",
		"project_id": "P1",
		"user_id":    "U1",
		"status":     "submitted",
		"namespace":  "proj-p1",
		"queue_name": "default-batch",
		"priority":   10000,
		"created_at": now.Add(-time.Minute).Format(time.RFC3339),
		"resources":  []any{volcanoVCJobResource("vc-train")},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	got, err := dynamicClient.Resource(vcJobGVR).Namespace("proj-p1").Get(ctx, "vc-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("VCJob was not created dynamically: %v", err)
	}
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "vc-train", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("native batch job lookup err = %v, want not found", err)
	}
	if scheduler, _, _ := unstructured.NestedString(got.Object, "spec", "schedulerName"); scheduler != "volcano" {
		t.Fatalf("VCJob schedulerName = %q, want volcano", scheduler)
	}
	if queue, _, _ := unstructured.NestedString(got.Object, "spec", "queue"); queue != "default-batch" {
		t.Fatalf("VCJob queue = %q, want default-batch", queue)
	}
	if priority, _, _ := unstructured.NestedString(got.Object, "spec", "priorityClassName"); priority != "platform-batch-high" {
		t.Fatalf("VCJob priorityClassName = %q, want platform-batch-high", priority)
	}
	if _, found, _ := unstructured.NestedMap(got.Object, "spec", "template"); found {
		t.Fatalf("VCJob has native spec.template subtree: %#v", got.Object["spec"])
	}
	taskLabels := firstVCJobTaskLabels(t, got)
	if taskLabels[cluster.LabelJobID] != "J2600099" || taskLabels[cluster.LabelProjectID] != "P1" || taskLabels[cluster.LabelUserID] != "U1" {
		t.Fatalf("VCJob task labels = %#v, want platform labels", taskLabels)
	}
	record, _ := app.Store.Get(ctx, jobsResource, "J2600099")
	if resources, ok := record.Data["created_resources"].([]map[string]any); !ok || len(resources) != 1 || resources[0]["kind"] != "VCJob" {
		t.Fatalf("created resources = %#v, want one VCJob", record.Data["created_resources"])
	}
}

func TestDispatchSubmittedWorkloadSynthesizesVolcanoVCJobFromBatchJob(t *testing.T) {
	now := time.Date(2026, 6, 14, 17, 40, 0, 0, time.UTC)
	vcJobGVR := schema.GroupVersionResource{Group: "batch.volcano.sh", Version: "v1alpha1", Resource: "jobs"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{vcJobGVR: "JobList"},
	)
	cl := cluster.NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":             "J2600100",
		"job_id":         "J2600100",
		"project_id":     "P1",
		"user_id":        "U1",
		"status":         "submitted",
		"namespace":      "proj-p1",
		"queue_name":     "default-batch",
		"scheduler_name": "volcano",
		"priority":       10000,
		"created_at":     now.Add(-time.Minute).Format(time.RFC3339),
		"resources":      []any{nativeParallelJobResource("train", 2)},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	got, err := dynamicClient.Resource(vcJobGVR).Namespace("proj-p1").Get(ctx, "j2600100", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("synthesized VCJob was not created dynamically: %v", err)
	}
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "train", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("native batch job lookup err = %v, want not found", err)
	}
	if minAvailable := testNestedInt64(got.Object, "spec", "minAvailable"); minAvailable != 2 {
		t.Fatalf("VCJob minAvailable = %d, want 2", minAvailable)
	}
	if queue, _, _ := unstructured.NestedString(got.Object, "spec", "queue"); queue != "default-batch" {
		t.Fatalf("VCJob queue = %q, want default-batch", queue)
	}
	taskLabels := firstVCJobTaskLabels(t, got)
	if taskLabels[cluster.LabelJobID] != "J2600100" || taskLabels["app"] != "trainer" {
		t.Fatalf("synthesized task labels = %#v, want platform and original labels", taskLabels)
	}
	taskSpec := firstVCJobTaskSpec(t, got)
	if taskSpec["schedulerName"] != "volcano" || taskSpec["priorityClassName"] != "platform-batch-high" {
		t.Fatalf("synthesized task spec = %#v, want volcano scheduler and priority", taskSpec)
	}
	assertMapPodSpecAutomountSATokenFalse(t, taskSpec)
	record, _ := app.Store.Get(ctx, jobsResource, "J2600100")
	if resources, ok := record.Data["created_resources"].([]map[string]any); !ok || len(resources) != 1 || resources[0]["kind"] != "VCJob" {
		t.Fatalf("created resources = %#v, want synthesized VCJob", record.Data["created_resources"])
	}
}

func TestDispatchSubmittedWorkloadFallsBackToPodsWhenSynthesizedVCJobCreateFails(t *testing.T) {
	now := time.Date(2026, 6, 14, 17, 55, 0, 0, time.UTC)
	dynamicClient, vcJobGVR, podGroupGVR := failingVolcanoDynamicClient()
	cl := cluster.NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createSubmittedVolcanoBatchJob(t, app, "J2600103", now)

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	assertVCJobNotCreated(t, ctx, dynamicClient, vcJobGVR, "j2600103")
	assertFallbackPodGroup(t, ctx, dynamicClient, podGroupGVR, "j2600103", 2)
	for _, name := range []string{"j2600103-0-0", "j2600103-0-1"} {
		assertFallbackPodCreated(t, ctx, cl, name, "J2600103", "j2600103")
	}
	assertCreatedResourceKinds(t, ctx, app, "J2600103", []string{"PodGroup", "Pod", "Pod"})
}

func TestDispatchSubmittedWorkloadRollsBackVolcanoFallbackOnPodCreateFailure(t *testing.T) {
	now := time.Date(2026, 6, 14, 18, 0, 0, 0, time.UTC)
	dynamicClient, _, podGroupGVR := failingVolcanoDynamicClient()
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("pod api unavailable"))
	})
	cl := cluster.NewWithDynamic(clientset, dynamicClient, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createSubmittedVolcanoBatchJob(t, app, "J2600104", now)

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	if _, err := dynamicClient.Resource(podGroupGVR).Namespace("proj-p1").Get(ctx, "j2600104", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("fallback PodGroup lookup err = %v, want cleaned up after pod failure", err)
	}
	pods, err := cl.Clientset().CoreV1().Pods("proj-p1").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("pods after rollback = %d, want none", len(pods.Items))
	}
	record, _ := app.Store.Get(ctx, jobsResource, "J2600104")
	if record.Data["status"] != jobStatusWaitingInfra || record.Data["retry_count"] != 1 {
		t.Fatalf("record after fallback failure = %#v, want waiting_infra retry 1", record.Data)
	}
	if !strings.Contains(record.Data["error_message"].(string), "pod api unavailable") {
		t.Fatalf("record error = %v, want pod failure", record.Data["error_message"])
	}
}

func failingVolcanoDynamicClient() (
	*dynamicfake.FakeDynamicClient,
	schema.GroupVersionResource,
	schema.GroupVersionResource,
) {
	vcJobGVR := schema.GroupVersionResource{Group: "batch.volcano.sh", Version: "v1alpha1", Resource: "jobs"}
	podGroupGVR := schema.GroupVersionResource{Group: "scheduling.volcano.sh", Version: "v1beta1", Resource: "podgroups"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			vcJobGVR:    "JobList",
			podGroupGVR: "PodGroupList",
		},
	)
	dynamicClient.PrependReactor("create", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("vcjob api unavailable"))
	})
	return dynamicClient, vcJobGVR, podGroupGVR
}

func createSubmittedVolcanoBatchJob(t *testing.T, app *platform.App, jobID string, now time.Time) {
	t.Helper()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":             jobID,
		"job_id":         jobID,
		"project_id":     "P1",
		"user_id":        "U1",
		"status":         "submitted",
		"namespace":      "proj-p1",
		"queue_name":     "default-batch",
		"scheduler_name": "volcano",
		"priority":       10000,
		"created_at":     now.Add(-time.Minute).Format(time.RFC3339),
		"resources":      []any{nativeParallelJobResource("train", 2)},
	})
}

func assertVCJobNotCreated(
	t *testing.T,
	ctx context.Context,
	dynamicClient *dynamicfake.FakeDynamicClient,
	gvr schema.GroupVersionResource,
	name string,
) {
	t.Helper()
	_, err := dynamicClient.Resource(gvr).Namespace("proj-p1").Get(ctx, name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("VCJob lookup err = %v, want not found after fallback", err)
	}
}

func assertFallbackPodGroup(
	t *testing.T,
	ctx context.Context,
	dynamicClient *dynamicfake.FakeDynamicClient,
	gvr schema.GroupVersionResource,
	name string,
	minMember int64,
) {
	t.Helper()
	pg, err := dynamicClient.Resource(gvr).Namespace("proj-p1").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fallback PodGroup was not created: %v", err)
	}
	if got := testNestedInt64(pg.Object, "spec", "minMember"); got != minMember {
		t.Fatalf("fallback PodGroup minMember = %d, want %d", got, minMember)
	}
}

func assertFallbackPodCreated(t *testing.T, ctx context.Context, cl *cluster.Client, name, jobID, groupName string) {
	t.Helper()
	pod, err := cl.Clientset().CoreV1().Pods("proj-p1").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fallback pod %s was not created: %v", name, err)
	}
	if pod.Labels[cluster.LabelJobID] != jobID || pod.Labels["app"] != "trainer" {
		t.Fatalf("pod %s labels = %#v, want platform and original labels", name, pod.Labels)
	}
	if pod.Labels[platformJobQueueLabelKey] != "default-batch" || pod.Labels[platformPreemptibleLabelKey] != "false" {
		t.Fatalf("pod %s queue labels = %#v, want queue and non-preemptible labels", name, pod.Labels)
	}
	if pod.Annotations[volcanoGroupAnnotationKey] != groupName || pod.Annotations[schedulingGroupAnnotationKey] != groupName {
		t.Fatalf("pod %s annotations = %#v, want PodGroup annotations", name, pod.Annotations)
	}
	if pod.Spec.SchedulerName != "volcano" || pod.Spec.PriorityClassName != "platform-batch-high" {
		t.Fatalf("pod %s spec = %#v, want volcano scheduler and priority", name, pod.Spec)
	}
	assertCorePodSpecAutomountSATokenFalse(t, pod.Spec)
}

func assertCreatedResourceKinds(t *testing.T, ctx context.Context, app *platform.App, jobID string, kinds []string) {
	t.Helper()
	record, _ := app.Store.Get(ctx, jobsResource, jobID)
	resources, ok := record.Data["created_resources"].([]map[string]any)
	if !ok || len(resources) != len(kinds) {
		t.Fatalf("created resources = %#v, want kinds %v", record.Data["created_resources"], kinds)
	}
	for i, kind := range kinds {
		if resources[i]["kind"] != kind {
			t.Fatalf("created resource %d = %#v, want kind %s", i, resources[i], kind)
		}
	}
}

func TestDispatchSubmittedWorkloadSynthesizesPodGroupForVolcanoNativePod(t *testing.T) {
	now := time.Date(2026, 6, 14, 17, 45, 0, 0, time.UTC)
	podGroupGVR := schema.GroupVersionResource{Group: "scheduling.volcano.sh", Version: "v1beta1", Resource: "podgroups"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGroupGVR: "PodGroupList"},
	)
	cl := cluster.NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":             "J2600101",
		"job_id":         "J2600101",
		"project_id":     "P1",
		"user_id":        "U1",
		"status":         "submitted",
		"namespace":      "proj-p1",
		"queue_name":     "gpu-queue",
		"scheduler_name": "volcano",
		"priority":       500000,
		"created_at":     now.Add(-time.Minute).Format(time.RFC3339),
		"resources":      []any{nativePodResource("worker")},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	pg, err := dynamicClient.Resource(podGroupGVR).Namespace("proj-p1").Get(ctx, "j2600101", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("PodGroup was not created dynamically: %v", err)
	}
	if minMember := testNestedInt64(pg.Object, "spec", "minMember"); minMember != 1 {
		t.Fatalf("PodGroup minMember = %d, want 1", minMember)
	}
	pod, err := cl.Clientset().CoreV1().Pods("proj-p1").Get(ctx, "worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("native pod was not created: %v", err)
	}
	if pod.Annotations[volcanoGroupAnnotationKey] != "j2600101" || pod.Annotations[schedulingGroupAnnotationKey] != "j2600101" {
		t.Fatalf("pod annotations = %#v, want PodGroup annotations", pod.Annotations)
	}
	if pod.Labels[volcanoPodGroupLabelKey] != "j2600101" || pod.Spec.SchedulerName != "volcano" {
		t.Fatalf("pod labels/spec = %#v/%#v, want volcano PodGroup label and scheduler", pod.Labels, pod.Spec)
	}
	assertCorePodSpecAutomountSATokenFalse(t, pod.Spec)
	record, _ := app.Store.Get(ctx, jobsResource, "J2600101")
	resources, ok := record.Data["created_resources"].([]map[string]any)
	if !ok || len(resources) != 2 || resources[0]["kind"] != "PodGroup" || resources[1]["kind"] != "Pod" {
		t.Fatalf("created resources = %#v, want PodGroup then Pod", record.Data["created_resources"])
	}
}

func TestDispatchSubmittedWorkloadMaterializesDRAClaimTemplateAndInjectsPod(t *testing.T) {
	now := time.Date(2026, 6, 14, 18, 15, 0, 0, time.UTC)
	claimTemplateGVR := schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceclaimtemplates"}
	vcJobGVR := schema.GroupVersionResource{Group: "batch.volcano.sh", Version: "v1alpha1", Resource: "jobs"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			claimTemplateGVR: "ResourceClaimTemplateList",
			vcJobGVR:         "JobList",
		},
	)
	cl := cluster.NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":                  "J2600105",
		"job_id":              "J2600105",
		"project_id":          "P1",
		"user_id":             "U1",
		"status":              "submitted",
		"namespace":           "proj-p1",
		"queue_name":          "gpu-queue",
		"scheduler_name":      "volcano",
		"priority":            10000,
		"gpu_count":           1,
		"sm_percentage":       50,
		"pinned_memory_limit": "8Gi",
		"device_class_name":   "rtx6000pro.gpu.nvidia.com",
		"created_at":          now.Add(-time.Minute).Format(time.RFC3339),
		"resources":           []any{nativeDRAPodResource("dra-worker")},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	claimName := "gpu-rtx6000pro-cnt1-sm50-mem8gi"
	claimTemplate, err := dynamicClient.Resource(claimTemplateGVR).Namespace("proj-p1").Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("DRA ResourceClaimTemplate was not created: %v", err)
	}
	assertDRAClaimTemplateSpec(t, claimTemplate, "rtx6000pro.gpu.nvidia.com", 1, 50, "8Gi")
	if _, err := dynamicClient.Resource(vcJobGVR).Namespace("proj-p1").Get(ctx, "j2600105", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("VCJob lookup err = %v, want no Volcano VCJob for DRA job", err)
	}
	pod, err := cl.Clientset().CoreV1().Pods("proj-p1").Get(ctx, "dra-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("DRA pod was not created: %v", err)
	}
	assertPodUsesDRAClaimTemplate(t, pod, claimName)
	if pod.Spec.SchedulerName != defaultDispatcherSchedulerName {
		t.Fatalf("DRA pod schedulerName = %q, want %q", pod.Spec.SchedulerName, defaultDispatcherSchedulerName)
	}
	assertCreatedResourceKinds(t, ctx, app, "J2600105", []string{"ResourceClaimTemplate", "Pod"})
}

func TestDispatchSubmittedWorkloadInjectsStorageMountsIntoNativePod(t *testing.T) {
	now := time.Date(2026, 6, 14, 18, 25, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":         "J2600106",
		"job_id":     "J2600106",
		"project_id": "P1",
		"user_id":    "U1",
		"status":     "submitted",
		"namespace":  "proj-p1",
		"created_at": now.Add(-time.Minute).Format(time.RFC3339),
		"storage_mounts": []any{map[string]any{
			"pvc_name":   "datasets-pvc",
			"mount_path": "/mnt/datasets",
			"read_only":  true,
			"sub_path":   "training",
		}},
		"resources": []any{nativePodResource("storage-worker")},
	})

	storagePlans := &fakeStorageMountPlanClient{plan: storageMountPlan{
		ManifestMounts: []storageMountPlanMount{{
			Name: "datasets-pvc", ClaimName: "datasets-pvc", MountPath: "/mnt/datasets", ReadOnly: true, SubPath: "training",
		}},
	}}
	if err := dispatchSubmittedWorkloadsWithStorageMountClient(ctx, app.Cluster, app.Store, storagePlans.Resolve, now); err != nil {
		t.Fatal(err)
	}

	pod, err := cl.Clientset().CoreV1().Pods("proj-p1").Get(ctx, "storage-worker", metav1.GetOptions{})
	if err != nil {
		record, _ := app.Store.Get(ctx, jobsResource, "J2600106")
		t.Fatalf("storage pod was not created: %v; job=%#v", err, record.Data)
	}
	if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].PersistentVolumeClaim == nil ||
		pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "datasets-pvc" {
		t.Fatalf("pod volumes = %#v, want datasets-pvc PVC volume", pod.Spec.Volumes)
	}
	mount := pod.Spec.Containers[0].VolumeMounts[0]
	if mount.Name != "datasets-pvc" || mount.MountPath != "/mnt/datasets" || !mount.ReadOnly || mount.SubPath != "training" {
		t.Fatalf("pod volumeMount = %#v, want read-only datasets mount", mount)
	}
}

func TestDispatchSubmittedWorkloadInjectsStorageMountsIntoBatchJobTemplate(t *testing.T) {
	now := time.Date(2026, 6, 14, 18, 30, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":         "J2600107",
		"job_id":     "J2600107",
		"project_id": "P1",
		"user_id":    "U1",
		"status":     "submitted",
		"namespace":  "proj-p1",
		"created_at": now.Add(-time.Minute).Format(time.RFC3339),
		"submission_payload": map[string]any{
			"storageMounts": []any{map[string]any{
				"claimName": "scratch-pvc",
				"mountPath": "/scratch",
				"name":      "scratch",
			}},
		},
		"resources": []any{nativeJobResource("storage-train")},
	})

	storagePlans := &fakeStorageMountPlanClient{plan: storageMountPlan{
		ManifestMounts: []storageMountPlanMount{{
			Name: "scratch", ClaimName: "scratch-pvc", MountPath: "/scratch",
		}},
	}}
	if err := dispatchSubmittedWorkloadsWithStorageMountClient(ctx, app.Cluster, app.Store, storagePlans.Resolve, now); err != nil {
		t.Fatal(err)
	}

	job, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "storage-train", metav1.GetOptions{})
	if err != nil {
		record, _ := app.Store.Get(ctx, jobsResource, "J2600107")
		t.Fatalf("storage job was not created: %v; job=%#v", err, record.Data)
	}
	volumes := job.Spec.Template.Spec.Volumes
	if len(volumes) != 1 || volumes[0].Name != "scratch" || volumes[0].PersistentVolumeClaim == nil ||
		volumes[0].PersistentVolumeClaim.ClaimName != "scratch-pvc" {
		t.Fatalf("job template volumes = %#v, want scratch PVC volume", volumes)
	}
	mounts := job.Spec.Template.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 || mounts[0].Name != "scratch" || mounts[0].MountPath != "/scratch" {
		t.Fatalf("job template mounts = %#v, want scratch mount", mounts)
	}
}

func TestDispatchSubmittedWorkloadMaterializesStorageShareBeforePodCreate(t *testing.T) {
	now := time.Date(2026, 6, 14, 18, 35, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(
		boundDispatchPVC("group-storage", "datasets", "pv-juicefs"),
		csiDispatchPV("pv-juicefs", "csi.juicefs.com", corev1.ReadWriteMany),
	), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":         "J2600108",
		"job_id":     "J2600108",
		"project_id": "P1",
		"user_id":    "U1",
		"status":     "submitted",
		"namespace":  "proj-p1",
		"created_at": now.Add(-time.Minute).Format(time.RFC3339),
		"storage_mounts": []any{map[string]any{
			"pvc_id":           "datasets",
			"source_namespace": "forged-storage",
			"source_pvc":       "forged-source",
			"target_pvc":       "forged-target",
			"mount_path":       "/mnt/datasets",
			"read_only":        true,
		}},
		"resources": []any{nativePodResource("shared-storage-worker")},
	})

	storagePlans := &fakeStorageMountPlanClient{plan: storageMountPlan{
		ManifestMounts: []storageMountPlanMount{{
			Name: "datasets", ClaimName: "datasets", MountPath: "/mnt/datasets", ReadOnly: true,
		}},
		PVCShareOperations: []storageMountPlanShareOp{{
			SourceNamespace: "group-storage", SourcePVC: "datasets", TargetPVC: "datasets",
		}},
	}}
	if err := dispatchSubmittedWorkloadsWithStorageMountClient(ctx, app.Cluster, app.Store, storagePlans.Resolve, now); err != nil {
		t.Fatal(err)
	}
	if storagePlans.got.Mounts[0].PVCID != "datasets" {
		t.Fatalf("storage mount request = %#v, want storage selector datasets", storagePlans.got.Mounts)
	}

	if _, err := cl.Clientset().CoreV1().PersistentVolumes().Get(ctx, "share-juicefs-proj-p1-datasets", metav1.GetOptions{}); err != nil {
		t.Fatalf("storage share PV was not materialized before dispatch: %v", err)
	}
	targetPVC, err := cl.Clientset().CoreV1().PersistentVolumeClaims("proj-p1").Get(ctx, "datasets", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("storage share PVC was not materialized before dispatch: %v", err)
	}
	if targetPVC.Spec.VolumeName != "share-juicefs-proj-p1-datasets" {
		t.Fatalf("target PVC volume = %q, want static JuiceFS share PV", targetPVC.Spec.VolumeName)
	}
	pod, err := cl.Clientset().CoreV1().Pods("proj-p1").Get(ctx, "shared-storage-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("shared storage pod was not created: %v", err)
	}
	if pod.Spec.Volumes[0].PersistentVolumeClaim == nil || pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "datasets" {
		t.Fatalf("pod volumes = %#v, want target datasets PVC", pod.Spec.Volumes)
	}
	if mount := pod.Spec.Containers[0].VolumeMounts[0]; mount.MountPath != "/mnt/datasets" || !mount.ReadOnly {
		t.Fatalf("pod volume mount = %#v, want read-only shared storage mount", mount)
	}
}

func TestCollectDispatchPVCClaimNamesPodAndDeployment(t *testing.T) {
	resources := []dispatchResource{
		{
			Name: "pod-a",
			Kind: "Pod",
			Raw: []byte(`{
				"kind":"Pod",
				"spec":{
					"containers":[{"name":"c","image":"alpine"}],
					"volumes":[
						{"name":"data","persistentVolumeClaim":{"claimName":"pvc-a"}},
						{"name":"cache","emptyDir":{}}
					]
				}
			}`),
		},
		{
			Name: "dep-b",
			Kind: "Deployment",
			Raw: []byte(`{
				"kind":"Deployment",
				"spec":{"template":{"spec":{
					"containers":[{"name":"c","image":"alpine"}],
					"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"pvc-b"}}]
				}}}
			}`),
		},
	}

	claims := collectDispatchPVCClaimNames(resources)
	if _, ok := claims["pvc-a"]; !ok {
		t.Fatalf("expected pvc-a claim, got %v", claims)
	}
	if _, ok := claims["pvc-b"]; !ok {
		t.Fatalf("expected pvc-b claim, got %v", claims)
	}
	if len(claims) != 2 {
		t.Fatalf("claims = %v, want two PVC claims", claims)
	}
}

func TestDispatchSubmittedWorkloadKeepsNativePathWithoutDynamicClient(t *testing.T) {
	now := time.Date(2026, 6, 14, 17, 50, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":             "J2600102",
		"job_id":         "J2600102",
		"status":         "submitted",
		"namespace":      "proj-p1",
		"scheduler_name": "volcano",
		"created_at":     now.Add(-time.Minute).Format(time.RFC3339),
		"resources":      []any{nativeJobResource("train")},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "train", metav1.GetOptions{}); err != nil {
		t.Fatalf("native job was not created without dynamic client: %v", err)
	}
}

func TestDispatchSubmittedWorkloadDefersWhenClusterUnavailable(t *testing.T) {
	now := time.Date(2026, 6, 14, 16, 30, 0, 0, time.UTC)
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":        "J2600002",
		"status":    "submitted",
		"namespace": "proj-p1",
		"resources": []any{nativeJobResource("train")},
	})

	if err := dispatchSubmittedWorkloads(ctx, nil, app.Store, now); err != nil {
		t.Fatal(err)
	}

	record, _ := app.Store.Get(ctx, jobsResource, "J2600002")
	if record.Data["status"] != "waiting_infra" || record.Data["retry_count"] != 1 {
		t.Fatalf("deferred job = %#v, want waiting_infra retry 1", record.Data)
	}
	if !strings.Contains(record.Data["error_message"].(string), "cluster client unavailable") {
		t.Fatalf("deferred reason = %v, want cluster unavailable", record.Data["error_message"])
	}
}

func TestDispatchSubmittedWorkloadFailsUnsupportedManifestAndDoesNotCreateResources(t *testing.T) {
	now := time.Date(2026, 6, 14, 16, 30, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "proj-p1"}}), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":        "J2600003",
		"job_id":    "J2600003",
		"status":    "submitted",
		"namespace": "proj-p1",
		"resources": []any{map[string]any{
			"name":      "hourly",
			"kind":      "CronJob",
			"json_data": `{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"name":"hourly"}}`,
		}},
	})

	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	record, _ := app.Store.Get(ctx, jobsResource, "J2600003")
	if record.Data["status"] != "failed" || !strings.Contains(record.Data["error_message"].(string), "unsupported Kubernetes manifest kind") {
		t.Fatalf("failed job = %#v, want unsupported-kind failure", record.Data)
	}
	jobs, _ := cl.Clientset().BatchV1().Jobs("proj-p1").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatalf("jobs created = %d, want none", len(jobs.Items))
	}
}

func TestDispatchWaitingInfraSkipsUntilRetryDueAndRegistrationRunsTask(t *testing.T) {
	now := time.Now().UTC()
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id":            "J2600004",
		"job_id":        "J2600004",
		"project_id":    "P1",
		"user_id":       "U1",
		"status":        "waiting_infra",
		"namespace":     "proj-p1",
		"next_retry_at": now.Add(time.Hour).Format(time.RFC3339),
		"resources":     []any{nativeJobResource("future")},
	})
	if err := dispatchSubmittedWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "future", metav1.GetOptions{}); err == nil {
		t.Fatal("future retry job was dispatched before next_retry_at")
	}

	record, _ := app.Store.Get(ctx, jobsResource, "J2600004")
	record.Data["next_retry_at"] = now.Add(-time.Second).Format(time.RFC3339)
	if _, ok := app.Store.Update(ctx, jobsResource, "J2600004", record.Data); !ok {
		t.Fatal("failed to update retry time")
	}
	Register(app)
	app.RunMaintenanceOnce(ctx, time.Minute)
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "future", metav1.GetOptions{}); err != nil {
		t.Fatalf("registered dispatcher did not create job: %v", err)
	}
}

func nativeJobResource(name string) map[string]any {
	return map[string]any{
		"name": name,
		"kind": "Job",
		"json_data": `{
			"apiVersion":"batch/v1",
			"kind":"Job",
			"metadata":{"name":"` + name + `","labels":{"existing":"label"}},
			"spec":{"template":{"metadata":{"labels":{"app":"trainer"}},"spec":{"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}]}}}
		}`,
	}
}

func nativeJobWithDeadlineResource(name string, seconds int) dispatchResource {
	return dispatchResource{
		Name: name,
		Kind: "Job",
		Raw: []byte(`{
			"apiVersion":"batch/v1",
			"kind":"Job",
			"metadata":{"name":"` + name + `"},
			"spec":{"activeDeadlineSeconds":` + strconv.Itoa(seconds) + `,"template":{"spec":{"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}]}}}
		}`),
	}
}

func nativeDeploymentResource(name string) map[string]any {
	return map[string]any{
		"name": name,
		"kind": "Deployment",
		"json_data": `{
			"apiVersion":"apps/v1",
			"kind":"Deployment",
			"metadata":{"name":"` + name + `","labels":{"existing":"label"}},
			"spec":{"replicas":1,"selector":{"matchLabels":{"app":"trainer"}},"template":{"metadata":{"labels":{"app":"trainer"}},"spec":{"containers":[{"name":"main","image":"busybox","command":["sleep","3600"]}]}}}
		}`,
	}
}

func nativeParallelJobResource(name string, parallelism int) map[string]any {
	return map[string]any{
		"name": name,
		"kind": "Job",
		"json_data": `{
			"apiVersion":"batch/v1",
			"kind":"Job",
			"metadata":{"name":"` + name + `","labels":{"existing":"label"}},
			"spec":{"parallelism":` + strconv.Itoa(parallelism) + `,"template":{"metadata":{"labels":{"app":"trainer"}},"spec":{"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}]}}}
		}`,
	}
}

func nativePodResource(name string) map[string]any {
	return map[string]any{
		"name": name,
		"kind": "Pod",
		"json_data": `{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"` + name + `","labels":{"app":"trainer"}},
			"spec":{"containers":[{"name":"main","image":"busybox"}]}
		}`,
	}
}

func nativeDRAPodResource(name string) map[string]any {
	return map[string]any{
		"name": name,
		"kind": "Pod",
		"json_data": `{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"` + name + `","labels":{"app":"trainer"}},
			"spec":{"containers":[{"name":"main","image":"busybox","resources":{"requests":{"cpu":"500m","nvidia.com/gpu":"1"},"limits":{"nvidia.com/gpu":"1"}}}]}
		}`,
	}
}

func volcanoVCJobResource(name string) map[string]any {
	return map[string]any{
		"name": name,
		"kind": "Job",
		"json_data": `{
			"apiVersion":"batch.volcano.sh/v1alpha1",
			"kind":"Job",
			"metadata":{"name":"` + name + `","labels":{"existing":"label"}},
			"spec":{"tasks":[{"name":"main","template":{"metadata":{"labels":{"app":"trainer"}},"spec":{"containers":[{"name":"main","image":"busybox"}]}}}]}
		}`,
	}
}

func firstVCJobTaskLabels(t *testing.T, obj *unstructured.Unstructured) map[string]string {
	t.Helper()
	task := firstVCJobTask(t, obj)
	labels, _, _ := unstructured.NestedStringMap(task, "template", "metadata", "labels")
	return labels
}

func firstVCJobTaskSpec(t *testing.T, obj *unstructured.Unstructured) map[string]any {
	t.Helper()
	task := firstVCJobTask(t, obj)
	spec, _, _ := unstructured.NestedMap(task, "template", "spec")
	return spec
}

func firstVCJobTask(t *testing.T, obj *unstructured.Unstructured) map[string]any {
	t.Helper()
	tasks, found, _ := unstructured.NestedSlice(obj.Object, "spec", "tasks")
	if !found || len(tasks) == 0 {
		t.Fatalf("VCJob tasks = %#v, want at least one task", tasks)
	}
	task, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatalf("VCJob first task = %#v, want object", tasks[0])
	}
	return task
}

func assertCorePodSpecAutomountSATokenFalse(t *testing.T, spec corev1.PodSpec) {
	t.Helper()
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Fatalf("automountServiceAccountToken = %#v, want false", spec.AutomountServiceAccountToken)
	}
}

func assertMapPodSpecAutomountSATokenFalse(t *testing.T, obj map[string]any, path ...string) {
	t.Helper()
	spec := nestedTestMap(t, obj, path...)
	if spec["automountServiceAccountToken"] != false {
		t.Fatalf("%v automountServiceAccountToken = %#v, want false in %#v", path, spec["automountServiceAccountToken"], spec)
	}
}

func nestedTestMap(t *testing.T, obj map[string]any, path ...string) map[string]any {
	t.Helper()
	var current any = obj
	for _, step := range path {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[step]
		case []any:
			index, err := strconv.Atoi(step)
			if err != nil || index < 0 || index >= len(typed) {
				t.Fatalf("invalid nested slice step %q in path %v", step, path)
			}
			current = typed[index]
		default:
			t.Fatalf("nested path %v hit %T at %q", path, current, step)
		}
	}
	out, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("nested path %v = %T, want map", path, current)
	}
	return out
}

func assertDRAClaimTemplateSpec(
	t *testing.T,
	obj *unstructured.Unstructured,
	deviceClass string,
	count int64,
	smPct int64,
	pinnedMem string,
) {
	t.Helper()
	requests, found, _ := unstructured.NestedSlice(obj.Object, "spec", "spec", "devices", "requests")
	if !found || len(requests) != 1 {
		t.Fatalf("DRA requests = %#v, want one request", requests)
	}
	request, ok := requests[0].(map[string]any)
	if !ok {
		t.Fatalf("DRA request = %#v, want object", requests[0])
	}
	exactly, _ := request["exactly"].(map[string]any)
	if exactly["deviceClassName"] != deviceClass || testAnyInt64(exactly["count"]) != count {
		t.Fatalf("DRA exactly = %#v, want %s count %d", exactly, deviceClass, count)
	}
	configs, found, _ := unstructured.NestedSlice(obj.Object, "spec", "spec", "devices", "config")
	if !found || len(configs) != 1 {
		t.Fatalf("DRA config = %#v, want one MPS config", configs)
	}
	config, _ := configs[0].(map[string]any)
	opaque, _ := config["opaque"].(map[string]any)
	parameters, _ := opaque["parameters"].(map[string]any)
	sharing, _ := parameters["sharing"].(map[string]any)
	mps, _ := sharing["mpsConfig"].(map[string]any)
	if testAnyInt64(mps["defaultActiveThreadPercentage"]) != smPct || mps["defaultPinnedDeviceMemoryLimit"] != pinnedMem {
		t.Fatalf("DRA MPS config = %#v, want sm %d pinned %s", mps, smPct, pinnedMem)
	}
}

func assertPodUsesDRAClaimTemplate(t *testing.T, pod *corev1.Pod, claimName string) {
	t.Helper()
	if len(pod.Spec.ResourceClaims) != 1 || pod.Spec.ResourceClaims[0].Name != draPodClaimName {
		t.Fatalf("pod resourceClaims = %#v, want one gpu claim", pod.Spec.ResourceClaims)
	}
	if pod.Spec.ResourceClaims[0].ResourceClaimTemplateName == nil || *pod.Spec.ResourceClaims[0].ResourceClaimTemplateName != claimName {
		t.Fatalf("pod resourceClaimTemplateName = %#v, want %s", pod.Spec.ResourceClaims[0].ResourceClaimTemplateName, claimName)
	}
	if len(pod.Spec.Containers[0].Resources.Claims) != 1 || pod.Spec.Containers[0].Resources.Claims[0].Name != draPodClaimName {
		t.Fatalf("container resource claims = %#v, want gpu claim", pod.Spec.Containers[0].Resources.Claims)
	}
	if _, ok := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")]; ok {
		t.Fatalf("container requests still contain nvidia.com/gpu: %#v", pod.Spec.Containers[0].Resources.Requests)
	}
	if _, ok := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")]; ok {
		t.Fatalf("container limits still contain nvidia.com/gpu: %#v", pod.Spec.Containers[0].Resources.Limits)
	}
	if pod.Labels[draLabelEffectiveGPU] != "0.5" || pod.Labels[draLabelGPUModel] != "rtx6000pro" || pod.Labels[draLabelClaimName] != claimName {
		t.Fatalf("pod DRA labels = %#v, want effective GPU, model, and claim name", pod.Labels)
	}
	if pod.Annotations[draVolcanoResourceRequestAnno] != "nvidia.com/rtx6000pro:1" {
		t.Fatalf("pod DRA annotation = %#v, want Volcano resource request", pod.Annotations)
	}
}

func testNestedInt64(obj map[string]any, fields ...string) int64 {
	value, found, _ := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found {
		return 0
	}
	return testAnyInt64(value)
}

func testAnyInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func boundDispatchPVC(namespace, name, volumeName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources:  corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
			VolumeName: volumeName,
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}

func csiDispatchPV(name, driver string, accessMode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Capacity:    corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: driver, VolumeHandle: name},
			},
		},
	}
}
