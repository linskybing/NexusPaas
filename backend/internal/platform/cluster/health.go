package cluster

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// errClusterNotConfigured is returned by Ping when no Kubernetes client is wired,
// i.e. the facade is in degraded (no-op) mode.
var errClusterNotConfigured = errors.New("kubernetes client is not configured")

// Configured reports whether the facade has a live Kubernetes client. A nil receiver
// or a nil clientset means the process is running in degraded mode (no cluster), which
// readiness uses to fail closed for cluster-dependent services.
func (c *Client) Configured() bool {
	return c != nil && c.clientset != nil
}

// Ping performs a context-bounded connectivity check against the API server using the
// same namespace-list access the reapers/reconcilers already rely on. It returns an
// error when the client is unconfigured or the cluster is unreachable, so readiness can
// fail closed. The fake clientset returns an empty list without error, so unit tests
// pass without a live cluster.
func (c *Client) Ping(ctx context.Context) error {
	if !c.Configured() {
		return errClusterNotConfigured
	}
	_, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	return err
}
