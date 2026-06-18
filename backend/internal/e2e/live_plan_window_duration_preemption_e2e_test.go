//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	"github.com/linskybing/nexuspaas/backend/internal/services/orgproject"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
	"github.com/linskybing/nexuspaas/backend/internal/services/workload"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLiveK8sPlanWindowDurationPreemptionE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_PLAN_WINDOW_DURATION_PREEMPTION")) != "1" {
		t.Skip("set TEST_LIVE_K8S_PLAN_WINDOW_DURATION_PREEMPTION=1 to run live plan-window/duration/preemption Kubernetes e2e")
	}
	requireLiveKubeconfig(t)
	liveCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil {
		t.Fatal("live Kubernetes client is unavailable")
	}
	if err := cl.Ping(liveCtx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}

	app := platform.NewApp(platform.Config{
		ServiceName:           "all",
		HTTPAddr:              ":0",
		ServiceAPIKey:         "e2e-service-key",
		RequireAuth:           false,
		PlanWindowPodDeletion: true,
		K8sNamespacePrefix:    "proj",
		DefaultQueueName:      "default-batch",
		AdapterTimeout:        5 * time.Second,
		MaintenanceInterval:   100 * time.Millisecond,
	}, platform.WithCluster(cl))
	registerLiveWorkloadSchedulerServices(app)

	suffix := truncateID(sanitizeID(time.Now().UTC().Format("150405.000000000")), 18)
	userID := "u" + suffix

	runLiveDurationJobScenario(t, liveCtx, app, cl, suffix, userID)
	runLiveDurationDeploymentScenario(t, liveCtx, app, cl, suffix, userID)
	runLivePlanWindowScenario(t, liveCtx, app, cl, suffix, userID)
	runLiveAutoPreemptionScenario(t, liveCtx, app, cl, suffix, userID)
}

func registerLiveWorkloadSchedulerServices(app *platform.App) {
	allowed := map[string]bool{
		orgProjectService:     true,
		schedulerQuotaService: true,
		workloadService:       true,
	}
	for _, spec := range services.Catalog() {
		if allowed[spec.Name] {
			app.RegisterService(spec)
		}
	}
	orgproject.Register(app)
	schedulerquota.Register(app)
	workload.Register(app)
}

func runLiveDurationJobScenario(t *testing.T, ctx context.Context, app *platform.App, cl *cluster.Client, suffix, userID string) {
	projectID := "durjob" + suffix
	queueName := "duration-job-" + suffix
	namespace := liveDeployNamespace(projectID, userID)
	t.Cleanup(func() { cleanupLiveNamespace(t, cl, namespace) })
	seedLiveProjectPlan(t, app, livePlanSeed{
		ProjectID: projectID,
		UserID:    userID,
		Queues: []liveQueueSpec{{
			ID: "q-" + projectID, Name: queueName, Priority: 1000, Preemptible: true, RuntimeSeconds: 10,
		}},
		CPULimit: 1, MemoryGB: 1,
	})

	jobID := "job-" + projectID
	jobName := "job-" + suffix
	jobRef := liveK8sResourceRef{Kind: "Job", Namespace: namespace, Name: jobName}
	postLiveJobSubmit(t, app, map[string]any{
		"job_id":          jobID,
		"project_id":      projectID,
		"user_id":         userID,
		"queue_name":      queueName,
		"required_cpu":    0.05,
		"required_memory": 32,
		"namespace":       namespace,
		"resources": []map[string]any{{
			"name":      jobName,
			"kind":      "Job",
			"json_data": livePauseJobManifest(t, jobName),
		}},
	}, http.StatusCreated)
	waitLiveDispatch(t, ctx, app, jobRef, jobID)

	k8sJob, err := cl.Clientset().BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get duration Job %s/%s: %v", namespace, jobName, err)
	}
	if k8sJob.Spec.ActiveDeadlineSeconds == nil || *k8sJob.Spec.ActiveDeadlineSeconds != 10 ||
		k8sJob.Labels[cluster.RuntimeLimitSecondsKey] != "10" ||
		k8sJob.Spec.Template.Labels[cluster.RuntimeLimitSecondsKey] != "10" {
		t.Fatalf("duration Job = deadline:%#v labels:%#v template:%#v, want runtime limit 10", k8sJob.Spec.ActiveDeadlineSeconds, k8sJob.Labels, k8sJob.Spec.Template.Labels)
	}
	waitLiveRuntimeDeletion(t, ctx, app, cl, jobRef, jobID, "failed")
}

func runLiveDurationDeploymentScenario(t *testing.T, ctx context.Context, app *platform.App, cl *cluster.Client, suffix, userID string) {
	projectID := "durdep" + suffix
	queueName := "duration-deploy-" + suffix
	namespace := liveDeployNamespace(projectID, userID)
	t.Cleanup(func() { cleanupLiveNamespace(t, cl, namespace) })
	seedLiveProjectPlan(t, app, livePlanSeed{
		ProjectID: projectID,
		UserID:    userID,
		Queues: []liveQueueSpec{{
			ID: "q-" + projectID, Name: queueName, Priority: 1000, Preemptible: true, RuntimeSeconds: 10,
		}},
		CPULimit: 1, MemoryGB: 1,
	})

	jobID := "job-" + projectID
	deployName := "dep-" + suffix
	deployRef := liveK8sResourceRef{Kind: "Deployment", Namespace: namespace, Name: deployName}
	postLiveJobSubmit(t, app, map[string]any{
		"job_id":          jobID,
		"project_id":      projectID,
		"user_id":         userID,
		"queue_name":      queueName,
		"required_cpu":    0.05,
		"required_memory": 32,
		"namespace":       namespace,
		"resources": []map[string]any{{
			"name":      deployName,
			"kind":      "Deployment",
			"json_data": livePauseDeploymentManifest(t, deployName),
		}},
	}, http.StatusCreated)
	waitLiveDispatch(t, ctx, app, deployRef, jobID)

	deployment, err := cl.Clientset().AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get duration Deployment %s/%s: %v", namespace, deployName, err)
	}
	if deployment.Labels[cluster.RuntimeLimitSecondsKey] != "10" ||
		deployment.Spec.Template.Labels[cluster.RuntimeLimitSecondsKey] != "10" ||
		deployment.Spec.Template.Spec.ActiveDeadlineSeconds != nil {
		t.Fatalf("duration Deployment labels/deadline = object:%#v template:%#v deadline:%#v, want labels only", deployment.Labels, deployment.Spec.Template.Labels, deployment.Spec.Template.Spec.ActiveDeadlineSeconds)
	}
	waitLiveRuntimeDeletion(t, ctx, app, cl, deployRef, jobID, "failed")
}

func runLivePlanWindowScenario(t *testing.T, ctx context.Context, app *platform.App, cl *cluster.Client, suffix, userID string) {
	projectID := "planwin" + suffix
	queueName := "plan-window-" + suffix
	namespace := liveDeployNamespace(projectID, userID)
	t.Cleanup(func() { cleanupLiveNamespace(t, cl, namespace) })
	planID := seedLiveProjectPlan(t, app, livePlanSeed{
		ProjectID: projectID,
		UserID:    userID,
		Queues: []liveQueueSpec{{
			ID: "q-" + projectID, Name: queueName, Priority: 1000, Preemptible: true, RuntimeSeconds: 60,
		}},
		CPULimit: 1, MemoryGB: 1,
	})

	jobID := "job-" + projectID
	deployName := "pwindep-" + suffix
	markerPodName := "pwinmarker-" + suffix
	deployRef := liveK8sResourceRef{Kind: "Deployment", Namespace: namespace, Name: deployName}
	markerRef := liveK8sResourceRef{Kind: "Pod", Namespace: namespace, Name: markerPodName}
	postLiveJobSubmit(t, app, map[string]any{
		"job_id":          jobID,
		"project_id":      projectID,
		"user_id":         userID,
		"queue_name":      queueName,
		"required_cpu":    0.05,
		"required_memory": 32,
		"namespace":       namespace,
		"resources": []map[string]any{{
			"name":      deployName,
			"kind":      "Deployment",
			"json_data": livePauseDeploymentManifest(t, deployName),
		}},
	}, http.StatusCreated)
	waitLiveDispatch(t, ctx, app, deployRef, jobID)
	createLiveMarkerPod(t, ctx, cl, namespace, markerPodName, jobID, projectID)

	if _, ok := app.Store.Update(ctx, schedulerPlansResource, planID, map[string]any{
		"valid_until": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	}); !ok {
		t.Fatalf("expire live plan %s failed", planID)
	}
	waitLiveMaintenanceStatus(t, ctx, app, jobID, "evicted")
	if err := waitLiveResourceDeleted(ctx, cl, deployRef, 10*time.Second); err != nil {
		t.Fatalf("plan-window Deployment %s/%s not deleted: %v", namespace, deployName, err)
	}
	if err := waitLiveResourceDeleted(ctx, cl, markerRef, 10*time.Second); err != nil {
		t.Fatalf("plan-window marker Pod %s/%s not deleted: %v", namespace, markerPodName, err)
	}
}

func runLiveAutoPreemptionScenario(t *testing.T, ctx context.Context, app *platform.App, cl *cluster.Client, suffix, userID string) {
	projectID := "preempt" + suffix
	lowQueue := "low-" + suffix
	highQueue := "high-" + suffix
	namespace := liveDeployNamespace(projectID, userID)
	t.Cleanup(func() { cleanupLiveNamespace(t, cl, namespace) })
	seedLiveProjectPlan(t, app, livePlanSeed{
		ProjectID: projectID,
		UserID:    userID,
		Queues: []liveQueueSpec{
			{ID: "ql-" + projectID, Name: lowQueue, Priority: 1000, Preemptible: true, RuntimeSeconds: 60},
			{ID: "qh-" + projectID, Name: highQueue, Priority: 10000, Preemptible: false, RuntimeSeconds: 60},
		},
		CPULimit: 0.15, MemoryGB: 1,
	})

	lowJobID := "low-" + projectID
	lowName := "lowdep-" + suffix
	lowRef := liveK8sResourceRef{Kind: "Deployment", Namespace: namespace, Name: lowName}
	postLiveJobSubmit(t, app, map[string]any{
		"job_id":          lowJobID,
		"project_id":      projectID,
		"user_id":         userID,
		"queue_name":      lowQueue,
		"required_cpu":    0.1,
		"required_memory": 32,
		"namespace":       namespace,
		"resources": []map[string]any{{
			"name":      lowName,
			"kind":      "Deployment",
			"json_data": livePauseDeploymentManifest(t, lowName),
		}},
	}, http.StatusCreated)
	waitLiveDispatch(t, ctx, app, lowRef, lowJobID)
	if _, ok := app.Store.Update(ctx, workloadJobsResource, lowJobID, map[string]any{
		"status":         "running",
		"priority_value": 1000,
		"preemptible":    true,
	}); !ok {
		t.Fatalf("force low-priority victim running status failed")
	}

	highJobID := "high-" + projectID
	highName := "highjob-" + suffix
	highRef := liveK8sResourceRef{Kind: "Job", Namespace: namespace, Name: highName}
	response := postLiveJobSubmit(t, app, map[string]any{
		"job_id":          highJobID,
		"project_id":      projectID,
		"user_id":         userID,
		"queue_name":      highQueue,
		"required_cpu":    0.1,
		"required_memory": 32,
		"namespace":       namespace,
		"resources": []map[string]any{{
			"name":      highName,
			"kind":      "Job",
			"json_data": livePauseJobManifest(t, highName),
		}},
	}, http.StatusCreated)
	recordData := liveResponseRecordData(t, response)
	if textE2E(recordData["admission_preemption_status"]) != "completed" {
		t.Fatalf("high-priority job response = %#v, want completed auto preemption metadata", recordData)
	}
	waitLiveDispatch(t, ctx, app, highRef, highJobID)

	lowRecord, _ := app.Store.Get(ctx, workloadJobsResource, lowJobID)
	if textE2E(lowRecord.Data["status"]) != "preempted" {
		t.Fatalf("low-priority job record = %#v, want preempted", lowRecord.Data)
	}
	if err := waitLiveResourceDeleted(ctx, cl, lowRef, 10*time.Second); err != nil {
		t.Fatalf("preempted Deployment %s/%s not deleted: %v", namespace, lowName, err)
	}
	highRecord, _ := app.Store.Get(ctx, workloadJobsResource, highJobID)
	highStatus := textE2E(highRecord.Data["status"])
	if (highStatus != "running" && highStatus != "queued") ||
		highRecord.Data["priority_value"] == nil ||
		!createdResourcesContain(recordListE2E(highRecord.Data["created_resources"]), "Job", namespace, highName) {
		t.Fatalf("high-priority job record = %#v, want admitted job with admission metadata and created resource", highRecord.Data)
	}
}

type liveQueueSpec struct {
	ID             string
	Name           string
	Priority       int
	Preemptible    bool
	RuntimeSeconds int
}

type livePlanSeed struct {
	ProjectID string
	UserID    string
	Queues    []liveQueueSpec
	CPULimit  float64
	MemoryGB  float64
}

type liveK8sResourceRef struct {
	Kind      string
	Namespace string
	Name      string
}

func (ref liveK8sResourceRef) String() string {
	return ref.Kind + " " + ref.Namespace + "/" + ref.Name
}

func seedLiveProjectPlan(t *testing.T, app *platform.App, seed livePlanSeed) string {
	t.Helper()
	queueIDs := make([]string, 0, len(seed.Queues))
	for _, queue := range seed.Queues {
		queueIDs = append(queueIDs, queue.ID)
		createE2ERecord(t, app, schedulerQueuesResource, map[string]any{
			"id":                  queue.ID,
			"name":                queue.Name,
			"priority_value":      queue.Priority,
			"is_preemptible":      queue.Preemptible,
			"max_runtime_seconds": queue.RuntimeSeconds,
		})
	}
	planID := "pl-" + seed.ProjectID
	plan := map[string]any{
		"id":              planID,
		"name":            "plan " + seed.ProjectID,
		"cpu_limit_cores": seed.CPULimit,
		"memory_limit_gb": seed.MemoryGB,
		"queue_ids":       queueIDs,
	}
	createE2ERecord(t, app, schedulerPlansResource, plan)
	createE2ERecord(t, app, orgProjectsResource, map[string]any{
		"id": seed.ProjectID, "project_name": seed.ProjectID, "plan_id": planID, "resource_plan_id": planID,
	})
	createE2ERecord(t, app, orgProjectMembersResource, map[string]any{
		"id": seed.ProjectID + "/" + seed.UserID, "project_id": seed.ProjectID, "user_id": seed.UserID, "role": "user",
	})
	return planID
}

func postLiveJobSubmit(t *testing.T, app http.Handler, payload map[string]any, want int) map[string]any {
	t.Helper()
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", &body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", fmt.Sprint(payload["user_id"]))
	req.Header.Set("Idempotency-Key", "idem-live-"+sanitizeID(fmt.Sprint(payload["job_id"])))
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("POST /api/v1/jobs returned %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func liveResponseRecordData(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response record = %#v, want nested data object", response)
	}
	return data
}

func waitLiveDispatch(t *testing.T, ctx context.Context, app *platform.App, ref liveK8sResourceRef, jobID string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		app.RunMaintenanceOnce(ctx, 200*time.Millisecond)
		record, found := app.Store.Get(ctx, workloadJobsResource, jobID)
		if found && createdResourcesContain(recordListE2E(record.Data["created_resources"]), ref.Kind, ref.Namespace, ref.Name) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s did not dispatch before deadline; job=%#v", ref, record.Data)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func waitLiveRuntimeDeletion(t *testing.T, ctx context.Context, app *platform.App, cl *cluster.Client, ref liveK8sResourceRef, jobID, status string) {
	t.Helper()
	time.Sleep(11 * time.Second)
	waitLiveMaintenanceStatus(t, ctx, app, jobID, status)
	if err := waitLiveResourceDeleted(ctx, cl, ref, 10*time.Second); err != nil {
		t.Fatalf("runtime-limited %s not deleted: %v", ref, err)
	}
}

func waitLiveMaintenanceStatus(t *testing.T, ctx context.Context, app *platform.App, jobID, status string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		app.RunMaintenanceOnce(ctx, 200*time.Millisecond)
		record, found := app.Store.Get(ctx, workloadJobsResource, jobID)
		if found && textE2E(record.Data["status"]) == status {
			return
		}
		if time.Now().After(deadline) {
			record, _ := app.Store.Get(ctx, workloadJobsResource, jobID)
			t.Fatalf("job %s did not reach status %s before deadline; record=%#v", jobID, status, record.Data)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func liveResourceGone(ctx context.Context, cl *cluster.Client, ref liveK8sResourceRef) (bool, error) {
	var err error
	switch ref.Kind {
	case "Job":
		_, err = cl.Clientset().BatchV1().Jobs(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Deployment":
		_, err = cl.Clientset().AppsV1().Deployments(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	case "Pod":
		_, err = cl.Clientset().CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	default:
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	return false, err
}

func waitLiveResourceDeleted(ctx context.Context, cl *cluster.Client, ref liveK8sResourceRef, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		gone, err := liveResourceGone(ctx, cl, ref)
		if gone {
			return nil
		}
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return os.ErrDeadlineExceeded
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func cleanupLiveNamespace(t *testing.T, cl *cluster.Client, namespace string) {
	t.Helper()
	err := cl.Clientset().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Logf("cleanup namespace %s: %v", namespace, err)
	}
}

func createLiveMarkerPod(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name, jobID, projectID string) {
	t.Helper()
	_, err := cl.Clientset().CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				cluster.LabelJobID:     jobID,
				cluster.LabelProjectID: projectID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: "registry.k8s.io/pause:3.9",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{},
				},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create live marker pod %s/%s: %v", namespace, name, err)
	}
}

func livePauseDeploymentManifest(t *testing.T, name string) string {
	t.Helper()
	manifest := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"replicas": int64(1),
			"selector": map[string]any{"matchLabels": map[string]any{
				"app": name,
			}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{
					"app": name,
				}},
				"spec": map[string]any{
					"containers": []map[string]any{livePauseContainer()},
				},
			},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal live deployment manifest: %v", err)
	}
	return string(raw)
}

func livePauseContainer() map[string]any {
	return map[string]any{
		"name":  "pause",
		"image": "registry.k8s.io/pause:3.9",
		"resources": map[string]any{
			"requests": map[string]any{
				"cpu":    "10m",
				"memory": "16Mi",
			},
		},
	}
}
