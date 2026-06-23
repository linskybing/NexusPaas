package platform

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

type App struct {
	Config             Config
	Mux                *http.ServeMux
	Store              RecordStore
	Events             EventStream
	Metrics            *Metrics
	Leases             contracts.WorkerLease
	ObjectStore        ObjectStore
	Revocations        RevocationStore
	PDP                contracts.PolicyDecisionPoint
	BackingChecker     BackingChecker
	Cluster            *cluster.Client
	Services           map[string]ServiceSpec
	Routes             []RouteSpec
	CatalogRoutes      []RouteSpec
	CustomHandlers     map[string]HandlerFunc
	Adapters           map[string]contracts.ExternalAdapter
	Rate               Limiter
	actions            map[string]ActionHandler
	maintenanceTasks   []maintenanceTask
	projections        *projectionRegistry
	instanceID         string
	crud               *crudValidator
	storeDependencies  map[string]map[string]bool
	registeredPatterns map[string]bool
	routeIndex         map[string][]RouteSpec
	routeIndexHash     uint64
	jwtVerifier        *jwtVerifier
	devTokenSigner     *devTokenSigner
	ownerReadDeps      map[string]map[string]bool
}

func NewApp(cfg Config, opts ...Option) *App {
	cfg = withRuntimeDefaults(cfg)
	app := newBaseApp(cfg)
	for _, opt := range opts {
		opt(app)
	}
	app.installCrossServiceStore()
	app.configureAdapters()
	app.registerBuiltinActions()
	app.registerCommonEndpoints()
	return app
}

func withRuntimeDefaults(cfg Config) Config {
	if cfg.AdapterTimeout == 0 {
		cfg.AdapterTimeout = 2 * time.Second
	}
	if cfg.AdapterRetries == 0 {
		cfg.AdapterRetries = 3
	}
	if cfg.AdapterThreshold == 0 {
		cfg.AdapterThreshold = 3
	}
	if cfg.AdapterOpenInterval == 0 {
		cfg.AdapterOpenInterval = 30 * time.Second
	}
	if strings.TrimSpace(cfg.LonghornNamespace) == "" {
		cfg.LonghornNamespace = "longhorn-system"
	}
	if strings.TrimSpace(cfg.WebUIDir) == "" {
		cfg.WebUIDir = defaultWebUIDir
	}
	if cfg.LonghornRWXHealthInterval == 0 {
		cfg.LonghornRWXHealthInterval = 30 * time.Second
	}
	if cfg.LonghornRWXRepairCooldown == 0 {
		cfg.LonghornRWXRepairCooldown = 10 * time.Minute
	}
	if cfg.LonghornRWXSnapshotWarn == 0 {
		cfg.LonghornRWXSnapshotWarn = 20
	}
	if cfg.LonghornRWXSnapshotBlock == 0 {
		cfg.LonghornRWXSnapshotBlock = 50
	}
	if cfg.PriorityClassSyncInterval == 0 {
		cfg.PriorityClassSyncInterval = time.Minute
	}
	cfg = withServiceRuntimeDefaults(cfg)
	if strings.TrimSpace(cfg.DockerCleanupNamespace) == "" {
		cfg.DockerCleanupNamespace = "default"
	}
	if strings.TrimSpace(cfg.DockerCleanupImage) == "" {
		cfg.DockerCleanupImage = "docker:24-dind"
	}
	return cfg
}

func withServiceRuntimeDefaults(cfg Config) Config {
	if strings.TrimSpace(cfg.CLICACertPEM) == "" {
		cfg.CLICACertPEM = defaultCLICACertPEM
	}
	if cfg.VPNAPITimeout == 0 {
		cfg.VPNAPITimeout = 5 * time.Second
	}
	if cfg.MinIOOperationTimeout == 0 {
		cfg.MinIOOperationTimeout = 10 * time.Second
	}
	if cfg.PGAdminSSOHTTPTimeout == 0 {
		cfg.PGAdminSSOHTTPTimeout = 10 * time.Second
	}
	if cfg.StreamTURNCredentialTTL == 0 {
		cfg.StreamTURNCredentialTTL = 8 * time.Hour
	}
	if cfg.StreamMaxBitrateKbps == 0 {
		cfg.StreamMaxBitrateKbps = 12000
	}
	if cfg.StreamMaxConcurrentSessions == 0 {
		cfg.StreamMaxConcurrentSessions = 64
	}
	if cfg.StreamEgressBudgetKbps == 0 {
		cfg.StreamEgressBudgetKbps = 800000
	}
	if len(cfg.StorageClassOptions) == 0 {
		cfg.StorageClassOptions = []string{"standard", "fast"}
	}
	return cfg
}

func defaultPDP(cfg Config) contracts.PolicyDecisionPoint {
	pdp := contracts.PolicyDecisionPoint(AllowAllPDP{})
	if cfg.RequireAuth && strings.TrimSpace(cfg.AuthorizationPolicyURL) != "" {
		pdp = NewRemotePDP(cfg.AuthorizationPolicyURL, cfg.AuthorizationPolicyAPIKey, cfg.AdapterTimeout, cfg)
	}
	return pdp
}

func newBaseApp(cfg Config) *App {
	return &App{
		Config:             cfg,
		Mux:                http.NewServeMux(),
		Store:              NewStore(),
		Events:             NewEventBus(),
		Metrics:            NewMetrics(),
		Leases:             NewWorkerLeases(),
		Revocations:        NewMemoryRevocations(),
		PDP:                defaultPDP(cfg),
		BackingChecker:     TCPBackingChecker{Timeout: cfg.AdapterTimeout},
		Services:           map[string]ServiceSpec{},
		Adapters:           map[string]contracts.ExternalAdapter{},
		CustomHandlers:     map[string]HandlerFunc{},
		Rate:               NewRateLimiter(600, time.Minute),
		actions:            map[string]ActionHandler{},
		projections:        newProjectionRegistry(),
		instanceID:         newInstanceID(),
		crud:               newCRUDValidator(),
		storeDependencies:  map[string]map[string]bool{},
		registeredPatterns: map[string]bool{},
		routeIndex:         map[string][]RouteSpec{},
		jwtVerifier:        newJWTVerifier(cfg),
		devTokenSigner:     newDevTokenSigner(cfg),
		ownerReadDeps:      map[string]map[string]bool{},
	}
}

func (a *App) installCrossServiceStore() {
	// In an isolated deployment, route reads of other services' resources over
	// HTTP to the owning service instead of the local store (finding 5). In
	// SERVICE_NAME=all every owner is co-hosted, so the decorator is a passthrough.
	// DISABLE_SERVICE_FALLBACK retires this synchronous fallback once event-fed
	// read models cover a service's reads, so it relies solely on local projections.
	if a.Config.ServiceName != "" && a.Config.ServiceName != "all" && len(a.Config.ServiceURLs) > 0 && !a.Config.ServiceFallbackDisabled {
		a.Store = &crossServiceStore{local: a.Store, cfg: a.Config, remote: NewRemoteServiceReader(a.Config)}
	}
}

func (a *App) configureAdapters() {
	for name, url := range a.Config.ExternalURLs {
		if a.Adapters[name] == nil {
			a.Adapters[name] = NewExternalAdapter(name, url, a.Config.AdapterTimeout, a.Config.AdapterRetries, a.Config.AdapterThreshold, a.Config.AdapterOpenInterval)
		}
	}
	for _, name := range []string{"k8s", "harbor", "minio", "pgadmin", "longhorn", "prometheus"} {
		if a.Adapters[name] == nil {
			a.Adapters[name] = NewExternalAdapter(name, "", a.Config.AdapterTimeout, a.Config.AdapterRetries, a.Config.AdapterThreshold, a.Config.AdapterOpenInterval)
		}
	}
	// Apply per-adapter upstream path rewriting and auth injection (finding 8).
	for name, adapterCfg := range a.Config.AdapterConfigs {
		if adapter, ok := a.Adapters[name].(*ExternalAdapter); ok {
			adapter.configure(adapterCfg)
		}
	}
}

// RegisterAction adds or overrides the handler for a RouteSpec.Action value.
// Services can extend dispatch without modifying the platform core.
func (a *App) RegisterAction(action string, handler ActionHandler) {
	a.actions[action] = handler
}

// ValidateAdminCoverage returns an error if any state-changing admin route
// would be unprotected. Such a route must either have a registered custom
// handler (which performs its own admin check) or be covered by the platform
// admin gate, which only activates when RequireAuth is set and the route is
// AuthRequired. Callers should fail production startup on a non-nil result so
// no mutating admin endpoint ships without enforcement (finding 26).
func (a *App) ValidateAdminCoverage() error {
	var gaps []string
	for _, route := range a.CatalogRoutes {
		if !route.Admin || !route.StateChanging {
			continue
		}
		if a.CustomHandlers[route.Method+" "+canonicalPattern(route.Pattern)] != nil {
			continue
		}
		if route.ServiceAuthRequired {
			continue // internal service-auth guard covers this route
		}
		if a.Config.RequireAuth && route.AuthRequired {
			continue // platform admin gate enforces RouteSpec.Admin
		}
		gaps = append(gaps, route.Method+" "+route.Pattern)
	}
	if len(gaps) > 0 {
		return fmt.Errorf("unprotected admin routes (no custom handler and platform admin gate inactive): %s", strings.Join(gaps, ", "))
	}
	return nil
}

// registerBuiltinActions wires the platform's own actions into the registry.
// Each entry delegates to the existing handler method unchanged.
func (a *App) registerBuiltinActions() {
	a.actions["config_commit"] = a.handleConfigCommit
	a.actions["quota_reserve"] = func(r *httpRequest, route RouteSpec) (int, any) {
		return a.handleReservation(r, route, "reserved")
	}
	a.actions["quota_commit"] = func(r *httpRequest, route RouteSpec) (int, any) {
		return a.handleReservationTransition(r, route, "committed")
	}
	a.actions["quota_release"] = func(r *httpRequest, route RouteSpec) (int, any) {
		return a.handleReservationTransition(r, route, "released")
	}
	a.actions["worker_lease"] = func(r *httpRequest, _ RouteSpec) (int, any) {
		return a.handleWorkerLease(r)
	}
	a.actions["event_ingest"] = a.handleEventIngest
	a.actions["command"] = a.handleCommand
	a.actions["proxy"] = a.handleProxy
	a.actions[gatewayProxyAction] = a.handleGatewayProxy
}

func (a *App) RegisterCustomHandler(method, pattern string, handler HandlerFunc) {
	a.CustomHandlers[method+" "+canonicalPattern(pattern)] = handler
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.serveServiceRoute(w, r) {
		return
	}
	if a.serveGatewayCatalogProxyRoute(w, r) {
		return
	}
	a.Mux.ServeHTTP(w, r)
}

func (a *App) serveServiceRoute(w http.ResponseWriter, r *http.Request) bool {
	var best RouteSpec
	bestScore := -1
	bestParams := map[string]string{}
	for _, route := range a.routeCandidates(r.Method, r.URL.Path) {
		if route.Method != r.Method {
			continue
		}
		params, ok := extractPathParams(route.Pattern, r.URL.Path)
		if !ok {
			continue
		}
		score := routeSpecificity(route.Pattern)
		if score <= bestScore {
			continue
		}
		best = route
		bestScore = score
		bestParams = params
	}
	if bestScore < 0 {
		return false
	}
	for key, value := range bestParams {
		r.SetPathValue(key, value)
	}
	a.wrap(routeService(best), best)(w, r)
	return true
}

func (a *App) serveGatewayCatalogProxyRoute(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(a.Config.ServiceName) != "platform-gateway" || len(a.Config.ServiceURLs) == 0 {
		return false
	}
	var best RouteSpec
	bestScore := -1
	bestParams := map[string]string{}
	for _, route := range a.catalogRouteCandidates(r.Method, r.URL.Path) {
		if route.Method != r.Method || !a.gatewayProxyableRoute(route) {
			continue
		}
		params, ok := extractPathParams(route.Pattern, r.URL.Path)
		if !ok {
			continue
		}
		score := routeSpecificity(route.Pattern)
		if score <= bestScore {
			continue
		}
		best = route
		bestScore = score
		bestParams = params
	}
	if bestScore < 0 {
		return false
	}
	for key, value := range bestParams {
		r.SetPathValue(key, value)
	}
	best.Action = gatewayProxyAction
	a.wrap(routeService(best), best)(w, r)
	return true
}

func (a *App) gatewayProxyableRoute(route RouteSpec) bool {
	owner := routeService(route)
	return owner != "" &&
		!a.Config.AllowsService(owner) &&
		!route.ServiceAuthRequired &&
		!isInternalRoutePattern(route.Pattern)
}

func (a *App) RegisterService(spec ServiceSpec) {
	catalogRoutes := make([]RouteSpec, 0, len(spec.Routes))
	for _, route := range spec.Routes {
		route.StateChanging = route.StateChanging || route.Method != http.MethodGet
		route.Resource = firstNonEmpty(route.Resource, route.OperationID)
		route.OperationID = firstNonEmpty(route.OperationID, operationID(spec.Name, route.Method, route.Pattern))
		routeCopy := route
		routeCopy.Resource = spec.Name + ":" + route.Resource
		routeCopy.OperationID = firstNonEmpty(route.OperationID, operationID(spec.Name, route.Method, route.Pattern))
		catalogRoutes = append(catalogRoutes, routeCopy)
	}
	a.CatalogRoutes = append(a.CatalogRoutes, catalogRoutes...)
	if !a.Config.AllowsService(spec.Name) {
		return
	}
	a.Services[spec.Name] = spec
	changedRoute := false
	for _, routeCopy := range catalogRoutes {
		registerKey := routeCopy.Method + " " + canonicalPattern(routeCopy.Pattern)
		if a.registeredPatterns[registerKey] {
			if routeCopy.Override && strings.TrimSpace(routeCopy.OverrideReason) != "" {
				a.replaceRegisteredRoute(registerKey, routeCopy)
				changedRoute = true
			}
			continue
		}
		a.registeredPatterns[registerKey] = true
		a.Routes = append(a.Routes, routeCopy)
		changedRoute = true
	}
	if changedRoute {
		a.rebuildRouteIndex()
	}
}

func (a *App) replaceRegisteredRoute(registerKey string, route RouteSpec) {
	for i, current := range a.Routes {
		if current.Method+" "+canonicalPattern(current.Pattern) == registerKey {
			a.Routes[i] = route
			return
		}
	}
	a.Routes = append(a.Routes, route)
}

func (a *App) handleRoute(r *httpRequest, route RouteSpec) (int, any, *Degraded) {
	if handler := a.CustomHandlers[route.Method+" "+canonicalPattern(route.Pattern)]; handler != nil {
		status, data, degraded := handler(a, r.Request, route)
		if shouldPublishRouteAudit(route, status) {
			a.publishAudit(r, route, status < 400)
		}
		return status, data, degraded
	}

	if route.Action != "proxy" && route.Action != gatewayProxyAction && route.ExternalAdapter != "" {
		result := a.callAdapter(r, route.ExternalAdapter, route)
		if result.Degraded {
			a.Metrics.Inc(route.ExternalAdapter + "_degraded")
			return http.StatusOK, result, degradedFromAdapterResult(result)
		}
	}

	action := a.actions[route.Action]
	if action == nil {
		action = a.handleCRUD
	}
	status, data := action(r, route)
	if degraded, ok := data.(actionDegradedResponse); ok {
		return status, degraded.data, &degraded.degraded
	}
	if shouldPublishRouteAudit(route, status) {
		a.publishAudit(r, route, status < 400)
	}
	return status, data, nil
}

func (a *App) publishAudit(r *httpRequest, route RouteSpec, success bool) {
	userID := firstNonEmpty(r.Header.Get(headerUserID), "anonymous")
	resourceID := firstNonEmpty(pathID(r.Request, route.IDParam), r.URL.Query().Get("project_id"), r.URL.Query().Get("group_id"), r.URL.Path)
	payload := map[string]any{
		"audit_event_id": NewUUID(),
		"user_id":        userID,
		"actor_user_id":  userID,
		"action":         route.OperationID,
		"resource":       route.Resource,
		"resource_type":  auditResourceType(route),
		"resource_id":    resourceID,
		"success":        success,
		"outcome":        auditOutcome(success),
		"source_ip":      r.RemoteAddr,
		"source_service": r.Service,
		"request_path":   r.URL.Path,
		"project_id":     r.URL.Query().Get("project_id"),
		"group_id":       r.URL.Query().Get("group_id"),
	}
	a.publishEvent(r, "AuditEvent", payload)
	a.Metrics.Inc("audit_events")
}

func auditOutcome(success bool) string {
	if success {
		return "allowed"
	}
	return "denied"
}

func auditResourceType(route RouteSpec) string {
	resource := route.Resource
	if prefix := route.ServicePrefix(); prefix != "" {
		resource = strings.TrimPrefix(resource, prefix+":")
	}
	resource = strings.Trim(resource, ":")
	if resource == "" {
		return "unknown"
	}
	return strings.ReplaceAll(resource, ":", "_")
}

func (a *App) publishDomainEvent(r *httpRequest, route RouteSpec, suffix string, data map[string]any) {
	a.publishEvent(r, domainEventName(route, suffix), data)
}

func (a *App) newDomainEvent(r *httpRequest, route RouteSpec, suffix string, data map[string]any) contracts.Event {
	return a.newEvent(r, domainEventName(route, suffix), data)
}

func domainEventName(route RouteSpec, suffix string) string {
	name := strings.TrimSuffix(route.Resource, "s") + suffix
	name = strings.TrimPrefix(name, route.ServicePrefix()+":")
	return name
}

func (route RouteSpec) ServicePrefix() string {
	parts := strings.Split(route.Resource, ":")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}
