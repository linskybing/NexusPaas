package platform

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

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
	{"admin", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
		if a.Config.RequireAuth && route.AuthRequired && route.Admin && !a.adminAllowed(r) {
			return true, http.StatusForbidden, "forbidden", "administrator privileges are required"
		}
		return false, 0, "", ""
	}},
	{"ratelimit", func(a *App, r *http.Request, route RouteSpec) (bool, int, string, string) {
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
			a.Metrics.Observe(route.Pattern, route.Method, rec.status, time.Since(start))
			span.SetAttributes(attribute.Int("http.response.status_code", rec.status))
			if rec.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(rec.status))
			}
			slog.Info("request", "service", service, "method", route.Method, "path", r.URL.Path, "status", rec.status, "request_id", RequestID(withIDs), "trace_id", TraceID(withIDs), "span_id", spanID(span), "user_id", logPrincipal(withIDs), "project_id", withIDs.URL.Query().Get("project_id"))
		}()

		for _, guard := range requestGuards {
			if denied, status, code, message := guard.apply(a, withIDs, route); denied {
				rec.status = status
				span.SetAttributes(attribute.String("nexuspaas.denied_by", guard.name))
				WriteError(rec, withIDs, status, code, message)
				return
			}
		}

		req := &httpRequest{
			Request:        withIDs,
			Service:        service,
			TraceID:        TraceID(withIDs),
			IdempotencyKey: withIDs.Header.Get("Idempotency-Key"),
		}
		if status, handled := a.maybeHandleStreamingProxy(rec, req, route); handled {
			rec.status = status
			if shouldPublishRouteAudit(route, status) {
				a.publishAudit(req, route, status < 400)
			}
			return
		}
		status, data, degraded := a.handleRoute(req, route)
		rec.status = status
		if degraded != nil {
			WriteDegraded(rec, withIDs, status, data, *degraded)
			return
		}
		WriteJSON(rec, withIDs, status, data)
	}
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
