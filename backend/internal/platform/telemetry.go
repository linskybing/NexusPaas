package platform

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// tracerName is the instrumentation scope shared by every span the platform
// runtime emits. Keeping it stable lets backends group spans by library.
const tracerName = "github.com/linskybing/nexuspaas/backend"

// InitTracing wires the process-global OpenTelemetry tracer provider and the
// W3C text-map propagator.
//
// The propagator is always installed so inbound and outbound trace context flows
// across services even when this process does not itself export spans. When no
// OTLP endpoint is configured (TracingEnabled is false) a no-op tracer provider
// is installed instead of an exporter, keeping local and test runs hermetic — no
// network, no background batcher.
//
// The returned shutdown function flushes buffered spans and stops the provider.
// Callers must invoke it on graceful shutdown so spans are not dropped on exit
// (12-factor IX, disposability).
func InitTracing(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.TracingEnabled() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	// otlptracehttp.New reads OTEL_EXPORTER_OTLP_(TRACES_)ENDPOINT and the
	// standard OTLP env vars, so the destination is configuration, not code.
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironment(cfg.EnvironmentName()),
	))
	if err != nil {
		return nil, fmt.Errorf("build telemetry resource: %w", err)
	}

	// No explicit sampler is set so the SDK honors OTEL_TRACES_SAMPLER /
	// OTEL_TRACES_SAMPLER_ARG (defaulting to parent-based always-on).
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// tracer returns the shared tracer for the platform runtime. It resolves against
// whatever provider InitTracing installed (real or no-op), so callers can start
// spans unconditionally.
func tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}
