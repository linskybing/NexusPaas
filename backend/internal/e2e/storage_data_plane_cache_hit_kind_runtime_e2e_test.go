//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	storageDataPlaneCacheHitKindRuntimeEnv = "TEST_LIVE_STORAGE_DATAPLANE_KIND_RUNTIME"
	storageDataPlaneRuntimeWorkerPod       = "data-plane-runtime-worker"
	e2eStorageCacheBindingsResource        = "storage-service:cache_bindings"
)

func TestStorageDataPlaneCacheHitKindRuntimeE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(storageDataPlaneCacheHitKindRuntimeEnv)) != "1" {
		t.Skip("set " + storageDataPlaneCacheHitKindRuntimeEnv + "=1 to run live storage DataPlane cache-hit kind runtime e2e")
	}
	requireLiveKubeconfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil {
		t.Fatal("live Kubernetes client is unavailable")
	}
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}
	ensureLiveStorageDataPlanePriorityClass(t, ctx, cl)

	store := platform.NewStore()
	events := platform.NewEventBus()
	storageApp := newStorageDataPlaneDispatchStorageApp(store, events)
	storageServer := httptest.NewServer(storageApp)
	t.Cleanup(storageServer.Close)
	overrideStorageDataPlaneScratchProfileForKind(t, store)

	ids := seedLiveStorageDataPlaneKindAdmissionRecords(t, store)
	seedStorageDataPlaneCacheHitRecord(t, store, ids)
	cleanupLiveStorageDataPlaneObjects(t, cl, ids)
	if err := cl.EnsureNamespace(ctx, ids.namespace); err != nil {
		t.Fatalf("create runtime namespace %s: %v", ids.namespace, err)
	}
	scratchPVC := "scratch-" + ids.jobID

	workloadApp := newStorageDataPlaneDispatchWorkloadApp(store, events, cl, storageServer.URL)
	createStorageDataPlaneCacheHitRuntimeJob(t, store, ids)

	pod := waitForStorageDataPlaneCacheHitRuntimeDispatch(t, ctx, workloadApp, cl, store, ids)
	assertStorageDataPlaneCacheHitRuntimePod(t, pod, ids)
	assertStorageDataPlaneRuntimeScratchPVC(t, ctx, cl, ids, scratchPVC)
	waitFastTransferMoverExecutionPodSucceeded(t, ctx, cl, ids.namespace, storageDataPlaneRuntimeWorkerPod)
	runFastTransferMoverExecutionPVCPod(t, ctx, cl, ids.namespace, "verify-"+ids.jobID, scratchPVC, `set -eu
grep -q cache-hit-runtime /pvc/checkpoints/runtime.txt`)

	assertDataPlanePlanBuiltEvent(t, events, ids)
	assertStorageDataPlaneCacheHitSkippedTargetPVC(t, ctx, cl, ids)
}

func overrideStorageDataPlaneScratchProfileForKind(t *testing.T, store *platform.Store) {
	t.Helper()
	if _, ok := store.Update(context.Background(), e2eStorageProfilesResource, "local-nvme-scratch", map[string]any{
		"storage_class_name": "",
	}); !ok {
		t.Fatal("local-nvme-scratch storage profile was not seeded")
	}
}

func seedStorageDataPlaneCacheHitRecord(t *testing.T, store *platform.Store, ids storageDataPlanePlanIDs) {
	t.Helper()
	createStorageDataPlaneRecord(t, store, e2eStorageCacheBindingsResource, map[string]any{
		"id":                 "cache-" + ids.projectID + "-" + ids.pvcID + "-dataset-v1",
		"project_id":         ids.projectID,
		"storage_binding_id": ids.pvcID,
		"cache_key":          "dataset-v1",
		"scratch_profile":    "local-nvme-scratch",
		"last_staged_at":     time.Now().UTC().Format(time.RFC3339),
	})
}

func createStorageDataPlaneCacheHitRuntimeJob(t *testing.T, store *platform.Store, ids storageDataPlanePlanIDs) {
	t.Helper()
	script := fmt.Sprintf(`set -eu
[ "${CHECKPOINT_DIR:-}" = "/nexuspaas/scratch/checkpoints" ]
[ "${NEXUSPAAS_CHECKPOINT_FLUSH_TARGET:-}" = "/checkpoints/%s" ]
[ "${NEXUSPAAS_CHECKPOINT_WRITE_POLICY:-}" = "local-first-async-flush" ]
mkdir -p "$CHECKPOINT_DIR"
printf cache-hit-runtime > "$CHECKPOINT_DIR/runtime.txt"
test -s "$CHECKPOINT_DIR/runtime.txt"`, ids.jobID)
	pod := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": storageDataPlaneRuntimeWorkerPod,
		},
		"spec": map[string]any{
			"restartPolicy": "Never",
			"containers": []any{map[string]any{
				"name":    "main",
				"image":   fastTransferMoverExecutionHelperImage,
				"command": []any{"/bin/sh", "-c"},
				"args":    []any{script},
			}},
		},
	}
	raw, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal data-plane runtime pod: %v", err)
	}
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
			"name":      storageDataPlaneRuntimeWorkerPod,
			"kind":      "Pod",
			"json_data": string(raw),
		}},
	})
}

func waitForStorageDataPlaneCacheHitRuntimeDispatch(
	t *testing.T,
	ctx context.Context,
	workloadApp *platform.App,
	cl *cluster.Client,
	store *platform.Store,
	ids storageDataPlanePlanIDs,
) *corev1.Pod {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		workloadApp.RunMaintenanceOnce(ctx, 200*time.Millisecond)
		pod, err := cl.Clientset().CoreV1().Pods(ids.namespace).Get(ctx, storageDataPlaneRuntimeWorkerPod, metav1.GetOptions{})
		if err == nil {
			return pod
		}
		if !apierrors.IsNotFound(err) {
			t.Fatalf("get DataPlane runtime Pod %s/%s: %v", ids.namespace, storageDataPlaneRuntimeWorkerPod, err)
		}
		if err := ctx.Err(); err != nil {
			record, _ := store.Get(context.Background(), workloadJobsResource, ids.jobID)
			t.Fatalf("wait for DataPlane runtime dispatch: %v job=%#v", err, record.Data)
		}
		if time.Now().After(deadline) {
			record, _ := store.Get(ctx, workloadJobsResource, ids.jobID)
			t.Fatalf("DataPlane runtime Pod not visible before deadline: %v job=%#v", err, record.Data)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func assertStorageDataPlaneCacheHitRuntimePod(t *testing.T, pod *corev1.Pod, ids storageDataPlanePlanIDs) {
	t.Helper()
	assertPodPVCVolume(t, pod, "nexuspaas-scratch", "scratch-"+ids.jobID)
	assertPodVolumeMount(t, pod.Spec.Containers[0].VolumeMounts, "nexuspaas-scratch", "/nexuspaas/scratch")
	assertPodEnv(t, pod.Spec.Containers[0].Env, "CHECKPOINT_DIR", "/nexuspaas/scratch/checkpoints")
	assertPodEnv(t, pod.Spec.Containers[0].Env, "NEXUSPAAS_CHECKPOINT_FLUSH_TARGET", "/checkpoints/"+ids.jobID)
	assertPodEnv(t, pod.Spec.Containers[0].Env, "NEXUSPAAS_CHECKPOINT_WRITE_POLICY", "local-first-async-flush")
	if len(pod.Spec.InitContainers) != 0 {
		t.Fatalf("initContainers = %#v, want none for cache-hit DataPlane runtime path", pod.Spec.InitContainers)
	}
}

func assertStorageDataPlaneRuntimeScratchPVC(t *testing.T, ctx context.Context, cl *cluster.Client, ids storageDataPlanePlanIDs, scratchPVC string) {
	t.Helper()
	pvc, err := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.namespace).Get(ctx, scratchPVC, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("dispatcher did not create scratch PVC %s/%s: %v", ids.namespace, scratchPVC, err)
	}
	if !hasStorageDataPlaneRuntimeAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteOnce) {
		t.Fatalf("scratch PVC accessModes = %#v, want ReadWriteOnce", pvc.Spec.AccessModes)
	}
	if got := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; got.String() != "1Gi" {
		t.Fatalf("scratch PVC storage request = %s, want default 1Gi", got.String())
	}
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == "local-nvme-scratch" {
		t.Fatalf("scratch PVC storageClassName = %q; kind test should use default StorageClass override", *pvc.Spec.StorageClassName)
	}
}

func assertStorageDataPlaneCacheHitSkippedTargetPVC(t *testing.T, ctx context.Context, cl *cluster.Client, ids storageDataPlanePlanIDs) {
	t.Helper()
	_, err := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.namespace).Get(ctx, ids.targetPVC, metav1.GetOptions{})
	if err == nil {
		t.Fatalf("target PVC %s/%s exists; cache-hit runtime path should not materialize stage source PVC", ids.namespace, ids.targetPVC)
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("get target PVC %s/%s: %v", ids.namespace, ids.targetPVC, err)
	}
}

func hasStorageDataPlaneRuntimeAccessMode(modes []corev1.PersistentVolumeAccessMode, want corev1.PersistentVolumeAccessMode) bool {
	for _, mode := range modes {
		if mode == want {
			return true
		}
	}
	return false
}
