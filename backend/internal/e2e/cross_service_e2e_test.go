//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services"
)

func TestServiceRouteIsolationContract(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: schedulerQuotaService, RequireAuth: false})
	services.RegisterAll(app)

	if len(app.Services) != 1 {
		t.Fatalf("registered service count = %d, want 1", len(app.Services))
	}
	if _, ok := app.Services[schedulerQuotaService]; !ok {
		t.Fatalf("scheduler service not registered: %#v", app.Services)
	}
	for _, route := range app.Routes {
		if strings.Split(route.Resource, ":")[0] != schedulerQuotaService {
			t.Fatalf("isolated scheduler exposed non-scheduler route: %#v", route)
		}
	}
}

func TestServiceIsolationValidationE2E(t *testing.T) {
	cohosted := platform.NewApp(platform.Config{ServiceName: "all"})
	services.RegisterAll(cohosted)
	if err := cohosted.ValidateServiceIsolation(); err != nil {
		t.Fatalf("co-hosted service isolation validation = %v, want nil", err)
	}

	identity := platform.NewApp(platform.Config{ServiceName: identityService})
	services.RegisterAll(identity)
	if err := identity.ValidateServiceIsolation(); err != nil {
		t.Fatalf("identity service isolation validation = %v, want nil", err)
	}

	scheduler := platform.NewApp(platform.Config{ServiceName: schedulerQuotaService})
	services.RegisterAll(scheduler)
	err := scheduler.ValidateServiceIsolation()
	if err == nil {
		t.Fatal("scheduler service isolation validation passed, want registered org-project/workload dependency gaps")
	}
	for _, gap := range []string{
		schedulerQuotaService + " -> org-project-service:projects",
		schedulerQuotaService + " -> org-project-service:project_members",
		schedulerQuotaService + " -> org-project-service:user_groups",
		schedulerQuotaService + " -> org-project-service:user_quotas",
		schedulerQuotaService + " -> workload-service:jobs",
	} {
		if !strings.Contains(err.Error(), gap) {
			t.Fatalf("scheduler isolation error %q does not include %q", err.Error(), gap)
		}
	}
}

func TestIsolatedRuntimeRegistrationE2E(t *testing.T) {
	h := newHarness(t, identityService, schedulerQuotaService, workloadService, storageService, usageObservabilityService)
	cohosted := h.startExtraService("cohosted-all", "all", nil)
	authorizationPolicy := platform.NewApp(platform.Config{ServiceName: authorizationPolicyService})
	services.RegisterAll(authorizationPolicy)

	assertRuntimeRegistration(t, h.services[identityService].app,
		[]string{"event-outbox-relay", "identity-auth-cleanup", "ldap-mirror-sync"},
		[]string{http.MethodPost + " /api/v1/login"},
		[]string{
			http.MethodPost + " /api/v1/jobs",
			http.MethodGet + " /api/v1/admin/request-usage",
		},
	)
	assertRuntimeRegistration(t, h.services[workloadService].app,
		[]string{
			"event-outbox-relay",
			"idle-reaper",
			"workload-dispatcher",
			"workload-runtime-reaper",
			"workload-status-reconciler",
		},
		[]string{http.MethodPost + " /api/v1/jobs"},
		[]string{
			http.MethodPost + " /api/v1/login",
			http.MethodGet + " /api/v1/admin/request-usage",
		},
	)
	assertRuntimeRegistration(t, h.services[storageService].app,
		[]string{"event-outbox-relay", "longhorn-rwx-health"},
		[]string{http.MethodGet + " /api/v1/storage/options"},
		[]string{
			http.MethodPost + " /api/v1/jobs",
			http.MethodPost + " /api/v1/login",
		},
	)
	assertRuntimeRegistration(t, h.services[usageObservabilityService].app,
		[]string{
			"cluster-resource-collector",
			"event-outbox-relay",
			"gpu-usage-telemetry-collector",
			"resource-hours-collector",
		},
		[]string{
			http.MethodGet + " /api/v1/admin/request-usage",
			http.MethodGet + " /api/v1/cluster/summary",
		},
		[]string{
			http.MethodPost + " /api/v1/jobs",
			http.MethodGet + " /api/v1/admin/vpn/clients",
		},
	)
	assertRuntimeRegistration(t, authorizationPolicy,
		[]string{"policy-data-sync"},
		[]string{http.MethodPost + " /api/v1/permissions/enforce"},
		[]string{
			http.MethodPost + " /api/v1/jobs",
			http.MethodGet + " /api/v1/admin/request-usage",
		},
	)
	assertRuntimeRegistration(t, cohosted.app,
		[]string{
			"audit-log-retention",
			"cluster-resource-collector",
			"event-outbox-relay",
			"gpu-reservation-drift",
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
		},
		[]string{
			http.MethodPost + " /api/v1/jobs",
			http.MethodPost + " /api/v1/login",
			http.MethodGet + " /api/v1/admin/request-usage",
		},
		nil,
	)
}

func TestNonBlobIsolatedServiceIgnoresObjectStoreConfigE2E(t *testing.T) {
	h := newHarness(t, identityService)
	identity := h.startExtraServiceWithConfig("identity-broken-object-store-"+h.runID, identityService, nil, func(cfg *platform.Config) {
		cfg.Production = true
		cfg.ObjectStoreURL = "http://127.0.0.1:1"
		cfg.ObjectStoreAccessKey = ""
		cfg.ObjectStoreSecretKey = ""
		cfg.ObjectStoreBucket = "missing-" + h.runID
	})

	resp, err := http.Get(identity.url + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("identity /readyz = %d, want 200: %s", resp.StatusCode, string(raw))
	}
	if bytes.Contains(raw, []byte("OBJECT_STORE_URL")) {
		t.Fatalf("identity /readyz mentioned object store despite non-blob service: %s", string(raw))
	}
}

func assertRuntimeRegistration(t *testing.T, app *platform.App, wantTasks, wantHandlers, unwantedHandlers []string) {
	t.Helper()
	if got := app.MaintenanceTaskNames(); !slices.Equal(got, wantTasks) {
		t.Fatalf("%s maintenance tasks = %v, want %v", app.Config.ServiceName, got, wantTasks)
	}
	for _, key := range wantHandlers {
		if app.CustomHandlers[key] == nil {
			t.Fatalf("%s missing custom handler %s", app.Config.ServiceName, key)
		}
	}
	for _, key := range unwantedHandlers {
		if app.CustomHandlers[key] != nil {
			t.Fatalf("%s unexpectedly installed custom handler %s", app.Config.ServiceName, key)
		}
	}
}

func TestProviderConsumerContractMatrix(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", RequireAuth: false})
	services.RegisterAll(app)
	routes := registeredRouteContracts(app)
	routeOwners := catalogRouteOwners()
	rawRoutes := mustReadE2EDoc(t, "../../docs/api-route-mapping.md")
	routeDocs := documentedRouteMappings(t, rawRoutes)
	requireCatalogRoutesRegistered(t, routes, routeOwners)
	requireCatalogRoutesDocumented(t, routeOwners, routeDocs)
	requireProviderConsumerRoutes(t, routes, routeOwners, rawRoutes)
	rawEvents := mustReadE2EDoc(t, "../../docs/event-contracts.md")
	docEvents := documentedEventContracts(t, rawEvents)
	catalogEvents := catalogEventContracts()
	requireDocumentedEventsInCatalog(t, catalogEvents, docEvents)
	requireCatalogEventsDocumented(t, catalogEvents, docEvents)
}

func registeredRouteContracts(app *platform.App) map[string]bool {
	routes := map[string]bool{}
	for _, route := range app.Routes {
		key := route.Method + " " + route.Pattern
		routes[key] = true
		routes[canonicalRouteKey(key)] = true
	}
	return routes
}

func catalogRouteOwners() map[string]string {
	routeOwners := map[string]string{}
	for _, spec := range services.Catalog() {
		for _, route := range spec.Routes {
			routeOwners[route.Method+" "+route.Pattern] = spec.Name
		}
	}
	return routeOwners
}

func requireCatalogRoutesRegistered(t *testing.T, routes map[string]bool, routeOwners map[string]string) {
	t.Helper()
	var missing []string
	for key, owner := range routeOwners {
		if owner == "platform-gateway" {
			continue
		}
		if !routes[canonicalRouteKey(key)] {
			missing = append(missing, fmt.Sprintf("%s owned by %s", key, owner))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("catalog routes missing from registered cohosted app:\n%s", strings.Join(missing, "\n"))
	}
}

func canonicalRouteKey(key string) string {
	method, pattern, ok := strings.Cut(key, " ")
	if !ok {
		return key
	}
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
	return method + " /" + strings.Join(parts, "/")
}

func requireCatalogRoutesDocumented(t *testing.T, routeOwners map[string]string, docs []documentedRouteMapping) {
	t.Helper()
	var mismatches []string
	for key, owner := range routeOwners {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) != 2 || !requiresRouteMappingDoc(owner, parts[1]) {
			continue
		}
		doc, ok := routeMappingFor(parts[1], docs)
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s owned by %s has no matching docs/api-route-mapping.md scope", key, owner))
			continue
		}
		if !slices.Contains(doc.owners, owner) {
			mismatches = append(mismatches, fmt.Sprintf("%s owner = %s, doc scope %q owners = %s", key, owner, doc.rawScope, strings.Join(doc.owners, ", ")))
		}
	}
	if len(mismatches) > 0 {
		sort.Strings(mismatches)
		t.Fatalf("catalog route ownership drifted from docs/api-route-mapping.md:\n%s", strings.Join(mismatches, "\n"))
	}
}

func requiresRouteMappingDoc(owner, pattern string) bool {
	if owner == "platform-gateway" || !strings.HasPrefix(pattern, "/api/v1/") {
		return false
	}
	if strings.Contains(pattern, "/internal/") {
		return false
	}
	for _, compat := range []string{
		"/api/v1/harbor-gpu23/",
		"/api/v1/gateway/",
	} {
		if strings.HasPrefix(pattern, compat) {
			return false
		}
	}
	return true
}

func requireProviderConsumerRoutes(t *testing.T, routes map[string]bool, routeOwners map[string]string, rawRoutes []byte) {
	t.Helper()
	for _, required := range routeMappingContracts() {
		requireRouteMappingDoc(t, rawRoutes, required.docScope, required.docOwner)
		for _, route := range required.routes {
			key := route.method + " " + route.pattern
			if !routes[key] {
				t.Fatalf("missing provider/consumer route %s", key)
			}
			if got := routeOwners[key]; got != route.owner {
				t.Fatalf("route %s owner = %q, want %q", key, got, route.owner)
			}
		}
	}
}

type documentedRouteMapping struct {
	rawScope string
	scopes   []string
	owners   []string
}

func documentedRouteMappings(t *testing.T, raw []byte) []documentedRouteMapping {
	t.Helper()
	var docs []documentedRouteMapping
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "---") || strings.Contains(line, "Current Route Scope") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		scope := strings.TrimSpace(parts[1])
		owner := strings.TrimSpace(parts[2])
		if scope == "" || owner == "" {
			continue
		}
		docs = append(docs, documentedRouteMapping{
			rawScope: scope,
			scopes:   append(routeDocScopes(scope), routeDocOwnerAliases(owner)...),
			owners:   routeDocOwners(owner),
		})
	}
	if len(docs) == 0 {
		t.Fatal("route mapping doc did not yield any route rows")
	}
	return docs
}

func routeDocScopes(scopeCell string) []string {
	var scopes []string
	for _, fragment := range strings.Split(scopeCell, ",") {
		fragment = strings.TrimSpace(fragment)
		if fragment == "" {
			continue
		}
		switch {
		case strings.Contains(fragment, "quota/preemption internal APIs"):
			scopes = append(scopes,
				"/api/v1/internal/quota/",
				"/api/v1/internal/scheduler/",
				"/api/v1/internal/workers/",
			)
		case strings.HasPrefix(fragment, "/api/"):
			scopes = append(scopes, trimRouteScopeNote(fragment))
		case strings.HasPrefix(fragment, "/"):
			scopes = append(scopes, trimRouteScopeNote("/api/v1"+fragment))
		}
	}
	return scopes
}

func trimRouteScopeNote(scope string) string {
	if idx := strings.IndexByte(scope, ' '); idx >= 0 {
		return scope[:idx]
	}
	return scope
}

func routeDocOwnerAliases(ownerCell string) []string {
	owners := routeDocOwners(ownerCell)
	if len(owners) != 1 {
		return nil
	}
	switch owners[0] {
	case identityService:
		return []string{
			"/api/v1/.well-known/",
			"/api/v1/authorize",
			"/api/v1/authorize/callback",
			"/api/v1/keys",
			"/api/v1/userinfo",
		}
	case orgProjectService:
		return []string{"/api/v1/admin/group-policy-options"}
	case workloadService:
		return []string{"/api/v1/projects/{id}/config-files"}
	case schedulerQuotaService:
		return []string{"/api/v1/projects/{id}/quota/live"}
	case k8sControlService:
		return []string{
			"/api/v1/admin/resources",
			"/api/v1/k8s/user-storage/browse",
			"/api/v1/k8s/user-storage/proxy",
			"/api/v1/k8s/user-storage/status",
			"/api/v1/projects/{id}/namespaces",
			"/api/v1/projects/{id}/resources",
		}
	case storageService:
		return []string{"/api/v1/k8s/user-storage"}
	case imageRegistryService:
		return []string{
			"/api/v1/harbor-projects",
			"/api/v1/harbor-statistics",
			"/api/v1/projects/{id}/builds",
			"/api/v1/projects/{id}/image-builds",
			"/api/v1/projects/{id}/image-requests",
		}
	case usageObservabilityService:
		return []string{
			"/api/v1/admin/dashboard-summary",
			"/api/v1/admin/mps-mapping",
			"/api/v1/projects/gpu-usage/by-user",
			"/api/v1/projects/{id}/gpu-usage",
			"/api/v1/resource-hours",
		}
	case integrationProxyService:
		return []string{
			"/api/v1/minio-console-sso",
			"/api/v1/pgadmin-auth-check",
			"/api/v1/pgadmin-sso",
		}
	default:
		return nil
	}
}

func routeDocOwners(ownerCell string) []string {
	if strings.HasPrefix(ownerCell, "split between ") {
		ownerCell = strings.TrimPrefix(ownerCell, "split between ")
	}
	parts := strings.Split(ownerCell, "/")
	owners := make([]string, 0, len(parts))
	for _, part := range parts {
		owner := strings.TrimSpace(part)
		if owner != "" {
			owners = append(owners, owner)
		}
	}
	return owners
}

func routeMappingFor(pattern string, docs []documentedRouteMapping) (documentedRouteMapping, bool) {
	var best documentedRouteMapping
	bestLen := -1
	for _, doc := range docs {
		for _, scope := range doc.scopes {
			if routeScopeMatches(pattern, scope) && len(scope) > bestLen {
				best = doc
				bestLen = len(scope)
			}
		}
	}
	return best, bestLen >= 0
}

func routeScopeMatches(pattern, scope string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return false
	}
	if strings.HasSuffix(scope, "/*") {
		scope = strings.TrimSuffix(scope, "/*")
		return pattern == scope || strings.HasPrefix(pattern, scope+"/")
	}
	if strings.HasSuffix(scope, "/") {
		return strings.HasPrefix(pattern, scope)
	}
	if pattern == scope {
		return true
	}
	return strings.HasPrefix(pattern, scope+"/") || strings.HasPrefix(pattern, scope+"{")
}

type routeMappingContract struct {
	docScope string
	docOwner string
	routes   []ownedRoute
}

type ownedRoute struct {
	method  string
	pattern string
	owner   string
}

func routeMappingContracts() []routeMappingContract {
	return []routeMappingContract{
		{
			docScope: "/api/v1/login",
			docOwner: identityService,
			routes: []ownedRoute{
				{http.MethodPost, "/api/v1/login", identityService},
				{http.MethodPost, "/api/v1/logout", identityService},
				{http.MethodPost, "/api/v1/refresh", identityService},
				{http.MethodPost, "/api/v1/register", identityService},
				{http.MethodGet, "/api/v1/captcha", identityService},
				{http.MethodGet, "/api/v1/me/api-tokens", identityService},
				{http.MethodGet, "/api/v1/users", identityService},
				{http.MethodGet, "/api/v1/oidc/jwks", identityService},
				{http.MethodPost, "/api/v1/cli/login", identityService},
				{http.MethodGet, "/api/v1/me/cli-ca", identityService},
			},
		},
		{
			docScope: "/api/v1/permissions",
			docOwner: authorizationPolicyService,
			routes: []ownedRoute{
				{http.MethodPost, "/api/v1/permissions/enforce", authorizationPolicyService},
				{http.MethodGet, "/api/v1/permissions/policies", authorizationPolicyService},
				{http.MethodGet, "/api/v1/admin/proxy-rbac/services", authorizationPolicyService},
			},
		},
		{
			docScope: "/api/v1/groups",
			docOwner: orgProjectService,
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/groups", orgProjectService},
				{http.MethodGet, "/api/v1/user-groups", orgProjectService},
				{http.MethodGet, "/api/v1/projects", orgProjectService},
				{http.MethodGet, "/api/v1/projects/{id}/members", orgProjectService},
				{http.MethodGet, "/api/v1/projects/{id}/workspace-settings", orgProjectService},
				{http.MethodGet, "/api/v1/projects/{id}/gpu-claims", orgProjectService},
			},
		},
		{
			docScope: "/api/v1/configfiles",
			docOwner: workloadService,
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/configfiles", workloadService},
				{http.MethodPost, "/api/v1/jobs", workloadService},
			},
		},
		{
			docScope: "quota/preemption internal APIs",
			docOwner: schedulerQuotaService,
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/plans", schedulerQuotaService},
				{http.MethodGet, "/api/v1/queues", schedulerQuotaService},
				{http.MethodPost, "/api/v1/internal/scheduler/admission", schedulerQuotaService},
				{http.MethodPost, "/api/v1/internal/scheduler/preemptions", schedulerQuotaService},
				{http.MethodPost, "/api/v1/internal/quota/reservations", schedulerQuotaService},
			},
		},
		{
			docScope: "/api/v1/k8s",
			docOwner: k8sControlService,
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/k8s/cluster", k8sControlService},
				{http.MethodGet, "/api/v1/resources", k8sControlService},
				{http.MethodGet, "/api/v1/ws/exec", k8sControlService},
			},
		},
		{
			docScope: "/api/v1/cluster",
			docOwner: "split between k8s-control-service / usage-observability-service",
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/k8s/cluster", k8sControlService},
				{http.MethodGet, "/api/v1/cluster/summary", usageObservabilityService},
			},
		},
		{
			docScope: "/api/v1/ide",
			docOwner: "ide-service",
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/ide", "ide-service"},
				{http.MethodGet, "/api/v1/ide/images", "ide-service"},
				{http.MethodPost, "/api/v1/ide/{id}/stop", "ide-service"},
			},
		},
		{
			docScope: "/api/v1/storage",
			docOwner: storageService,
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/storage/permissions", storageService},
				{http.MethodGet, "/api/v1/projects/{id}/storage/bindings", storageService},
				{http.MethodGet, "/api/v1/admin/user-storage", storageService},
				{http.MethodGet, "/api/v1/admin/group-storage", storageService},
			},
		},
		{
			docScope: "/api/v1/image-requests",
			docOwner: "image-registry-service",
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/image-requests", "image-registry-service"},
				{http.MethodPost, "/api/v1/images/build", "image-registry-service"},
				{http.MethodGet, "/api/v1/image-catalog", "image-registry-service"},
				{http.MethodGet, "/api/v1/projects/{id}/images", "image-registry-service"},
				{http.MethodGet, "/api/v1/harbor-status", "image-registry-service"},
			},
		},
		{
			docScope: "/api/v1/me/usage",
			docOwner: usageObservabilityService,
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/me/usage", usageObservabilityService},
				{http.MethodGet, "/api/v1/me/gpu/jobs", usageObservabilityService},
				{http.MethodGet, "/api/v1/me/request-usage", usageObservabilityService},
				{http.MethodGet, "/api/v1/admin/usage", usageObservabilityService},
				{http.MethodGet, "/api/v1/admin/request-usage", usageObservabilityService},
				{http.MethodGet, "/api/v1/admin/gpu/users", usageObservabilityService},
				{http.MethodGet, "/api/v1/dashboard/overview", usageObservabilityService},
			},
		},
		{
			docScope: "/api/v1/audit",
			docOwner: "audit-compliance-service",
			routes: []ownedRoute{
				{http.MethodPost, "/api/v1/audit/events", "audit-compliance-service"},
				{http.MethodGet, "/api/v1/admin/security/posture", "audit-compliance-service"},
			},
		},
		{
			docScope: "/api/v1/forms",
			docOwner: requestNotificationService,
			routes: []ownedRoute{
				{http.MethodPost, "/api/v1/forms", requestNotificationService},
				{http.MethodPut, "/api/v1/notifications/read-all", requestNotificationService},
				{http.MethodGet, "/api/v1/announcements/active", requestNotificationService},
				{http.MethodGet, "/api/v1/admin/announcements", requestNotificationService},
			},
		},
		{
			docScope: "/api/v1/grafana",
			docOwner: "integration-proxy-service",
			routes: []ownedRoute{
				{http.MethodGet, "/api/v1/grafana/{path...}", "integration-proxy-service"},
				{http.MethodGet, "/api/v1/minio-console/{path...}", "integration-proxy-service"},
				{http.MethodGet, "/api/v1/pgadmin/{path...}", "integration-proxy-service"},
				{http.MethodGet, "/api/v1/longhorn/{path...}", "integration-proxy-service"},
				{http.MethodGet, "/api/v1/harbor/{path...}", "integration-proxy-service"},
				{http.MethodGet, "/api/v1/admin/vpn", "integration-proxy-service"},
			},
		},
		{
			docScope: "/api/v1/uploads/images",
			docOwner: mediaUploadService,
			routes: []ownedRoute{
				{http.MethodPost, "/api/v1/uploads/images", mediaUploadService},
				{http.MethodGet, "/api/v1/uploads/images/{key...}", mediaUploadService},
			},
		},
	}
}

func catalogEventContracts() map[string]bool {
	union := map[string]bool{}
	for _, spec := range services.Catalog() {
		for _, name := range spec.Events {
			union[name] = true
		}
	}
	return union
}

func requireDocumentedEventsInCatalog(t *testing.T, union map[string]bool, events []string) {
	t.Helper()
	for _, event := range events {
		if !union[event] {
			t.Fatalf("missing event contract %s", event)
		}
	}
}

func requireCatalogEventsDocumented(t *testing.T, union map[string]bool, documented []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, event := range documented {
		seen[event] = true
	}
	for event := range union {
		if !seen[event] {
			t.Fatalf("catalog event %s is missing from docs/event-contracts.md", event)
		}
	}
}

func mustReadE2EDoc(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func requireRouteMappingDoc(t *testing.T, raw []byte, scope, owner string) {
	t.Helper()
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, scope) && strings.Contains(line, owner) {
			return
		}
	}
	t.Fatalf("route mapping doc missing scope %q owned by %q", scope, owner)
}

func documentedEventContracts(t *testing.T, raw []byte) []string {
	t.Helper()
	seen := map[string]bool{}
	var events []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		eventCell := strings.TrimSpace(parts[1])
		if eventCell == "" || eventCell == "Event" || strings.Contains(eventCell, "---") {
			continue
		}
		for _, name := range strings.Split(eventCell, "/") {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			events = append(events, name)
		}
	}
	if len(events) == 0 {
		t.Fatal("event contract doc did not yield any event names")
	}
	return events
}

func TestCriticalCrossServiceJourneys(t *testing.T) {
	h := newHarness(t,
		identityService,
		orgProjectService,
		schedulerQuotaService,
		workloadService,
		mediaUploadService,
		requestNotificationService,
	)

	ids := h.seedIdentityContracts()
	h.seedSchedulerAdmissionData(ids.userID)

	t.Run("identity internal read and remote auth", func(t *testing.T) {
		h.assertIdentityInternalContracts(ids)
		h.assertRemoteIdentityAuth(ids)
	})

	t.Run("org project group membership feeds scheduler read path", func(t *testing.T) {
		h.assertOrgProjectGroupAdmissionSnapshot()
	})

	t.Run("workload calls scheduler admission", func(t *testing.T) {
		h.assertWorkloadSchedulerSubmit(ids)
	})

	t.Run("scheduler quota state events", func(t *testing.T) {
		h.assertQuotaReservationEvents()
	})

	t.Run("media upload stores metadata and minio bytes", func(t *testing.T) {
		h.assertMediaUploadRoundTrip()
	})

	t.Run("request notification emits form and audit events", func(t *testing.T) {
		h.assertRequestNotificationEvents()
	})

	t.Run("scheduler unavailable does not persist workload job", func(t *testing.T) {
		h.assertSchedulerUnavailableDoesNotPersist(ids)
	})
}

func TestSchedulerAdmissionOwnerReadContractsE2E(t *testing.T) {
	h := newHarnessWithPeers(t, map[string][]string{
		schedulerQuotaService: {orgProjectService, workloadService},
	}, orgProjectService, workloadService, schedulerQuotaService)
	userID := "owneruser" + h.runID
	badUserID := "badowneruser" + h.runID
	projectID, queueName := h.seedSchedulerOwnerReadAdmissionData(userID, badUserID)

	resp := h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/scheduler/admission", map[string]any{
		"job_id":          "ownerjobsubmit" + h.runID,
		"project_id":      projectID,
		"user_id":         userID,
		"queue_name":      queueName,
		"required_cpu":    1,
		"required_memory": 1024,
		"e2e_run_id":      h.runID,
	}, h.serviceKey, http.StatusOK)
	h.requireEnvelopeCorrelation(resp)
	data := resp.dataMap(t)
	if data["allowed"] != true || data["project_id"] != projectID || data["user_id"] != userID {
		t.Fatalf("owner-read admission = %#v, want allowed owner snapshot", data)
	}
	usage, ok := data["usage"].(map[string]any)
	if !ok {
		t.Fatalf("admission usage = %#v, want object", data["usage"])
	}
	requireE2ENumber(t, usage, "project_gpu", 1.5)
	requireE2ENumber(t, usage, "project_cpu", 2.5)
	requireE2ENumber(t, usage, "project_memory_mb", 2048)
	requireE2ENumber(t, usage, "user_running_jobs", 1)

	admissionID := projectID + "/" + userID + "/" + queueName
	if record := h.getRecord(schedulerAdmissionsResource, admissionID); record.Data["allowed"] != true {
		t.Fatalf("persisted owner-read admission = %#v, want allowed", record.Data)
	}
	h.updateRecord(schedulerAdmissionsResource, admissionID, map[string]any{})
	h.requireCorrelatedEvent("SubmitAdmissionReviewed", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["project_id"] == projectID && event.Data["user_id"] == userID
	})

	beforeAdmissions := len(h.listRecords(schedulerAdmissionsResource))
	beforeJobs := len(h.listRecords(workloadJobsResource))
	badScheduler := h.startExtraServiceWithConfig("scheduler-owner-bad-key-"+h.runID, schedulerQuotaService, map[string]string{
		orgProjectService: h.services[orgProjectService].url,
		workloadService:   h.services[workloadService].url,
	}, func(cfg *platform.Config) {
		cfg.ServiceAPIKey = "wrong-" + h.serviceKey
	})
	badResp := h.doURLInternalJSON(badScheduler.url, http.MethodPost, "/api/v1/internal/scheduler/admission", map[string]any{
		"job_id":          "ownerbadjob" + h.runID,
		"project_id":      projectID,
		"user_id":         badUserID,
		"queue_name":      queueName,
		"required_cpu":    1,
		"required_memory": 1024,
		"e2e_run_id":      h.runID,
	}, "wrong-"+h.serviceKey, http.StatusNotFound)
	h.requireEnvelopeCorrelation(badResp)
	badData := badResp.dataMap(t)
	if badData["allowed"] != false || !strings.Contains(badData["reason"].(string), "project not found") {
		t.Fatalf("bad owner service key admission = %#v, want fail-closed project not found", badData)
	}
	if afterAdmissions := len(h.listRecords(schedulerAdmissionsResource)); afterAdmissions != beforeAdmissions {
		t.Fatalf("admissions after wrong owner service key = %d, want unchanged %d", afterAdmissions, beforeAdmissions)
	}
	if afterJobs := len(h.listRecords(workloadJobsResource)); afterJobs != beforeJobs {
		t.Fatalf("workload jobs after wrong owner service key = %d, want unchanged %d", afterJobs, beforeJobs)
	}
}

type identityIDs struct {
	userID     string
	roleID     string
	session    string
	apiTokenID string
	apiToken   string
}

func (h *e2eHarness) seedIdentityContracts() identityIDs {
	ids := identityIDs{
		userID:     "user" + h.runID,
		roleID:     "role" + h.runID,
		session:    "session" + h.runID,
		apiTokenID: "AT" + h.runID,
	}
	ids.apiToken = platform.FormatUserAPIToken(ids.apiTokenID, "rawtoken"+h.runID)
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	h.createRecord(identityUsersResource, ids.userID, map[string]any{
		"username":    "alice-" + h.runID,
		"role":        "user",
		"role_id":     ids.roleID,
		"system_role": 2,
		"status":      "online",
	})
	h.createRecord(identityRolesResource, ids.roleID, map[string]any{
		"name":        "member-" + h.runID,
		"admin_panel": false,
	})
	h.createRecord(identitySessionsResource, ids.session, map[string]any{
		"user_id":    ids.userID,
		"expires_at": expires,
		"revoked":    false,
	})
	h.createRecord(identityAPITokensResource, ids.apiTokenID, map[string]any{
		"user_id":    ids.userID,
		"token_hash": platform.HashSecret(ids.apiToken),
		"expires_at": expires,
		"revoked":    false,
	})
	h.installStaticUserAPIKey(ids.apiToken, ids.userID, "alice-"+h.runID, "user", false)
	return ids
}

func (h *e2eHarness) seedSchedulerAdmissionData(userID string) {
	groupID := h.groupID()
	groupOnlyUserID := h.groupOnlyUserID()
	queueID := "queue" + h.runID
	queueName := "queue-" + h.runID
	planID := "plan" + h.runID
	projectID := h.projectID()
	h.createRecord(schedulerQueuesResource, queueID, map[string]any{"name": queueName})
	h.createRecord(schedulerPlansResource, planID, map[string]any{
		"name":               "default-" + h.runID,
		"gpu_limit":          4.0,
		"cpu_limit_cores":    8.0,
		"memory_limit_gb":    16.0,
		"queue_ids":          []string{queueID},
		"allowed_gpu_models": []string{"gpu.nvidia.com"},
	})
	h.createRecord(orgGroupsResource, groupID, map[string]any{
		"group_name": "research-" + h.runID,
		"name":       "research-" + h.runID,
	})
	h.createRecord(orgProjectsResource, projectID, map[string]any{
		"project_name":                 "trainer-" + h.runID,
		"owner_id":                     groupID,
		"plan_id":                      planID,
		"max_concurrent_jobs_per_user": 3,
		"max_queued_jobs_per_user":     5,
	})
	h.createRecord(orgUserGroupsResource, userID+"/"+groupID, map[string]any{
		"project_id": projectID,
		"group_id":   groupID,
		"user_id":    userID,
		"role":       "user",
	})
	h.createRecord(orgUserGroupsResource, groupOnlyUserID+"/"+groupID, map[string]any{
		"project_id": projectID,
		"group_id":   groupID,
		"user_id":    groupOnlyUserID,
		"role":       "user",
	})
	h.createRecord(orgProjectMembersResource, projectID+"/"+userID, map[string]any{
		"project_id": projectID,
		"user_id":    userID,
		"role":       "user",
	})
}

func (h *e2eHarness) seedSchedulerOwnerReadAdmissionData(userID, badUserID string) (string, string) {
	queueID := "ownerqueue" + h.runID
	queueName := "owner-queue-" + h.runID
	planID := "ownerplan" + h.runID
	projectID := "ownerproject" + h.runID
	h.createRecord(schedulerQueuesResource, queueID, map[string]any{
		"name":            queueName,
		"priority_value":  700,
		"is_preemptible":  false,
		"preemption_mode": "never",
	})
	h.createRecord(schedulerPlansResource, planID, map[string]any{
		"name":               "owner-read-" + h.runID,
		"gpu_limit":          8.0,
		"cpu_limit_cores":    16.0,
		"memory_limit_gb":    32.0,
		"queue_ids":          []string{queueID},
		"allowed_gpu_models": []string{"gpu.nvidia.com"},
	})
	h.createRecord(orgProjectsResource, projectID, map[string]any{
		"project_name":                 "owner-read-" + h.runID,
		"plan_id":                      planID,
		"max_concurrent_jobs_per_user": 3,
		"max_queued_jobs_per_user":     5,
	})
	for _, id := range []string{userID, badUserID} {
		h.createRecord(orgProjectMembersResource, projectID+"/"+id, map[string]any{
			"project_id": projectID,
			"user_id":    id,
			"role":       "user",
		})
	}
	h.createRecord(workloadJobsResource, "ownerrunning"+h.runID, map[string]any{
		"job_id":          "ownerrunning" + h.runID,
		"project_id":      projectID,
		"user_id":         userID,
		"status":          "running",
		"required_gpu":    1.5,
		"required_cpu":    2.5,
		"required_memory": 2048,
	})
	return projectID, queueName
}

func (h *e2eHarness) assertIdentityInternalContracts(ids identityIDs) {
	reader := platform.NewRemoteServiceReader(platform.Config{
		ServiceURLs:    map[string]string{identityService: h.services[identityService].url},
		ServiceAPIKey:  h.serviceKey,
		AdapterTimeout: time.Second,
	})
	h.assertIdentityRemoteReadContracts(reader, ids)
	h.assertIdentityInternalHTTPContracts(ids)
	h.assertIdentityInternalAuthContracts(ids)
}

func (h *e2eHarness) assertIdentityRemoteReadContracts(reader *platform.RemoteServiceReader, ids identityIDs) {
	users, err := reader.List(context.Background(), identityUsersResource)
	if err != nil {
		h.t.Fatalf("remote identity list: %v", err)
	}
	if !recordListContains(users, ids.userID) {
		h.t.Fatalf("identity users = %#v, want seeded user", users)
	}
	roles, err := reader.List(context.Background(), identityRolesResource)
	if err != nil {
		h.t.Fatalf("remote identity roles list: %v", err)
	}
	if !recordListContains(roles, ids.roleID) {
		h.t.Fatalf("identity roles = %#v, want seeded role", roles)
	}
	user, ok, err := reader.Get(context.Background(), identityUsersResource, ids.userID)
	if err != nil || !ok || user.Data["username"] == "" {
		h.t.Fatalf("identity get user = %#v ok=%v err=%v", user, ok, err)
	}
	_, ok, err = reader.Get(context.Background(), identityUsersResource, "missing"+h.runID)
	if err != nil || ok {
		h.t.Fatalf("identity missing user ok=%v err=%v, want false/nil", ok, err)
	}
	role, ok, err := reader.Get(context.Background(), identityRolesResource, ids.roleID)
	if err != nil || !ok || role.Data["name"] == "" {
		h.t.Fatalf("identity get role = %#v ok=%v err=%v", role, ok, err)
	}
	_, ok, err = reader.Get(context.Background(), identityRolesResource, "missing"+h.runID)
	if err != nil || ok {
		h.t.Fatalf("identity missing role ok=%v err=%v, want false/nil", ok, err)
	}
}

func (h *e2eHarness) assertIdentityInternalHTTPContracts(ids identityIDs) {
	req := h.newRequest(identityService, http.MethodGet, "/internal/identity/roles", nil, "")
	h.do(req, http.StatusUnauthorized)
	req = h.newRequest(identityService, http.MethodGet, "/internal/identity/users", nil, "")
	req.Header.Set("X-Service-Key", "wrong-"+h.serviceKey)
	h.do(req, http.StatusUnauthorized)
	req = h.newRequest(identityService, http.MethodGet, "/internal/identity/users", nil, "")
	req.Header.Set("X-Service-Key", h.serviceKey)
	resp := h.do(req, http.StatusOK)
	env := h.requireEnvelopeCorrelation(resp)
	if env["success"] != true {
		h.t.Fatalf("identity list envelope = %#v", env)
	}
	req = h.newRequest(identityService, http.MethodGet, "/internal/identity/roles/"+ids.roleID, nil, "")
	req.Header.Set("X-Service-Key", h.serviceKey)
	roleResp := h.do(req, http.StatusOK)
	if roleResp.dataMap(h.t)["id"] != ids.roleID {
		h.t.Fatalf("identity role get = %#v, want %s", roleResp.dataMap(h.t), ids.roleID)
	}
	req = h.newRequest(identityService, http.MethodGet, "/internal/identity/roles/missing"+h.runID, nil, "")
	req.Header.Set("X-Service-Key", h.serviceKey)
	missingRole := h.do(req, http.StatusNotFound)
	if missingRole.envelope(h.t)["success"] != false {
		h.t.Fatalf("identity missing role envelope = %#v, want success=false", missingRole.envelope(h.t))
	}
}

func (h *e2eHarness) assertIdentityInternalAuthContracts(ids identityIDs) {
	h.postInternalIdentityAuth("/internal/identity/auth/session", ids.session, http.StatusOK)
	apiAuth := h.postInternalIdentityAuth("/internal/identity/auth/api-token", ids.apiToken, http.StatusOK)
	if apiAuth.dataMap(h.t)["api_token_id"] != ids.apiTokenID {
		h.t.Fatalf("api token auth = %#v, want token id %s", apiAuth.dataMap(h.t), ids.apiTokenID)
	}
	h.postInternalIdentityAuth("/internal/identity/auth/session", "missing"+h.runID, http.StatusUnauthorized)
	h.postInternalIdentityAuthWithServiceKey("/internal/identity/auth/api-token", ids.apiToken, "wrong-"+h.serviceKey, http.StatusUnauthorized)
}

func (h *e2eHarness) postInternalIdentityAuth(path, token string, want int) testResponse {
	return h.postInternalIdentityAuthWithServiceKey(path, token, h.serviceKey, want)
}

func (h *e2eHarness) postInternalIdentityAuthWithServiceKey(path, token, serviceKey string, want int) testResponse {
	req := h.newRequest(identityService, http.MethodPost, path, strings.NewReader(`{"token":"`+token+`"}`), "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Key", serviceKey)
	return h.do(req, want)
}

func (h *e2eHarness) assertRemoteIdentityAuth(ids identityIDs) {
	formID := "form" + h.runID
	otherID := "forged" + h.runID
	now := time.Now().UTC().Format(time.RFC3339)
	h.createRecord(formsResource, formID, map[string]any{
		"user_id":     ids.userID,
		"title":       "mine",
		"description": "visible",
		"tag":         "",
		"status":      "Pending",
		"created_at":  now,
		"updated_at":  now,
	})
	h.createRecord(formsResource, otherID, map[string]any{
		"user_id":     "FORGED",
		"title":       "not mine",
		"description": "must not be visible",
		"tag":         "",
		"status":      "Pending",
		"created_at":  now,
		"updated_at":  now,
	})

	cfg := h.serviceConfig(requestNotificationService, map[string]string{identityService: h.services[identityService].url})
	backing, err := platform.NewBackingResources(h.ctx, cfg)
	if err != nil {
		h.t.Fatalf("remote auth consumer backing: %v", err)
	}
	defer backing.Close()
	app := platform.NewApp(cfg, backing.Options...)
	services.RegisterAll(app)
	server := httptest.NewServer(app)
	defer server.Close()

	req, err := http.NewRequestWithContext(h.ctx, http.MethodGet, server.URL+"/api/v1/forms/my", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+ids.session)
	req.Header.Set("X-User-ID", "FORGED")
	req.Header.Set("X-Request-ID", "req-"+h.runID)
	req.Header.Set("X-Trace-ID", "trace-"+h.runID)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("remote auth consumer returned %d: %s", resp.StatusCode, string(raw))
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		h.t.Fatalf("decode remote auth response: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0]["id"] != formID {
		h.t.Fatalf("forms/my data = %#v, want only real authenticated user form", env.Data)
	}
}

func (h *e2eHarness) assertOrgProjectGroupAdmissionSnapshot() {
	groupUserID := h.groupOnlyUserID()
	resp := h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/scheduler/admission", map[string]any{
		"job_id":          "groupjob" + h.runID,
		"project_id":      h.projectID(),
		"user_id":         groupUserID,
		"queue_name":      h.queueName(),
		"required_cpu":    1,
		"required_memory": 1024,
		"e2e_run_id":      h.runID,
	}, h.serviceKey, http.StatusOK)
	h.requireEnvelopeCorrelation(resp)
	data := resp.dataMap(h.t)
	if data["allowed"] != true || data["project_id"] != h.projectID() || data["user_id"] != groupUserID || data["queue_name"] != h.queueName() {
		h.t.Fatalf("group-backed admission = %#v, want allowed project/user/queue snapshot", data)
	}
	admissionID := h.projectID() + "/" + groupUserID + "/" + h.queueName()
	record := h.getRecord(schedulerAdmissionsResource, admissionID)
	if record.Data["allowed"] != true || record.Data["project_id"] != h.projectID() || record.Data["user_id"] != groupUserID {
		h.t.Fatalf("persisted group-backed admission = %#v, want matching snapshot", record.Data)
	}
	h.updateRecord(schedulerAdmissionsResource, admissionID, map[string]any{})
}

func (h *e2eHarness) assertWorkloadSchedulerSubmit(ids identityIDs) {
	resp := h.submitJob(ids, h.services[workloadService].url, http.StatusCreated)
	h.requireEnvelopeCorrelation(resp)
	record := resp.dataMap(h.t)
	jobData, ok := record["data"].(map[string]any)
	if !ok {
		h.t.Fatalf("job record data = %#v", record["data"])
	}
	jobID := record["id"].(string)
	if jobData["status"] != "submitted" || jobData["project_id"] != h.projectID() || jobData["e2e_run_id"] != h.runID {
		h.t.Fatalf("job data = %#v, want submitted project job tagged with run id", jobData)
	}
	h.getRecord(workloadJobsResource, jobID)
	admissionID := h.projectID() + "/" + ids.userID + "/" + h.queueName()
	admission := h.getRecord(schedulerAdmissionsResource, admissionID)
	if admission.Data["allowed"] != true {
		h.t.Fatalf("admission = %#v, want allowed", admission.Data)
	}
	h.updateRecord(schedulerAdmissionsResource, admissionID, map[string]any{})
	h.requireCorrelatedEvent("SubmitAdmissionReviewed", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["project_id"] == h.projectID()
	})
	h.requireCorrelatedEvent("JobSubmitted", func(event contracts.Event) bool {
		return event.Source == workloadService && event.Data["job_id"] == jobID
	})
}

func (h *e2eHarness) submitJob(ids identityIDs, baseURL string, want int) testResponse {
	return h.submitJobWithIdempotencyKey(ids, baseURL, "", want)
}

func (h *e2eHarness) submitJobWithIdempotencyKey(ids identityIDs, baseURL string, idempotencyKey string, want int) testResponse {
	payload := map[string]any{
		"project_id":      h.projectID(),
		"user_id":         ids.userID,
		"queue_name":      h.queueName(),
		"required_cpu":    1,
		"required_memory": 1024,
		"e2e_run_id":      h.runID,
	}
	return h.doURLJSONWithIdempotencyKey(baseURL, http.MethodPost, "/api/v1/jobs", payload, h.apiKey, idempotencyKey, want)
}

func (h *e2eHarness) assertQuotaReservationEvents() {
	reservationID := "reservation" + h.runID
	reserve := h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/quota/reservations", map[string]any{
		"id":         reservationID,
		"project_id": h.projectID(),
		"gpu":        1,
		"e2e_run_id": h.runID,
	}, h.serviceKey, http.StatusCreated)
	h.requireEnvelopeCorrelation(reserve)
	record := reserve.dataMap(h.t)
	id := record["id"].(string)
	if id != reservationID {
		h.t.Fatalf("reservation id = %s, want %s", id, reservationID)
	}
	recordData, ok := record["data"].(map[string]any)
	if !ok || recordData["idempotency_key"] == "" {
		h.t.Fatalf("reservation data = %#v, want persisted idempotency key", record["data"])
	}
	h.updateRecord(schedulerReservations, id, map[string]any{})
	beforeDuplicateReserve := len(h.listRecords(schedulerReservations))
	h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/quota/reservations", map[string]any{
		"id":         reservationID,
		"project_id": h.projectID(),
		"gpu":        1,
		"e2e_run_id": h.runID,
	}, h.serviceKey, http.StatusConflict)
	if after := len(h.listRecords(schedulerReservations)); after != beforeDuplicateReserve {
		h.t.Fatalf("reservation count after duplicate reserve = %d, want %d", after, beforeDuplicateReserve)
	}

	h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/quota/reservations/"+id+"/commit", map[string]any{}, h.serviceKey, http.StatusOK)
	commitEvents := h.countEvents("QuotaCommitted", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id
	})
	h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/quota/reservations/"+id+"/commit", map[string]any{}, h.serviceKey, http.StatusOK)
	if replayEvents := h.countEvents("QuotaCommitted", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id
	}); replayEvents != commitEvents {
		h.t.Fatalf("QuotaCommitted events after replay = %d, want %d", replayEvents, commitEvents)
	}
	h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/quota/reservations/"+id+"/release", map[string]any{}, h.serviceKey, http.StatusOK)
	releaseEvents := h.countEvents("QuotaReleased", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id
	})
	h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/quota/reservations/"+id+"/release", map[string]any{}, h.serviceKey, http.StatusOK)
	if replayEvents := h.countEvents("QuotaReleased", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id
	}); replayEvents != releaseEvents {
		h.t.Fatalf("QuotaReleased events after replay = %d, want %d", replayEvents, releaseEvents)
	}

	h.requireCorrelatedEvent("QuotaReserved", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id && event.Data["state"] == "reserved"
	})
	h.requireCorrelatedEvent("QuotaCommitted", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id && event.Data["state"] == "committed"
	})
	h.requireCorrelatedEvent("QuotaReleased", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["reservation_id"] == id && event.Data["state"] == "released"
	})
}

func (h *e2eHarness) assertMediaUploadRoundTrip() {
	filename := h.runID + ".png"
	uploaded := h.doMultipart(mediaUploadService, "/api/v1/uploads/images", "file", filename, "image/png", pngBytes(), http.StatusOK)
	key, ok := uploaded.rawMap(h.t)["key"].(string)
	if !ok || !strings.Contains(key, h.runID) {
		h.t.Fatalf("upload key = %#v, want key containing run id", uploaded.rawMap(h.t)["key"])
	}
	record := h.getRecord(mediaResource, key)
	if _, inline := record.Data["body_base64"]; inline {
		h.t.Fatalf("media metadata contains inline body: %#v", record.Data)
	}
	h.updateRecord(mediaResource, key, map[string]any{})
	body, contentType, found, err := h.objectStore.Get(h.ctx, key)
	if err != nil || !found {
		h.t.Fatalf("object store get found=%v err=%v", found, err)
	}
	if contentType != "image/png" || !bytes.Equal(body, pngBytes()) {
		h.t.Fatalf("object = %q/%v, want uploaded png", contentType, body)
	}
	req := h.newRequest(mediaUploadService, http.MethodGet, "/api/v1/uploads/images/"+key, nil, h.apiKey)
	served := h.do(req, http.StatusOK)
	if !bytes.Equal(served.Body, pngBytes()) || served.Header.Get("Content-Type") != "image/png" {
		h.t.Fatalf("served image = %q/%v, want uploaded png", served.Header.Get("Content-Type"), served.Body)
	}
}

func (h *e2eHarness) assertRequestNotificationEvents() {
	resp := h.doJSON(requestNotificationService, http.MethodPost, "/api/v1/forms", map[string]any{
		"title":       "E2E " + h.runID,
		"description": "cross-service event check",
		"e2e_run_id":  h.runID,
	}, h.apiKey, http.StatusCreated)
	form := resp.dataMap(h.t)
	formID := form["id"].(string)
	h.updateRecord(formsResource, formID, map[string]any{})
	h.requireCorrelatedEvent("FormCreated", func(event contracts.Event) bool {
		return event.Source == requestNotificationService && event.Data["id"] == formID
	})
	h.requireCorrelatedEvent("AuditEvent", func(event contracts.Event) bool {
		return event.Data["resource"] == requestNotificationService+":forms" && event.Data["success"] == true
	})
}

func (h *e2eHarness) assertSchedulerUnavailableDoesNotPersist(ids identityIDs) {
	before := len(h.listRecords(workloadJobsResource))
	failing := h.startExtraService("workload-failing-"+h.runID, workloadService, map[string]string{
		schedulerQuotaService: "http://127.0.0.1:1",
		orgProjectService:     h.services[orgProjectService].url,
	})
	h.submitJobWithIdempotencyKey(ids, failing.url, "idem-"+h.runID+"-scheduler-unavailable", http.StatusServiceUnavailable)
	after := len(h.listRecords(workloadJobsResource))
	if after != before {
		h.t.Fatalf("workload jobs after scheduler outage = %d, want unchanged %d", after, before)
	}
	beforeBadRemoteKeyJobs := len(h.listRecords(workloadJobsResource))
	beforeBadRemoteKeyAdmissions := len(h.listRecords(schedulerAdmissionsResource))
	unauthorizedScheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "bad scheduler service key")
	}))
	h.t.Cleanup(unauthorizedScheduler.Close)
	badKeyWorkload := h.startExtraService("workload-bad-key-"+h.runID, workloadService, map[string]string{
		schedulerQuotaService: unauthorizedScheduler.URL,
		orgProjectService:     h.services[orgProjectService].url,
	})
	h.submitJobWithIdempotencyKey(ids, badKeyWorkload.url, "idem-"+h.runID+"-scheduler-bad-key", http.StatusUnauthorized)
	if afterJobs := len(h.listRecords(workloadJobsResource)); afterJobs != beforeBadRemoteKeyJobs {
		h.t.Fatalf("workload jobs after bad remote scheduler key = %d, want unchanged %d", afterJobs, beforeBadRemoteKeyJobs)
	}
	if afterAdmissions := len(h.listRecords(schedulerAdmissionsResource)); afterAdmissions != beforeBadRemoteKeyAdmissions {
		h.t.Fatalf("scheduler admissions after bad remote scheduler key = %d, want unchanged %d", afterAdmissions, beforeBadRemoteKeyAdmissions)
	}
	beforeBadKeyJobs := len(h.listRecords(workloadJobsResource))
	beforeBadKeyAdmissions := len(h.listRecords(schedulerAdmissionsResource))
	h.doInternalJSON(schedulerQuotaService, http.MethodPost, "/api/v1/internal/scheduler/admission", map[string]any{}, "wrong-"+h.serviceKey, http.StatusUnauthorized)
	if afterJobs := len(h.listRecords(workloadJobsResource)); afterJobs != beforeBadKeyJobs {
		h.t.Fatalf("workload jobs after bad scheduler key = %d, want unchanged %d", afterJobs, beforeBadKeyJobs)
	}
	if afterAdmissions := len(h.listRecords(schedulerAdmissionsResource)); afterAdmissions != beforeBadKeyAdmissions {
		h.t.Fatalf("scheduler admissions after bad key = %d, want unchanged %d", afterAdmissions, beforeBadKeyAdmissions)
	}
}

func (h *e2eHarness) projectID() string       { return "project" + h.runID }
func (h *e2eHarness) groupID() string         { return "group" + h.runID }
func (h *e2eHarness) groupOnlyUserID() string { return "groupuser" + h.runID }
func (h *e2eHarness) queueName() string       { return "queue-" + h.runID }

func pngBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
}

func recordListContains(records []contracts.Record[map[string]any], id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func requireE2ENumber(t *testing.T, data map[string]any, key string, want float64) {
	t.Helper()
	got, ok := data[key].(float64)
	if !ok || got != want {
		t.Fatalf("%s = %#v, want %.2f", key, data[key], want)
	}
}
