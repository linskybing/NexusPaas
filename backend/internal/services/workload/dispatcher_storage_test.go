package workload

import (
	"errors"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStorageMountPlanSelectorsFromSubmissionPayloadJSON(t *testing.T) {
	mounts, err := storageMountPlanSelectorsFromJob(map[string]any{
		"submission_payload": map[string]any{
			"storageMounts": `{"items":[{"claimName":"team-data","mountPath":"/data","readOnly":true}]}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].PVCID != "team-data" ||
		mounts[0].MountPath != "/data" || !mounts[0].ReadOnly {
		t.Fatalf("mounts = %#v, want parsed JSON storage mount", mounts)
	}
}

func TestStorageMountPlanSelectorsRejectMissingPVCID(t *testing.T) {
	_, err := storageMountPlanSelectorsFromJob(map[string]any{
		"storage_mounts": []any{map[string]any{"mountPath": "/data"}},
	})
	if !errors.Is(err, cluster.ErrInvalidManifest) || !strings.Contains(err.Error(), "pvc_id") {
		t.Fatalf("err = %v, want invalid missing pvc_id", err)
	}
}

func TestStorageMountPlanSelectorsIgnoreForgedSourceDetails(t *testing.T) {
	mounts, err := storageMountPlanSelectorsFromJob(map[string]any{
		"storage_mounts": []any{map[string]any{
			"pvc_id":           "datasets",
			"source_namespace": "forged-source",
			"source_pvc":       "forged-pvc",
			"target_pvc":       "forged-target",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].PVCID != "datasets" || mounts[0].MountPath != "" {
		t.Fatalf("selectors = %#v, want only storage-owned selector fields", mounts)
	}
}

func TestInjectStorageMountsRejectsVolumeAndMountConflicts(t *testing.T) {
	resource := dispatchResource{
		Name: "conflict",
		Kind: "Pod",
		Raw: []byte(`{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"conflict"},
			"spec":{
				"volumes":[{"name":"datasets","persistentVolumeClaim":{"claimName":"other-pvc"}}],
				"containers":[{"name":"main","image":"busybox"}]
			}
		}`),
	}
	_, _, err := injectStorageMountsIntoResource(resource, []storageMountSpec{{
		Name: "datasets", ClaimName: "datasets-pvc", MountPath: "/mnt/datasets",
	}})
	if !errors.Is(err, cluster.ErrInvalidManifest) || !strings.Contains(err.Error(), "different PVC") {
		t.Fatalf("err = %v, want PVC conflict", err)
	}

	resource.Raw = []byte(`{
		"apiVersion":"v1",
		"kind":"Pod",
		"metadata":{"name":"conflict"},
		"spec":{"containers":[{"name":"main","image":"busybox","volumeMounts":[{"name":"cache","mountPath":"/mnt/datasets"}]}]}
	}`)
	_, _, err = injectStorageMountsIntoResource(resource, []storageMountSpec{{
		Name: "datasets", ClaimName: "datasets-pvc", MountPath: "/mnt/datasets",
	}})
	if !errors.Is(err, cluster.ErrInvalidManifest) || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("err = %v, want mount path conflict", err)
	}
}

func TestInjectStorageMountsIntoSuppliedVCJobTasks(t *testing.T) {
	resource := dispatchResource{
		Name: "vc-storage",
		Kind: "Job",
		Raw: []byte(`{
			"apiVersion":"batch.volcano.sh/v1alpha1",
			"kind":"Job",
			"metadata":{"name":"vc-storage"},
			"spec":{"tasks":[{"name":"main","template":{"spec":{"containers":[{"name":"main","image":"busybox"}]}}}]}
		}`),
	}

	updated, applied, err := injectStorageMountsIntoResource(resource, []storageMountSpec{{
		Name: "datasets", ClaimName: "datasets-pvc", MountPath: "/mnt/datasets", ReadOnly: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("storage mounts were not applied to supplied VCJob")
	}
	u, err := dispatchObject(updated)
	if err != nil {
		t.Fatal(err)
	}
	task := firstVCJobTask(t, u)
	volumes, _, _ := unstructured.NestedSlice(task, "template", "spec", "volumes")
	if len(volumes) != 1 {
		t.Fatalf("VCJob volumes = %#v, want one injected volume", volumes)
	}
	containers, _, _ := unstructured.NestedSlice(task, "template", "spec", "containers")
	container := containers[0].(map[string]any)
	containerMounts := container["volumeMounts"].([]any)
	if len(containerMounts) != 1 {
		t.Fatalf("VCJob container mounts = %#v, want one injected mount", containerMounts)
	}
}

func TestStorageMountSpecFromMapCoversManifestAndSharePlans(t *testing.T) {
	mount, ok, err := storageMountSpecFromMap(map[string]any{
		"name":             "Team Data",
		"claim_name":       "datasets-pvc",
		"mount_path":       "/mnt/datasets",
		"read_only":        "true",
		"sub_path":         "team-a",
		"source_namespace": "group-storage",
		"source_pvc":       "datasets-source",
		"target_pvc":       "datasets-pvc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || mount.Name != "team-data" || mount.ClaimName != "datasets-pvc" ||
		mount.MountPath != "/mnt/datasets" || !mount.ReadOnly || mount.SubPath != "team-a" ||
		mount.SourceNamespace != "group-storage" || mount.SourcePVC != "datasets-source" || mount.TargetPVC != "datasets-pvc" {
		t.Fatalf("mount = %#v, want manifest mount with share details", mount)
	}

	opOnly, ok, err := storageMountSpecFromMap(map[string]any{
		"source_namespace": "group-storage",
		"source_pvc":       "datasets-source",
		"target_pvc":       "datasets-pvc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || opOnly.ClaimName != "" || opOnly.SourcePVC != "datasets-source" || opOnly.TargetPVC != "datasets-pvc" {
		t.Fatalf("opOnly = %#v, want PVC share operation-only spec", opOnly)
	}

	empty, ok, err := storageMountSpecFromMap(map[string]any{"ignored": true})
	if err != nil || ok || empty != (storageMountSpec{}) {
		t.Fatalf("empty = %#v ok=%v err=%v, want ignored", empty, ok, err)
	}
}

func TestStorageMountSpecFromMapRejectsInvalidShapes(t *testing.T) {
	tests := []map[string]any{
		{"claim_name": "datasets-pvc"},
		{"mount_path": "/mnt/datasets"},
		{"source_pvc": "datasets-source", "target_pvc": "datasets-pvc"},
	}
	for _, tc := range tests {
		if _, _, err := storageMountSpecFromMap(tc); !errors.Is(err, cluster.ErrInvalidManifest) {
			t.Fatalf("storageMountSpecFromMap(%#v) err = %v, want invalid manifest", tc, err)
		}
	}
}

func TestPVCMountOpsFromSpecsDedupesAndFilters(t *testing.T) {
	ops := pvcMountOpsFromSpecs([]storageMountSpec{
		{SourceNamespace: "group-storage", SourcePVC: "datasets-source", TargetPVC: "datasets-pvc"},
		{SourceNamespace: "group-storage", SourcePVC: "datasets-source", TargetPVC: "datasets-pvc", MountPath: "/mnt/datasets"},
		{SourceNamespace: "group-storage", SourcePVC: "", TargetPVC: "skip"},
	})
	if len(ops) != 1 || ops[0].sourceNamespace != "group-storage" || ops[0].sourcePVC != "datasets-source" || ops[0].targetPVC != "datasets-pvc" {
		t.Fatalf("ops = %#v, want one deduped PVC mount op", ops)
	}
}

func TestExistingStorageMountMergeBranches(t *testing.T) {
	existing := map[string]any{"name": "datasets", "mountPath": "/mnt/datasets"}
	if err := mergeExistingStorageMount(existing, storageMountSpec{Name: "datasets", MountPath: "/mnt/datasets", ReadOnly: true, SubPath: "team-a"}); err != nil {
		t.Fatal(err)
	}
	if existing["readOnly"] != true || existing["subPath"] != "team-a" {
		t.Fatalf("existing = %#v, want readOnly and subPath merged", existing)
	}
	if err := mergeExistingStorageMount(existing, storageMountSpec{Name: "datasets", MountPath: "/different"}); !errors.Is(err, cluster.ErrInvalidManifest) {
		t.Fatalf("err = %v, want mountPath conflict", err)
	}
	if err := mergeExistingStorageSubPath(existing, storageMountSpec{Name: "datasets", SubPath: "other"}); !errors.Is(err, cluster.ErrInvalidManifest) {
		t.Fatalf("err = %v, want subPath conflict", err)
	}
}

func TestCollectDispatchPVCClaimNamesWalksNestedSpecs(t *testing.T) {
	claims := collectDispatchPVCClaimNames([]dispatchResource{{
		Name: "deploy",
		Kind: "Deployment",
		Raw: []byte(`{
			"apiVersion":"apps/v1",
			"kind":"Deployment",
			"spec":{"template":{"spec":{"volumes":[
				{"name":"data","persistentVolumeClaim":{"claimName":"datasets-pvc"}},
				{"name":"empty","emptyDir":{}}
			],"containers":[{"name":"main"}]}}}
		}`),
	}, {
		Name: "bad",
		Kind: "Pod",
		Raw:  []byte(`{bad-json`),
	}})
	if _, ok := claims["datasets-pvc"]; !ok || len(claims) != 1 {
		t.Fatalf("claims = %#v, want nested deployment PVC only", claims)
	}
}
