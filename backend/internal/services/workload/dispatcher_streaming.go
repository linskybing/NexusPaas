package workload

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Streaming sessions attach a Selkies sidecar to the user's own pod instead of
// requiring the app image to bake in Selkies. The sidecar runs the GPU-backed
// X display server + NVENC capture + WebRTC signaling; the user's container
// renders into the shared DISPLAY :0. Both containers share the same DRA/MPS
// GPU claim because injectContainerDRAClaims (dispatcher_dra.go) wires the claim
// into every container in the pod spec once the DRA step runs after this one.
const (
	streamSidecarContainerName = "selkies"
	streamSidecarSignalPort    = 8080
	streamSidecarMetricsPort   = 9090
	streamX11SocketVolumeName  = "selkies-x11-socket"
	streamX11SocketMountPath   = "/tmp/.X11-unix"
	streamSHMVolumeName        = "selkies-shm"
	streamSHMMountPath         = "/dev/shm"
	streamDisplayValue         = ":0"
)

type streamSidecarConfig struct {
	Image       string
	BitrateKbps int
}

func streamingSessionEnabled(job map[string]any) bool {
	return shared.BoolValue(job, "streaming_session", "streamingSession", "StreamingSession")
}

func streamSidecarConfigFromJob(job map[string]any) (streamSidecarConfig, error) {
	image := strings.TrimSpace(shared.TextValue(job, "stream_sidecar_image", "streamSidecarImage"))
	if image == "" {
		return streamSidecarConfig{}, fmt.Errorf("streaming session requires a configured stream sidecar image (STREAM_SIDECAR_IMAGE)")
	}
	return streamSidecarConfig{
		Image:       image,
		BitrateKbps: shared.IntValue(job, "stream_max_bitrate_kbps", "streamMaxBitrateKbps", "StreamMaxBitrateKbps"),
	}, nil
}

// prepareStreamingDispatchResources injects the Selkies sidecar into each
// pod-bearing resource when the job is a streaming session. It runs before the
// DRA step so the sidecar shares the pod GPU/MPS claim. Non-streaming jobs and
// non-pod resources (Service, etc.) pass through unchanged.
func prepareStreamingDispatchResources(job map[string]any, resources []dispatchResource) ([]dispatchResource, error) {
	if !streamingSessionEnabled(job) {
		return resources, nil
	}
	cfg, err := streamSidecarConfigFromJob(job)
	if err != nil {
		return nil, err
	}
	updated := make([]dispatchResource, 0, len(resources))
	for _, item := range resources {
		injected, err := injectStreamingSidecarIfNeeded(item, cfg)
		if err != nil {
			return nil, err
		}
		updated = append(updated, injected)
	}
	return updated, nil
}

func injectStreamingSidecarIfNeeded(resource dispatchResource, cfg streamSidecarConfig) (dispatchResource, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, err
	}
	paths := podSpecPaths(u.GetKind())
	if len(paths) == 0 {
		return resource, nil
	}
	mutated := false
	for _, path := range paths {
		ok, err := injectSidecarIntoPodSpec(u, cfg, path)
		if err != nil {
			return resource, err
		}
		mutated = mutated || ok
	}
	if !mutated {
		return resource, nil
	}
	raw, err := json.Marshal(u.Object)
	if err != nil {
		return resource, fmt.Errorf(dispatcherMarshalResourceError, u.GetName(), err)
	}
	resource.Raw = raw
	return resource, nil
}

func injectSidecarIntoPodSpec(u *unstructured.Unstructured, cfg streamSidecarConfig, path []string) (bool, error) {
	containersPath := childPath(path, "containers")
	containers, found, err := unstructured.NestedSlice(u.Object, containersPath...)
	if err != nil {
		return false, fmt.Errorf("read pod containers: %w", err)
	}
	if !found || len(containers) == 0 {
		return false, nil
	}
	// Idempotent: a user-supplied selkies container means nothing to inject.
	if sliceHasNamedItem(containers, streamSidecarContainerName) {
		return false, nil
	}
	for i, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		addNamedItem(container, "env", map[string]any{"name": "DISPLAY", "value": streamDisplayValue})
		for _, mount := range streamSidecarMounts() {
			addNamedItem(container, "volumeMounts", mount)
		}
		containers[i] = container
	}
	containers = append(containers, streamSidecarContainer(cfg))
	if err := unstructured.SetNestedSlice(u.Object, containers, containersPath...); err != nil {
		return false, fmt.Errorf("set pod containers: %w", err)
	}
	if err := addSharedStreamVolumes(u, path); err != nil {
		return false, err
	}
	return true, nil
}

func streamSidecarContainer(cfg streamSidecarConfig) map[string]any {
	env := []any{
		map[string]any{"name": "DISPLAY", "value": streamDisplayValue},
		map[string]any{"name": "SELKIES_ENABLE_BASIC_AUTH", "value": "false"},
		map[string]any{"name": "NGINX_PORT", "value": strconv.Itoa(streamSidecarSignalPort)},
		map[string]any{"name": "SELKIES_METRICS_HTTP_PORT", "value": strconv.Itoa(streamSidecarMetricsPort)},
	}
	if cfg.BitrateKbps > 0 {
		bitrate := strconv.Itoa(cfg.BitrateKbps)
		env = append(env,
			map[string]any{"name": "STREAM_MAX_BITRATE_KBPS", "value": bitrate},
			map[string]any{"name": "SELKIES_VIDEO_BITRATE", "value": bitrate},
		)
	}
	return map[string]any{
		"name":  streamSidecarContainerName,
		"image": cfg.Image,
		"env":   env,
		"ports": []any{
			map[string]any{"name": "stream", "containerPort": int64(streamSidecarSignalPort)},
			map[string]any{"name": "stream-metrics", "containerPort": int64(streamSidecarMetricsPort)},
		},
		"volumeMounts": streamSidecarMountsAny(),
	}
}

func addSharedStreamVolumes(u *unstructured.Unstructured, path []string) error {
	volumesPath := childPath(path, "volumes")
	volumes, _, err := unstructured.NestedSlice(u.Object, volumesPath...)
	if err != nil {
		return fmt.Errorf("read pod volumes: %w", err)
	}
	for _, vol := range sharedStreamVolumes() {
		if !sliceHasNamedItem(volumes, vol["name"].(string)) {
			volumes = append(volumes, vol)
		}
	}
	if err := unstructured.SetNestedSlice(u.Object, volumes, volumesPath...); err != nil {
		return fmt.Errorf("set pod volumes: %w", err)
	}
	return nil
}

func sharedStreamVolumes() []map[string]any {
	return []map[string]any{
		{"name": streamX11SocketVolumeName, "emptyDir": map[string]any{}},
		{"name": streamSHMVolumeName, "emptyDir": map[string]any{"medium": "Memory"}},
	}
}

func streamSidecarMounts() []map[string]any {
	return []map[string]any{
		{"name": streamX11SocketVolumeName, "mountPath": streamX11SocketMountPath},
		{"name": streamSHMVolumeName, "mountPath": streamSHMMountPath},
	}
}

func streamSidecarMountsAny() []any {
	mounts := streamSidecarMounts()
	out := make([]any, 0, len(mounts))
	for _, mount := range mounts {
		out = append(out, mount)
	}
	return out
}

// addNamedItem appends item to the container's named list (env, volumeMounts)
// unless an entry with the same name already exists.
func addNamedItem(container map[string]any, key string, item map[string]any) {
	list, _ := container[key].([]any)
	if sliceHasNamedItem(list, item["name"].(string)) {
		return
	}
	container[key] = append(list, item)
}

func sliceHasNamedItem(items []any, name string) bool {
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && shared.TextValue(item, "name") == name {
			return true
		}
	}
	return false
}

func childPath(path []string, child string) []string {
	out := make([]string, 0, len(path)+1)
	out = append(out, path...)
	return append(out, child)
}
