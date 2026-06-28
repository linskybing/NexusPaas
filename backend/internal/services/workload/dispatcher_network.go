package workload

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type dispatchNetworkSpec struct {
	Annotations map[string]string
	Env         map[string]string
}

func prepareNetworkDispatchResources(job map[string]any, resources []dispatchResource) ([]dispatchResource, error) {
	spec := networkSpecFromJob(job)
	if networkSpecEmpty(spec) {
		return resources, nil
	}
	updated := make([]dispatchResource, 0, len(resources))
	for _, resource := range resources {
		next, err := injectNetworkIntoResource(resource, spec)
		if err != nil {
			return nil, err
		}
		updated = append(updated, next)
	}
	return updated, nil
}

func networkSpecFromJob(job map[string]any) dispatchNetworkSpec {
	return dispatchNetworkSpec{
		Annotations: networkStringMap(firstAnyValue(job, "network_annotations", "networkAnnotations")),
		Env:         networkStringMap(firstAnyValue(job, "network_env", "networkEnv")),
	}
}

func networkSpecEmpty(spec dispatchNetworkSpec) bool {
	return len(spec.Annotations) == 0 && len(spec.Env) == 0
}

func injectNetworkIntoResource(resource dispatchResource, spec dispatchNetworkSpec) (dispatchResource, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, err
	}
	if isVolcanoVCJob(u) {
		if err := injectNetworkIntoVCJob(u, spec); err != nil {
			return resource, err
		}
	} else {
		for _, path := range podSpecPaths(u.GetKind()) {
			if err := injectNetworkIntoPodTemplate(u.Object, path, spec); err != nil {
				return resource, err
			}
		}
	}
	raw, err := json.Marshal(u.Object)
	if err != nil {
		return resource, fmt.Errorf(dispatcherMarshalResourceError, u.GetName(), err)
	}
	resource.Raw = raw
	return resource, nil
}

func injectNetworkIntoVCJob(u *unstructured.Unstructured, spec dispatchNetworkSpec) error {
	tasks, found, _ := unstructured.NestedSlice(u.Object, "spec", "tasks")
	if !found {
		return nil
	}
	for i := range tasks {
		task, ok := tasks[i].(map[string]any)
		if !ok {
			continue
		}
		if err := injectNetworkIntoPodTemplate(task, []string{"template", "spec"}, spec); err != nil {
			return err
		}
		tasks[i] = task
	}
	return unstructured.SetNestedSlice(u.Object, tasks, "spec", "tasks")
}

func injectNetworkIntoPodTemplate(obj map[string]any, podSpecPath []string, spec dispatchNetworkSpec) error {
	if len(spec.Annotations) > 0 {
		if err := mergeNetworkAnnotations(obj, podMetadataPath(podSpecPath), spec.Annotations); err != nil {
			return err
		}
	}
	if len(spec.Env) == 0 {
		return nil
	}
	containersPath := childPath(podSpecPath, "containers")
	containers, found, _ := unstructured.NestedSlice(obj, containersPath...)
	if !found {
		return nil
	}
	for i, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		mergeNetworkEnv(container, spec.Env)
		containers[i] = container
	}
	return unstructured.SetNestedSlice(obj, containers, containersPath...)
}

func podMetadataPath(podSpecPath []string) []string {
	parent := append([]string{}, podSpecPath...)
	if len(parent) > 0 {
		parent = parent[:len(parent)-1]
	}
	return append(parent, "metadata", "annotations")
}

func mergeNetworkAnnotations(obj map[string]any, path []string, annotations map[string]string) error {
	current, _, _ := unstructured.NestedStringMap(obj, path...)
	if current == nil {
		current = map[string]string{}
	}
	for _, key := range sortedNetworkKeys(annotations) {
		current[key] = annotations[key]
	}
	return unstructured.SetNestedStringMap(obj, current, path...)
}

func mergeNetworkEnv(container map[string]any, env map[string]string) {
	items, _ := container["env"].([]any)
	for _, key := range sortedNetworkKeys(env) {
		items = setNetworkEnv(items, key, env[key])
	}
	container["env"] = items
}

func setNetworkEnv(items []any, name, value string) []any {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok && item["name"] == name {
			item["value"] = value
			return items
		}
	}
	return append(items, map[string]any{"name": name, "value": value})
}

func networkStringMap(raw any) map[string]string {
	data, ok := raw.(map[string]any)
	if !ok || len(data) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range data {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = text
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedNetworkKeys(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
