package workload

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type dispatchPlacementSpec struct {
	Labels      map[string]string
	Annotations map[string]string
}

func preparePlacementDispatchResources(job map[string]any, resources []dispatchResource) ([]dispatchResource, error) {
	spec := placementSpecFromJob(job)
	if placementSpecEmpty(spec) {
		return resources, nil
	}
	updated := make([]dispatchResource, 0, len(resources))
	for _, resource := range resources {
		next, err := injectPlacementIntoResource(resource, spec)
		if err != nil {
			return nil, err
		}
		updated = append(updated, next)
	}
	return updated, nil
}

func placementSpecFromJob(job map[string]any) dispatchPlacementSpec {
	return dispatchPlacementSpec{
		Labels:      placementStringMap(firstAnyValue(job, "placement_labels", "placementLabels")),
		Annotations: placementStringMap(firstAnyValue(job, "placement_annotations", "placementAnnotations")),
	}
}

func placementSpecEmpty(spec dispatchPlacementSpec) bool {
	return len(spec.Labels) == 0 && len(spec.Annotations) == 0
}

func injectPlacementIntoResource(resource dispatchResource, spec dispatchPlacementSpec) (dispatchResource, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, err
	}
	mergePlacementObjectMetadata(u, spec)
	if isVolcanoVCJob(u) {
		injectPlacementIntoVCJob(u, spec)
	} else {
		for _, path := range podSpecPaths(u.GetKind()) {
			injectPlacementIntoPodTemplate(u.Object, path, spec)
		}
	}
	raw, err := json.Marshal(u.Object)
	if err != nil {
		return resource, fmt.Errorf(dispatcherMarshalResourceError, u.GetName(), err)
	}
	resource.Raw = raw
	return resource, nil
}

func injectPlacementIntoVCJob(u *unstructured.Unstructured, spec dispatchPlacementSpec) {
	tasks, found, _ := unstructured.NestedSlice(u.Object, "spec", "tasks")
	if !found {
		return
	}
	for i := range tasks {
		task, ok := tasks[i].(map[string]any)
		if !ok {
			continue
		}
		injectPlacementIntoPodTemplate(task, []string{"template", "spec"}, spec)
		tasks[i] = task
	}
	_ = unstructured.SetNestedSlice(u.Object, tasks, "spec", "tasks")
}

func injectPlacementIntoPodTemplate(obj map[string]any, podSpecPath []string, spec dispatchPlacementSpec) {
	metadataPath := placementPodMetadataPath(podSpecPath)
	if len(spec.Labels) > 0 {
		mergePlacementStringMap(obj, spec.Labels, append(metadataPath, "labels")...)
	}
	if len(spec.Annotations) > 0 {
		mergePlacementStringMap(obj, spec.Annotations, append(metadataPath, "annotations")...)
	}
}

func mergePlacementObjectMetadata(u *unstructured.Unstructured, spec dispatchPlacementSpec) {
	if len(spec.Labels) > 0 {
		labels := u.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		for _, key := range sortedPlacementKeys(spec.Labels) {
			labels[key] = spec.Labels[key]
		}
		u.SetLabels(labels)
	}
	if len(spec.Annotations) > 0 {
		annotations := u.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		for _, key := range sortedPlacementKeys(spec.Annotations) {
			annotations[key] = spec.Annotations[key]
		}
		u.SetAnnotations(annotations)
	}
}

func placementPodMetadataPath(podSpecPath []string) []string {
	parent := append([]string{}, podSpecPath...)
	if len(parent) > 0 {
		parent = parent[:len(parent)-1]
	}
	return append(parent, "metadata")
}

func mergePlacementStringMap(obj map[string]any, values map[string]string, fields ...string) {
	current, _, _ := unstructured.NestedStringMap(obj, fields...)
	if current == nil {
		current = map[string]string{}
	}
	for _, key := range sortedPlacementKeys(values) {
		current[key] = values[key]
	}
	_ = unstructured.SetNestedStringMap(obj, current, fields...)
}

func placementStringMap(raw any) map[string]string {
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

func sortedPlacementKeys(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
