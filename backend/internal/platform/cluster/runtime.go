package cluster

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuntimeResource is a platform-managed workload carrying a runtime-limit label,
// unified across Pods, Deployments and Jobs so the runtime reaper can apply one
// expiry rule to all kinds (reference cron.runtimeLimitedResource).
type RuntimeResource struct {
	Kind      string
	Namespace string
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
}

// ListRuntimeLimited lists every Pod, Deployment and Job carrying the
// runtime-limit-seconds label across all namespaces. Mirrors the reference
// runtime reaper's three list passes.
func (c *Client) ListRuntimeLimited(ctx context.Context) ([]RuntimeResource, error) {
	if c == nil || c.clientset == nil {
		return nil, nil
	}
	opts := listBy(RuntimeLimitSecondsKey)
	var out []RuntimeResource

	pods, err := c.clientset.CoreV1().Pods("").List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list runtime-limited pods: %w", err)
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		out = append(out, RuntimeResource{Kind: "Pod", Namespace: p.Namespace, Name: p.Name, Labels: p.Labels, CreatedAt: p.CreationTimestamp.Time})
	}

	deps, err := c.clientset.AppsV1().Deployments("").List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list runtime-limited deployments: %w", err)
	}
	for i := range deps.Items {
		d := &deps.Items[i]
		out = append(out, RuntimeResource{Kind: "Deployment", Namespace: d.Namespace, Name: d.Name, Labels: d.Labels, CreatedAt: d.CreationTimestamp.Time})
	}

	jobs, err := c.clientset.BatchV1().Jobs("").List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list runtime-limited jobs: %w", err)
	}
	for i := range jobs.Items {
		j := &jobs.Items[i]
		out = append(out, RuntimeResource{Kind: "Job", Namespace: j.Namespace, Name: j.Name, Labels: j.Labels, CreatedAt: j.CreationTimestamp.Time})
	}
	return out, nil
}

// DeleteResource deletes a single workload of the given kind, swallowing NotFound.
// Jobs use background propagation so their child pods are reaped too.
func (c *Client) DeleteResource(ctx context.Context, kind, namespace, name string) error {
	if c == nil || c.clientset == nil {
		return nil
	}
	var err error
	switch kind {
	case "Pod":
		err = c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Deployment":
		err = c.clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Job":
		background := metav1.DeletePropagationBackground
		err = c.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &background})
	default:
		return fmt.Errorf("unsupported runtime resource kind %q", kind)
	}
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("delete %s %s/%s: %w", kind, namespace, name, err)
	}
	return nil
}
