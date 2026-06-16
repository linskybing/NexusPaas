package cluster

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	DockerCleanupCronJobName      = "docker-image-cleanup"
	DockerCleanupContainerName    = "docker-cleanup"
	DockerCleanupDefaultNamespace = "default"
	DockerCleanupDefaultImage     = "docker:24-dind"
	DockerCleanupSchedule         = "0 2 * * *"
	DockerCleanupSocketPath       = "/var/run/docker.sock"
	DockerCleanupSocketVolumeName = "docker-sock"
)

const (
	DockerCleanupManagedByLabel = "app.kubernetes.io/managed-by"
	DockerCleanupPartOfLabel    = "app.kubernetes.io/part-of"
	DockerCleanupComponentLabel = "app.kubernetes.io/component"
	DockerCleanupOwnerLabel     = "nexuspaas.io/owner"

	DockerCleanupManagedByValue = "platform-backend"
	DockerCleanupPartOfValue    = "platform"
	DockerCleanupComponentValue = DockerCleanupCronJobName
	DockerCleanupOwnerValue     = "k8s-control-service"

	DockerCleanupManagedAnnotation = "nexuspaas.io/managed-resource"
	DockerCleanupManagedResource   = DockerCleanupCronJobName
)

const (
	DockerCleanupActionCreated   = "created"
	DockerCleanupActionUpdated   = "updated"
	DockerCleanupActionAdopted   = "adopted"
	DockerCleanupActionUnchanged = "unchanged"
	DockerCleanupActionInvalid   = "invalid"
	DockerCleanupActionConflict  = "conflict"
	DockerCleanupActionFailed    = "failed"
	DockerCleanupActionDegraded  = "degraded"
)

const dockerCleanupScript = `set -e
echo "Starting Docker cleanup..."
# Remove all unused images, containers, and networks older than 24 hours
docker system prune -af --filter "until=24h"
echo "Docker cleanup completed successfully"`

type DockerCleanupCronJobOptions struct {
	Namespace string
	Image     string
}

type DockerCleanupCronJobResult struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (c *Client) EnsureDockerCleanupCronJob(ctx context.Context, opts DockerCleanupCronJobOptions) DockerCleanupCronJobResult {
	opts = normalizeDockerCleanupOptions(opts)
	result := DockerCleanupCronJobResult{Namespace: opts.Namespace, Name: DockerCleanupCronJobName}
	if err := validateDockerCleanupOptions(opts); err != nil {
		result.Action = DockerCleanupActionInvalid
		result.Reason = err.Error()
		return result
	}
	if c == nil || c.clientset == nil {
		result.Action = DockerCleanupActionDegraded
		result.Reason = "cluster client unavailable"
		return result
	}

	desired := buildDockerCleanupCronJob(opts)
	existing, err := c.clientset.BatchV1().CronJobs(opts.Namespace).Get(ctx, DockerCleanupCronJobName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return c.createDockerCleanupCronJob(ctx, desired, result)
	}
	if err != nil {
		return failedDockerCleanupResult(result, "get", err)
	}

	managed, conflictReason := dockerCleanupManagedByPlatform(existing)
	if !managed {
		return c.adoptDockerCleanupCronJob(ctx, existing, desired, result, conflictReason)
	}
	return c.reconcileManagedDockerCleanupCronJob(ctx, existing, desired, result)
}

func normalizeDockerCleanupOptions(opts DockerCleanupCronJobOptions) DockerCleanupCronJobOptions {
	opts.Namespace = strings.TrimSpace(opts.Namespace)
	if opts.Namespace == "" {
		opts.Namespace = DockerCleanupDefaultNamespace
	}
	opts.Image = strings.TrimSpace(opts.Image)
	if opts.Image == "" {
		opts.Image = DockerCleanupDefaultImage
	}
	return opts
}

func validateDockerCleanupOptions(opts DockerCleanupCronJobOptions) error {
	if errs := validation.IsDNS1123Label(opts.Namespace); len(errs) > 0 {
		return fmt.Errorf("invalid namespace")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return fmt.Errorf("image required")
	}
	return nil
}

func (c *Client) createDockerCleanupCronJob(ctx context.Context, desired *batchv1.CronJob, result DockerCleanupCronJobResult) DockerCleanupCronJobResult {
	if _, err := c.clientset.BatchV1().CronJobs(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil {
		return failedDockerCleanupResult(result, "create", err)
	}
	result.Action = DockerCleanupActionCreated
	return result
}

func (c *Client) adoptDockerCleanupCronJob(ctx context.Context, existing, desired *batchv1.CronJob, result DockerCleanupCronJobResult, conflictReason string) DockerCleanupCronJobResult {
	if conflictReason != "" {
		result.Action = DockerCleanupActionConflict
		result.Reason = conflictReason
		return result
	}
	if !dockerCleanupCronJobSafelyAdoptable(existing, desired) {
		result.Action = DockerCleanupActionConflict
		result.Reason = "unmanaged cronjob has incompatible cleanup intent"
		return result
	}
	next := existing.DeepCopy()
	copyDockerCleanupCronJobMutableFields(next, desired)
	if _, err := c.clientset.BatchV1().CronJobs(desired.Namespace).Update(ctx, next, metav1.UpdateOptions{}); err != nil {
		return failedDockerCleanupResult(result, "adopt", err)
	}
	result.Action = DockerCleanupActionAdopted
	return result
}

func (c *Client) reconcileManagedDockerCleanupCronJob(ctx context.Context, existing, desired *batchv1.CronJob, result DockerCleanupCronJobResult) DockerCleanupCronJobResult {
	if dockerCleanupCronJobMutableEqual(existing, desired) {
		result.Action = DockerCleanupActionUnchanged
		return result
	}
	next := existing.DeepCopy()
	copyDockerCleanupCronJobMutableFields(next, desired)
	if _, err := c.clientset.BatchV1().CronJobs(desired.Namespace).Update(ctx, next, metav1.UpdateOptions{}); err != nil {
		return failedDockerCleanupResult(result, "update", err)
	}
	result.Action = DockerCleanupActionUpdated
	return result
}

func buildDockerCleanupCronJob(opts DockerCleanupCronJobOptions) *batchv1.CronJob {
	automountServiceAccountToken := false
	privileged := true
	labels := dockerCleanupManagedLabels()
	annotations := map[string]string{DockerCleanupManagedAnnotation: DockerCleanupManagedResource}
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        DockerCleanupCronJobName,
			Namespace:   opts.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: DockerCleanupSchedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyOnFailure,
							AutomountServiceAccountToken: &automountServiceAccountToken,
							Containers: []corev1.Container{{
								Name:    DockerCleanupContainerName,
								Image:   opts.Image,
								Command: []string{"/bin/sh", "-c"},
								Args:    []string{dockerCleanupScript},
								SecurityContext: &corev1.SecurityContext{
									Privileged: &privileged,
								},
								VolumeMounts: []corev1.VolumeMount{{
									Name:      DockerCleanupSocketVolumeName,
									MountPath: DockerCleanupSocketPath,
								}},
							}},
							Volumes: []corev1.Volume{{
								Name: DockerCleanupSocketVolumeName,
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{Path: DockerCleanupSocketPath},
								},
							}},
						},
					},
				},
			},
		},
	}
}

func dockerCleanupManagedLabels() map[string]string {
	return map[string]string{
		DockerCleanupManagedByLabel: DockerCleanupManagedByValue,
		DockerCleanupPartOfLabel:    DockerCleanupPartOfValue,
		DockerCleanupComponentLabel: DockerCleanupComponentValue,
		DockerCleanupOwnerLabel:     DockerCleanupOwnerValue,
	}
}

func dockerCleanupManagedByPlatform(cronJob *batchv1.CronJob) (bool, string) {
	labels := cronJob.GetLabels()
	allLabelsPresent := true
	for key, want := range dockerCleanupManagedLabels() {
		got, ok := labels[key]
		if ok && got != want {
			return false, "conflicting ownership marker " + key
		}
		if !ok {
			allLabelsPresent = false
		}
	}
	annotations := cronJob.GetAnnotations()
	annotation, annotationPresent := annotations[DockerCleanupManagedAnnotation]
	if annotationPresent && annotation != DockerCleanupManagedResource {
		return false, "conflicting ownership marker " + DockerCleanupManagedAnnotation
	}
	return allLabelsPresent && annotationPresent, ""
}

func dockerCleanupCronJobSafelyAdoptable(existing, desired *batchv1.CronJob) bool {
	return existing.Name == desired.Name &&
		existing.Namespace == desired.Namespace &&
		dockerCleanupCriticalIntentEqual(existing, desired)
}

func dockerCleanupCriticalIntentEqual(a, b *batchv1.CronJob) bool {
	aPod, bPod := a.Spec.JobTemplate.Spec.Template.Spec, b.Spec.JobTemplate.Spec.Template.Spec
	return a.Spec.Schedule == b.Spec.Schedule &&
		aPod.RestartPolicy == bPod.RestartPolicy &&
		len(aPod.Containers) == len(bPod.Containers) &&
		len(aPod.Volumes) == len(bPod.Volumes) &&
		dockerCleanupContainerIntentEqual(aPod.Containers, bPod.Containers) &&
		dockerCleanupSocketVolumeEqual(aPod.Volumes, bPod.Volumes)
}

func dockerCleanupContainerIntentEqual(a, b []corev1.Container) bool {
	aContainer, okA := dockerCleanupContainer(a)
	bContainer, okB := dockerCleanupContainer(b)
	if !okA || !okB {
		return false
	}
	return reflect.DeepEqual(aContainer.Command, bContainer.Command) &&
		reflect.DeepEqual(aContainer.Args, bContainer.Args) &&
		containerPrivileged(aContainer) &&
		reflect.DeepEqual(aContainer.VolumeMounts, bContainer.VolumeMounts)
}

func dockerCleanupContainer(containers []corev1.Container) (corev1.Container, bool) {
	for _, container := range containers {
		if container.Name == DockerCleanupContainerName {
			return container, true
		}
	}
	return corev1.Container{}, false
}

func containerPrivileged(container corev1.Container) bool {
	return container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged
}

func dockerCleanupSocketVolumeEqual(a, b []corev1.Volume) bool {
	aVolume, okA := dockerCleanupSocketVolume(a)
	bVolume, okB := dockerCleanupSocketVolume(b)
	if !okA || !okB {
		return false
	}
	return reflect.DeepEqual(aVolume, bVolume)
}

func dockerCleanupSocketVolume(volumes []corev1.Volume) (corev1.Volume, bool) {
	for _, volume := range volumes {
		if volume.Name == DockerCleanupSocketVolumeName && volume.HostPath != nil && volume.HostPath.Path == DockerCleanupSocketPath {
			return volume, true
		}
	}
	return corev1.Volume{}, false
}

func dockerCleanupCronJobMutableEqual(a, b *batchv1.CronJob) bool {
	return maps.Equal(a.Labels, b.Labels) &&
		maps.Equal(a.Annotations, b.Annotations) &&
		reflect.DeepEqual(a.Spec, b.Spec)
}

func copyDockerCleanupCronJobMutableFields(dst, src *batchv1.CronJob) {
	dst.Labels = maps.Clone(src.Labels)
	dst.Annotations = maps.Clone(src.Annotations)
	dst.Spec = src.Spec
}

func failedDockerCleanupResult(result DockerCleanupCronJobResult, operation string, err error) DockerCleanupCronJobResult {
	result.Action = DockerCleanupActionFailed
	result.Reason = operation + " cronjob failed"
	result.Error = err.Error()
	return result
}
