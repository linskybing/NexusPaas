package cluster

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// CleanupJobResources deletes all platform-managed resources belonging to a job,
// selected by the "platform-go/job-id=<jobID>" label, across the standard kinds.
// Mirrors reference pkg/k8s.CleanupJobResources for core Kubernetes kinds and
// the scheduler CRDs that this service can submit through the dynamic client.
func (c *Client) CleanupJobResources(ctx context.Context, namespace, jobID string) (CleanupResult, error) {
	var result CleanupResult
	if c == nil || c.clientset == nil || jobID == "" {
		return result, nil
	}
	selector := LabelJobID + "=" + jobID
	cs := c.clientset
	var errs []error
	result.Pods, errs = runDelete(errs, func() (int, error) { return deletePods(ctx, cs, namespace, selector) })
	result.Deployments, errs = runDelete(errs, func() (int, error) { return deleteDeployments(ctx, cs, namespace, selector) })
	result.StatefulSets, errs = runDelete(errs, func() (int, error) { return deleteStatefulSets(ctx, cs, namespace, selector) })
	result.Services, errs = runDelete(errs, func() (int, error) { return deleteServices(ctx, cs, namespace, selector) })
	result.Jobs, errs = runDelete(errs, func() (int, error) { return deleteJobs(ctx, cs, namespace, selector) })
	result.VCJobs, errs = runDelete(errs, func() (int, error) { return c.deleteDynamicResources(ctx, volcanoVCJobGVR, namespace, selector) })
	result.PodGroups, errs = runDelete(errs, func() (int, error) { return c.deleteDynamicResources(ctx, volcanoPodGroupGVR, namespace, selector) })
	result.ConfigMaps, errs = runDelete(errs, func() (int, error) { return deleteConfigMaps(ctx, cs, namespace, selector) })
	result.Secrets, errs = runDelete(errs, func() (int, error) { return deleteSecrets(ctx, cs, namespace, selector) })
	result.Ingresses, errs = runDelete(errs, func() (int, error) { return deleteIngresses(ctx, cs, namespace, selector) })
	return result, errors.Join(errs...)
}

func (c *Client) deleteDynamicResources(ctx context.Context, gvr schema.GroupVersionResource, namespace, selector string) (int, error) {
	if c == nil || c.dynamicClient == nil {
		return 0, nil
	}
	items, err := c.dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, listBy(selector))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("list %s: %w", gvr.Resource, err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].GetName())
	}
	return deleteByName(names, func(name string) error {
		return c.dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func runDelete(errs []error, fn func() (int, error)) (int, []error) {
	count, err := fn()
	if err != nil {
		errs = append(errs, err)
	}
	return count, errs
}

func deletePods(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.CoreV1().Pods(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list pods: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	return deleteByName(names, func(name string) error {
		return cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteDeployments(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.AppsV1().Deployments(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list deployments: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	return deleteByName(names, func(name string) error {
		return cs.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteStatefulSets(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.AppsV1().StatefulSets(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list statefulsets: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	return deleteByName(names, func(name string) error {
		return cs.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteServices(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.CoreV1().Services(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list services: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		if items.Items[i].Name != "kubernetes" {
			names = append(names, items.Items[i].Name)
		}
	}
	return deleteByName(names, func(name string) error {
		return cs.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteJobs(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.BatchV1().Jobs(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list jobs: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	background := metav1.DeletePropagationBackground
	options := metav1.DeleteOptions{PropagationPolicy: &background}
	return deleteByName(names, func(name string) error {
		return cs.BatchV1().Jobs(namespace).Delete(ctx, name, options)
	})
}

func deleteConfigMaps(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list configmaps: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	return deleteByName(names, func(name string) error {
		return cs.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteSecrets(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.CoreV1().Secrets(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list secrets: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	return deleteByName(names, func(name string) error {
		return cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteIngresses(ctx context.Context, cs kubernetes.Interface, namespace, selector string) (int, error) {
	items, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, listBy(selector))
	if err != nil {
		return 0, fmt.Errorf("list ingresses: %w", err)
	}
	names := make([]string, 0, len(items.Items))
	for i := range items.Items {
		names = append(names, items.Items[i].Name)
	}
	return deleteByName(names, func(name string) error {
		return cs.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

func deleteByName(names []string, delete func(string) error) (int, error) {
	var errs []error
	count := 0
	for _, name := range names {
		if err := delete(name); err != nil {
			if !isNotFound(err) {
				errs = append(errs, err)
			}
			continue
		}
		count++
	}
	return count, errors.Join(errs...)
}

func listBy(selector string) metav1.ListOptions {
	return metav1.ListOptions{LabelSelector: selector}
}
