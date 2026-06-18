package requestnotification

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "request-notification-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want request-notification-service routes", spec)
	}
}
