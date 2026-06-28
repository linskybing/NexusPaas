//go:build e2e

package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	workloadDataPlaneStageInKindRuntimeEnv = "TEST_LIVE_WORKLOAD_DATAPLANE_STAGEIN_KIND_RUNTIME"
	workloadDataPlaneStageInImage          = "busybox:1.36"
	workloadDataPlaneStageInContent        = "workload-dataplane-stagein-kind"
)

func TestWorkloadDataPlaneStageInKindRuntimeE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(workloadDataPlaneStageInKindRuntimeEnv)) != "1" {
		t.Skip("set " + workloadDataPlaneStageInKindRuntimeEnv + "=1 to run live workload DataPlane stage-in kind runtime e2e")
	}
	requireWorkloadDataPlaneLiveKubeconfig(t)

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
	ensureWorkloadDataPlanePriorityClass(t, ctx, cl)

	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	namespace := "wl-dp-stagein-" + suffix
	jobID := "job-" + suffix
	podName := "stagein-worker"
	stagePVC := "stage-dataset"
	scratchPVC := "scratch-" + suffix
	store := platform.NewStore()

	if err := cl.EnsureNamespace(ctx, namespace); err != nil {
		t.Fatalf("create namespace %s: %v", namespace, err)
	}
	t.Cleanup(func() {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer deleteCancel()
		_ = cl.Clientset().CoreV1().Namespaces().Delete(deleteCtx, namespace, metav1.DeleteOptions{})
	})

	createWorkloadDataPlaneStageInPVC(t, ctx, cl, namespace, stagePVC)
	runWorkloadDataPlanePVCPod(t, ctx, cl, namespace, "seed-"+suffix, stagePVC, fmt.Sprintf(`set -eu
printf %%s %q > /pvc/hello.txt`, workloadDataPlaneStageInContent))

	createWorkloadDataPlaneStageInJob(t, store, jobID, namespace, podName)
	resolver := workloadDataPlaneStageInPlanResolver(t, jobID, namespace, stagePVC, scratchPVC)
	if err := dispatchSubmittedWorkloadsWithStorageClients(ctx, cl, store, nil, resolver, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertWorkloadDataPlaneJobRunning(t, store, jobID)

	assertWorkloadDataPlaneScratchPVC(t, ctx, cl, namespace, scratchPVC)
	pod := waitWorkloadDataPlanePodSucceeded(t, ctx, cl, namespace, podName)
	assertWorkloadDataPlaneStageInPod(t, pod, scratchPVC, stagePVC)
	runWorkloadDataPlanePVCPod(t, ctx, cl, namespace, "verify-"+suffix, scratchPVC, fmt.Sprintf(`set -eu
got="$(cat /pvc/datasets/dataset-v1/hello.txt)"
[ "$got" = %q ]
marker="$(cat /pvc/checkpoints/stagein.txt)"
[ "$marker" = %q ]`, workloadDataPlaneStageInContent, workloadDataPlaneStageInContent))
}

func requireWorkloadDataPlaneLiveKubeconfig(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("KUBECONFIG")) == "" {
		t.Fatal("KUBECONFIG must point at a live kind cluster")
	}
}

func ensureWorkloadDataPlanePriorityClass(t *testing.T, ctx context.Context, cl *cluster.Client) {
	t.Helper()
	result := cl.EnsurePriorityClassDefinition(ctx, cluster.PriorityClassDefinition{
		Name:             "platform-batch-low",
		Value:            100,
		PreemptionPolicy: corev1.PreemptLowerPriority,
		Description:      "NexusPaas workload DataPlane kind runtime test priority class",
	})
	switch result.Action {
	case cluster.PriorityClassActionCreated, cluster.PriorityClassActionUpdated, cluster.PriorityClassActionRecreated, cluster.PriorityClassActionAdopted, cluster.PriorityClassActionUnchanged:
		return
	default:
		t.Fatalf("ensure PriorityClass platform-batch-low: %#v", result)
	}
}

func createWorkloadDataPlaneStageInPVC(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) {
	t.Helper()
	_, err := cl.Clientset().CoreV1().PersistentVolumeClaims(namespace).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"nexuspaas.io/e2e": "workload-dataplane-stagein-kind"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("64Mi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create PVC %s/%s: %v", namespace, name, err)
	}
}

func runWorkloadDataPlanePVCPod(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name, pvcName, script string) *corev1.Pod {
	t.Helper()
	_, err := cl.Clientset().CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"nexuspaas.io/e2e": "workload-dataplane-stagein-kind"},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "pvc-helper",
				Image:   workloadDataPlaneStageInImage,
				Command: []string{"/bin/sh", "-c"},
				Args:    []string{script},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "pvc",
					MountPath: "/pvc",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "pvc",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				}},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create helper Pod %s/%s: %v", namespace, name, err)
	}
	return waitWorkloadDataPlanePodSucceeded(t, ctx, cl, namespace, name)
}

func createWorkloadDataPlaneStageInJob(t *testing.T, store *platform.Store, jobID, namespace, podName string) {
	t.Helper()
	script := fmt.Sprintf(`set -eu
[ "${CHECKPOINT_DIR:-}" = "/nexuspaas/scratch/checkpoints" ]
[ "${NEXUSPAAS_CHECKPOINT_FLUSH_TARGET:-}" = "/checkpoints/%s" ]
[ "${NEXUSPAAS_CHECKPOINT_WRITE_POLICY:-}" = "local-first-async-flush" ]
got="$(cat /nexuspaas/scratch/datasets/dataset-v1/hello.txt)"
[ "$got" = %q ]
mkdir -p "$CHECKPOINT_DIR"
printf %%s "$got" > "$CHECKPOINT_DIR/stagein.txt"`, jobID, workloadDataPlaneStageInContent)
	pod := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": podName,
		},
		"spec": map[string]any{
			"restartPolicy": "Never",
			"containers": []any{map[string]any{
				"name":    "main",
				"image":   workloadDataPlaneStageInImage,
				"command": []any{"/bin/sh", "-c"},
				"args":    []any{script},
			}},
		},
	}
	raw, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal stage-in pod: %v", err)
	}
	_, err = store.Create(context.Background(), jobsResource, map[string]any{
		"id":         jobID,
		"job_id":     jobID,
		"project_id": "project-" + jobID,
		"user_id":    "user-" + jobID,
		"status":     "submitted",
		"namespace":  namespace,
		"created_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"data_plane": map[string]any{
			"scratch_profile": "kind-default-scratch",
			"dataset_sources": []any{map[string]any{
				"storage_binding_id": "datasets",
				"cache_key":          "dataset-v1",
			}},
			"checkpoint": map[string]any{
				"flush_target_profile": "kind-default-authority",
				"write_policy":         "local-first-async-flush",
			},
		},
		"resources": []any{map[string]any{
			"name":      podName,
			"kind":      "Pod",
			"json_data": string(raw),
		}},
	})
	if err != nil {
		t.Fatalf("seed workload job: %v", err)
	}
}

func workloadDataPlaneStageInPlanResolver(t *testing.T, jobID, namespace, stagePVC, scratchPVC string) dataPlanePlanResolver {
	t.Helper()
	return func(_ context.Context, projectID string, req dataPlanePlanRequest) (dataPlanePlan, error) {
		if req.JobID != jobID || req.Namespace != namespace || projectID == "" {
			t.Fatalf("unexpected DataPlane request project=%q req=%#v", projectID, req)
		}
		return dataPlanePlan{
			ProjectID: projectID,
			JobID:     jobID,
			Namespace: namespace,
			Scratch: dataPlaneScratchPlan{
				ProfileID:        "kind-default-scratch",
				VolumeName:       "nexuspaas-scratch",
				ClaimName:        scratchPVC,
				MountPath:        "/nexuspaas/scratch",
				StorageClassName: "",
				AccessMode:       "rwo",
			},
			StageInOperations: []dataPlaneStageInOperation{{
				StorageBindingID: "datasets",
				CacheKey:         "dataset-v1",
				TargetPVC:        stagePVC,
				VolumeName:       "stage-dataset-v1",
				SourcePath:       "/nexuspaas/stage-in/dataset-v1",
				ScratchPath:      "/nexuspaas/scratch/datasets/dataset-v1",
			}},
			Checkpoint: dataPlaneCheckpointPlan{
				FlushTargetProfileID: "kind-default-authority",
				WritePolicy:          "local-first-async-flush",
				LocalPath:            "/nexuspaas/scratch/checkpoints",
				FlushTargetPath:      "/checkpoints/" + jobID,
			},
		}, nil
	}
}

func assertWorkloadDataPlaneJobRunning(t *testing.T, store *platform.Store, jobID string) {
	t.Helper()
	record, ok := store.Get(context.Background(), jobsResource, jobID)
	if !ok {
		t.Fatalf("workload job %s was not found after dispatch", jobID)
	}
	if got := currentJobStatus(record.Data); got != jobStatusRunning {
		t.Fatalf("workload job status = %q after dispatch; record=%#v", got, record.Data)
	}
}

func assertWorkloadDataPlaneScratchPVC(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) {
	t.Helper()
	pvc, err := cl.Clientset().CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("dispatcher did not create scratch PVC %s/%s: %v", namespace, name, err)
	}
	if !workloadDataPlaneHasAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteOnce) {
		t.Fatalf("scratch PVC accessModes = %#v, want ReadWriteOnce", pvc.Spec.AccessModes)
	}
	if got := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; got.String() != "1Gi" {
		t.Fatalf("scratch PVC storage request = %s, want default 1Gi", got.String())
	}
}

func assertWorkloadDataPlaneStageInPod(t *testing.T, pod *corev1.Pod, scratchPVC, stagePVC string) {
	t.Helper()
	if len(pod.Spec.InitContainers) != 1 || pod.Spec.InitContainers[0].Name != dataPlaneStageInContainerName {
		t.Fatalf("initContainers = %#v, want one DataPlane stage-in init container", pod.Spec.InitContainers)
	}
	if !workloadDataPlanePodHasPVCVolume(pod, "nexuspaas-scratch", scratchPVC) {
		t.Fatalf("Pod volumes = %#v, want scratch PVC %s", pod.Spec.Volumes, scratchPVC)
	}
	if !workloadDataPlanePodHasPVCVolume(pod, "stage-dataset-v1", stagePVC) {
		t.Fatalf("Pod volumes = %#v, want stage PVC %s", pod.Spec.Volumes, stagePVC)
	}
	if !workloadDataPlaneContainerHasMount(pod.Spec.InitContainers[0].VolumeMounts, "stage-dataset-v1", "/nexuspaas/stage-in/dataset-v1") {
		t.Fatalf("stage-in mounts = %#v, want stage source mount", pod.Spec.InitContainers[0].VolumeMounts)
	}
}

func waitWorkloadDataPlanePodSucceeded(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) *corev1.Pod {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		pod, err := cl.Clientset().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			if pod.Status.Phase == corev1.PodSucceeded && pod.Spec.NodeName != "" {
				return pod
			}
			if pod.Status.Phase == corev1.PodFailed {
				logWorkloadDataPlanePodDiagnostics(t, ctx, cl, namespace, name)
				t.Fatalf("Pod %s/%s failed", namespace, name)
			}
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get Pod %s/%s: %v", namespace, name, err)
		}
		if err := ctx.Err(); err != nil {
			logWorkloadDataPlanePodDiagnostics(t, context.Background(), cl, namespace, name)
			t.Fatalf("wait for Pod %s/%s succeeded: %v", namespace, name, err)
		}
		if time.Now().After(deadline) {
			logWorkloadDataPlanePodDiagnostics(t, ctx, cl, namespace, name)
			t.Fatalf("Pod %s/%s did not succeed before deadline", namespace, name)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func logWorkloadDataPlanePodDiagnostics(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) {
	t.Helper()
	pod, err := cl.Clientset().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		t.Logf("Pod %s/%s phase=%s node=%q reason=%q message=%q init=%#v statuses=%#v", namespace, name, pod.Status.Phase, pod.Spec.NodeName, pod.Status.Reason, pod.Status.Message, pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses)
	} else if !apierrors.IsNotFound(err) {
		t.Logf("get Pod %s/%s diagnostics: %v", namespace, name, err)
	}
	if events, err := cl.Clientset().CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + name}); err == nil {
		for _, event := range events.Items {
			t.Logf("Pod event %s/%s: %s %s", namespace, name, event.Reason, event.Message)
		}
	}
	req := cl.Clientset().CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{})
	stream, err := req.Stream(ctx)
	if err != nil {
		t.Logf("get Pod %s/%s logs: %v", namespace, name, err)
		return
	}
	defer stream.Close()
	logs, err := io.ReadAll(io.LimitReader(stream, 64<<10))
	if err != nil {
		t.Logf("read Pod %s/%s logs: %v", namespace, name, err)
		return
	}
	if len(logs) > 0 {
		t.Logf("Pod %s/%s logs:\n%s", namespace, name, strings.TrimSpace(string(logs)))
	}
}

func workloadDataPlanePodHasPVCVolume(pod *corev1.Pod, name, claim string) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == name && volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claim {
			return true
		}
	}
	return false
}

func workloadDataPlaneContainerHasMount(mounts []corev1.VolumeMount, name, path string) bool {
	for _, mount := range mounts {
		if mount.Name == name && mount.MountPath == path {
			return true
		}
	}
	return false
}

func workloadDataPlaneHasAccessMode(modes []corev1.PersistentVolumeAccessMode, want corev1.PersistentVolumeAccessMode) bool {
	for _, mode := range modes {
		if mode == want {
			return true
		}
	}
	return false
}
