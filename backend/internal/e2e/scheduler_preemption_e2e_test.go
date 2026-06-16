//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSchedulerPreemptionEngineE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_PREEMPTION")) != "1" {
		t.Skip("TEST_LIVE_K8S_PREEMPTION=1 not set; skipping live Kubernetes preemption e2e")
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
	namespace := "nexuspaas-preempt-e2e-" + sanitizeID(time.Now().UTC().Format("150405.000000000"))
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
			Name: "victim-pod",
			Labels: map[string]string{
				cluster.LabelJobID:        "victim-live",
				"platform-go/preemptible": "true",
			},
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
		t.Fatalf("create victim pod: %v", err)
	}

	app := platform.NewApp(platform.Config{
		ServiceName:   "all",
		HTTPAddr:      ":0",
		ServiceAPIKey: "e2e-service-key",
		RequireAuth:   false,
	}, platform.WithCluster(cl))
	services.RegisterAll(app)
	createE2ERecord(t, app, schedulerQueuesResource, map[string]any{
		"id": "q-live", "name": "batch-low", "is_preemptible": true, "priority_value": 1000,
	})
	createE2ERecord(t, app, workloadJobsResource, map[string]any{
		"id": "requester-live", "job_id": "requester-live", "status": "submitted", "namespace": namespace,
		"queue_name": "interactive", "priority_value": 10000, "required_gpu": 1.0, "required_cpu": 1.0,
	})
	createE2ERecord(t, app, workloadJobsResource, map[string]any{
		"id": "victim-live", "job_id": "victim-live", "status": "running", "namespace": namespace,
		"queue_name": "batch-low", "priority_value": 1000, "preemptible": true, "required_gpu": 1.0, "required_cpu": 1.0,
		"created_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/scheduler/preemptions", strings.NewReader(`{"requester_job_id":"requester-live"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "live-preempt-"+namespace)
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preemption returned %d: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["status"] != "completed" {
		t.Fatalf("preemption data = %#v, want completed", envelope.Data)
	}
	if err := waitForPodDeletion(ctx, cl, namespace, "victim-pod"); err != nil {
		t.Fatal(err)
	}
	victim, _ := app.Store.Get(ctx, workloadJobsResource, "victim-live")
	if victim.Data["status"] != "preempted" {
		t.Fatalf("victim record = %#v, want preempted", victim.Data)
	}
	if got := len(app.Store.List(ctx, schedulerQuotaService+":preemptions:commands")); got != 0 {
		t.Fatalf("generic command records = %d, want 0", got)
	}
	if countE2EEvents(app, "JobPreempted") != 1 {
		t.Fatalf("JobPreempted events = %d, want 1", countE2EEvents(app, "JobPreempted"))
	}
}

func waitForPodDeletion(ctx context.Context, cl *cluster.Client, namespace, name string) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		pod, err := cl.Clientset().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if pod.DeletionTimestamp != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return os.ErrDeadlineExceeded
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func ensureDefaultKubeconfig(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("KUBECONFIG")) != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home directory for kubeconfig: %v", err)
	}
	kubeconfig := filepath.Join(home, ".kube", "config")
	if _, err := os.Stat(kubeconfig); err != nil {
		t.Skipf("default kubeconfig %s is unavailable: %v", kubeconfig, err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
}

func createE2ERecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}

func countE2EEvents(app *platform.App, name string) int {
	count := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			count++
		}
	}
	return count
}
