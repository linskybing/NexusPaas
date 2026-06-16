package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureResourceQuota creates or updates a namespace ResourceQuota to the given
// hard limits. Mirrors reference pkg/k8s.EnsureResourceQuota.
func (c *Client) EnsureResourceQuota(ctx context.Context, namespace, name string, hard corev1.ResourceList) error {
	if namespace == "" || name == "" {
		return fmt.Errorf("resource quota requires namespace and name")
	}
	if c == nil || c.clientset == nil {
		return nil
	}
	quotas := c.clientset.CoreV1().ResourceQuotas(namespace)
	existing, err := quotas.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get resource quota: %w", err)
		}
		quota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       corev1.ResourceQuotaSpec{Hard: hard},
		}
		if _, err := quotas.Create(ctx, quota, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create resource quota: %w", err)
		}
		return nil
	}
	existing.Spec.Hard = hard
	if _, err := quotas.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update resource quota: %w", err)
	}
	return nil
}

// BuildQuotaResources returns the ResourceList for the given limits. GPU is
// intentionally excluded (enforced by admission, not ResourceQuota), matching the
// reference rationale: rq.Status.Used has GC lag for terminated pods.
func BuildQuotaResources(_ float64, cpuCores, memoryGiB float64, pods int) corev1.ResourceList {
	resources := corev1.ResourceList{}
	if cpuCores > 0 {
		resources[corev1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%g", cpuCores))
	}
	if memoryGiB > 0 {
		resources[corev1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%gGi", memoryGiB))
	}
	if pods > 0 {
		resources[corev1.ResourcePods] = *resource.NewQuantity(int64(pods), resource.DecimalSI)
	}
	return resources
}

func isNotFound(err error) bool { return apierrors.IsNotFound(err) }
