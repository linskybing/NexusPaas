package services

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/auditcompliance"
	"github.com/linskybing/nexuspaas/backend/internal/services/authorizationpolicy"
	"github.com/linskybing/nexuspaas/backend/internal/services/clusterread"
	"github.com/linskybing/nexuspaas/backend/internal/services/dashboard"
	"github.com/linskybing/nexuspaas/backend/internal/services/gpuusage"
	"github.com/linskybing/nexuspaas/backend/internal/services/identity"
	"github.com/linskybing/nexuspaas/backend/internal/services/ideworkspace"
	"github.com/linskybing/nexuspaas/backend/internal/services/imageregistry"
	"github.com/linskybing/nexuspaas/backend/internal/services/integrationproxy"
	"github.com/linskybing/nexuspaas/backend/internal/services/k8scontrol"
	"github.com/linskybing/nexuspaas/backend/internal/services/mediaupload"
	"github.com/linskybing/nexuspaas/backend/internal/services/orgproject"
	"github.com/linskybing/nexuspaas/backend/internal/services/requestnotification"
	"github.com/linskybing/nexuspaas/backend/internal/services/resourcehours"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
	storageservice "github.com/linskybing/nexuspaas/backend/internal/services/storage"
	"github.com/linskybing/nexuspaas/backend/internal/services/workload"
)

const (
	serviceIdentity            = "identity-service"
	serviceAuthorizationPolicy = "authorization-policy-service"
	serviceOrgProject          = "org-project-service"
	serviceWorkload            = "workload-service"
	serviceSchedulerQuota      = "scheduler-quota-service"
	serviceK8sControl          = "k8s-control-service"
	serviceIDE                 = "ide-service"
	serviceStorage             = "storage-service"
	serviceImageRegistry       = "image-registry-service"
	serviceUsageObservability  = "usage-observability-service"
	serviceRequestNotification = "request-notification-service"
	serviceIntegrationProxy    = "integration-proxy-service"
	serviceMediaUpload         = "media-upload-service"
	serviceAuditCompliance     = "audit-compliance-service"
)

func RegisterAll(app *platform.App) {
	for _, spec := range Catalog() {
		app.RegisterService(spec)
	}
	registerStoreDependencies(app)
	for _, registration := range serviceRegistrations() {
		if app.Config.AllowsService(registration.owner) {
			registration.register(app)
		}
	}
}

type serviceRegistration struct {
	owner    string
	register func(*platform.App)
}

type ownerReadDependency struct {
	service  string
	resource string
}

func registerStoreDependencies(app *platform.App) {
	for _, dependency := range serviceOwnerReadDependencies() {
		app.RegisterOwnerReadDependencies(dependency.service, dependency.resource)
	}
}

func serviceOwnerReadDependencies() []ownerReadDependency {
	return []ownerReadDependency{
		{
			service:  serviceSchedulerQuota,
			resource: serviceImageRegistry + ":image_allow_lists",
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":project_members",
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":projects",
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":user_groups",
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":user_quotas",
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceWorkload + ":jobs",
		},
		{
			service:  serviceWorkload,
			resource: serviceOrgProject + ":project_members",
		},
		{
			service:  serviceWorkload,
			resource: serviceOrgProject + ":projects",
		},
	}
}

func serviceRegistrations() []serviceRegistration {
	return []serviceRegistration{
		{owner: serviceRequestNotification, register: requestnotification.Register},
		{owner: serviceMediaUpload, register: mediaupload.Register},
		{owner: serviceUsageObservability, register: resourcehours.Register},
		{owner: serviceSchedulerQuota, register: schedulerquota.Register},
		{owner: serviceWorkload, register: workload.Register},
		{owner: serviceK8sControl, register: k8scontrol.Register},
		{owner: serviceStorage, register: storageservice.Register},
		{owner: serviceUsageObservability, register: gpuusage.Register},
		{owner: serviceUsageObservability, register: clusterread.Register},
		{owner: serviceIDE, register: ideworkspace.Register},
		{owner: serviceImageRegistry, register: imageregistry.Register},
		{owner: serviceAuditCompliance, register: auditcompliance.Register},
		{owner: serviceUsageObservability, register: dashboard.Register},
		{owner: serviceIntegrationProxy, register: integrationproxy.Register},
		{owner: serviceOrgProject, register: orgproject.Register},
		{owner: serviceAuthorizationPolicy, register: authorizationpolicy.Register},
		{owner: serviceIdentity, register: identity.Register},
	}
}

func Catalog() []platform.ServiceSpec {
	specs := []platform.ServiceSpec{
		platformGateway(),
		identity.Spec(),
		authorizationpolicy.Spec(),
		orgproject.Spec(),
		workload.Spec(),
		schedulerquota.Spec(),
		k8scontrol.Spec(),
		ideworkspace.Spec(),
		storageservice.Spec(),
		imageregistry.Spec(),
		resourcehours.Spec(),
		auditcompliance.Spec(),
		requestnotification.Spec(),
		integrationproxy.Spec(),
		mediaupload.Spec(),
	}
	return specs
}

func platformGateway() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        "platform-gateway",
		Category:    "edge",
		Phase:       "1",
		Description: "External API gateway metadata, auth entry, rate limiting, route mapping, and degraded downstream proxy.",
		Tables:      []string{"route_config_cache", "jwks_cache", "rate_limit_counters"},
		Events:      []string{"PolicyChanged", "ProxyPolicyChanged", "AnnouncementPublished"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/gateway/routes", "routes", "list"),
			route(http.MethodGet, "/api/v1/gateway/health", "health", "list"),
		},
	}
}

func route(method, pattern, resource, action string, opts ...func(*platform.RouteSpec)) platform.RouteSpec {
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
