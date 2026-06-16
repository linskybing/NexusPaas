package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListPodsByLabel lists pods in a namespace matching the label selector. An empty
// namespace lists across all namespaces. Mirrors reference pkg/k8s.ListPodsByLabel.
func (c *Client) ListPodsByLabel(ctx context.Context, namespace, labelSelector string) ([]PodInfo, error) {
	if c == nil || c.clientset == nil {
		return nil, nil
	}
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	result := make([]PodInfo, 0, len(pods.Items))
	for i := range pods.Items {
		result = append(result, podInfo(&pods.Items[i]))
	}
	return result, nil
}

// DeletePod deletes a single pod, returning nil if it is already gone.
func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	if c == nil || c.clientset == nil {
		return nil
	}
	if err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete pod %s/%s: %w", namespace, name, err)
	}
	return nil
}

func podInfo(p *corev1.Pod) PodInfo {
	info := PodInfo{
		Name:        p.Name,
		Namespace:   p.Namespace,
		Phase:       string(p.Status.Phase),
		PodIP:       p.Status.PodIP,
		NodeName:    p.Spec.NodeName,
		Annotations: p.Annotations,
		Labels:      p.Labels,
	}
	if p.Status.StartTime != nil {
		info.StartTime = p.Status.StartTime.Format("2006-01-02T15:04:05Z")
	}
	for _, cs := range p.Status.ContainerStatuses {
		state := "waiting"
		switch {
		case cs.State.Running != nil:
			state = "running"
		case cs.State.Terminated != nil:
			state = "terminated"
		}
		info.Containers = append(info.Containers, ContainerStatusInfo{
			Name: cs.Name, Ready: cs.Ready, RestartCount: cs.RestartCount, State: state,
		})
	}
	return info
}
