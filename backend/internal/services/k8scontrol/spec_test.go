package k8scontrol

import (
	"net/http"
	"testing"
)

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "k8s-control-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want k8s-control-service routes", spec)
	}
	for _, route := range spec.Routes {
		if route.Method == http.MethodPost && route.Pattern == "/internal/k8s-control/fast-transfers/mover-jobs" {
			if !route.ServiceAuthRequired || route.AuthRequired || !route.PolicyBypass {
				t.Fatalf("mover route = %#v, want service-internal", route)
			}
			return
		}
	}
	t.Fatal("Spec() missing FastTransfer mover job route")
}
