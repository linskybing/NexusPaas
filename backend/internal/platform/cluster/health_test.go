package cluster

import (
	"context"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

func TestClientConfigured(t *testing.T) {
	if (*Client)(nil).Configured() {
		t.Fatal("nil client must report not configured")
	}
	if New(nil, "proj").Configured() {
		t.Fatal("client with nil clientset must report not configured")
	}
	if !New(fake.NewSimpleClientset(), "proj").Configured() {
		t.Fatal("client with clientset must report configured")
	}
}

func TestClientPing(t *testing.T) {
	if err := New(nil, "proj").Ping(context.Background()); err == nil {
		t.Fatal("ping on unconfigured client should error")
	}
	if err := New(fake.NewSimpleClientset(), "proj").Ping(context.Background()); err != nil {
		t.Fatalf("ping against fake clientset should succeed: %v", err)
	}
}
