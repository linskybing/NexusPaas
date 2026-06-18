package storage

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "storage-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want storage-service routes", spec)
	}
}
