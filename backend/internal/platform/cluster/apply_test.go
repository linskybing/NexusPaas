package cluster

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureNamespaceCreatesMissingNamespace(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")

	if err := cl.EnsureNamespace(context.Background(), "proj-p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Clientset().CoreV1().Namespaces().Get(context.Background(), "proj-p1", metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace was not created: %v", err)
	}
}

func TestCreateByJSONCreatesNativeObjectsIdempotently(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "proj-p1"}}), "proj")
	raw := []byte(`{
		"apiVersion":"batch/v1",
		"kind":"Job",
		"metadata":{"name":"train"},
		"spec":{"template":{"spec":{"restartPolicy":"Never","containers":[{"name":"main","image":"busybox"}]}}}
	}`)

	created, err := cl.CreateByJSON(ctx, "proj-p1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "Job" || created.Namespace != "proj-p1" || created.Name != "train" {
		t.Fatalf("created object = %#v, want batch job identity", created)
	}
	if _, err := cl.CreateByJSON(ctx, "proj-p1", raw); err != nil {
		t.Fatalf("idempotent create returned error: %v", err)
	}
	if _, err := cl.Clientset().BatchV1().Jobs("proj-p1").Get(ctx, "train", metav1.GetOptions{}); err != nil {
		t.Fatalf("job was not created: %v", err)
	}
}

func TestCreateByJSONCreatesDeploymentAndRejectsUnsupportedKind(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "proj-p1"}}), "proj")
	deploy := []byte(`{
		"apiVersion":"apps/v1",
		"kind":"Deployment",
		"metadata":{"name":"worker"},
		"spec":{"selector":{"matchLabels":{"app":"worker"}},"template":{"metadata":{"labels":{"app":"worker"}},"spec":{"containers":[{"name":"main","image":"busybox"}]}}}
	}`)

	if _, err := cl.CreateByJSON(ctx, "proj-p1", deploy); err != nil {
		t.Fatal(err)
	}
	got, err := cl.Clientset().AppsV1().Deployments("proj-p1").Get(ctx, "worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment was not created: %v", err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "busybox" {
		t.Fatalf("deployment = %#v, want submitted spec preserved", got)
	}
	_, err = cl.CreateByJSON(ctx, "proj-p1", []byte(`{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"name":"hourly"}}`))
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("unsupported kind error = %v, want ErrUnsupportedKind", err)
	}
}

func TestCreateByJSONCreatesRemainingNativeObjects(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "proj-p1"}}), "proj")
	cases := []struct {
		name string
		raw  []byte
		kind string
	}{
		{
			name: "pod",
			kind: "Pod",
			raw:  []byte(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"shell"},"spec":{"containers":[{"name":"main","image":"busybox"}]}}`),
		},
		{
			name: "configmap",
			kind: "ConfigMap",
			raw:  []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"settings"},"data":{"mode":"test"}}`),
		},
		{
			name: "secret",
			kind: "Secret",
			raw:  []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"creds"},"stringData":{"token":"secret"}}`),
		},
		{
			name: "service",
			kind: "Service",
			raw:  []byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"api"},"spec":{"ports":[{"port":80}]}}`),
		},
		{
			name: "ingress",
			kind: "Ingress",
			raw:  []byte(`{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"web"}}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := cl.CreateByJSON(ctx, "proj-p1", tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if created.Kind != tc.kind || created.Namespace != "proj-p1" || created.Name == "" {
				t.Fatalf("created object = %#v, want %s in proj-p1", created, tc.kind)
			}
		})
	}
}

func TestCreateByJSONRequiresConfiguredClientAndValidManifest(t *testing.T) {
	if err := (*Client)(nil).EnsureNamespace(context.Background(), "proj-p1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil EnsureNamespace err = %v, want ErrUnavailable", err)
	}
	if err := New(fake.NewSimpleClientset(), "proj").EnsureNamespace(context.Background(), " "); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("blank EnsureNamespace err = %v, want ErrInvalidManifest", err)
	}
	if _, err := (*Client)(nil).CreateByJSON(context.Background(), "proj-p1", []byte(`{"kind":"Pod"}`)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil CreateByJSON err = %v, want ErrUnavailable", err)
	}
	cl := New(fake.NewSimpleClientset(), "proj")
	if _, err := cl.CreateByJSON(context.Background(), "proj-p1", nil); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("empty manifest err = %v, want ErrInvalidManifest", err)
	}
	if _, err := cl.CreateByJSON(context.Background(), "proj-p1", []byte(`{`)); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("invalid manifest err = %v, want ErrInvalidManifest", err)
	}
	if _, err := cl.CreateByJSON(context.Background(), "proj-p1", []byte(`{"apiVersion":"v1","metadata":{"name":"missing-kind"}}`)); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("missing kind err = %v, want ErrInvalidManifest", err)
	}
}

func TestCreateByJSONNamespaceManifestCreatesNamespace(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")

	created, err := cl.CreateByJSON(context.Background(), "", []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"proj-p2"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "Namespace" || created.Name != "proj-p2" {
		t.Fatalf("created namespace = %#v, want proj-p2", created)
	}
	if _, err := cl.Clientset().CoreV1().Namespaces().Get(context.Background(), "proj-p2", metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace was not created: %v", err)
	}
}
