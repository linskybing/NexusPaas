package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is the injectable cluster facade. It wraps a client-go
// kubernetes.Interface (real in production, fake in tests) plus the configured
// project-namespace prefix used to scope listing operations.
type Client struct {
	clientset                     kubernetes.Interface
	dynamicClient                 dynamic.Interface
	nsPrefix                      string
	shareConfig                   volumeShareConfig
	longhornShareEndpointResolver func(context.Context, string) (string, error)
}

// New builds a Client over an existing kubernetes.Interface. Tests pass
// fake.NewSimpleClientset(); production passes a real clientset. nsPrefix defaults
// to "proj" when empty.
func New(clientset kubernetes.Interface, nsPrefix string) *Client {
	return NewWithDynamic(clientset, nil, nsPrefix)
}

// NewWithDynamic builds a Client with both typed and dynamic Kubernetes clients.
// The dynamic client is optional in tests and degraded deployments, but is needed
// for CRD-backed resources such as Volcano VCJobs and PodGroups.
func NewWithDynamic(clientset kubernetes.Interface, dynamicClient dynamic.Interface, nsPrefix string) *Client {
	prefix := strings.Trim(strings.TrimSpace(nsPrefix), "-")
	if prefix == "" {
		prefix = DefaultProjectNamespaceP
	}
	c := &Client{clientset: clientset, dynamicClient: dynamicClient, nsPrefix: prefix, shareConfig: volumeShareConfigFromEnv()}
	c.longhornShareEndpointResolver = c.resolveLonghornShareEndpoint
	return c
}

// Clientset exposes the underlying interface for callers that need typed access
// beyond the helper methods (e.g. future reconcilers).
func (c *Client) Clientset() kubernetes.Interface { return c.clientset }

// DynamicClient exposes the optional dynamic client for CRD-oriented callers and
// tests that need to inspect custom resources.
func (c *Client) DynamicClient() dynamic.Interface { return c.dynamicClient }

// NewFromEnv constructs a real cluster Client from the ambient Kubernetes config:
// in-cluster service-account config when running inside a pod, otherwise the
// kubeconfig referenced by KUBECONFIG. It returns (nil, nil) when no cluster
// configuration is present so the composition root can run in degraded mode
// without a cluster, exactly as the reference treats a nil Clientset.
func NewFromEnv(nsPrefix string) (*Client, error) {
	cfg, err := restConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes clientset: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes dynamic client: %w", err)
	}
	return NewWithDynamic(clientset, dynamicClient, nsPrefix), nil
}

// restConfig resolves the cluster REST config, returning (nil, nil) when neither
// in-cluster credentials nor a kubeconfig are available.
func restConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	} else if err != rest.ErrNotInCluster {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	kubeconfig := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if kubeconfig == "" {
		return nil, nil
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig %q: %w", kubeconfig, err)
	}
	return cfg, nil
}

// projectNamespacePrefix returns the lowercase prefix shared by all user-scoped
// namespaces of a project, e.g. "proj-<projectID>-".
func (c *Client) projectNamespacePrefix(projectID string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s-", c.nsPrefix, projectID))
}
