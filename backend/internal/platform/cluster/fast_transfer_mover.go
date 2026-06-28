package cluster

import (
	"context"
	"fmt"
	"path"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	FastTransferMoverDefaultImage  = "instrumentisto/rsync-ssh:alpine"
	FastTransferMoverContainerName = "fast-transfer-mover"
	FastTransferMoverToolRsync     = "rsync"

	FastTransferMoverActionCreated       = "created"
	FastTransferMoverActionAlreadyExists = "already_exists"
	FastTransferMoverActionInvalid       = "invalid"
	FastTransferMoverActionFailed        = "failed"
	FastTransferMoverActionDegraded      = "degraded"
)

const (
	fastTransferMoverSourceVolume = "source-pvc"
	fastTransferMoverTargetVolume = "target-pvc"
	fastTransferMoverSourceMount  = "/mnt/source"
	fastTransferMoverTargetMount  = "/mnt/target"

	fastTransferMoverManagedByLabel = "app.kubernetes.io/managed-by"
	fastTransferMoverPartOfLabel    = "app.kubernetes.io/part-of"
	fastTransferMoverComponentLabel = "app.kubernetes.io/component"
	fastTransferMoverOwnerLabel     = "nexuspaas.io/owner"

	fastTransferMoverManagedByValue = "platform-backend"
	fastTransferMoverPartOfValue    = "platform"
	fastTransferMoverComponentValue = "fast-transfer-mover"
	fastTransferMoverOwnerValue     = "k8s-control-service"

	fastTransferMoverManagedAnnotation = "nexuspaas.io/managed-resource"
	fastTransferMoverTransferID        = "nexuspaas.io/fast-transfer-id"
)

type FastTransferMoverEndpoint struct {
	Namespace string
	PVC       string
	Path      string
}

type FastTransferMoverJobOptions struct {
	ProjectID   string
	TransferID  string
	Namespace   string
	Name        string
	Source      FastTransferMoverEndpoint
	Target      FastTransferMoverEndpoint
	Tool        string
	Image       string
	ProgressURL string
}

type FastTransferMoverJobResult struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (c *Client) EnsureFastTransferMoverJob(ctx context.Context, opts FastTransferMoverJobOptions) FastTransferMoverJobResult {
	opts = normalizeFastTransferMoverOptions(opts)
	result := FastTransferMoverJobResult{Namespace: opts.Namespace, Name: opts.Name}
	if err := validateFastTransferMoverOptions(opts); err != nil {
		result.Action = FastTransferMoverActionInvalid
		result.Reason = err.Error()
		return result
	}
	if c == nil || c.clientset == nil {
		result.Action = FastTransferMoverActionDegraded
		result.Reason = "cluster client unavailable"
		return result
	}

	desired := buildFastTransferMoverJob(opts)
	existing, err := c.clientset.BatchV1().Jobs(opts.Namespace).Get(ctx, opts.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := c.clientset.BatchV1().Jobs(opts.Namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return failedFastTransferMoverResult(result, "create", err)
		}
		result.Action = FastTransferMoverActionCreated
		return result
	}
	if err != nil {
		return failedFastTransferMoverResult(result, "get", err)
	}
	if fastTransferMoverJobMatches(existing, opts.TransferID) {
		result.Action = FastTransferMoverActionAlreadyExists
		return result
	}
	result.Action = FastTransferMoverActionFailed
	result.Reason = "conflicting existing mover job"
	return result
}

func normalizeFastTransferMoverOptions(opts FastTransferMoverJobOptions) FastTransferMoverJobOptions {
	opts.ProjectID = strings.TrimSpace(opts.ProjectID)
	opts.TransferID = strings.TrimSpace(opts.TransferID)
	opts.Namespace = strings.TrimSpace(opts.Namespace)
	opts.Name = strings.TrimSpace(opts.Name)
	if opts.Name != "" && !strings.HasPrefix(opts.Name, "fast-transfer-") {
		opts.Name = "fast-transfer-" + opts.Name
	}
	opts.Source = normalizeFastTransferMoverEndpoint(opts.Source)
	opts.Target = normalizeFastTransferMoverEndpoint(opts.Target)
	opts.Tool = strings.TrimSpace(opts.Tool)
	if opts.Tool == "" {
		opts.Tool = FastTransferMoverToolRsync
	}
	opts.Image = strings.TrimSpace(opts.Image)
	if opts.Image == "" {
		opts.Image = FastTransferMoverDefaultImage
	}
	opts.ProgressURL = strings.TrimSpace(opts.ProgressURL)
	return opts
}

func normalizeFastTransferMoverEndpoint(endpoint FastTransferMoverEndpoint) FastTransferMoverEndpoint {
	endpoint.Namespace = strings.TrimSpace(endpoint.Namespace)
	endpoint.PVC = strings.TrimSpace(endpoint.PVC)
	endpoint.Path = path.Clean("/" + strings.TrimSpace(endpoint.Path))
	return endpoint
}

func validateFastTransferMoverOptions(opts FastTransferMoverJobOptions) error {
	if opts.ProjectID == "" || opts.TransferID == "" {
		return fmt.Errorf("project_id and transfer_id are required")
	}
	if !validDNS1123Label(opts.Namespace) || !validDNS1123Label(opts.Name) {
		return fmt.Errorf("invalid namespace or job name")
	}
	if opts.Source.Namespace != opts.Namespace || opts.Target.Namespace != opts.Namespace {
		return fmt.Errorf("source and target namespaces must match job namespace")
	}
	if !validDNS1123Label(opts.Source.PVC) || !validDNS1123Label(opts.Target.PVC) {
		return fmt.Errorf("invalid pvc")
	}
	if !validFastTransferMoverPath(opts.Source.Path) || !validFastTransferMoverPath(opts.Target.Path) {
		return fmt.Errorf("invalid path")
	}
	if opts.Tool != FastTransferMoverToolRsync {
		return fmt.Errorf("tool is not allowed")
	}
	if opts.Image == "" {
		return fmt.Errorf("image required")
	}
	return nil
}

func validDNS1123Label(value string) bool {
	return len(validation.IsDNS1123Label(value)) == 0
}

func validFastTransferMoverPath(value string) bool {
	if value == "" || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\x00\n\r\t'") {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '/', '.', '_', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func buildFastTransferMoverJob(opts FastTransferMoverJobOptions) *batchv1.Job {
	automountServiceAccountToken := false
	backoffLimit := int32(1)
	labels := fastTransferMoverManagedLabels()
	annotations := map[string]string{
		fastTransferMoverManagedAnnotation: fastTransferMoverComponentValue,
		fastTransferMoverTransferID:        opts.TransferID,
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Namespace:   opts.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: &automountServiceAccountToken,
					Containers: []corev1.Container{{
						Name:    FastTransferMoverContainerName,
						Image:   opts.Image,
						Command: []string{"/bin/sh", "-c"},
						Args:    []string{fastTransferMoverScript(opts)},
						VolumeMounts: []corev1.VolumeMount{
							{Name: fastTransferMoverSourceVolume, MountPath: fastTransferMoverSourceMount, ReadOnly: true},
							{Name: fastTransferMoverTargetVolume, MountPath: fastTransferMoverTargetMount},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: fastTransferMoverSourceVolume,
							VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: opts.Source.PVC,
								ReadOnly:  true,
							}},
						},
						{
							Name: fastTransferMoverTargetVolume,
							VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: opts.Target.PVC,
							}},
						},
					},
				},
			},
		},
	}
}

func fastTransferMoverScript(opts FastTransferMoverJobOptions) string {
	source := path.Join(fastTransferMoverSourceMount, strings.TrimPrefix(opts.Source.Path, "/")) + "/"
	target := path.Join(fastTransferMoverTargetMount, strings.TrimPrefix(opts.Target.Path, "/")) + "/"
	return fmt.Sprintf("set -eu\nrsync -a --delete -- %q %q", source, target)
}

func fastTransferMoverManagedLabels() map[string]string {
	return map[string]string{
		fastTransferMoverManagedByLabel: fastTransferMoverManagedByValue,
		fastTransferMoverPartOfLabel:    fastTransferMoverPartOfValue,
		fastTransferMoverComponentLabel: fastTransferMoverComponentValue,
		fastTransferMoverOwnerLabel:     fastTransferMoverOwnerValue,
	}
}

func fastTransferMoverJobMatches(job *batchv1.Job, transferID string) bool {
	if job == nil || job.Annotations[fastTransferMoverTransferID] != transferID {
		return false
	}
	for key, want := range fastTransferMoverManagedLabels() {
		if job.Labels[key] != want {
			return false
		}
	}
	return job.Annotations[fastTransferMoverManagedAnnotation] == fastTransferMoverComponentValue
}

func failedFastTransferMoverResult(result FastTransferMoverJobResult, op string, err error) FastTransferMoverJobResult {
	result.Action = FastTransferMoverActionFailed
	result.Reason = op + " failed"
	result.Error = err.Error()
	return result
}
