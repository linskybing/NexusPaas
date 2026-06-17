package platform

import (
	"context"
	"net/http"
	"time"
)

// clusterReadinessProbeTimeout bounds the Kubernetes API connectivity check so a
// hung API server cannot block the readiness probe indefinitely (review Finding 6).
const clusterReadinessProbeTimeout = 3 * time.Second

func (a *App) registerCommonEndpoints() {
	a.Mux.HandleFunc("OPTIONS /{path...}", func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w.Header(), r.Header.Get("Origin"), a.Config.AllowedOrigins)
		w.WriteHeader(http.StatusNoContent)
	})
	a.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, r, http.StatusOK, map[string]any{
			"status": "ok",
		})
	})
	a.Mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if ok, reason := a.ready(r.Context()); !ok {
			WriteJSON(w, r, http.StatusServiceUnavailable, map[string]any{
				"status": "unavailable",
				"reason": reason,
			})
			return
		}
		WriteJSON(w, r, http.StatusOK, map[string]any{
			"status": "ok",
		})
	})
	// Non-production dev-token mint endpoint: the secure bootstrap for signed local
	// development tokens, registered only when a dev signing key is configured.
	if a.devTokenSigner != nil {
		a.Mux.HandleFunc("POST /api/v1/dev/token", a.handleDevToken)
	}
	a.registerOperationalEndpoint(operationalRoute("/metrics", "metrics", "platform_runtime_metrics"), func(app *App, r *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return app.rawHTTPResponse(app.Metrics, r)
	})
	a.registerOperationalEndpoint(operationalRoute("/openapi.json", "openapi", "platform_runtime_openapi"), func(app *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, app.OpenAPI(), nil
	})
	a.registerOperationalEndpoint(operationalRoute("/swagger/", "swagger", "platform_runtime_swagger"), func(app *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, app.SwaggerHTML(), nil
	})
	a.registerOperationalEndpoint(operationalRoute("/service-registry", "service-registry", "platform_runtime_service_registry"), func(app *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, app.ServiceRegistryView(), nil
	})
	a.registerOperationalEndpoint(operationalRoute("/outbox", "outbox", "platform_runtime_outbox"), func(app *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, redactedOutbox(app.Events.Outbox()), nil
	})
	a.registerOperationalEndpoint(operationalRoute("/projections", "projections", "platform_runtime_projections"), func(app *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, app.ProjectionStatuses(), nil
	})
}

// ready reports whether the service can serve traffic. It fails closed: a service
// that requires authentication but has no way to authenticate anyone (no API keys
// and no JWKS endpoint) is not ready, so Kubernetes stops routing to it instead of
// returning a blanket 200 (addresses the always-ok readiness probe).
func (a *App) ready(ctx context.Context) (bool, string) {
	if ok, reason := a.authReadiness(); !ok {
		return false, reason
	}
	if !a.Config.Production {
		return true, ""
	}
	if ok, reason := a.backingReadiness(ctx); !ok {
		return false, reason
	}
	return a.clusterReadiness(ctx)
}

// authReadiness fails closed when authentication is required but no mechanism can
// authenticate a caller (no API keys / JWKS), or JWKS is set without issuer+audience.
func (a *App) authReadiness() (bool, string) {
	if a.Config.RequireAuth && !hasEnabledAPIKey(a.Config.APIKeys) && a.Config.JWKSURL == "" {
		return false, "authentication is required but no API keys or JWKS endpoint are configured"
	}
	if a.Config.RequireAuth && a.Config.JWKSURL != "" && (a.Config.JWTIssuer == "" || len(a.Config.JWTAudiences) == 0) {
		return false, "jwt issuer and audience are required when JWKS endpoint is configured"
	}
	return true, ""
}

// backingReadiness probes every configured backing dependency (Postgres/Redis/etc.).
func (a *App) backingReadiness(ctx context.Context) (bool, string) {
	for _, dependency := range a.Config.BackingDependencies() {
		if err := a.BackingChecker.Check(ctx, dependency); err != nil {
			return false, dependency.Name + " is unavailable: " + err.Error()
		}
	}
	return true, ""
}

// clusterReadiness fails closed when a hosted service requires the Kubernetes cluster
// facade but the client is absent or the API server is unreachable.
func (a *App) clusterReadiness(ctx context.Context) (bool, string) {
	if !a.requiresClusterAccess() {
		return true, ""
	}
	if !a.Cluster.Configured() {
		return false, "cluster access is required but no Kubernetes client is configured"
	}
	pingCtx, cancel := context.WithTimeout(ctx, clusterReadinessProbeTimeout)
	defer cancel()
	if err := a.Cluster.Ping(pingCtx); err != nil {
		return false, "kubernetes cluster is unavailable: " + err.Error()
	}
	return true, ""
}

// requiresClusterAccess reports whether any hosted service depends on the Kubernetes
// cluster facade. a.Services only contains services this process actually hosts (set by
// RegisterService after the AllowsService gate), so an isolated deployment is gated on
// its own capability rather than the full catalog.
func (a *App) requiresClusterAccess() bool {
	for _, spec := range a.Services {
		if spec.RequiresCluster {
			return true
		}
	}
	return false
}

func (a *App) registerOperationalEndpoint(route RouteSpec, handler HandlerFunc) {
	a.RegisterCustomHandler(route.Method, route.Pattern, handler)
	a.Mux.HandleFunc(route.Method+" "+route.Pattern, a.wrap("platform-runtime", route))
}

func operationalRoute(pattern, resource, operationID string) RouteSpec {
	return RouteSpec{
		Method:       http.MethodGet,
		Pattern:      pattern,
		OperationID:  operationID,
		Resource:     "platform-runtime:" + resource,
		Action:       "operational",
		AuthRequired: true,
		Admin:        true,
	}
}

func (a *App) rawHTTPResponse(handler http.Handler, r *http.Request) (int, RawResponse, *Degraded) {
	rec := newRawResponseRecorder()
	handler.ServeHTTP(rec, r)
	return rec.statusCode(), rec.rawResponse(), nil
}
