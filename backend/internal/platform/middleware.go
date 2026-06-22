package platform

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const rateLimitRetryAfterSeconds = 60

func setCORSHeaders(header http.Header, origin string, allowedOrigins map[string]bool) {
	if allowedOrigin(origin, allowedOrigins) {
		header.Set("Access-Control-Allow-Origin", origin)
	} else {
		header.Del("Access-Control-Allow-Origin")
	}
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,Idempotency-Key,Traceparent,X-API-Key,X-Request-ID,X-Trace-ID")
	header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
}

func allowedOrigin(origin string, allowedOrigins map[string]bool) bool {
	return origin != "" && allowedOrigins[origin]
}

// routeGuard is one ordered step in the request middleware chain. Each guard
// inspects the (possibly mutated) request and either lets it continue or denies
// it with a status/code/message. Expressing the chain as data means a new
// cross-cutting concern is an appended guard rather than another inline branch in
// wrap (Open/Closed).
type routeGuard struct {
	name  string
	apply func(a *App, r *http.Request, route RouteSpec) (denied bool, status int, code, message string)
}

// requestGuards runs in order; the first denial short-circuits the request. authn
// runs first because it populates the verified principal that admin gating reads.
var requestGuards = []routeGuard{
	{"authn", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
		if !a.authorized(r, route) {
			return true, http.StatusUnauthorized, "unauthorized", "authentication is required"
		}
		return false, 0, "", ""
	}},
	{"service-auth", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
		if !route.ServiceAuthRequired {
			return false, 0, "", ""
		}
		if a.Config.ServiceAPIKey == "" {
			return true, http.StatusNotFound, "not_found", "not found"
		}
		if !a.ServiceRequestAuthorized(r) {
			return true, http.StatusUnauthorized, "unauthorized", "service authentication is required"
		}
		return false, 0, "", ""
	}},
	{"admin", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
		if a.Config.RequireAuth && route.AuthRequired && route.Admin && !a.adminAllowed(r) {
			return true, http.StatusForbidden, "forbidden", "administrator privileges are required"
		}
		return false, 0, "", ""
	}},
	{"ratelimit", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
		if !rateLimitApplies(route) {
			return false, 0, "", ""
		}
		if !a.Rate.Allow(rateLimitKey(r, route, a.Config.TrustedProxyCIDRs)) {
			return true, http.StatusTooManyRequests, "rate_limited", "too many requests"
		}
		return false, 0, "", ""
	}},
	{"policy", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
		if a.Config.RequireAuth && route.AuthRequired && !a.policyAllowed(r, route) {
			return true, http.StatusForbidden, "forbidden", "authorization policy denied the request"
		}
		return false, 0, "", ""
	}},
}

func rateLimitApplies(route RouteSpec) bool {
	return !route.ServiceAuthRequired
}

func (a *App) wrap(service string, route RouteSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		withIDs := ensureRequestIDs(r)

		ctx, span := tracer().Start(withIDs.Context(), route.Method+" "+route.Pattern,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("nexuspaas.service", service),
				attribute.String("http.request.method", route.Method),
				attribute.String("http.route", route.Pattern),
			),
		)
		defer span.End()
		withIDs = withIDs.WithContext(ctx)
		alignTraceHeaders(withIDs, span)

		setCORSHeaders(rec.Header(), withIDs.Header.Get("Origin"), a.Config.AllowedOrigins)
		defer func() {
			a.observeRequest(service, route, rec.status, start, r, withIDs, span)
		}()
		if a.writeMiddlewareDenial(rec, withIDs, route, span) {
			return
		}

		req := &httpRequest{
			Request:        withIDs,
			Service:        service,
			TraceID:        TraceID(withIDs),
			IdempotencyKey: withIDs.Header.Get("Idempotency-Key"),
		}
		a.writeRouteResponse(rec, req, route)
	}
}

func (a *App) observeRequest(service string, route RouteSpec, status int, start time.Time, original, withIDs *http.Request, span trace.Span) {
	a.Metrics.Observe(route.Pattern, route.Method, status, time.Since(start))
	span.SetAttributes(attribute.Int("http.response.status_code", status))
	if status >= 500 {
		span.SetStatus(codes.Error, http.StatusText(status))
	}
	slog.Info("request", "service", service, "method", route.Method, "path", original.URL.Path, "status", status, "request_id", RequestID(withIDs), "trace_id", TraceID(withIDs), "span_id", spanID(span), "user_id", logPrincipal(withIDs), "project_id", withIDs.URL.Query().Get("project_id"))
}

func (a *App) writeMiddlewareDenial(rec *statusRecorder, r *http.Request, route RouteSpec, span trace.Span) bool {
	if denied, status, code, message := a.limitRequestBody(rec, r, route); denied {
		writeDenied(rec, r, status, code, message)
		return true
	}
	for _, guard := range requestGuards {
		if denied, status, code, message := guard.apply(a, r, route); denied {
			span.SetAttributes(attribute.String("nexuspaas.denied_by", guard.name))
			a.writeGuardDenial(rec, r, guard.name, status, code, message)
			return true
		}
	}
	return false
}

func (a *App) writeGuardDenial(rec *statusRecorder, r *http.Request, guardName string, status int, code, message string) {
	if guardName == "ratelimit" {
		rec.Header().Set("Retry-After", strconv.Itoa(rateLimitRetryAfterSeconds))
		message = "too many requests; retry after 60 seconds"
	}
	writeDenied(rec, r, status, code, message)
}

func writeDenied(rec *statusRecorder, r *http.Request, status int, code, message string) {
	rec.status = status
	WriteError(rec, r, status, code, message)
}

func (a *App) writeRouteResponse(rec *statusRecorder, req *httpRequest, route RouteSpec) {
	if status, handled := a.maybeHandleStreamingProxy(rec, req, route); handled {
		rec.status = status
		a.publishRouteAudit(req, route, status)
		return
	}
	status, data, degraded := a.handleRoute(req, route)
	rec.status = status
	if degraded != nil {
		WriteDegraded(rec, req.Request, status, data, *degraded)
		return
	}
	WriteJSON(rec, req.Request, status, data)
}

func (a *App) publishRouteAudit(req *httpRequest, route RouteSpec, status int) {
	if shouldPublishRouteAudit(route, status) {
		a.publishAudit(req, route, status < 400)
	}
}

func (a *App) limitRequestBody(w http.ResponseWriter, r *http.Request, route RouteSpec) (bool, int, string, string) {
	if !route.StateChanging || r.Body == nil {
		return false, 0, "", ""
	}
	limit := int64(a.Config.EffectiveMaxAPIBodyBytes())
	if r.ContentLength > limit {
		return true, http.StatusRequestEntityTooLarge, "request_body_too_large", "request body exceeds max byte size"
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return false, 0, "", ""
}

// alignTraceHeaders sets X-Trace-ID to the active span's trace id so logs, the
// response envelope, and any emitted events share the OpenTelemetry trace id.
// When tracing is a no-op the span context has no trace id and the existing
// header-derived value (from Traceparent/X-Trace-ID) is left untouched.
func alignTraceHeaders(r *http.Request, span trace.Span) {
	if sc := span.SpanContext(); sc.HasTraceID() {
		r.Header.Set(headerTraceID, sc.TraceID().String())
	}
}

func spanID(span trace.Span) string {
	if sc := span.SpanContext(); sc.HasSpanID() {
		return sc.SpanID().String()
	}
	return ""
}

// logPrincipal returns the verified user id for the access log, never the
// client-supplied X-User-ID header, so the log cannot be spoofed.
func logPrincipal(r *http.Request) string {
	if user, ok := verifiedUser(r); ok {
		if id := asString(user["id"]); id != "" {
			return id
		}
	}
	return "anonymous"
}

func ensureRequestIDs(r *http.Request) *http.Request {
	requestID := firstNonEmpty(r.Header.Get(headerRequestID), newID())
	traceID := firstNonEmpty(r.Header.Get("Traceparent"), r.Header.Get(headerTraceID), requestID)
	r.Header.Set(headerRequestID, requestID)
	r.Header.Set(headerTraceID, traceID)
	return r
}

func RequestID(r *http.Request) string {
	return r.Header.Get(headerRequestID)
}

func TraceID(r *http.Request) string {
	return r.Header.Get(headerTraceID)
}
