package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrUnavailable     = errors.New("cluster client unavailable")
	ErrInvalidManifest = errors.New("invalid Kubernetes manifest")
	ErrUnsupportedKind = errors.New("unsupported Kubernetes manifest kind")
)

// CreatedObject identifies a Kubernetes object created from a submitted
// workload manifest.
type CreatedObject struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

type manifestTypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
}

// EnsureNamespace creates namespace when it is absent. It mirrors the reference
// executor's ensure-namespace step but remains injectable and fake-client
// testable.
func (c *Client) EnsureNamespace(ctx context.Context, namespace string) error {
	if c == nil || c.clientset == nil {
		return ErrUnavailable
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidManifest)
	}
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get namespace %s: %w", namespace, err)
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	if _, err := c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", namespace, err)
	}
	return nil
}

// CreateByJSON creates a Kubernetes object from a JSON manifest. The facade
// supports the native kinds the scheduler dispatcher can submit plus scheduler
// CRDs through the optional dynamic client. Existing objects are treated as
// success so retries are idempotent.
func (c *Client) CreateByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	if c == nil || c.clientset == nil {
		return CreatedObject{}, ErrUnavailable
	}
	meta, err := decodeManifestType(raw)
	if err != nil {
		return CreatedObject{}, err
	}
	namespace = manifestNamespace(meta, namespace)
	if target, ok := schedulerManifestTarget(meta); ok {
		return c.createDynamicByJSON(ctx, namespace, raw, target)
	}
	switch strings.ToLower(meta.Kind) {
	case "pod":
		return c.createPodByJSON(ctx, namespace, raw)
	case "deployment":
		return c.createDeploymentByJSON(ctx, namespace, raw)
	case "job":
		return c.createJobByJSON(ctx, namespace, raw)
	case "configmap":
		return c.createConfigMapByJSON(ctx, namespace, raw)
	case "secret":
		return c.createSecretByJSON(ctx, namespace, raw)
	case "service":
		return c.createServiceByJSON(ctx, namespace, raw)
	case "ingress":
		return c.createIngressByJSON(ctx, namespace, raw)
	case "namespace":
		return c.createNamespaceByJSON(ctx, meta)
	default:
		return CreatedObject{}, fmt.Errorf("%w: %s", ErrUnsupportedKind, meta.Kind)
	}
}

func (c *Client) createPodByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var pod corev1.Pod
	if err := decodeNativeManifest(raw, "pod", &pod); err != nil {
		return CreatedObject{}, err
	}
	pod.Namespace = namespace
	_, err := c.clientset.CoreV1().Pods(namespace).Create(ctx, &pod, metav1.CreateOptions{})
	return createdObject("Pod", namespace, pod.Name), ignoreAlreadyExists(err)
}

func (c *Client) createDeploymentByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var deployment appsv1.Deployment
	if err := decodeNativeManifest(raw, "deployment", &deployment); err != nil {
		return CreatedObject{}, err
	}
	deployment.Namespace = namespace
	_, err := c.clientset.AppsV1().Deployments(namespace).Create(ctx, &deployment, metav1.CreateOptions{})
	return createdObject("Deployment", namespace, deployment.Name), ignoreAlreadyExists(err)
}

func (c *Client) createJobByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var job batchv1.Job
	if err := decodeNativeManifest(raw, "job", &job); err != nil {
		return CreatedObject{}, err
	}
	job.Namespace = namespace
	_, err := c.clientset.BatchV1().Jobs(namespace).Create(ctx, &job, metav1.CreateOptions{})
	return createdObject("Job", namespace, job.Name), ignoreAlreadyExists(err)
}

func (c *Client) createConfigMapByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var cm corev1.ConfigMap
	if err := decodeNativeManifest(raw, "configmap", &cm); err != nil {
		return CreatedObject{}, err
	}
	cm.Namespace = namespace
	_, err := c.clientset.CoreV1().ConfigMaps(namespace).Create(ctx, &cm, metav1.CreateOptions{})
	return createdObject("ConfigMap", namespace, cm.Name), ignoreAlreadyExists(err)
}

func (c *Client) createSecretByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var secret corev1.Secret
	if err := decodeNativeManifest(raw, "secret", &secret); err != nil {
		return CreatedObject{}, err
	}
	secret.Namespace = namespace
	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, &secret, metav1.CreateOptions{})
	return createdObject("Secret", namespace, secret.Name), ignoreAlreadyExists(err)
}

func (c *Client) createServiceByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var svc corev1.Service
	if err := decodeNativeManifest(raw, "service", &svc); err != nil {
		return CreatedObject{}, err
	}
	svc.Namespace = namespace
	_, err := c.clientset.CoreV1().Services(namespace).Create(ctx, &svc, metav1.CreateOptions{})
	return createdObject("Service", namespace, svc.Name), ignoreAlreadyExists(err)
}

func (c *Client) createIngressByJSON(ctx context.Context, namespace string, raw []byte) (CreatedObject, error) {
	var ingress networkingv1.Ingress
	if err := decodeNativeManifest(raw, "ingress", &ingress); err != nil {
		return CreatedObject{}, err
	}
	ingress.Namespace = namespace
	_, err := c.clientset.NetworkingV1().Ingresses(namespace).Create(ctx, &ingress, metav1.CreateOptions{})
	return createdObject("Ingress", namespace, ingress.Name), ignoreAlreadyExists(err)
}

func (c *Client) createNamespaceByJSON(ctx context.Context, meta manifestTypeMeta) (CreatedObject, error) {
	ns := strings.TrimSpace(meta.Metadata.Name)
	if ns == "" {
		return CreatedObject{}, fmt.Errorf("%w: namespace metadata.name is required", ErrInvalidManifest)
	}
	return CreatedObject{Kind: "Namespace", Name: ns}, c.EnsureNamespace(ctx, ns)
}

func decodeNativeManifest(raw []byte, kind string, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidManifest, kind, err)
	}
	return nil
}

func decodeManifestType(raw []byte) (manifestTypeMeta, error) {
	var meta manifestTypeMeta
	if len(strings.TrimSpace(string(raw))) == 0 {
		return meta, fmt.Errorf("%w: empty manifest", ErrInvalidManifest)
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if strings.TrimSpace(meta.Kind) == "" {
		return meta, fmt.Errorf("%w: kind is required", ErrInvalidManifest)
	}
	return meta, nil
}

func manifestNamespace(meta manifestTypeMeta, fallback string) string {
	namespace := strings.TrimSpace(meta.Metadata.Namespace)
	if namespace != "" {
		return namespace
	}
	return strings.TrimSpace(fallback)
}

func createdObject(kind, namespace, name string) CreatedObject {
	return CreatedObject{Kind: kind, Namespace: namespace, Name: name}
}

func ignoreAlreadyExists(err error) error {
	if err == nil || apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
