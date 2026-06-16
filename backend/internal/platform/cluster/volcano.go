package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	volcanoVCJobGVR = schema.GroupVersionResource{
		Group:    "batch.volcano.sh",
		Version:  "v1alpha1",
		Resource: "jobs",
	}
	volcanoPodGroupGVR = schema.GroupVersionResource{
		Group:    "scheduling.volcano.sh",
		Version:  "v1beta1",
		Resource: "podgroups",
	}
	draResourceClaimTemplateGVR = schema.GroupVersionResource{
		Group:    "resource.k8s.io",
		Version:  "v1",
		Resource: "resourceclaimtemplates",
	}
	draResourceClaimGVR = schema.GroupVersionResource{
		Group:    "resource.k8s.io",
		Version:  "v1",
		Resource: "resourceclaims",
	}
)

type dynamicManifestTarget struct {
	gvr         schema.GroupVersionResource
	createdKind string
}

func schedulerManifestTarget(meta manifestTypeMeta) (dynamicManifestTarget, bool) {
	gv, err := schema.ParseGroupVersion(strings.TrimSpace(meta.APIVersion))
	if err != nil {
		return dynamicManifestTarget{}, false
	}
	switch {
	case strings.EqualFold(meta.Kind, "Job") && gv.Group == volcanoVCJobGVR.Group && gv.Version == volcanoVCJobGVR.Version:
		return dynamicManifestTarget{gvr: volcanoVCJobGVR, createdKind: "VCJob"}, true
	case strings.EqualFold(meta.Kind, "PodGroup") && gv.Group == volcanoPodGroupGVR.Group && gv.Version == volcanoPodGroupGVR.Version:
		return dynamicManifestTarget{gvr: volcanoPodGroupGVR, createdKind: "PodGroup"}, true
	case strings.EqualFold(meta.Kind, "ResourceClaimTemplate") && gv.Group == draResourceClaimTemplateGVR.Group && gv.Version == draResourceClaimTemplateGVR.Version:
		return dynamicManifestTarget{gvr: draResourceClaimTemplateGVR, createdKind: "ResourceClaimTemplate"}, true
	case strings.EqualFold(meta.Kind, "ResourceClaim") && gv.Group == draResourceClaimGVR.Group && gv.Version == draResourceClaimGVR.Version:
		return dynamicManifestTarget{gvr: draResourceClaimGVR, createdKind: "ResourceClaim"}, true
	default:
		return dynamicManifestTarget{}, false
	}
}

func (c *Client) createDynamicByJSON(
	ctx context.Context,
	namespace string,
	raw []byte,
	target dynamicManifestTarget,
) (CreatedObject, error) {
	if c == nil || c.dynamicClient == nil {
		return CreatedObject{}, ErrUnavailable
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal(raw, &obj.Object); err != nil {
		return CreatedObject{}, fmt.Errorf("%w: %s: %v", ErrInvalidManifest, target.createdKind, err)
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return CreatedObject{}, fmt.Errorf("%w: %s metadata.name is required", ErrInvalidManifest, target.createdKind)
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return CreatedObject{}, fmt.Errorf("%w: namespace is required", ErrInvalidManifest)
	}
	obj.SetNamespace(namespace)
	_, err := c.dynamicClient.Resource(target.gvr).Namespace(namespace).Create(ctx, &obj, metav1.CreateOptions{})
	return createdObject(target.createdKind, namespace, obj.GetName()), ignoreAlreadyExists(err)
}

func (c *Client) volcanoLifecycleStatuses(ctx context.Context, namespace, jobID string) ([]JobLifecycle, error) {
	if c == nil || c.dynamicClient == nil {
		return nil, nil
	}
	vcJobs, err := c.listVolcanoLifecycle(ctx, namespace, jobID, volcanoVCJobGVR, lifecycleFromVolcanoVCJob)
	if err != nil {
		return nil, err
	}
	podGroups, err := c.listVolcanoLifecycle(ctx, namespace, jobID, volcanoPodGroupGVR, lifecycleFromVolcanoPodGroup)
	if err != nil {
		return nil, err
	}
	return append(vcJobs, podGroups...), nil
}

func (c *Client) listVolcanoLifecycle(
	ctx context.Context,
	namespace string,
	jobID string,
	gvr schema.GroupVersionResource,
	mapper func(*unstructured.Unstructured) JobLifecycle,
) ([]JobLifecycle, error) {
	items, err := c.dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: LabelJobID + "=" + jobID})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s for lifecycle: %w", gvr.Resource, err)
	}
	statuses := make([]JobLifecycle, 0, len(items.Items))
	for i := range items.Items {
		if items.Items[i].GetLabels()[LabelJobID] == jobID {
			statuses = append(statuses, mapper(&items.Items[i]))
		}
	}
	return statuses, nil
}

func lifecycleFromVolcanoVCJob(obj *unstructured.Unstructured) JobLifecycle {
	phase := volcanoPhase(obj)
	status := JobLifecycle{Found: true, Status: JobLifecycleQueued, Reason: volcanoReason(obj, "Volcano VCJob is queued")}
	switch phase {
	case "Running":
		status.Status = JobLifecycleRunning
		status.Reason = volcanoReason(obj, "Volcano VCJob is running")
	case "Completed":
		status.Status = JobLifecycleCompleted
		status.Reason = volcanoReason(obj, "Volcano VCJob completed")
		status.CompletedAt = volcanoConditionTime(obj)
	case "Failed", "Aborted":
		status.Status = JobLifecycleFailed
		status.Reason = volcanoReason(obj, "Volcano VCJob failed")
		status.CompletedAt = volcanoConditionTime(obj)
	}
	return status
}

func lifecycleFromVolcanoPodGroup(obj *unstructured.Unstructured) JobLifecycle {
	phase := volcanoPhase(obj)
	status := JobLifecycle{Found: true, Status: JobLifecycleQueued, Reason: volcanoReason(obj, "Volcano PodGroup is queued")}
	switch phase {
	case "Running":
		status.Status = JobLifecycleRunning
		status.Reason = volcanoReason(obj, "Volcano PodGroup is running")
	case "Finished":
		status.Status = JobLifecycleCompleted
		status.Reason = volcanoReason(obj, "Volcano PodGroup finished")
		status.CompletedAt = volcanoConditionTime(obj)
	case "Failed":
		status.Status = JobLifecycleFailed
		status.Reason = volcanoReason(obj, "Volcano PodGroup failed")
		status.CompletedAt = volcanoConditionTime(obj)
	}
	return status
}

func volcanoPhase(obj *unstructured.Unstructured) string {
	for _, path := range [][]string{{"status", "phase"}, {"status", "state", "phase"}} {
		if phase, found, _ := unstructured.NestedString(obj.Object, path...); found {
			return strings.TrimSpace(phase)
		}
	}
	return ""
}

func volcanoReason(obj *unstructured.Unstructured, fallback string) string {
	if reason, ok := latestVolcanoConditionText(obj); ok {
		return reason
	}
	phase := volcanoPhase(obj)
	if phase == "" {
		return fallback
	}
	return fallback + " (phase " + phase + ")"
}

func latestVolcanoConditionText(obj *unstructured.Unstructured) (string, bool) {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || len(conditions) == 0 {
		return "", false
	}
	for i := len(conditions) - 1; i >= 0; i-- {
		condition, ok := conditions[i].(map[string]any)
		if !ok {
			continue
		}
		if text := lifecycleReason(textField(condition, "reason"), textField(condition, "message"), textField(condition, "type")); text != "" {
			return text, true
		}
	}
	return "", false
}

func volcanoConditionTime(obj *unstructured.Unstructured) *time.Time {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return nil
	}
	for i := len(conditions) - 1; i >= 0; i-- {
		condition, ok := conditions[i].(map[string]any)
		if !ok {
			continue
		}
		if t, err := time.Parse(time.RFC3339, textField(condition, "lastTransitionTime")); err == nil {
			return &t
		}
	}
	return nil
}

func textField(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}
