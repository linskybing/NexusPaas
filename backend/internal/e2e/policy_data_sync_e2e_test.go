//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPolicyDataSyncConfigMapE2E(t *testing.T) {
	if os.Getenv("TEST_LIVE_K8S_POLICY_DATA_SYNC") != "1" {
		t.Skip("set TEST_LIVE_K8S_POLICY_DATA_SYNC=1 to run live Kubernetes policy-data sync E2E")
	}
	if os.Getenv("KUBECONFIG") == "" {
		if home, err := os.UserHomeDir(); err == nil {
			_ = os.Setenv("KUBECONFIG", filepath.Join(home, ".kube", "config"))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	clusterClient, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("build live kubernetes client: %v", err)
	}
	if clusterClient == nil || clusterClient.Clientset() == nil {
		t.Fatal("live kubernetes client is nil; set KUBECONFIG or run inside a cluster")
	}

	runID := strings.ToLower(strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", ""))
	projectID := "policydata" + runID
	namespace := "proj-" + projectID + "-alice"
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: namespace,
		Labels: map[string]string{
			"app.kubernetes.io/part-of":    "platform",
			"app.kubernetes.io/component":  "policy-data-e2e",
			"app.kubernetes.io/managed-by": "platform-backend-e2e",
		},
	}}
	if _, err := clusterClient.Clientset().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create live namespace %s: %v", namespace, err)
	}
	defer func() {
		_ = clusterClient.Clientset().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	}()

	app := platform.NewApp(platform.Config{
		ServiceName:         authorizationPolicyService,
		ImageCheckEnabled:   true,
		MaintenanceInterval: time.Second,
	}, platform.WithCluster(clusterClient))
	services.RegisterAll(app)
	createPolicyDataE2ERecord(t, app, "authorization-policy-service:policy_projects", projectID, map[string]any{
		"id":                      projectID,
		"plan_id":                 "plan-" + runID,
		"max_job_runtime_seconds": 2400,
	})
	createPolicyDataE2ERecord(t, app, "authorization-policy-service:policy_plans", "plan-"+runID, map[string]any{
		"id":        "plan-" + runID,
		"gpu_limit": 3,
	})
	createPolicyDataE2ERecord(t, app, "authorization-policy-service:policy_image_allow_lists", projectID+":image", map[string]any{
		"id":              projectID + ":image",
		"project_id":      projectID,
		"image_reference": "registry.example/platform/runtime:latest",
		"enabled":         true,
	})

	app.RunMaintenanceOnce(ctx, time.Second)

	got, err := clusterClient.Clientset().CoreV1().ConfigMaps(namespace).Get(ctx, cluster.PolicyDataConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get live policy data configmap: %v", err)
	}
	if got.Labels["app.kubernetes.io/component"] != "policy-data" {
		t.Fatalf("configmap labels = %#v, want policy-data component", got.Labels)
	}
	want := map[string]string{
		"maxJobRuntimeSeconds": "2400",
		"gpuLimit":             "3",
		"imageCheckEnabled":    "true",
		"timeAllowed":          "true",
		"gpuNamespaceUsage":    "0",
		"allowedProxyImages":   ",registry.example/platform/runtime:latest,",
	}
	for key, value := range want {
		if got.Data[key] != value {
			t.Fatalf("%s = %q, want %q in %#v", key, got.Data[key], value, got.Data)
		}
	}
}

func createPolicyDataE2ERecord(t *testing.T, app *platform.App, resource, id string, data map[string]any) {
	t.Helper()
	data["id"] = id
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatalf("create %s/%s: %v", resource, id, err)
	}
}
