package workload

import (
	"strings"
	"testing"
)

func TestPrepareAcceleratorDispatchResourcesInjectsPodSelectorsAndLabels(t *testing.T) {
	resources := []dispatchResource{{
		Name: "trainer",
		Kind: "Pod",
		Raw: []byte(`{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"trainer","labels":{"app":"trainer"}},
			"spec":{"containers":[{"name":"main","image":"busybox"}]}
		}`),
	}}
	job := map[string]any{
		"accelerator_node_selector": map[string]any{"nexuspaas.io/gpu": "true"},
		"accelerator_labels": map[string]any{
			"nexuspaas.io/accelerator-profile": "nvidia-gpu-default",
			"app":                              "ignored",
		},
	}

	updated, err := prepareAcceleratorDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	spec := obj["spec"].(map[string]any)
	selector := spec["nodeSelector"].(map[string]any)
	if selector["nexuspaas.io/gpu"] != "true" {
		t.Fatalf("nodeSelector = %#v, want gpu selector", selector)
	}
	labels := obj["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["nexuspaas.io/accelerator-profile"] != "nvidia-gpu-default" || labels["app"] != "trainer" {
		t.Fatalf("labels = %#v, want accelerator label and original app", labels)
	}
}

func TestPrepareAcceleratorDispatchResourcesInjectsDeploymentTemplate(t *testing.T) {
	resources := []dispatchResource{{
		Name: "trainer",
		Kind: "Deployment",
		Raw: []byte(`{
			"apiVersion":"apps/v1",
			"kind":"Deployment",
			"metadata":{"name":"trainer"},
			"spec":{"template":{"metadata":{"labels":{"app":"trainer"}},"spec":{"containers":[{"name":"main","image":"busybox"}]}}}
		}`),
	}}
	job := map[string]any{
		"accelerator_node_selector": map[string]any{"nexuspaas.io/gpu-class": "h100-sxm"},
		"accelerator_labels":        map[string]any{"accelerator.nexuspaas.io/profile": "h100"},
	}

	updated, err := prepareAcceleratorDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	template := obj["spec"].(map[string]any)["template"].(map[string]any)
	podSpec := template["spec"].(map[string]any)
	selector := podSpec["nodeSelector"].(map[string]any)
	if selector["nexuspaas.io/gpu-class"] != "h100-sxm" {
		t.Fatalf("deployment nodeSelector = %#v, want h100", selector)
	}
	labels := template["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["accelerator.nexuspaas.io/profile"] != "h100" || labels["app"] != "trainer" {
		t.Fatalf("deployment labels = %#v, want accelerator label", labels)
	}
}

func TestPrepareAcceleratorDispatchResourcesInjectsVCJobTaskTemplates(t *testing.T) {
	resources := []dispatchResource{{
		Name: "train",
		Kind: "Job",
		Raw: []byte(`{
			"apiVersion":"batch.volcano.sh/v1alpha1",
			"kind":"Job",
			"metadata":{"name":"train"},
			"spec":{"tasks":[{"name":"worker","template":{"spec":{"containers":[{"name":"main","image":"busybox"}]}}}]}
		}`),
	}}
	job := map[string]any{
		"accelerator_node_selector": map[string]any{"nexuspaas.io/rdma": "true"},
		"accelerator_labels":        map[string]any{"nexuspaas.io/accelerator-profile": "nvidia-h100-sxm-rdma"},
	}

	updated, err := prepareAcceleratorDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	tasks := obj["spec"].(map[string]any)["tasks"].([]any)
	template := tasks[0].(map[string]any)["template"].(map[string]any)
	podSpec := template["spec"].(map[string]any)
	if podSpec["nodeSelector"].(map[string]any)["nexuspaas.io/rdma"] != "true" {
		t.Fatalf("vcjob nodeSelector = %#v, want rdma", podSpec["nodeSelector"])
	}
	labels := template["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["nexuspaas.io/accelerator-profile"] != "nvidia-h100-sxm-rdma" {
		t.Fatalf("vcjob labels = %#v, want accelerator profile", labels)
	}
}

func TestPrepareAcceleratorDispatchResourcesRejectsSelectorConflict(t *testing.T) {
	resources := []dispatchResource{{
		Name: "trainer",
		Kind: "Pod",
		Raw: []byte(`{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"trainer"},
			"spec":{"nodeSelector":{"nexuspaas.io/gpu":"false"},"containers":[{"name":"main"}]}
		}`),
	}}
	job := map[string]any{"accelerator_node_selector": map[string]any{"nexuspaas.io/gpu": "true"}}

	_, err := prepareAcceleratorDispatchResources(job, resources)
	if err == nil || !strings.Contains(err.Error(), acceleratorSelectorConflictError) {
		t.Fatalf("err = %v, want accelerator selector conflict", err)
	}
}

func TestPrepareAcceleratorDispatchResourcesNoopWhenAbsent(t *testing.T) {
	resources := []dispatchResource{{Name: "trainer", Kind: "Pod", Raw: []byte(`{"kind":"Pod","metadata":{"name":"trainer"},"spec":{"containers":[{"name":"main"}]}}`)}}

	updated, err := prepareAcceleratorDispatchResources(map[string]any{}, resources)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated[0].Raw) != string(resources[0].Raw) {
		t.Fatalf("accelerator no-op changed manifest: %s", updated[0].Raw)
	}
}
