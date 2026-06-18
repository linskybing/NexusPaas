package k8scontrol

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "k8s-control-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want k8s-control-service routes", spec)
	}
}
