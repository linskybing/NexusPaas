package workload

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const acceleratorSelectorConflictError = "accelerator selector conflict: key exists with different value"

type dispatchAcceleratorSpec struct {
	NodeSelector map[string]string
	Labels       map[string]string
}

func prepareAcceleratorDispatchResources(job map[string]any, resources []dispatchResource) ([]dispatchResource, error) {
	spec := acceleratorSpecFromJob(job)
	if acceleratorSpecEmpty(spec) {
		return resources, nil
	}
	updated := make([]dispatchResource, 0, len(resources))
	for _, resource := range resources {
		next, err := injectAcceleratorIntoResource(resource, spec)
		if err != nil {
			return nil, err
		}
		updated = append(updated, next)
	}
	return updated, nil
}

func acceleratorSpecFromJob(job map[string]any) dispatchAcceleratorSpec {
	return dispatchAcceleratorSpec{
		NodeSelector: networkStringMap(firstAnyValue(job, "accelerator_node_selector", "acceleratorNodeSelector")),
		Labels:       networkStringMap(firstAnyValue(job, "accelerator_labels", "acceleratorLabels")),
	}
}

func acceleratorSpecEmpty(spec dispatchAcceleratorSpec) bool {
	return len(spec.NodeSelector) == 0 && len(spec.Labels) == 0
}

func injectAcceleratorIntoResource(resource dispatchResource, spec dispatchAcceleratorSpec) (dispatchResource, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, err
	}
	if isVolcanoVCJob(u) {
		if err := injectAcceleratorIntoVCJob(u, spec); err != nil {
			return resource, err
		}
	} else {
		for _, path := range podSpecPaths(u.GetKind()) {
			if err := injectAcceleratorIntoPodTemplate(u.Object, path, spec); err != nil {
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

func injectAcceleratorIntoVCJob(u *unstructured.Unstructured, spec dispatchAcceleratorSpec) error {
	tasks, found, _ := unstructured.NestedSlice(u.Object, "spec", "tasks")
	if !found {
		return nil
	}
	for i := range tasks {
		task, ok := tasks[i].(map[string]any)
		if !ok {
			continue
		}
		if err := injectAcceleratorIntoPodTemplate(task, []string{"template", "spec"}, spec); err != nil {
			return err
		}
		tasks[i] = task
	}
	return unstructured.SetNestedSlice(u.Object, tasks, "spec", "tasks")
}

func injectAcceleratorIntoPodTemplate(obj map[string]any, podSpecPath []string, spec dispatchAcceleratorSpec) error {
	if len(spec.NodeSelector) > 0 {
		if err := mergeAcceleratorNodeSelector(obj, childPath(podSpecPath, "nodeSelector"), spec.NodeSelector); err != nil {
			return err
		}
	}
	if len(spec.Labels) > 0 {
		mergeAcceleratorLabels(obj, acceleratorPodMetadataPath(podSpecPath), spec.Labels)
	}
	return nil
}

func mergeAcceleratorNodeSelector(obj map[string]any, path []string, selector map[string]string) error {
	current, _, _ := unstructured.NestedStringMap(obj, path...)
	if current == nil {
		current = map[string]string{}
	}
	for _, key := range sortedNetworkKeys(selector) {
		value := selector[key]
		if existing, found := current[key]; found && existing != value {
			return fmt.Errorf("%s: %s", acceleratorSelectorConflictError, key)
		}
		current[key] = value
	}
	return unstructured.SetNestedStringMap(obj, current, path...)
}

func mergeAcceleratorLabels(obj map[string]any, metadataPath []string, labels map[string]string) {
	path := append(metadataPath, "labels")
	current, _, _ := unstructured.NestedStringMap(obj, path...)
	if current == nil {
		current = map[string]string{}
	}
	for _, key := range sortedNetworkKeys(labels) {
		if !acceleratorOwnedLabelKey(key) {
			continue
		}
		current[key] = labels[key]
	}
	_ = unstructured.SetNestedStringMap(obj, current, path...)
}

func acceleratorOwnedLabelKey(key string) bool {
	return strings.HasPrefix(key, "accelerator.") || strings.HasPrefix(key, "nexuspaas.io/accelerator-")
}

func acceleratorPodMetadataPath(podSpecPath []string) []string {
	parent := append([]string{}, podSpecPath...)
	if len(parent) > 0 {
		parent = parent[:len(parent)-1]
	}
	return append(parent, "metadata")
}
