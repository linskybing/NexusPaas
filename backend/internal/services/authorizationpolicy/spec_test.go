package authorizationpolicy

import (
	"net/http"
	"testing"
)

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "authorization-policy-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want authorization-policy-service routes", spec)
	}
}

func TestSpecPermissionEnforceRouteScopeMetadata(t *testing.T) {
	spec := Spec()
	for _, route := range spec.Routes {
		if route.Method == http.MethodPost && route.Pattern == "/api/v1/permissions/enforce" {
			if route.Resource != "permissions" || route.Action != "enforce" {
				t.Fatalf("enforce route = %#v, want permissions/enforce metadata", route)
			}
			if route.AuthRequired || !route.ServiceAuthRequired || !route.PolicyBypass {
				t.Fatalf("enforce route = %#v, want service-internal policy-bypass metadata", route)
			}
			return
		}
	}
	t.Fatal("missing POST /api/v1/permissions/enforce route")
}
