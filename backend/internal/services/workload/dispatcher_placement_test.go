package workload

import "testing"

func TestPreparePlacementDispatchResourcesInjectsKueueLabels(t *testing.T) {
	resources := []dispatchResource{{
		Name: "trainer",
		Kind: "Job",
		Raw: []byte(`{
			"apiVersion":"batch/v1",
			"kind":"Job",
			"metadata":{"name":"trainer"},
			"spec":{"template":{"metadata":{"labels":{"app":"trainer"}},"spec":{"containers":[{"name":"main","image":"busybox"}]}}}
		}`),
	}}
	job := map[string]any{
		"placement_labels": map[string]any{"kueue.x-k8s.io/queue-name": "default-batch"},
	}

	updated, err := preparePlacementDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	metadata := obj["metadata"].(map[string]any)
	labels := metadata["labels"].(map[string]any)
	if labels["kueue.x-k8s.io/queue-name"] != "default-batch" {
		t.Fatalf("object labels = %#v, want Kueue queue label", labels)
	}
	template := obj["spec"].(map[string]any)["template"].(map[string]any)
	templateLabels := template["metadata"].(map[string]any)["labels"].(map[string]any)
	if templateLabels["kueue.x-k8s.io/queue-name"] != "default-batch" || templateLabels["app"] != "trainer" {
		t.Fatalf("template labels = %#v, want merged Kueue queue label", templateLabels)
	}
}

func TestPreparePlacementDispatchResourcesInjectsVCJobTaskTemplates(t *testing.T) {
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
		"placement_labels":      map[string]any{"platform-go/placement-profile": "volcano-gang"},
		"placement_annotations": map[string]any{"scheduling.platform-go/backend": "volcano"},
	}

	updated, err := preparePlacementDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	metadata := obj["metadata"].(map[string]any)
	if metadata["labels"].(map[string]any)["platform-go/placement-profile"] != "volcano-gang" {
		t.Fatalf("vcjob labels = %#v, want placement profile label", metadata["labels"])
	}
	tasks := obj["spec"].(map[string]any)["tasks"].([]any)
	template := tasks[0].(map[string]any)["template"].(map[string]any)
	templateMetadata := template["metadata"].(map[string]any)
	if templateMetadata["annotations"].(map[string]any)["scheduling.platform-go/backend"] != "volcano" {
		t.Fatalf("vcjob template annotations = %#v, want backend annotation", templateMetadata["annotations"])
	}
}

func TestPrepareDispatchManifestAppliesVolcanoSchedulerName(t *testing.T) {
	resource := dispatchResource{
		Name: "trainer",
		Kind: "Job",
		Raw: []byte(`{
			"apiVersion":"batch/v1",
			"kind":"Job",
			"metadata":{"name":"trainer"},
			"spec":{"template":{"spec":{"containers":[{"name":"main","image":"busybox"}]}}}
		}`),
	}
	raw, err := prepareDispatchManifest(map[string]any{"scheduler_name": "volcano"}, resource, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, raw)
	spec := obj["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	podSpec := template["spec"].(map[string]any)
	if podSpec["schedulerName"] != "volcano" {
		t.Fatalf("pod template schedulerName = %#v, want volcano", podSpec["schedulerName"])
	}
}

func TestPreparePlacementDispatchResourcesNoopWhenAbsent(t *testing.T) {
	resources := []dispatchResource{{Name: "trainer", Kind: "Pod", Raw: []byte(`{"kind":"Pod","metadata":{"name":"trainer"},"spec":{"containers":[{"name":"main"}]}}`)}}

	updated, err := preparePlacementDispatchResources(map[string]any{}, resources)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated[0].Raw) != string(resources[0].Raw) {
		t.Fatalf("placement no-op changed manifest: %s", updated[0].Raw)
	}
}
