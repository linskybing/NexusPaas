//go:build e2e

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestPlanWindowReaperE2E proves the scheduler-quota plan-window reaper, running as a
// real maintenance task against live Docker Desktop Kubernetes, evicts the job
// resources of a project whose plan window has expired and drives the workload job to
// the terminal "evicted" status through the workload-owned eviction contract — without
// scheduler-quota writing the workload job record directly.
func TestPlanWindowReaperE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_PLAN_WINDOW_REAPER")) != "1" {
		t.Skip("TEST_LIVE_K8S_PLAN_WINDOW_REAPER=1 not set; skipping live Kubernetes plan-window reaper e2e")
	}
	ensureDefaultKubeconfig(t)
	ctx := context.Background()
	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create cluster client: %v", err)
	}
	if cl == nil {
		t.Skip("no Kubernetes client available")
	}

	// The reaper resolves a project's namespaces by the "proj-<projectID>-" prefix, so
	// the live namespace must follow that convention.
	projectID := "pwe2e" + sanitizeID(time.Now().UTC().Format("150405.000000000"))
	namespace := "proj-" + projectID + "-e2e"
	jobID := "planjob-" + projectID

	if _, err := cl.Clientset().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace %s: %v", namespace, err)
	}
	t.Cleanup(func() {
		_ = cl.Clientset().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	})
	if _, err := cl.Clientset().CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "plan-job-pod",
			Labels: map[string]string{cluster.LabelJobID: jobID},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "pause",
				Image:   "registry.k8s.io/pause:3.9",
				Command: []string{"/pause"},
			}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create plan-job pod: %v", err)
	}

	app := platform.NewApp(platform.Config{
		ServiceName:           "all",
		HTTPAddr:              ":0",
		ServiceAPIKey:         "e2e-service-key",
		RequireAuth:           false,
		PlanWindowPodDeletion: true,
	}, platform.WithCluster(cl))
	services.RegisterAll(app)

	// An expired plan bound to a project with a running job.
	createE2ERecord(t, app, schedulerPlansResource, map[string]any{
		"id":          "PL-" + projectID,
		"valid_until": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})
	createE2ERecord(t, app, orgProjectsResource, map[string]any{
		"id": projectID, "plan_id": "PL-" + projectID,
	})
	createE2ERecord(t, app, workloadJobsResource, map[string]any{
		"id": jobID, "job_id": jobID, "status": "running", "namespace": namespace,
		"created_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})

	app.RunMaintenanceOnce(ctx, time.Second)

	if err := waitForPodDeletion(ctx, cl, namespace, "plan-job-pod"); err != nil {
		t.Fatalf("plan-job pod was not evicted: %v", err)
	}
	job, _ := app.Store.Get(ctx, workloadJobsResource, jobID)
	if job.Data["status"] != "evicted" {
		t.Fatalf("job record = %#v, want status evicted", job.Data)
	}
	if got, _ := job.Data["status_reason"].(string); !strings.Contains(got, "Plan") {
		t.Fatalf("status_reason = %q, want a plan-window reason", got)
	}

	// scheduler-quota must not own/duplicate workload job records.
	if got := len(app.Store.List(ctx, schedulerQuotaService+":jobs")); got != 0 {
		t.Fatalf("scheduler-quota job records = %d, want 0 (workload owns job state)", got)
	}
}
