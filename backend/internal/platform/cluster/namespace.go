package cluster

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListProjectNamespaces returns all namespace names matching the project prefix.
// It mirrors reference pkg/k8s.ListProjectNamespaces: a nil clientset yields no
// namespaces (degraded mode) rather than an error.
func (c *Client) ListProjectNamespaces(ctx context.Context, projectID string) ([]string, error) {
	if c == nil || c.clientset == nil {
		return nil, nil
	}
	prefix := c.projectNamespacePrefix(projectID)
	list, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	var result []string
	for i := range list.Items {
		if strings.HasPrefix(list.Items[i].Name, prefix) {
			result = append(result, list.Items[i].Name)
		}
	}
	return result, nil
}
