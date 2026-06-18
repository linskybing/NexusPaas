package auditcompliance

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "audit-compliance-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want audit-compliance-service routes", spec)
	}
}
