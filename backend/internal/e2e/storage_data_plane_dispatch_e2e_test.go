//go:build e2e

package e2e

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWorkloadDataPlaneDispatchConsumesStoragePlanE2E(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	events := platform.NewEventBus()
	storageApp := newStorageDataPlaneDispatchStorageApp(store, events)
	storageServer := httptest.NewServer(storageApp)
	t.Cleanup(storageServer.Close)

	ids := seedStorageDataPlanePlanRecords(t, store)
	cl := e2eDataPlaneDispatchFakeCluster(ids)
	workloadApp := newStorageDataPlaneDispatchWorkloadApp(store, events, cl, storageServer.URL)
	createStorageDataPlaneDispatchJob(t, store, ids)

	workloadApp.RunMaintenanceOnce(ctx, time.Minute)

	record, _ := store.Get(ctx, workloadJobsResource, ids.jobID)
	if status := textE2E(record.Data["status"]); status != "queued" {
		t.Fatalf("job status = %q data=%#v, want queued after dispatch/status reconciliation", status, record.Data)
	}
	pod, err := cl.Clientset().CoreV1().Pods(ids.namespace).Get(ctx, "data-plane-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("data-plane pod was not created: %v; job=%#v", err, record.Data)
	}
	assertDataPlaneDispatchPod(t, pod, ids)
	assertDataPlanePlanBuiltEvent(t, events, ids)
}

func newStorageDataPlaneDispatchStorageApp(store *platform.Store, events *platform.EventBus) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:   storageService,
		RequireAuth:   true,
		ServiceAPIKey: e2eStorageDataPlanePlanKey,
	}, platform.WithStore(store), platform.WithEventBus(events))
	services.RegisterAll(app)
	return app
}

func newStorageDataPlaneDispatchWorkloadApp(store *platform.Store, events *platform.EventBus, cl *cluster.Client, storageURL string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:   workloadService,
		RequireAuth:   true,
		ServiceAPIKey: e2eStorageDataPlanePlanKey,
		ServiceURLs:   map[string]string{storageService: storageURL},
	}, platform.WithStore(store), platform.WithEventBus(events), platform.WithCluster(cl))
	services.RegisterAll(app)
	return app
}

func createStorageDataPlaneDispatchJob(t *testing.T, store *platform.Store, ids storageDataPlanePlanIDs) {
	t.Helper()
	createStorageDataPlaneRecord(t, store, workloadJobsResource, map[string]any{
		"id":         ids.jobID,
		"job_id":     ids.jobID,
		"project_id": ids.projectID,
		"user_id":    ids.userID,
		"status":     "submitted",
		"namespace":  ids.namespace,
		"created_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"data_plane": map[string]any{
			"scratch_profile": "local-nvme-scratch",
			"dataset_sources": []any{map[string]any{
				"storage_binding_id": ids.pvcID,
				"cache_key":          "dataset-v1",
			}},
			"checkpoint": map[string]any{
				"flush_target_profile": "cephfs-rwx-authority",
				"write_policy":         "local-first-async-flush",
				"retain_local_last_n":  2,
			},
		},
		"resources": []any{map[string]any{
			"name": "data-plane-worker",
			"kind": "Pod",
			"json_data": `{
				"apiVersion":"v1",
				"kind":"Pod",
				"metadata":{"name":"data-plane-worker"},
				"spec":{"containers":[{"name":"main","image":"busybox"}]}
			}`,
		}},
	})
}

func e2eDataPlaneDispatchFakeCluster(ids storageDataPlanePlanIDs) *cluster.Client {
	return cluster.New(fake.NewSimpleClientset(
		e2eBoundPVC(ids.sourceNamespace, ids.sourcePVC, "pv-data-plane-dispatch"),
		e2eJuiceFSPV("pv-data-plane-dispatch"),
	), "proj")
}

func assertDataPlaneDispatchPod(t *testing.T, pod *corev1.Pod, ids storageDataPlanePlanIDs) {
	t.Helper()
	assertPodPVCVolume(t, pod, "nexuspaas-scratch", "scratch-"+ids.jobID)
	assertPodPVCVolume(t, pod, "stage-dataset-v1", ids.targetPVC)
	assertPodVolumeMount(t, pod.Spec.Containers[0].VolumeMounts, "nexuspaas-scratch", "/nexuspaas/scratch")
	assertPodEnv(t, pod.Spec.Containers[0].Env, "CHECKPOINT_DIR", "/nexuspaas/scratch/checkpoints")
	assertPodEnv(t, pod.Spec.Containers[0].Env, "NEXUSPAAS_CHECKPOINT_FLUSH_TARGET", "/checkpoints/"+ids.jobID)
	assertPodEnv(t, pod.Spec.Containers[0].Env, "NEXUSPAAS_CHECKPOINT_WRITE_POLICY", "local-first-async-flush")

	if len(pod.Spec.InitContainers) != 1 || pod.Spec.InitContainers[0].Name != "nexuspaas-stage-in" {
		t.Fatalf("initContainers = %#v, want one data-plane stage-in initContainer", pod.Spec.InitContainers)
	}
	init := pod.Spec.InitContainers[0]
	assertPodVolumeMount(t, init.VolumeMounts, "nexuspaas-scratch", "/nexuspaas/scratch")
	assertPodVolumeMount(t, init.VolumeMounts, "stage-dataset-v1", "/nexuspaas/stage-in/dataset-v1")
	if len(init.Args) != 1 || !strings.Contains(init.Args[0], "cp -a '/nexuspaas/stage-in/dataset-v1/.' '/nexuspaas/scratch/datasets/dataset-v1/'") {
		t.Fatalf("stage-in args = %#v, want copy from storage plan source projection to scratch", init.Args)
	}
}

func assertPodPVCVolume(t *testing.T, pod *corev1.Pod, name, claimName string) {
	t.Helper()
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == name && volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claimName {
			return
		}
	}
	t.Fatalf("pod volumes = %#v, want %s PVC %s", pod.Spec.Volumes, name, claimName)
}

func assertPodVolumeMount(t *testing.T, mounts []corev1.VolumeMount, name, mountPath string) {
	t.Helper()
	for _, mount := range mounts {
		if mount.Name == name && mount.MountPath == mountPath {
			return
		}
	}
	t.Fatalf("volumeMounts = %#v, want %s at %s", mounts, name, mountPath)
}

func assertPodEnv(t *testing.T, env []corev1.EnvVar, name, value string) {
	t.Helper()
	for _, item := range env {
		if item.Name == name && item.Value == value {
			return
		}
	}
	t.Fatalf("env = %#v, want %s=%s", env, name, value)
}
