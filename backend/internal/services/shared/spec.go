package shared

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func Route(method, pattern, resource, action string, opts ...func(*platform.RouteSpec)) platform.RouteSpec {
	spec := platform.RouteSpec{
		Method:        method,
		Pattern:       pattern,
		Resource:      resource,
		Action:        action,
		AuthRequired:  true,
		StateChanging: method != http.MethodGet,
	}
	for _, opt := range opts {
		opt(&spec)
	}
	return spec
}

func Public(spec platform.RouteSpec) platform.RouteSpec {
	spec.AuthRequired = false
	return spec
}

func ID(name string) func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.IDParam = name
	}
}

func Admin() func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.Admin = true
	}
}

func ServiceInternal() func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.AuthRequired = false
		spec.PolicyBypass = true
	}
}

func Adapter(name string) func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.ExternalAdapter = name
	}
}

func PolicyBypass() func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.PolicyBypass = true
	}
}
