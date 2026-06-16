package platform

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestInitTracingNoopWhenDisabled(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), Config{ServiceName: "identity-service"})
	if err != nil {
		t.Fatalf("InitTracing returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracing returned nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}

	// With no endpoint the tracer must be a no-op: spans carry no trace id.
	_, span := tracer().Start(context.Background(), "probe")
	span.End()
	if span.SpanContext().HasTraceID() {
		t.Fatal("expected no-op span without a trace id when tracing is disabled")
	}
}

func TestInitTracingInstallsCompositePropagator(t *testing.T) {
	if _, err := InitTracing(context.Background(), Config{ServiceName: "identity-service"}); err != nil {
		t.Fatalf("InitTracing returned error: %v", err)
	}
	fields := otel.GetTextMapPropagator().Fields()
	for _, want := range []string{"traceparent", "baggage"} {
		if !containsString(fields, want) {
			t.Fatalf("propagator fields %v missing %q", fields, want)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
