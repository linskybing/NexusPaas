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
