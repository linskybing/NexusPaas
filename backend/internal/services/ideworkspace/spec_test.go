package ideworkspace

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "ide-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want ide-service routes", spec)
	}
}
