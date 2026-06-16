package cluster

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelDRAEffectiveGPU = "platform-go/dra-effective-gpu"
	LabelDRAGPUCount     = "platform-go/dra-gpu-count"
)

// PodResourceUsage is the Kubernetes-derived input for usage accounting. It is
// intentionally independent of the resource-hours store shape so the cluster
// facade remains a pure adapter.
type PodResourceUsage struct {
	JobID             string
	ProjectID         string
	UserID            string
	Namespace         string
	Name              string
	UID               string
	RequestedGPU      float64
	RequestedCPU      float64
	RequestedMemoryMB float64
	ScheduledAt       time.Time
	RunningAt         *time.Time
	TerminatedAt      *time.Time
	LastSeenAt        time.Time
	Phase             string
	IsActive          bool
}

// ListJobPodResourceUsage lists all pods carrying the platform job label and
// extracts the same billable resource fields the reference resource-hours worker
// scans from Kubernetes.
func (c *Client) ListJobPodResourceUsage(ctx context.Context, now time.Time) ([]PodResourceUsage, error) {
	if c == nil || c.clientset == nil {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{LabelSelector: LabelJobID})
	if err != nil {
		return nil, fmt.Errorf("list job pods: %w", err)
	}
	out := make([]PodResourceUsage, 0, len(pods.Items))
	for i := range pods.Items {
		if usage, ok := podResourceUsage(&pods.Items[i], now); ok {
			out = append(out, usage)
		}
	}
	return out, nil
}

func podResourceUsage(pod *corev1.Pod, now time.Time) (PodResourceUsage, bool) {
	jobID := pod.Labels[LabelJobID]
	if jobID == "" {
		return PodResourceUsage{}, false
	}
	gpu, cpu, memMB := podRequestedResources(pod)
	usage := PodResourceUsage{
		JobID:             jobID,
		ProjectID:         pod.Labels[LabelProjectID],
		UserID:            pod.Labels[LabelUserID],
		Namespace:         pod.Namespace,
		Name:              pod.Name,
		UID:               string(pod.UID),
		RequestedGPU:      gpu,
		RequestedCPU:      cpu,
		RequestedMemoryMB: memMB,
		ScheduledAt:       scheduledAtFromPod(pod, now),
		LastSeenAt:        now,
		Phase:             string(pod.Status.Phase),
		IsActive:          podActive(pod),
	}
	if runningAt, ok := runningStartedAtFromPod(pod); ok {
		usage.RunningAt = &runningAt
	}
	if terminatedAt, ok := terminatedAtFromPod(pod); ok {
		usage.TerminatedAt = &terminatedAt
	}
	if !usage.IsActive && usage.TerminatedAt == nil {
		terminatedAt := now
		usage.TerminatedAt = &terminatedAt
	}
	return usage, true
}

func podRequestedResources(pod *corev1.Pod) (float64, float64, float64) {
	if pod == nil {
		return 0, 0, 0
	}
	gpu := podRequestedGPU(pod)
	var cpu, memMB float64
	for _, container := range pod.Spec.Containers {
		requests := container.Resources.Requests
		cpu += requests.Cpu().AsApproximateFloat64()
		memMB += float64(requests.Memory().Value()) / (1024.0 * 1024.0)
	}
	return gpu, cpu, memMB
}

func podRequestedGPU(pod *corev1.Pod) float64 {
	for _, label := range []string{LabelDRAEffectiveGPU, LabelDRAGPUCount} {
		if value, err := strconv.ParseFloat(pod.Labels[label], 64); err == nil && value > 0 {
			return value
		}
	}
	var gpu float64
	for _, container := range pod.Spec.Containers {
		for name, quantity := range container.Resources.Requests {
			if string(name) == gpuResourceName {
				gpu += quantity.AsApproximateFloat64()
			}
		}
	}
	return gpu
}

func scheduledAtFromPod(pod *corev1.Pod, now time.Time) time.Time {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionTrue && !condition.LastTransitionTime.IsZero() {
			return condition.LastTransitionTime.Time
		}
	}
	if runningAt, ok := runningStartedAtFromPod(pod); ok {
		return runningAt
	}
	if pod.Status.StartTime != nil {
		return pod.Status.StartTime.Time
	}
	if !pod.CreationTimestamp.IsZero() {
		return pod.CreationTimestamp.Time
	}
	return now
}

func runningStartedAtFromPod(pod *corev1.Pod) (time.Time, bool) {
	if timestamp, ok := earliestContainerStart(pod.Status.ContainerStatuses); ok {
		return timestamp, true
	}
	for _, condition := range pod.Status.Conditions {
		if conditionReady(condition) {
			return condition.LastTransitionTime.Time, true
		}
	}
	return time.Time{}, false
}

func earliestContainerStart(statuses []corev1.ContainerStatus) (time.Time, bool) {
	var earliest *time.Time
	for i := range statuses {
		startedAt, ok := containerStartedAt(statuses[i])
		if !ok {
			continue
		}
		if earliest == nil || startedAt.Before(*earliest) {
			value := startedAt
			earliest = &value
		}
	}
	if earliest == nil {
		return time.Time{}, false
	}
	return *earliest, true
}

func containerStartedAt(status corev1.ContainerStatus) (time.Time, bool) {
	switch {
	case status.State.Running != nil && !status.State.Running.StartedAt.IsZero():
		return status.State.Running.StartedAt.Time, true
	case status.State.Terminated != nil && !status.State.Terminated.StartedAt.IsZero():
		return status.State.Terminated.StartedAt.Time, true
	default:
		return time.Time{}, false
	}
}

func conditionReady(condition corev1.PodCondition) bool {
	return (condition.Type == corev1.ContainersReady || condition.Type == corev1.PodReady) &&
		condition.Status == corev1.ConditionTrue &&
		!condition.LastTransitionTime.IsZero()
}

func terminatedAtFromPod(pod *corev1.Pod) (time.Time, bool) {
	var latest *time.Time
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated == nil || status.State.Terminated.FinishedAt.IsZero() {
			continue
		}
		finishedAt := status.State.Terminated.FinishedAt.Time
		if latest == nil || finishedAt.After(*latest) {
			value := finishedAt
			latest = &value
		}
	}
	if latest == nil {
		return time.Time{}, false
	}
	return *latest, true
}

func podActive(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}
	return !isTerminalPhase(pod.Status.Phase)
}
