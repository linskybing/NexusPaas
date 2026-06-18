package integrationproxy

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "integration-proxy-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want integration-proxy-service routes", spec)
	}
}
