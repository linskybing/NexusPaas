package orgproject

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "org-project-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want org-project-service routes", spec)
	}
}
