package workload

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPrepareDataPlaneDispatchResourcesInjectsScratchStageInAndCheckpoint(t *testing.T) {
	resources := []dispatchResource{{
		Name: "trainer",
		Kind: "Pod",
		Raw: []byte(`{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"trainer"},
			"spec":{"containers":[{"name":"main","image":"busybox"}]}
		}`),
	}}
	plan := dataPlanePlan{
		Scratch: dataPlaneScratchPlan{
			VolumeName: "nexuspaas-scratch",
			ClaimName:  "scratch-job-train-1",
			MountPath:  "/nexuspaas/scratch",
		},
		StageInOperations: []dataPlaneStageInOperation{{
			StorageBindingID: "pvc-data",
			TargetPVC:        "project-data-claim",
			VolumeName:       "stage-dataset-v1",
			SourcePath:       "/nexuspaas/stage-in/dataset-v1",
			ScratchPath:      "/nexuspaas/scratch/datasets/dataset-v1",
		}},
		Checkpoint: dataPlaneCheckpointPlan{
			WritePolicy:     "local-first-async-flush",
			LocalPath:       "/nexuspaas/scratch/checkpoints",
			FlushTargetPath: "/checkpoints/job-train-1",
		},
	}

	updated, err := prepareDataPlaneDispatchResources(plan, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	volumes := namedMaps(t, obj, "spec", "volumes")
	assertNamedPVCVolume(t, volumes, "nexuspaas-scratch", "scratch-job-train-1")
	assertNamedPVCVolume(t, volumes, "stage-dataset-v1", "project-data-claim")

	containers := namedMaps(t, obj, "spec", "containers")
	main := containers["main"]
	assertNamedVolumeMount(t, main, "nexuspaas-scratch", "/nexuspaas/scratch")
	assertNamedEnv(t, main, checkpointDirEnv, "/nexuspaas/scratch/checkpoints")
	assertNamedEnv(t, main, checkpointFlushTargetEnv, "/checkpoints/job-train-1")
	assertNamedEnv(t, main, checkpointWritePolicyEnv, "local-first-async-flush")

	initContainers := namedMaps(t, obj, "spec", "initContainers")
	init := initContainers[dataPlaneStageInContainerName]
	if init == nil {
		t.Fatalf("initContainers = %#v, want stage-in init container", initContainers)
	}
	assertNamedVolumeMount(t, init, "nexuspaas-scratch", "/nexuspaas/scratch")
	assertNamedVolumeMount(t, init, "stage-dataset-v1", "/nexuspaas/stage-in/dataset-v1")
	args, _ := init["args"].([]any)
	if len(args) != 1 || !strings.Contains(args[0].(string), "cp -a '/nexuspaas/stage-in/dataset-v1/.' '/nexuspaas/scratch/datasets/dataset-v1/'") {
		t.Fatalf("stage-in args = %#v, want copy from authority projection to scratch", args)
	}
}

func TestResolveDispatchDataPlanePlanNoopWhenAbsent(t *testing.T) {
	called := false
	client := func(context.Context, string, dataPlanePlanRequest) (dataPlanePlan, error) {
		called = true
		return dataPlanePlan{}, nil
	}
	plan, err := resolveDispatchDataPlanePlan(context.Background(), client, map[string]any{
		"id":         "job-train-1",
		"project_id": "P1",
		"user_id":    "U1",
	}, "proj-p1")
	if err != nil {
		t.Fatal(err)
	}
	if called || !dataPlanePlanEmpty(plan) {
		t.Fatalf("called=%v plan=%#v, want no-op without data_plane", called, plan)
	}
}

func TestResolveDispatchDataPlanePlanBuildsRequestFromSubmissionPayload(t *testing.T) {
	var gotProject string
	var got dataPlanePlanRequest
	client := func(_ context.Context, projectID string, req dataPlanePlanRequest) (dataPlanePlan, error) {
		gotProject = projectID
		got = req
		return dataPlanePlan{Scratch: dataPlaneScratchPlan{ClaimName: "scratch-job-train-1", MountPath: "/nexuspaas/scratch"}}, nil
	}
	_, err := resolveDispatchDataPlanePlan(context.Background(), client, map[string]any{
		"id":         "job-train-1",
		"project_id": "P1",
		"user_id":    "U1",
		"submission_payload": map[string]any{
			"data_plane": map[string]any{
				"scratch_profile": "local-nvme-scratch",
				"dataset_sources": []any{map[string]any{
					"storage_binding_id": "pvc-data",
					"cache_key":          "dataset-v1",
				}},
				"checkpoint": map[string]any{"retain_local_last_n": 2},
			},
		},
	}, "proj-p1")
	if err != nil {
		t.Fatal(err)
	}
	if gotProject != "P1" || got.JobID != "job-train-1" || got.UserID != "U1" || got.Namespace != "proj-p1" {
		t.Fatalf("request identity project=%q req=%#v", gotProject, got)
	}
	if got.ScratchProfile != "local-nvme-scratch" || len(got.DatasetSources) != 1 ||
		got.DatasetSources[0].StorageBindingID != "pvc-data" || got.DatasetSources[0].CacheKey != "dataset-v1" {
		t.Fatalf("request data_plane = %#v, want dataset source and scratch profile", got)
	}
	if got.Checkpoint.RetainLocalLastN != 2 {
		t.Fatalf("checkpoint = %#v, want retain_local_last_n propagated", got.Checkpoint)
	}
}

func TestEnsureDispatchDataPlanePVCMountsMaterializesStageSource(t *testing.T) {
	ctx := context.Background()
	cl := cluster.New(fake.NewSimpleClientset(
		boundDispatchPVC("group-storage", "datasets", "pv-juicefs"),
		csiDispatchPV("pv-juicefs", "csi.juicefs.com", corev1.ReadWriteMany),
	), "proj")
	plan := dataPlanePlan{StageInOperations: []dataPlaneStageInOperation{
		{SourceNamespace: "group-storage", SourcePVC: "datasets", TargetPVC: "datasets"},
		{SourceNamespace: "group-storage", SourcePVC: "cached-datasets", TargetPVC: "cached-datasets", CacheHit: true},
	}}

	if err := ensureDispatchDataPlanePVCMounts(ctx, cl, plan, "proj-p1"); err != nil {
		t.Fatal(err)
	}
	targetPVC, err := cl.Clientset().CoreV1().PersistentVolumeClaims("proj-p1").Get(ctx, "datasets", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("data plane stage source PVC was not materialized: %v", err)
	}
	if targetPVC.Spec.VolumeName != "share-juicefs-proj-p1-datasets" {
		t.Fatalf("target PVC volume = %q, want projected JuiceFS share", targetPVC.Spec.VolumeName)
	}
	if _, err := cl.Clientset().CoreV1().PersistentVolumeClaims("proj-p1").Get(ctx, "cached-datasets", metav1.GetOptions{}); err == nil {
		t.Fatal("cache-hit stage source should not materialize a target PVC")
	}
}

func TestEnsureDispatchDataPlanePVCMountsCreatesScratchPVC(t *testing.T) {
	ctx := context.Background()
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	plan := dataPlanePlan{
		Scratch: dataPlaneScratchPlan{
			ClaimName:        "scratch-job-train-1",
			StorageClassName: "local-nvme-scratch",
			AccessMode:       string(corev1.ReadWriteOnce),
		},
		StageInOperations: []dataPlaneStageInOperation{{
			SourceNamespace: "missing-source",
			SourcePVC:       "missing-dataset",
			TargetPVC:       "cached-dataset",
			CacheHit:        true,
		}},
	}

	if err := ensureDispatchDataPlanePVCMounts(ctx, cl, plan, "proj-p1"); err != nil {
		t.Fatal(err)
	}
	pvc, err := cl.Clientset().CoreV1().PersistentVolumeClaims("proj-p1").Get(ctx, "scratch-job-train-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("scratch PVC was not created: %v", err)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "local-nvme-scratch" {
		t.Fatalf("storageClassName = %#v, want local-nvme-scratch", pvc.Spec.StorageClassName)
	}
	if !hasDispatchAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteOnce) {
		t.Fatalf("scratch accessModes = %#v, want ReadWriteOnce", pvc.Spec.AccessModes)
	}
	if got := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; got.String() != "1Gi" {
		t.Fatalf("scratch storage request = %s, want default 1Gi", got.String())
	}
	if _, err := cl.Clientset().CoreV1().PersistentVolumeClaims("proj-p1").Get(ctx, "cached-dataset", metav1.GetOptions{}); err == nil {
		t.Fatal("cache-hit source should not materialize a target PVC")
	}
}

func TestDataPlaneScratchAccessModeNormalizesProfileAndKubernetesValues(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want corev1.PersistentVolumeAccessMode
	}{
		{name: "profile rwo", mode: "rwo", want: corev1.ReadWriteOnce},
		{name: "profile rwx", mode: "rwx", want: corev1.ReadWriteMany},
		{name: "profile rox", mode: "rox", want: corev1.ReadOnlyMany},
		{name: "kubernetes RWO", mode: string(corev1.ReadWriteOnce), want: corev1.ReadWriteOnce},
		{name: "kubernetes RWX", mode: string(corev1.ReadWriteMany), want: corev1.ReadWriteMany},
		{name: "kubernetes ROX", mode: string(corev1.ReadOnlyMany), want: corev1.ReadOnlyMany},
		{name: "unknown defaults RWO", mode: "object", want: corev1.ReadWriteOnce},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dataPlaneScratchAccessMode(tt.mode); got != tt.want {
				t.Fatalf("access mode %q = %s, want %s", tt.mode, got, tt.want)
			}
		})
	}
}

func decodeDispatchObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}

func namedMaps(t *testing.T, obj map[string]any, path ...string) map[string]map[string]any {
	t.Helper()
	value := any(obj)
	for _, part := range path {
		body, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("path %v reached %T, want object", path, value)
		}
		value = body[part]
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("path %v = %#v, want array", path, value)
	}
	out := map[string]map[string]any{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		if name != "" {
			out[name] = item
		}
	}
	return out
}

func assertNamedPVCVolume(t *testing.T, volumes map[string]map[string]any, name, claimName string) {
	t.Helper()
	volume := volumes[name]
	if volume == nil {
		t.Fatalf("volumes = %#v, missing %s", volumes, name)
	}
	claim, _ := volume["persistentVolumeClaim"].(map[string]any)
	if claim["claimName"] != claimName {
		t.Fatalf("volume %s = %#v, want claimName %q", name, volume, claimName)
	}
}

func assertNamedVolumeMount(t *testing.T, container map[string]any, name, mountPath string) {
	t.Helper()
	mounts, _ := container["volumeMounts"].([]any)
	for _, raw := range mounts {
		mount, ok := raw.(map[string]any)
		if ok && mount["name"] == name && mount["mountPath"] == mountPath {
			return
		}
	}
	t.Fatalf("container %s mounts = %#v, want %s at %s", container["name"], mounts, name, mountPath)
}

func assertNamedEnv(t *testing.T, container map[string]any, name, value string) {
	t.Helper()
	env, _ := container["env"].([]any)
	for _, raw := range env {
		item, ok := raw.(map[string]any)
		if ok && item["name"] == name && item["value"] == value {
			return
		}
	}
	t.Fatalf("container %s env = %#v, want %s=%s", container["name"], env, name, value)
}

func hasDispatchAccessMode(modes []corev1.PersistentVolumeAccessMode, want corev1.PersistentVolumeAccessMode) bool {
	for _, mode := range modes {
		if mode == want {
			return true
		}
	}
	return false
}
