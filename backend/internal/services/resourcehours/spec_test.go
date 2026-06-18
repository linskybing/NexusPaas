package resourcehours

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "usage-observability-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want usage-observability-service routes", spec)
	}
}
