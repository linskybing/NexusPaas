package workload

import (
	"encoding/json"
	"testing"
)

func deploymentResource(t *testing.T, appImage string, gpu bool) dispatchResource {
	t.Helper()
	container := map[string]any{
		"name":  "app",
		"image": appImage,
		"env":   []any{map[string]any{"name": "FOO", "value": "bar"}},
	}
	if gpu {
		container["resources"] = map[string]any{
			"limits": map[string]any{"nvidia.com/gpu": "1"},
		}
	}
	obj := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "cad-app"},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{"containers": []any{container}},
			},
		},
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return dispatchResource{Name: "cad-app", Kind: "Deployment", Raw: raw}
}

func podSpecFrom(t *testing.T, resource dispatchResource) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(resource.Raw, &obj); err != nil {
		t.Fatalf("unmarshal resource: %v", err)
	}
	spec, ok := obj["spec"].(map[string]any)
	if !ok {
		t.Fatal("missing spec")
	}
	tmpl, ok := spec["template"].(map[string]any)
	if !ok {
		t.Fatal("missing template")
	}
	podSpec, ok := tmpl["spec"].(map[string]any)
	if !ok {
		t.Fatal("missing pod spec")
	}
	return podSpec
}

func containerByName(podSpec map[string]any, name string) map[string]any {
	containers, _ := podSpec["containers"].([]any)
	for _, raw := range containers {
		if c, ok := raw.(map[string]any); ok && c["name"] == name {
			return c
		}
	}
	return nil
}

func hasNamed(list any, name string) bool {
	items, _ := list.([]any)
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && item["name"] == name {
			return true
		}
	}
	return false
}

func TestPrepareStreamingDispatchResourcesInjectsSidecar(t *testing.T) {
	job := map[string]any{
		"streaming_session":       true,
		"stream_sidecar_image":    "registry.example.com/nexuspaas/selkies-gl-desktop:24.04",
		"stream_max_bitrate_kbps": 12000,
	}
	out, err := prepareStreamingDispatchResources(job, []dispatchResource{deploymentResource(t, "my-cad:1.0", true)})
	if err != nil {
		t.Fatal(err)
	}
	podSpec := podSpecFrom(t, out[0])

	sidecar := containerByName(podSpec, streamSidecarContainerName)
	if sidecar == nil {
		t.Fatal("selkies sidecar was not injected")
	}
	if sidecar["image"] != "registry.example.com/nexuspaas/selkies-gl-desktop:24.04" {
		t.Fatalf("sidecar image = %v", sidecar["image"])
	}
	if !hasNamed(sidecar["env"], "SELKIES_VIDEO_BITRATE") {
		t.Fatal("sidecar missing SELKIES_VIDEO_BITRATE env")
	}

	app := containerByName(podSpec, "app")
	if app["image"] != "my-cad:1.0" {
		t.Fatalf("app image was rewritten to %v; user image must stay unchanged", app["image"])
	}
	if !hasNamed(app["env"], "DISPLAY") {
		t.Fatal("app missing DISPLAY=:0 env")
	}
	if !hasNamed(app["volumeMounts"], streamX11SocketVolumeName) || !hasNamed(app["volumeMounts"], streamSHMVolumeName) {
		t.Fatal("app missing shared X11/shm volume mounts")
	}
	if !hasNamed(podSpec["volumes"], streamX11SocketVolumeName) || !hasNamed(podSpec["volumes"], streamSHMVolumeName) {
		t.Fatal("pod missing shared X11/shm volumes")
	}
}

func TestPrepareStreamingDispatchResourcesNoStreamingIsUnchanged(t *testing.T) {
	in := []dispatchResource{deploymentResource(t, "my-cad:1.0", true)}
	out, err := prepareStreamingDispatchResources(map[string]any{}, in)
	if err != nil {
		t.Fatal(err)
	}
	if string(out[0].Raw) != string(in[0].Raw) {
		t.Fatal("non-streaming manifest must be untouched")
	}
}

func TestPrepareStreamingDispatchResourcesRequiresSidecarImage(t *testing.T) {
	job := map[string]any{"streaming_session": true}
	if _, err := prepareStreamingDispatchResources(job, []dispatchResource{deploymentResource(t, "my-cad:1.0", true)}); err == nil {
		t.Fatal("expected error when stream_sidecar_image is unset")
	}
}

func TestPrepareStreamingDispatchResourcesIsIdempotent(t *testing.T) {
	job := map[string]any{
		"streaming_session":    true,
		"stream_sidecar_image": "selkies:24.04",
	}
	out, err := prepareStreamingDispatchResources(job, []dispatchResource{deploymentResource(t, "my-cad:1.0", true)})
	if err != nil {
		t.Fatal(err)
	}
	twice, err := prepareStreamingDispatchResources(job, out)
	if err != nil {
		t.Fatal(err)
	}
	podSpec := podSpecFrom(t, twice[0])
	containers, _ := podSpec["containers"].([]any)
	count := 0
	for _, raw := range containers {
		if c, ok := raw.(map[string]any); ok && c["name"] == streamSidecarContainerName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("selkies container count = %d, want exactly 1 (no double injection)", count)
	}
}
