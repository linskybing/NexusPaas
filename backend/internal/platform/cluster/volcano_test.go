package cluster

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCreateByJSONCreatesVolcanoObjectsWithDynamicClient(t *testing.T) {
	ctx := context.Background()
	dynamicClient := newFakeVolcanoDynamicClient()
	cl := NewWithDynamic(
		fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "proj-p1"}}),
		dynamicClient,
		"proj",
	)

	vcJob := []byte(`{
		"apiVersion":"batch.volcano.sh/v1alpha1",
		"kind":"Job",
		"metadata":{"name":"train"},
		"spec":{"tasks":[{"name":"main","template":{"spec":{"containers":[{"name":"main","image":"busybox"}]}}}]}
	}`)
	created, err := cl.CreateByJSON(ctx, "proj-p1", vcJob)
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "VCJob" || created.Namespace != "proj-p1" || created.Name != "train" {
		t.Fatalf("created VCJob = %#v, want dynamic VCJob identity", created)
	}
	if _, err := cl.CreateByJSON(ctx, "proj-p1", vcJob); err != nil {
		t.Fatalf("idempotent VCJob create returned error: %v", err)
	}
	if _, err := dynamicClient.Resource(volcanoVCJobGVR).Namespace("proj-p1").Get(ctx, "train", metav1.GetOptions{}); err != nil {
		t.Fatalf("VCJob was not created dynamically: %v", err)
	}
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "train", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("native batch job lookup err = %v, want not found", err)
	}

	podGroup := []byte(`{
		"apiVersion":"scheduling.volcano.sh/v1beta1",
		"kind":"PodGroup",
		"metadata":{"name":"train-pg"},
		"spec":{"minMember":1,"queue":"default-batch"}
	}`)
	created, err = cl.CreateByJSON(ctx, "proj-p1", podGroup)
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "PodGroup" || created.Name != "train-pg" {
		t.Fatalf("created PodGroup = %#v, want dynamic PodGroup identity", created)
	}
}

func TestCreateByJSONCreatesDRAObjectsWithDynamicClient(t *testing.T) {
	ctx := context.Background()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		kruntime.NewScheme(),
		map[schema.GroupVersionResource]string{
			draResourceClaimTemplateGVR: "ResourceClaimTemplateList",
			draResourceClaimGVR:         "ResourceClaimList",
		},
	)
	cl := NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")

	claimTemplate := []byte(`{
		"apiVersion":"resource.k8s.io/v1",
		"kind":"ResourceClaimTemplate",
		"metadata":{"name":"gpu-shared-cnt1-sm100-memmax"},
		"spec":{"spec":{"devices":{"requests":[{"name":"gpu","exactly":{"allocationMode":"ExactCount","deviceClassName":"gpu.nvidia.com","count":1}}]}}}
	}`)
	created, err := cl.CreateByJSON(ctx, "proj-p1", claimTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "ResourceClaimTemplate" || created.Namespace != "proj-p1" || created.Name != "gpu-shared-cnt1-sm100-memmax" {
		t.Fatalf("created ResourceClaimTemplate = %#v", created)
	}
	if _, err := dynamicClient.Resource(draResourceClaimTemplateGVR).Namespace("proj-p1").Get(ctx, "gpu-shared-cnt1-sm100-memmax", metav1.GetOptions{}); err != nil {
		t.Fatalf("ResourceClaimTemplate was not created dynamically: %v", err)
	}

	claim := []byte(`{
		"apiVersion":"resource.k8s.io/v1",
		"kind":"ResourceClaim",
		"metadata":{"name":"gpu-direct"},
		"spec":{"devices":{"requests":[{"name":"gpu","exactly":{"allocationMode":"ExactCount","deviceClassName":"gpu.nvidia.com","count":1}}]}}
	}`)
	created, err = cl.CreateByJSON(ctx, "proj-p1", claim)
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "ResourceClaim" || created.Namespace != "proj-p1" || created.Name != "gpu-direct" {
		t.Fatalf("created ResourceClaim = %#v", created)
	}
}

func TestCreateByJSONReturnsUnavailableForVolcanoWithoutDynamicClient(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")
	_, err := cl.CreateByJSON(context.Background(), "proj-p1", []byte(`{
		"apiVersion":"batch.volcano.sh/v1alpha1",
		"kind":"Job",
		"metadata":{"name":"train"}
	}`))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Volcano create without dynamic client err = %v, want ErrUnavailable", err)
	}
}

func TestNativeJobLifecycleMapsVolcanoPhases(t *testing.T) {
	dynamicClient := newFakeVolcanoDynamicClient(
		volcanoStatusObject("batch.volcano.sh/v1alpha1", "Job", "proj-p1", "vc-running", "j-running", "Running"),
		volcanoStatusObject("batch.volcano.sh/v1alpha1", "Job", "proj-p1", "vc-aborted", "j-aborted", "Aborted"),
		volcanoStatusObject("scheduling.volcano.sh/v1beta1", "PodGroup", "proj-p1", "pg-finished", "j-finished", "Finished"),
		volcanoStatusObject("scheduling.volcano.sh/v1beta1", "PodGroup", "proj-p1", "pg-unsched", "j-unsched", "Unschedulable"),
	)
	cl := NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "vcjob running", id: "j-running", want: JobLifecycleRunning},
		{name: "vcjob aborted", id: "j-aborted", want: JobLifecycleFailed},
		{name: "podgroup finished", id: "j-finished", want: JobLifecycleCompleted},
		{name: "podgroup unschedulable", id: "j-unsched", want: JobLifecycleQueued},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cl.NativeJobLifecycle(context.Background(), "proj-p1", tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Found || got.Status != tt.want {
				t.Fatalf("lifecycle = %#v, want %s", got, tt.want)
			}
		})
	}
}

func TestCleanupJobResourcesDeletesVolcanoCRDs(t *testing.T) {
	ctx := context.Background()
	dynamicClient := newFakeVolcanoDynamicClient(
		volcanoStatusObject("batch.volcano.sh/v1alpha1", "Job", "proj-p1", "vc-job", "j1", "Running"),
		volcanoStatusObject("scheduling.volcano.sh/v1beta1", "PodGroup", "proj-p1", "pg", "j1", "Running"),
		volcanoStatusObject("batch.volcano.sh/v1alpha1", "Job", "proj-p1", "other", "j2", "Running"),
	)
	cl := NewWithDynamic(fake.NewSimpleClientset(), dynamicClient, "proj")

	result, err := cl.CleanupJobResources(ctx, "proj-p1", "j1")
	if err != nil {
		t.Fatal(err)
	}
	if result.VCJobs != 1 || result.PodGroups != 1 || result.Total() != 2 {
		t.Fatalf("cleanup result = %+v, want one VCJob and one PodGroup", result)
	}
	if _, err := dynamicClient.Resource(volcanoVCJobGVR).Namespace("proj-p1").Get(ctx, "vc-job", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("deleted VCJob err = %v, want not found", err)
	}
	if _, err := dynamicClient.Resource(volcanoVCJobGVR).Namespace("proj-p1").Get(ctx, "other", metav1.GetOptions{}); err != nil {
		t.Fatalf("unrelated VCJob should remain: %v", err)
	}
}

func newFakeVolcanoDynamicClient(objects ...kruntime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		kruntime.NewScheme(),
		map[schema.GroupVersionResource]string{
			volcanoVCJobGVR:    "JobList",
			volcanoPodGroupGVR: "PodGroupList",
		},
		objects...,
	)
}

func volcanoStatusObject(apiVersion, kind, namespace, name, jobID, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]any{
				LabelJobID: jobID,
			},
		},
		"status": map[string]any{
			"phase": phase,
		},
	}}
}
