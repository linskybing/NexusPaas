package cluster

import "testing"

// A pod with automountServiceAccountToken=false sees the in-cluster env vars
// but has no token file. NewFromEnv must degrade to (nil, nil) instead of
// failing the whole composition root.
func TestNewFromEnvWithoutTokenDegradesToNil(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	t.Setenv("KUBECONFIG", "")

	client, err := NewFromEnv("proj")
	if err != nil {
		t.Fatalf("NewFromEnv with in-cluster env but no token = %v, want nil error", err)
	}
	if client != nil {
		t.Fatalf("NewFromEnv client = %v, want nil (degraded mode)", client)
	}
}
