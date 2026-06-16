package storage

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var (
	storageLonghornVolumeGVR = schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "volumes",
	}
	storageLonghornSnapshotGVR = schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "snapshots",
	}
)

func TestLonghornRWXHealthWorkerPersistsAndPublishesDegradedWithoutCluster(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	Register(app)

	app.RunMaintenanceOnce(ctx, time.Second)

	record, ok := app.Store.Get(ctx, longhornRWXHealthResource, longhornRWXHealthRecord)
	if !ok {
		t.Fatal("missing Longhorn RWX health summary record")
	}
	if record.Data["degraded"] != true || record.Data["error"] != "cluster client unavailable" {
		t.Fatalf("record = %#v, want degraded cluster-unavailable summary", record.Data)
	}
	if record.Data["failed_count"] != 1 {
		t.Fatalf("failed_count = %#v, want 1", record.Data["failed_count"])
	}
	assertLonghornRWXHealthEvent(t, app, true)

	if err := runLonghornRWXHealth(ctx, app); err != nil {
		t.Fatalf("second Longhorn RWX health run: %v", err)
	}
	updated, ok := app.Store.Get(ctx, longhornRWXHealthResource, longhornRWXHealthRecord)
	if !ok || updated.Version != 2 {
		t.Fatalf("updated record = %#v ok=%v, want version 2", updated, ok)
	}
}

func TestLonghornRWXHealthWorkerPersistsHealthyClusterSummary(t *testing.T) {
	ctx := context.Background()
	namespace := "longhorn-system"
	cl := storageLonghornCluster(namespace, "vol-healthy")
	app := platform.NewApp(platform.Config{
		ServiceName:               serviceName,
		HTTPAddr:                  ":0",
		LonghornNamespace:         namespace,
		LonghornRWXHealthInterval: time.Second,
		LonghornRWXRepairCooldown: time.Minute,
		LonghornRWXSnapshotWarn:   20,
		LonghornRWXSnapshotBlock:  50,
	}, platform.WithCluster(cl))
	Register(app)

	app.RunMaintenanceOnce(ctx, time.Second)

	record, ok := app.Store.Get(ctx, longhornRWXHealthResource, longhornRWXHealthRecord)
	if !ok {
		t.Fatal("missing Longhorn RWX health summary record")
	}
	if record.Data["degraded"] != false || record.Data["volumes_checked"] != 1 || record.Data["unavailable_count"] != 0 {
		t.Fatalf("record = %#v, want healthy one-volume summary", record.Data)
	}
	results, ok := record.Data["results"].([]map[string]any)
	if !ok || len(results) != 1 || results[0]["volume"] != "vol-healthy" || results[0]["available"] != true {
		t.Fatalf("results = %#v, want available vol-healthy row", record.Data["results"])
	}
	assertLonghornRWXHealthEvent(t, app, false)
}

func TestLonghornRWXHealthWorkerRegistersOnlyForStorageService(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "identity-service", HTTPAddr: ":0"})
	Register(app)

	if got := app.MaintenanceTaskNames(); len(got) != 0 {
		t.Fatalf("storage Register on identity service installed tasks = %v, want none", got)
	}
}

func storageLonghornCluster(namespace, volume string) *cluster.Client {
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			storageLonghornVolumeGVR:   "VolumeList",
			storageLonghornSnapshotGVR: "SnapshotList",
		},
		storageLonghornVolume(namespace, volume, "rwx", "healthy"),
	)
	return cluster.NewWithDynamic(k8sfake.NewSimpleClientset(
		storageLonghornShareManagerService(namespace, volume),
		storageLonghornShareManagerEndpoints(namespace, volume),
	), dynamicClient, "proj")
}

func storageLonghornVolume(namespace, name, accessMode, robustness string) *unstructured.Unstructured {
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

func storageLonghornShareManagerService(namespace, volume string) *corev1.Service {
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

func storageLonghornShareManagerEndpoints(namespace, volume string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "share-manager-" + volume, Namespace: namespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}},
			Ports:     []corev1.EndpointPort{{Port: 2049}},
		}},
	}
}

func assertLonghornRWXHealthEvent(t *testing.T, app *platform.App, wantDegraded bool) {
	t.Helper()
	for _, event := range app.Events.Outbox() {
		if event.Name != longhornRWXHealthEvent {
			continue
		}
		if event.Source != serviceName || event.SchemaVersion != 1 {
			t.Fatalf("event metadata = %#v, want storage source schema v1", event)
		}
		if event.Data["degraded"] != wantDegraded || event.Data["id"] != longhornRWXHealthRecord {
			t.Fatalf("event data = %#v, want degraded=%v latest summary", event.Data, wantDegraded)
		}
		return
	}
	t.Fatalf("missing %s event in %#v", longhornRWXHealthEvent, app.Events.Outbox())
}
