package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	JobLifecycleQueued    = "queued"
	JobLifecycleRunning   = "running"
	JobLifecycleCompleted = "completed"
	JobLifecycleFailed    = "failed"
)

// JobLifecycle is the cluster-observed lifecycle state for native Kubernetes
// objects belonging to one platform job.
type JobLifecycle struct {
	JobID       string
	Namespace   string
	Status      string
	Reason      string
	StartedAt   *time.Time
	CompletedAt *time.Time
	Found       bool
}

// NativeJobLifecycle reads native Kubernetes objects and supported scheduler CRDs
// carrying the platform job label, then maps them to the scheduler-facing job
// lifecycle vocabulary.
func (c *Client) NativeJobLifecycle(ctx context.Context, namespace, jobID string) (JobLifecycle, error) {
	status := JobLifecycle{JobID: jobID, Namespace: namespace}
	if c == nil || c.clientset == nil || strings.TrimSpace(jobID) == "" {
		return status, nil
	}
	selector := LabelJobID + "=" + jobID
	jobs, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return status, fmt.Errorf("list jobs for lifecycle: %w", err)
	}
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return status, fmt.Errorf("list pods for lifecycle: %w", err)
	}
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return status, fmt.Errorf("list deployments for lifecycle: %w", err)
	}
	native := collapseNativeLifecycle(status, jobs.Items, pods.Items, deployments.Items)
	observed := make([]JobLifecycle, 0, 1)
	if native.Found {
		observed = append(observed, native)
	}
	volcano, err := c.volcanoLifecycleStatuses(ctx, namespace, jobID)
	if err != nil {
		return status, err
	}
	observed = append(observed, volcano...)
	return collapseLifecycleStatuses(status, observed), nil
}

func collapseNativeLifecycle(
	base JobLifecycle,
	jobs []batchv1.Job,
	pods []corev1.Pod,
	deployments []appsv1.Deployment,
) JobLifecycle {
	statuses := []JobLifecycle{}
	for i := range jobs {
		statuses = append(statuses, lifecycleFromBatchJob(&jobs[i]))
	}
	for i := range pods {
		statuses = append(statuses, lifecycleFromPod(&pods[i]))
	}
	for i := range deployments {
		statuses = append(statuses, lifecycleFromDeployment(&deployments[i]))
	}
	return collapseLifecycleStatuses(base, statuses)
}

func collapseLifecycleStatuses(base JobLifecycle, statuses []JobLifecycle) JobLifecycle {
	if len(statuses) == 0 {
		return base
	}
	base.Found = true
	for _, status := range statuses {
		base = mergeLifecycleTimes(base, status)
	}
	if failed, ok := firstLifecycleStatus(statuses, JobLifecycleFailed); ok {
		base.Status = JobLifecycleFailed
		base.Reason = failed.Reason
		base.CompletedAt = firstTime(base.CompletedAt, failed.CompletedAt)
		return base
	}
	if running, ok := firstLifecycleStatus(statuses, JobLifecycleRunning); ok {
		base.Status = JobLifecycleRunning
		base.Reason = running.Reason
		return base
	}
	if queued, ok := firstLifecycleStatus(statuses, JobLifecycleQueued); ok {
		base.Status = JobLifecycleQueued
		base.Reason = queued.Reason
		return base
	}
	base.Status = JobLifecycleCompleted
	if completed, ok := firstLifecycleStatus(statuses, JobLifecycleCompleted); ok {
		base.Reason = completed.Reason
		base.CompletedAt = firstTime(base.CompletedAt, completed.CompletedAt)
	}
	return base
}

func lifecycleFromBatchJob(job *batchv1.Job) JobLifecycle {
	status := JobLifecycle{Found: true, Status: JobLifecycleQueued, Reason: "Kubernetes Job is queued"}
	if job == nil {
		return status
	}
	status.StartedAt = timePtr(job.Status.StartTime)
	status.CompletedAt = timePtr(job.Status.CompletionTime)
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			status.Status = JobLifecycleCompleted
			status.Reason = lifecycleReason(condition.Reason, condition.Message, "Kubernetes Job completed")
			return status
		case batchv1.JobFailed:
			status.Status = JobLifecycleFailed
			status.Reason = lifecycleReason(condition.Reason, condition.Message, "Kubernetes Job failed")
			return status
		}
	}
	if job.Status.Active > 0 {
		status.Status = JobLifecycleRunning
		status.Reason = "Kubernetes Job has active pods"
	}
	return status
}

func lifecycleFromPod(pod *corev1.Pod) JobLifecycle {
	status := JobLifecycle{Found: true, Status: JobLifecycleQueued, Reason: "Pod is queued"}
	if pod == nil {
		return status
	}
	status.StartedAt = timePtr(pod.Status.StartTime)
	if completedAt, ok := terminatedAtFromPod(pod); ok {
		status.CompletedAt = &completedAt
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		status.Status = JobLifecycleCompleted
		status.Reason = "Pod succeeded"
	case corev1.PodFailed:
		status.Status = JobLifecycleFailed
		status.Reason = "Pod failed"
	case corev1.PodRunning:
		status.Status = JobLifecycleRunning
		status.Reason = "Pod is running"
	default:
		status.Status = JobLifecycleQueued
		status.Reason = "Pod is queued"
	}
	return status
}

func lifecycleFromDeployment(deployment *appsv1.Deployment) JobLifecycle {
	status := JobLifecycle{Found: true, Status: JobLifecycleQueued, Reason: "Deployment is queued"}
	if deployment == nil {
		return status
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse {
			status.Status = JobLifecycleFailed
			status.Reason = lifecycleReason(condition.Reason, condition.Message, "Deployment failed progressing")
			completedAt := condition.LastUpdateTime.Time
			status.CompletedAt = &completedAt
			return status
		}
	}
	if deployment.Status.AvailableReplicas > 0 || deployment.Status.ReadyReplicas > 0 {
		status.Status = JobLifecycleRunning
		status.Reason = "Deployment has available replicas"
	}
	return status
}

func firstLifecycleStatus(statuses []JobLifecycle, value string) (JobLifecycle, bool) {
	for _, status := range statuses {
		if status.Status == value {
			return status, true
		}
	}
	return JobLifecycle{}, false
}

func mergeLifecycleTimes(base, observed JobLifecycle) JobLifecycle {
	if observed.StartedAt != nil && (base.StartedAt == nil || observed.StartedAt.Before(*base.StartedAt)) {
		base.StartedAt = observed.StartedAt
	}
	if observed.CompletedAt != nil && (base.CompletedAt == nil || observed.CompletedAt.After(*base.CompletedAt)) {
		base.CompletedAt = observed.CompletedAt
	}
	return base
}

func lifecycleReason(reason, message, fallback string) string {
	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	switch {
	case reason != "" && message != "":
		return reason + ": " + message
	case message != "":
		return message
	case reason != "":
		return reason
	default:
		return fallback
	}
}

func firstTime(left, right *time.Time) *time.Time {
	if left != nil {
		return left
	}
	return right
}

func timePtr(value *metav1.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	t := value.Time
	return &t
}
