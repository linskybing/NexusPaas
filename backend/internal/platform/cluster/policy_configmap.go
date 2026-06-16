package cluster

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PolicyDataConfigMapName = "platform-policy-data"

	policyDataLabelManagedBy = "app.kubernetes.io/managed-by"
	policyDataLabelPartOf    = "app.kubernetes.io/part-of"
	policyDataLabelComponent = "app.kubernetes.io/component"
)

var policyDataConfigMapLabels = map[string]string{
	policyDataLabelManagedBy: "platform-backend",
	policyDataLabelPartOf:    "platform",
	policyDataLabelComponent: "policy-data",
}

// EnsurePolicyDataConfigMap creates or updates the runtime policy data ConfigMap
// in a project namespace. Nil clients are a no-op to match the existing cluster
// facade behavior for optional local Kubernetes dependencies.
func (c *Client) EnsurePolicyDataConfigMap(ctx context.Context, namespace string, data map[string]string) error {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return fmt.Errorf("policy data configmap requires namespace")
	}
	if c == nil || c.clientset == nil {
		return nil
	}
	configMaps := c.clientset.CoreV1().ConfigMaps(namespace)
	existing, err := configMaps.Get(ctx, PolicyDataConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get policy data configmap: %w", err)
		}
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:   PolicyDataConfigMapName,
				Labels: policyDataLabels(),
			},
			Data: cloneStringMap(data),
		}
		if _, err := configMaps.Create(ctx, configMap, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create policy data configmap: %w", err)
		}
		return nil
	}
	existing.Labels = mergeStringMaps(existing.Labels, policyDataLabels())
	existing.Data = cloneStringMap(data)
	if _, err := configMaps.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update policy data configmap: %w", err)
	}
	return nil
}

func policyDataLabels() map[string]string {
	return cloneStringMap(policyDataConfigMapLabels)
}

func cloneStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func mergeStringMaps(base, overlay map[string]string) map[string]string {
	out := cloneStringMap(base)
	for key, value := range overlay {
		out[key] = value
	}
	return out
}
