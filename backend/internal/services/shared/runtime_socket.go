package shared

import "strings"

var containerRuntimeSocketPaths = map[string]struct{}{
	"/var/run/docker.sock":                {},
	"/run/docker.sock":                    {},
	"/var/run/containerd/containerd.sock": {},
	"/run/containerd/containerd.sock":     {},
	"/var/run/crio/crio.sock":             {},
	"/run/crio/crio.sock":                 {},
}

func RuntimeSocketHostPath(podSpec map[string]any) (string, bool) {
	for _, volume := range mapItems(podSpec["volumes"]) {
		hostPath, _ := volume["hostPath"].(map[string]any)
		path := strings.TrimSpace(TextValue(hostPath, "path", "Path"))
		if _, found := containerRuntimeSocketPaths[path]; found {
			return path, true
		}
	}
	return "", false
}

func mapItems(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if typed, ok := item.(map[string]any); ok {
			out = append(out, typed)
		}
	}
	return out
}
