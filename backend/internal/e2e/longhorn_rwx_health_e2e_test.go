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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var (
	longhornRWXE2EVolumeGVR = schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "volumes",
	}
	longhornRWXE2ESnapshotGVR = schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "snapshots",
	}
)

func TestLonghornRWXHealthWorkerE2E(t *testing.T) {
	ctx := context.Background()
	namespace := "longhorn-system"
	app := newLonghornRWXStorageApp(newE2ELonghornCluster(namespace, "vol-e2e"), namespace)

	app.RunMaintenanceOnce(ctx, time.Second)

	record := requireLonghornRWXHealthRecord(t, app)
	if record.Data["degraded"] != false || record.Data["volumes_checked"] != 1 || record.Data["unavailable_count"] != 0 {
		t.Fatalf("Longhorn RWX health record = %#v, want healthy one-volume summary", record.Data)
	}
	results, ok := record.Data["results"].([]map[string]any)
	if !ok || len(results) != 1 || results[0]["volume"] != "vol-e2e" || results[0]["available"] != true {
		t.Fatalf("Longhorn RWX results = %#v, want available vol-e2e row", record.Data["results"])
	}
	requireLonghornRWXHealthEvent(t, app, false)
}

func TestLonghornRWXHealthWorkerLiveK8sSmokeE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_LONGHORN_RWX_SMOKE")) != "1" {
		t.Skip("TEST_LIVE_K8S_LONGHORN_RWX_SMOKE=1 not set; skipping live Longhorn RWX smoke e2e")
	}
	ensureDefaultKubeconfigNoSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create cluster client: %v", err)
	}
	if cl == nil || cl.DynamicClient() == nil {
		t.Fatal("live Kubernetes dynamic client is unavailable")
	}
	namespace := envDefault("LONGHORN_NAMESPACE", "longhorn-system")
	_, probeErr := cl.DynamicClient().Resource(longhornRWXE2EVolumeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: 1})

	app := newLonghornRWXStorageApp(cl, namespace)
	app.RunMaintenanceOnce(ctx, time.Second)

	record := requireLonghornRWXHealthRecord(t, app)
	if probeErr != nil {
		if record.Data["degraded"] != true || record.Data["error"] == "" {
			t.Fatalf("probe error %v produced record %#v, want degraded/error summary", probeErr, record.Data)
		}
		if record.Data["volumes_checked"] == 0 && record.Data["degraded"] != true {
			t.Fatalf("probe error %v produced healthy empty summary: %#v", probeErr, record.Data)
		}
		return
	}
	if record.Data["degraded"] == true {
		t.Fatalf("Longhorn volumes API was reachable but worker degraded: %#v", record.Data)
	}
	requireLonghornRWXHealthEvent(t, app, false)
}

func TestLonghornRWXHealthWorkerLiveLonghornE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_LONGHORN_RWX")) != "1" {
		t.Skip("TEST_LIVE_LONGHORN_RWX=1 not set; skipping live Longhorn RWX e2e")
	}
	ensureDefaultKubeconfigNoSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create cluster client: %v", err)
	}
	if cl == nil || cl.DynamicClient() == nil {
		t.Fatal("live Kubernetes dynamic client is unavailable")
	}
	namespace := envDefault("LONGHORN_NAMESPACE", "longhorn-system")
	if _, err := cl.DynamicClient().Resource(longhornRWXE2EVolumeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		t.Fatalf("Longhorn volumes API must be reachable for TEST_LIVE_LONGHORN_RWX=1: %v", err)
	}

	app := newLonghornRWXStorageApp(cl, namespace)
	app.RunMaintenanceOnce(ctx, time.Second)

	record := requireLonghornRWXHealthRecord(t, app)
	if record.Data["degraded"] == true || record.Data["error"] != "" {
		t.Fatalf("live Longhorn worker record = %#v, want non-degraded accessible Longhorn summary", record.Data)
	}
	requireLonghornRWXHealthEvent(t, app, false)
}

func newLonghornRWXStorageApp(cl *cluster.Client, namespace string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:               storageService,
		HTTPAddr:                  ":0",
		RequireAuth:               false,
		LonghornNamespace:         namespace,
		LonghornRWXHealthInterval: time.Second,
		LonghornRWXAutoRepair:     false,
		LonghornRWXRepairCooldown: time.Minute,
		LonghornRWXSnapshotWarn:   20,
		LonghornRWXSnapshotBlock:  50,
	}, platform.WithCluster(cl))
	services.RegisterAll(app)
	return app
}

func newE2ELonghornCluster(namespace, volume string) *cluster.Client {
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			longhornRWXE2EVolumeGVR:   "VolumeList",
			longhornRWXE2ESnapshotGVR: "SnapshotList",
		},
		e2eLonghornVolume(namespace, volume, "rwx", "healthy"),
	)
	return cluster.NewWithDynamic(k8sfake.NewSimpleClientset(
		e2eLonghornShareManagerService(namespace, volume),
		e2eLonghornShareManagerEndpoints(namespace, volume),
	), dynamicClient, "proj")
}

func e2eLonghornVolume(namespace, name, accessMode, robustness string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "longhorn.io/v1beta2",
		"kind":       "Volume",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{"accessMode": accessMode},
		"status": map[string]any{
			"robustness": robustness,
		},
	}}
}

func e2eLonghornShareManagerService(namespace, volume string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "share-manager-" + volume,
			Namespace: namespace,
			Labels:    map[string]string{"longhorn.io/share-manager": volume},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Ports:     []corev1.ServicePort{{Port: 2049}},
		},
	}
}

func e2eLonghornShareManagerEndpoints(namespace, volume string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "share-manager-" + volume, Namespace: namespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}},
			Ports:     []corev1.EndpointPort{{Port: 2049}},
		}},
	}
}

func requireLonghornRWXHealthRecord(t *testing.T, app *platform.App) platformRecord {
	t.Helper()
	record, ok := app.Store.Get(context.Background(), "storage-service:longhorn_rwx_health", "latest")
	if !ok {
		t.Fatal("missing storage-service:longhorn_rwx_health/latest")
	}
	return platformRecord{Data: record.Data}
}

type platformRecord struct {
	Data map[string]any
}

func requireLonghornRWXHealthEvent(t *testing.T, app *platform.App, wantDegraded bool) {
	t.Helper()
	for _, event := range app.Events.Outbox() {
		if event.Name != "LonghornRWXHealthChecked" {
			continue
		}
		if event.Source != storageService || event.SchemaVersion != 1 {
			t.Fatalf("event = %#v, want storage source schema v1", event)
		}
		if event.Data["degraded"] != wantDegraded || event.Data["id"] != "latest" {
			t.Fatalf("event data = %#v, want degraded=%v latest", event.Data, wantDegraded)
		}
		return
	}
	t.Fatalf("missing LonghornRWXHealthChecked event in %#v", app.Events.Outbox())
}

func ensureDefaultKubeconfigNoSkip(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("KUBECONFIG")) != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot resolve home directory for kubeconfig: %v", err)
	}
	kubeconfig := filepath.Join(home, ".kube", "config")
	if _, err := os.Stat(kubeconfig); err != nil {
		t.Fatalf("default kubeconfig %s is unavailable: %v", kubeconfig, err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
}
