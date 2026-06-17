package workload

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestApplyPodGroupDispatchSchedulingSetsQueueAndPriority(t *testing.T) {
	podGroup := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "scheduling.volcano.sh/v1beta1",
		"kind":       "PodGroup",
		"metadata": map[string]any{
			"name": "pg-a",
		},
	}}
	applyDispatchScheduling(podGroup, map[string]any{
		"queue_name": "gpu",
		"priority":   float64(500000),
	}, "ignored")

	queue, _, _ := unstructured.NestedString(podGroup.Object, "spec", "queue")
	priority, _, _ := unstructured.NestedString(podGroup.Object, "spec", "priorityClassName")
	if queue != "gpu" || priority != "platform-interactive-high" {
		t.Fatalf("podGroup scheduling queue=%q priority=%q, want gpu/platform-interactive-high", queue, priority)
	}
}

func TestApplyDispatchSchedulingSetsPodAndTemplateMetadata(t *testing.T) {
	pod := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "pod-a"},
		"spec":       map[string]any{},
	}}
	applyDispatchScheduling(pod, map[string]any{"queueName": "batch", "schedulerName": "custom", "priority": 1000}, "group-a")
	if scheduler, _, _ := unstructured.NestedString(pod.Object, "spec", "schedulerName"); scheduler != "custom" {
		t.Fatalf("pod scheduler = %q, want custom", scheduler)
	}
	if priority, _, _ := unstructured.NestedString(pod.Object, "spec", "priorityClassName"); priority != "platform-batch-medium" {
		t.Fatalf("pod priority = %q, want platform-batch-medium", priority)
	}
	if pod.GetAnnotations()[volcanoQueueAnnotationKey] != "batch" || pod.GetAnnotations()[volcanoGroupAnnotationKey] != "group-a" {
		t.Fatalf("pod annotations = %#v, want queue and group annotations", pod.GetAnnotations())
	}
	if pod.GetLabels()[volcanoPodGroupLabelKey] != "group-a" {
		t.Fatalf("pod labels = %#v, want group label", pod.GetLabels())
	}

	deployment := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "deploy-a"},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{},
				"spec":     map[string]any{},
			},
		},
	}}
	applyDispatchScheduling(deployment, map[string]any{"queue_name": "batch", "priority": 1000000}, "group-b")
	if scheduler, _, _ := unstructured.NestedString(deployment.Object, "spec", "template", "spec", "schedulerName"); scheduler != defaultDispatcherSchedulerName {
		t.Fatalf("deployment scheduler = %q, want default scheduler", scheduler)
	}
	if priority, _, _ := unstructured.NestedString(deployment.Object, "spec", "template", "spec", "priorityClassName"); priority != "platform-critical" {
		t.Fatalf("deployment priority = %q, want platform-critical", priority)
	}
	annotations, _, _ := unstructured.NestedStringMap(deployment.Object, "spec", "template", "metadata", "annotations")
	if annotations[volcanoQueueAnnotationKey] != "batch" || annotations[volcanoGroupAnnotationKey] != "group-b" {
		t.Fatalf("deployment annotations = %#v, want queue and group annotations", annotations)
	}
	labels, _, _ := unstructured.NestedStringMap(deployment.Object, "spec", "template", "metadata", "labels")
	if labels[volcanoPodGroupLabelKey] != "group-b" {
		t.Fatalf("deployment labels = %#v, want group label", labels)
	}
}

func TestVolcanoVCJobSchedulingBranches(t *testing.T) {
	vcJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch.volcano.sh/v1alpha1",
		"kind":       "Job",
		"metadata":   map[string]any{"name": "vc-a"},
		"spec": map[string]any{
			"tasks": []any{
				map[string]any{"name": "main", "template": map[string]any{
					"metadata": map[string]any{},
					"spec":     map[string]any{},
				}},
				"ignored",
			},
		},
	}}
	applyDispatchScheduling(vcJob, map[string]any{"queue_name": "gpu", "priority": 10000}, "group")
	if queue, _, _ := unstructured.NestedString(vcJob.Object, "spec", "queue"); queue != "gpu" {
		t.Fatalf("vcjob queue = %q, want gpu", queue)
	}
	if scheduler, _, _ := unstructured.NestedString(vcJob.Object, "spec", "schedulerName"); scheduler != volcanoSchedulerName {
		t.Fatalf("vcjob scheduler = %q, want volcano", scheduler)
	}
	if priority, _, _ := unstructured.NestedString(vcJob.Object, "spec", "priorityClassName"); priority != "platform-batch-high" {
		t.Fatalf("vcjob priority = %q, want platform-batch-high", priority)
	}
	tasks, _, _ := unstructured.NestedSlice(vcJob.Object, "spec", "tasks")
	task := tasks[0].(map[string]any)
	annotations, _, _ := unstructured.NestedStringMap(task, "template", "metadata", "annotations")
	if annotations[volcanoQueueAnnotationKey] != "gpu" {
		t.Fatalf("task annotations = %#v, want queue annotation", annotations)
	}
	if priority, _, _ := unstructured.NestedString(task, "template", "spec", "priorityClassName"); priority != "platform-batch-high" {
		t.Fatalf("task priority = %q, want propagated priority", priority)
	}
}
