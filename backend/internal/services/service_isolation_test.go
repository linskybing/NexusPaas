package services

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterAllServiceIsolationAllowsCoHostedCatalog(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("co-hosted catalog should pass service isolation validation: %v", err)
	}
}

func TestRegisterAllServiceIsolationAllowsIndependentIdentity(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "identity-service", HTTPAddr: ":0"})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("identity service should not depend on another service store: %v", err)
	}
}

func TestRegisterAllServiceIsolationAllowsUsageObservability(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "usage-observability-service", HTTPAddr: ":0"})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("usage-observability should use local event-fed read models: %v", err)
	}
}

func TestRegisterAllServiceIsolationAllowsAuthorizationPolicy(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "authorization-policy-service", HTTPAddr: ":0"})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("authorization-policy should use local event-fed identity read models: %v", err)
	}
}

func TestRegisterAllServiceIsolationAllowsOrgProject(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "org-project-service", HTTPAddr: ":0"})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("org-project should use local event-fed identity read models: %v", err)
	}
}

func TestRegisterAllServiceIsolationAllowsIDEWorkspace(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "ide-service", HTTPAddr: ":0"})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("ide-service should use local event-fed read models: %v", err)
	}
}

func TestRegisterAllServiceIsolationFailsSchedulerQuotaWithoutContractConfig(t *testing.T) {
	// The org-project/workload read contracts exist, but isolation still fails
	// closed when this isolated process has no SERVICE_URLS/SERVICE_API_KEY to
	// reach the owners. A contract alone is not enough.
	app := platform.NewApp(platform.Config{ServiceName: serviceSchedulerQuota, HTTPAddr: ":0"})
	RegisterAll(app)

	err := app.ValidateServiceIsolation()
	if err == nil {
		t.Fatal("scheduler-quota should fail isolation validation without owner SERVICE_URLS/SERVICE_API_KEY")
	}
	assertIsolationGaps(t, err, []string{
		serviceSchedulerQuota + " -> " + serviceOrgProject + ":projects",
		serviceSchedulerQuota + " -> " + serviceOrgProject + ":user_quotas",
		serviceSchedulerQuota + " -> " + serviceOrgProject + ":project_members",
		serviceSchedulerQuota + " -> " + serviceOrgProject + ":user_groups",
		serviceSchedulerQuota + " -> " + serviceWorkload + ":jobs",
	})
}

func TestRegisterAllServiceIsolationAllowsSchedulerQuotaWithOwnerContracts(t *testing.T) {
	// With the org-project + workload read contracts registered and their owners
	// reachable via SERVICE_URLS + a service key, isolated scheduler-quota resolves its
	// foreign reads through owner APIs and passes isolation validation.
	app := platform.NewApp(platform.Config{
		ServiceName: serviceSchedulerQuota,
		HTTPAddr:    ":0",
		ServiceURLs: map[string]string{
			serviceOrgProject: "http://org-project-service",
			serviceWorkload:   "http://workload-service",
		},
		ServiceAPIKey: "service-key",
	})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("scheduler-quota with org-project/workload read contracts should pass isolation: %v", err)
	}
}

func TestRegisterAllServiceIsolationFailsWorkloadWithoutContractConfig(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceWorkload, HTTPAddr: ":0"})
	RegisterAll(app)

	err := app.ValidateServiceIsolation()
	if err == nil {
		t.Fatal("workload should fail isolation validation without org-project SERVICE_URLS/SERVICE_API_KEY")
	}
	assertIsolationGaps(t, err, []string{
		serviceWorkload + " -> " + serviceOrgProject + ":project_members",
		serviceWorkload + " -> " + serviceOrgProject + ":projects",
	})
}

func TestRegisterAllServiceIsolationAllowsWorkloadWithOwnerContracts(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:   serviceWorkload,
		HTTPAddr:      ":0",
		ServiceURLs:   map[string]string{serviceOrgProject: "http://org-project-service"},
		ServiceAPIKey: "service-key",
	})
	RegisterAll(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("workload with org-project owner read contracts should pass isolation: %v", err)
	}
}

func TestRegisterAllServiceIsolationFailsSchedulerQuotaWithUnrelatedIdentityContract(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:   serviceSchedulerQuota,
		HTTPAddr:      ":0",
		ServiceURLs:   map[string]string{serviceIdentity: "http://identity-service"},
		ServiceAPIKey: "service-key",
	})
	RegisterAll(app)

	err := app.ValidateServiceIsolation()
	if err == nil {
		t.Fatal("identity service contract must not satisfy scheduler org-project/workload dependencies")
	}
	assertIsolationGaps(t, err, []string{
		serviceSchedulerQuota + " -> " + serviceOrgProject + ":projects",
		serviceSchedulerQuota + " -> " + serviceWorkload + ":jobs",
	})
}

func TestSchedulerQuotaUsesOwnerReadDependenciesNotStoreDependencies(t *testing.T) {
	got := ownerReadResourcesForService(serviceSchedulerQuota)
	want := []string{
		serviceOrgProject + ":project_members",
		serviceOrgProject + ":projects",
		serviceOrgProject + ":user_groups",
		serviceOrgProject + ":user_quotas",
		serviceWorkload + ":jobs",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("scheduler-quota owner-read dependencies = %v, want %v", got, want)
	}
}

func TestWorkloadUsesOwnerReadDependenciesNotStoreDependencies(t *testing.T) {
	got := ownerReadResourcesForService(serviceWorkload)
	want := []string{
		serviceOrgProject + ":project_members",
		serviceOrgProject + ":projects",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("workload owner-read dependencies = %v, want %v", got, want)
	}
}

func TestRegisterAllIsolatedIdentityOwnsOnlyIdentitySideEffects(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "identity-service", HTTPAddr: ":0"})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{"identity-auth-cleanup", "ldap-mirror-sync"})
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/login")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerAbsent(t, app, http.MethodGet, "/api/v1/admin/request-usage")
}

func TestRegisterAllIsolatedWorkloadOwnsOnlyWorkloadSideEffects(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "workload-service", HTTPAddr: ":0"})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{
		"idle-reaper",
		"workload-dispatcher",
		"workload-runtime-reaper",
		"workload-status-reconciler",
	})
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/jobs")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/jobs/{id}")
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/jobs/{id}/cancel")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/jobs/{id}/logs")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/configfiles")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/configfiles/tree")
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/configfiles/{id}/versions")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/login")
	assertCustomHandlerAbsent(t, app, http.MethodGet, "/api/v1/admin/request-usage")
}

func TestRegisterAllIsolatedUsageObservabilityOwnsOnlyUsageSideEffects(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "usage-observability-service", HTTPAddr: ":0"})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{
		"cluster-resource-collector",
		"gpu-usage-telemetry-collector",
		"resource-hours-collector",
	})
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/admin/request-usage")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/cluster/summary")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerAbsent(t, app, http.MethodGet, "/api/v1/admin/vpn/clients")
}

func TestRegisterAllIsolatedAuthorizationPolicyOwnsOnlyPolicySideEffects(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "authorization-policy-service", HTTPAddr: ":0"})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{"policy-data-sync"})
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/permissions/enforce")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerAbsent(t, app, http.MethodGet, "/api/v1/admin/request-usage")
}

func TestRegisterAllIsolatedStorageOwnsOnlyStorageSideEffects(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "storage-service", HTTPAddr: ":0"})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{"longhorn-rwx-health"})
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/storage/options")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/login")
}

func TestRegisterAllIsolatedK8sControlOwnsOptInDockerCleanup(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:          "k8s-control-service",
		HTTPAddr:             ":0",
		DockerCleanupEnabled: true,
	})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{"docker-image-cleanup"})
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/k8s/user-storage/status")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerAbsent(t, app, http.MethodPost, "/api/v1/login")
}

func TestRegisterAllCoHostedOwnsAllMaintenanceSideEffects(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	RegisterAll(app)

	assertMaintenanceTasks(t, app, []string{
		"audit-log-retention",
		"cluster-resource-collector",
		"gpu-usage-telemetry-collector",
		"harbor-catalog-sync",
		"harbor-health",
		"identity-auth-cleanup",
		"idle-reaper",
		"ldap-mirror-sync",
		"longhorn-rwx-health",
		"plan-window-reaper",
		"policy-data-sync",
		"priority-class-sync",
		"reservation-drift-detector",
		"resource-hours-collector",
		"resource-quota-reconciler",
		"vpn-usage-collector",
		"workload-dispatcher",
		"workload-runtime-reaper",
		"workload-status-reconciler",
	})
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/jobs")
	assertCustomHandlerPresent(t, app, http.MethodPost, "/api/v1/login")
	assertCustomHandlerPresent(t, app, http.MethodGet, "/api/v1/admin/request-usage")
}

func TestRegisterAllCoHostedIncludesOptInDockerCleanup(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", DockerCleanupEnabled: true})
	RegisterAll(app)

	got := app.MaintenanceTaskNames()
	if !slices.Contains(got, "docker-image-cleanup") {
		t.Fatalf("co-hosted maintenance tasks = %v, want docker-image-cleanup", got)
	}
}

func assertMaintenanceTasks(t *testing.T, app *platform.App, want []string) {
	t.Helper()
	got := app.MaintenanceTaskNames()
	if !slices.Equal(got, want) {
		t.Fatalf("maintenance tasks = %v, want %v", got, want)
	}
}

func assertCustomHandlerPresent(t *testing.T, app *platform.App, method, pattern string) {
	t.Helper()
	if app.CustomHandlers[method+" "+testCanonicalPattern(pattern)] == nil {
		t.Fatalf("missing custom handler %s %s", method, pattern)
	}
}

func assertCustomHandlerAbsent(t *testing.T, app *platform.App, method, pattern string) {
	t.Helper()
	if app.CustomHandlers[method+" "+testCanonicalPattern(pattern)] != nil {
		t.Fatalf("unexpected custom handler %s %s", method, pattern)
	}
}

func testCanonicalPattern(pattern string) string {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "...}") {
			parts[i] = "{...}"
			continue
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[i] = "{}"
		}
	}
	return "/" + strings.Join(parts, "/")
}

func assertIsolationGaps(t *testing.T, err error, want []string) {
	t.Helper()
	message := err.Error()
	for _, gap := range want {
		if !strings.Contains(message, gap) {
			t.Fatalf("isolation error %q does not include %q", message, gap)
		}
	}
}

func ownerReadResourcesForService(service string) []string {
	var resources []string
	for _, dependency := range serviceOwnerReadDependencies() {
		if dependency.service == service {
			resources = append(resources, dependency.resource)
		}
	}
	return resources
}
