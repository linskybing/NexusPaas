package authorizationpolicy

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "authorization-policy-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want authorization-policy-service routes", spec)
	}
}
