package identity

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "identity-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want identity-service routes", spec)
	}
}
