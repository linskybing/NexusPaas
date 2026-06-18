package workload

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "workload-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want workload-service routes", spec)
	}
}
