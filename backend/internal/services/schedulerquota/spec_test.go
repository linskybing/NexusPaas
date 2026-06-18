package schedulerquota

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "scheduler-quota-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want scheduler-quota-service routes", spec)
	}
}
