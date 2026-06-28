package workload

import "testing"

func TestPrepareNetworkDispatchResourcesInjectsPodAnnotationsAndEnv(t *testing.T) {
	resources := []dispatchResource{{
		Name: "trainer",
		Kind: "Pod",
		Raw: []byte(`{
			"apiVersion":"v1",
			"kind":"Pod",
			"metadata":{"name":"trainer"},
			"spec":{"containers":[{"name":"main","image":"busybox","env":[{"name":"NCCL_SOCKET_IFNAME","value":"eth0"}]}]}
		}`),
	}}
	job := map[string]any{
		"network_annotations": map[string]any{"k8s.v1.cni.cncf.io/networks": "nexuspaas-system/rdma-net"},
		"network_env":         map[string]any{"NCCL_SOCKET_IFNAME": "net1", "NCCL_IB_DISABLE": "0"},
	}

	updated, err := prepareNetworkDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	metadata := obj["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)
	if annotations["k8s.v1.cni.cncf.io/networks"] != "nexuspaas-system/rdma-net" {
		t.Fatalf("annotations = %#v, want rdma-net", annotations)
	}
	containers := namedMaps(t, obj, "spec", "containers")
	assertNamedEnv(t, containers["main"], "NCCL_SOCKET_IFNAME", "net1")
	assertNamedEnv(t, containers["main"], "NCCL_IB_DISABLE", "0")
}

func TestPrepareNetworkDispatchResourcesInjectsVCJobTaskTemplates(t *testing.T) {
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
		"network_annotations": map[string]any{"k8s.v1.cni.cncf.io/networks": "nexuspaas-system/rdma-net"},
		"network_env":         map[string]any{"NCCL_SOCKET_IFNAME": "net1"},
	}

	updated, err := prepareNetworkDispatchResources(job, resources)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeDispatchObject(t, updated[0].Raw)
	tasks := obj["spec"].(map[string]any)["tasks"].([]any)
	task := tasks[0].(map[string]any)
	template := task["template"].(map[string]any)
	metadata := template["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)
	if annotations["k8s.v1.cni.cncf.io/networks"] != "nexuspaas-system/rdma-net" {
		t.Fatalf("vcjob annotations = %#v, want rdma-net", annotations)
	}
	containers := namedMaps(t, template, "spec", "containers")
	assertNamedEnv(t, containers["main"], "NCCL_SOCKET_IFNAME", "net1")
}

func TestPrepareNetworkDispatchResourcesNoopWhenAbsent(t *testing.T) {
	resources := []dispatchResource{{Name: "trainer", Kind: "Pod", Raw: []byte(`{"kind":"Pod","metadata":{"name":"trainer"},"spec":{"containers":[{"name":"main"}]}}`)}}

	updated, err := prepareNetworkDispatchResources(map[string]any{}, resources)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated[0].Raw) != string(resources[0].Raw) {
		t.Fatalf("network no-op changed manifest: %s", updated[0].Raw)
	}
}
