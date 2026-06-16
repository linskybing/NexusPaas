package cluster

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnsurePolicyDataConfigMapCreatesThenUpdates(t *testing.T) {
	c := newFakeClient()
	ctx := context.Background()
	initial := map[string]string{"timeAllowed": "false", "gpuLimit": "0"}

	if err := c.EnsurePolicyDataConfigMap(ctx, "proj-p1-alice", initial); err != nil {
		t.Fatal(err)
	}
	got, err := c.Clientset().CoreV1().ConfigMaps("proj-p1-alice").Get(ctx, PolicyDataConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Data["timeAllowed"] != "false" || got.Labels[policyDataLabelComponent] != "policy-data" {
		t.Fatalf("created configmap = %#v labels=%#v", got.Data, got.Labels)
	}

	if err := c.EnsurePolicyDataConfigMap(ctx, "proj-p1-alice", map[string]string{"timeAllowed": "true", "gpuLimit": "2"}); err != nil {
		t.Fatal(err)
	}
	got, err = c.Clientset().CoreV1().ConfigMaps("proj-p1-alice").Get(ctx, PolicyDataConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Data["timeAllowed"] != "true" || got.Data["gpuLimit"] != "2" {
		t.Fatalf("updated configmap data = %#v", got.Data)
	}
	if got.Labels[policyDataLabelManagedBy] != "platform-backend" || got.Labels[policyDataLabelPartOf] != "platform" {
		t.Fatalf("updated labels = %#v", got.Labels)
	}
}

func TestEnsurePolicyDataConfigMapPreservesUnrelatedLabels(t *testing.T) {
	c := newFakeClient(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "proj-p1-alice",
			Name:      PolicyDataConfigMapName,
			Labels:    map[string]string{"custom": "keep"},
		},
		Data: map[string]string{"old": "value"},
	})
	ctx := context.Background()

	if err := c.EnsurePolicyDataConfigMap(ctx, "proj-p1-alice", map[string]string{"new": "value"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Clientset().CoreV1().ConfigMaps("proj-p1-alice").Get(ctx, PolicyDataConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels["custom"] != "keep" || got.Labels[policyDataLabelComponent] != "policy-data" {
		t.Fatalf("labels = %#v, want custom plus policy labels", got.Labels)
	}
	if got.Data["old"] != "" || got.Data["new"] != "value" {
		t.Fatalf("data = %#v, want replacement data", got.Data)
	}
}

func TestEnsurePolicyDataConfigMapRejectsEmptyNamespace(t *testing.T) {
	err := newFakeClient().EnsurePolicyDataConfigMap(context.Background(), " ", map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("error = %v, want namespace validation", err)
	}
}

func TestEnsurePolicyDataConfigMapNilClientNoop(t *testing.T) {
	var c *Client
	if err := c.EnsurePolicyDataConfigMap(context.Background(), "proj-p1-alice", map[string]string{}); err != nil {
		t.Fatalf("nil client returned error: %v", err)
	}
}
