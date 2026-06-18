package imageregistry

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "image-registry-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want image-registry-service routes", spec)
	}
}
